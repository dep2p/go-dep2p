package holepunch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/p2p/protocol/holepunch/pb"
	"github.com/dep2p/go-dep2p/p2p/protocol/identify"
	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"
	logging "github.com/dep2p/log"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// Protocol 是用于打洞的 dep2p 协议标识
const Protocol protocol.ID = "/dep2p/dcutr"

var log = logging.Logger("p2p-holepunch")

// StreamTimeout 是打洞协议流的超时时间
var StreamTimeout = 1 * time.Minute

const (
	// ServiceName 是打洞服务的名称
	ServiceName = "dep2p.holepunch"

	// maxMsgSize 是消息的最大大小,4KB
	maxMsgSize = 4 * 1024
)

// ErrClosed 当打洞服务关闭时返回此错误
var ErrClosed = errors.New("打洞服务正在关闭")

// Option 是配置 Service 的函数类型
type Option func(*Service) error

// Service 在支持 DCUtR 协议的每个节点上运行
type Service struct {
	// ctx 是服务的上下文
	ctx       context.Context
	ctxCancel context.CancelFunc

	// host 是本地节点
	host host.Host
	// ids 用于连接反转。我们等待标识完成,如果对等点可以公开访问,则尝试直接连接
	ids identify.IDService
	// listenAddrs 提供用于打洞的主机地址。我们使用这个而不是 host.Addrs,
	// 因为 host.Addrs 可能会删除不可公开访问的地址,只公布可公开访问的中继地址
	listenAddrs func() []ma.Multiaddr

	// holePuncherMx 用于保护 holePuncher
	holePuncherMx sync.Mutex
	// holePuncher 是打洞器实例
	holePuncher *holePuncher

	// hasPublicAddrsChan 用于通知是否有公共地址
	hasPublicAddrsChan chan struct{}

	// tracer 用于追踪打洞过程
	tracer *tracer
	// filter 用于过滤地址
	filter AddrFilter

	// refCount 用于等待所有 goroutine 完成
	refCount sync.WaitGroup
}

// NewService 创建一个可用于打洞的新服务
// 该服务在所有支持 DCUtR 协议的主机上运行,无论它们是否在 NAT/防火墙后面
// 该服务处理 DCUtR 流(当我们通过中继建立连接后,由 NAT/防火墙后的节点发起)
//
// 参数:
//   - h: host.Host 本地节点
//   - ids: identify.IDService 标识服务
//   - listenAddrs: func() []ma.Multiaddr 返回公共地址的函数
//   - opts: ...Option 可选的配置选项
//
// 返回值:
//   - *Service 打洞服务实例
//   - error 错误信息
//
// 注意:
//   - listenAddrs 必须只返回公共地址
func NewService(h host.Host, ids identify.IDService, listenAddrs func() []ma.Multiaddr, opts ...Option) (*Service, error) {
	if ids == nil {
		log.Debugf("标识服务不能为空")
		return nil, errors.New("标识服务不能为空")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		ctx:                ctx,
		ctxCancel:          cancel,
		host:               h,
		ids:                ids,
		listenAddrs:        listenAddrs,
		hasPublicAddrsChan: make(chan struct{}),
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			cancel()
			log.Debugf("配置打洞服务失败: %v", err)
			return nil, err
		}
	}
	s.tracer.Start()

	s.refCount.Add(1)
	go s.waitForPublicAddr()

	return s, nil
}

// waitForPublicAddr 等待直到有至少一个公共地址
func (s *Service) waitForPublicAddr() {
	defer s.refCount.Done()

	log.Debug("等待直到我们有至少一个公共地址", "peer", s.host.ID())

	// TODO: 当标识发现新地址时,我们应该在这里有一个事件
	// 由于目前没有这样的事件,只需定期检查我们观察到的地址(从 250ms 开始的指数退避,上限为 5s)
	duration := 250 * time.Millisecond
	const maxDuration = 5 * time.Second
	t := time.NewTimer(duration)
	defer t.Stop()
	for {
		if len(s.listenAddrs()) > 0 {
			log.Debug("主机现在有一个公共地址。启动打洞协议。")
			s.host.SetStreamHandler(Protocol, s.handleNewStream)
			break
		}

		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			duration *= 2
			if duration > maxDuration {
				duration = maxDuration
			}
			t.Reset(duration)
		}
	}

	s.holePuncherMx.Lock()
	if s.ctx.Err() != nil {
		// 服务已关闭
		return
	}
	s.holePuncher = newHolePuncher(s.host, s.ids, s.listenAddrs, s.tracer, s.filter)
	s.holePuncherMx.Unlock()
	close(s.hasPublicAddrsChan)
}

// Close 关闭打洞服务
//
// 返回值:
//   - error 关闭过程中的错误
func (s *Service) Close() error {
	var err error
	s.ctxCancel()
	s.holePuncherMx.Lock()
	if s.holePuncher != nil {
		err = s.holePuncher.Close()
	}
	s.holePuncherMx.Unlock()
	s.tracer.Close()
	s.host.RemoveStreamHandler(Protocol)
	s.refCount.Wait()
	return err
}

// incomingHolePunch 处理入站打洞请求
//
// 参数:
//   - str: network.Stream 网络流
//
// 返回值:
//   - rtt: time.Duration 往返时间
//   - remoteAddrs: []ma.Multiaddr 远程地址列表
//   - ownAddrs: []ma.Multiaddr 本地地址列表
//   - err: error 错误信息
func (s *Service) incomingHolePunch(str network.Stream) (rtt time.Duration, remoteAddrs []ma.Multiaddr, ownAddrs []ma.Multiaddr, err error) {
	// 健全性检查:打洞请求应该只来自中继后面的对等点
	if !isRelayAddress(str.Conn().RemoteMultiaddr()) {
		log.Debugf("收到打洞流: %s", str.Conn().RemoteMultiaddr())
		return 0, nil, nil, fmt.Errorf("收到打洞流: %s", str.Conn().RemoteMultiaddr())
	}
	ownAddrs = s.listenAddrs()
	if s.filter != nil {
		ownAddrs = s.filter.FilterLocal(str.Conn().RemotePeer(), ownAddrs)
	}

	// 如果我们无法告诉对等点在哪里拨号,就没有必要开始打洞
	if len(ownAddrs) == 0 {
		log.Debugf("拒绝打洞请求,因为我们没有任何公共地址")
		return 0, nil, nil, errors.New("拒绝打洞请求,因为我们没有任何公共地址")
	}

	if err := str.Scope().ReserveMemory(maxMsgSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为流预留内存时出错: %s", err)
		return 0, nil, nil, err
	}
	defer str.Scope().ReleaseMemory(maxMsgSize)

	wr := pbio.NewDelimitedWriter(str)
	rd := pbio.NewDelimitedReader(str, maxMsgSize)

	// 读取 Connect 消息
	msg := new(pb.HolePunch)

	str.SetDeadline(time.Now().Add(StreamTimeout))

	if err := rd.ReadMsg(msg); err != nil {
		log.Debugf("从发起者读取消息失败: %v", err)
		return 0, nil, nil, err
	}
	if t := msg.GetType(); t != pb.HolePunch_CONNECT {
		log.Debugf("期望从发起者收到 CONNECT 消息但收到 %d", t)
		return 0, nil, nil, fmt.Errorf("期望从发起者收到 CONNECT 消息但收到 %d", t)
	}

	obsDial := removeRelayAddrs(addrsFromBytes(msg.ObsAddrs))
	if s.filter != nil {
		obsDial = s.filter.FilterRemote(str.Conn().RemotePeer(), obsDial)
	}

	log.Debugw("收到打洞请求", "peer", str.Conn().RemotePeer(), "addrs", obsDial)
	if len(obsDial) == 0 {
		log.Debugf("期望 CONNECT 消息包含至少一个地址")
		return 0, nil, nil, errors.New("期望 CONNECT 消息包含至少一个地址")
	}

	// 写入 CONNECT 消息
	msg.Reset()
	msg.Type = pb.HolePunch_CONNECT.Enum()
	msg.ObsAddrs = addrsToBytes(ownAddrs)
	tstart := time.Now()
	if err := wr.WriteMsg(msg); err != nil {
		log.Debugf("向发起者写入 CONNECT 消息失败: %v", err)
		return 0, nil, nil, err
	}

	// 读取 SYNC 消息
	msg.Reset()
	if err := rd.ReadMsg(msg); err != nil {
		log.Debugf("从发起者读取消息失败: %v", err)
		return 0, nil, nil, err
	}
	if t := msg.GetType(); t != pb.HolePunch_SYNC {
		log.Debugf("期望从发起者收到 SYNC 消息但收到 %d", t)
		return 0, nil, nil, fmt.Errorf("期望从发起者收到 SYNC 消息但收到 %d", t)
	}
	return time.Since(tstart), obsDial, ownAddrs, nil
}

// handleNewStream 处理新的打洞流
//
// 参数:
//   - str: network.Stream 网络流
func (s *Service) handleNewStream(str network.Stream) {
	// 检查底层连接的方向
	// 对等点 A 从对等点 B 接收入站连接
	// 对等点 A 向对等点 B 打开新的打洞流
	// 对等点 B 接收此流,调用此函数
	// 对等点 B 将底层连接视为出站连接
	if str.Conn().Stat().Direction == network.DirInbound {
		str.Reset()
		return
	}

	if err := str.Scope().SetService(ServiceName); err != nil {
		log.Debugf("将流附加到打洞服务时出错: %s", err)
		str.Reset()
		return
	}

	rp := str.Conn().RemotePeer()
	rtt, addrs, ownAddrs, err := s.incomingHolePunch(str)
	if err != nil {
		s.tracer.ProtocolError(rp, err)
		log.Debugf("处理来自对等点的打洞流时出错", "peer", rp, "error", err)
		str.Reset()
		return
	}
	str.Close()

	// 现在通过强制连接进行打洞
	pi := peer.AddrInfo{
		ID:    rp,
		Addrs: addrs,
	}
	s.tracer.StartHolePunch(rp, addrs, rtt)
	log.Debugw("开始打洞", "peer", rp)
	start := time.Now()
	s.tracer.HolePunchAttempt(pi.ID)
	err = holePunchConnect(s.ctx, s.host, pi, false)
	dt := time.Since(start)
	s.tracer.EndHolePunch(rp, dt, err)
	s.tracer.HolePunchFinished("receiver", 1, addrs, ownAddrs, getDirectConnection(s.host, rp))
}

// DirectConnect 仅用于测试目的
// TODO: 为此找到解决方案
//
// 参数:
//   - p: peer.ID 对等点 ID
//
// 返回值:
//   - error 错误信息
func (s *Service) DirectConnect(p peer.ID) error {
	<-s.hasPublicAddrsChan
	s.holePuncherMx.Lock()
	holePuncher := s.holePuncher
	s.holePuncherMx.Unlock()
	return holePuncher.DirectConnect(p)
}
