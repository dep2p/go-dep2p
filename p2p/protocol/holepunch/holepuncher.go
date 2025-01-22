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
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/p2p/protocol/holepunch/pb"
	"github.com/dep2p/go-dep2p/p2p/protocol/identify"
	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"
)

// ErrHolePunchActive 当另一个打洞尝试正在进行时，DirectConnect 会返回此错误
var ErrHolePunchActive = errors.New("另一个打洞尝试正在进行中")

const (
	// dialTimeout 拨号超时时间
	dialTimeout = 5 * time.Second
	// maxRetries 最大重试次数
	maxRetries = 3
)

// holePuncher 在位于 NAT/防火墙后的节点上运行
// 它观察通过中继节点的新入站连接(该节点与中继节点有预约),并与它们启动 DCUtR 协议
// 它首先尝试建立直接连接,如果失败则启动打洞
type holePuncher struct {
	// ctx 上下文对象
	ctx context.Context
	// ctxCancel 取消上下文的函数
	ctxCancel context.CancelFunc

	// host dep2p 主机
	host host.Host
	// refCount 引用计数
	refCount sync.WaitGroup

	// ids 身份识别服务
	ids identify.IDService
	// listenAddrs 返回监听地址的函数
	listenAddrs func() []ma.Multiaddr

	// activeMx 活动打洞的互斥锁
	activeMx sync.Mutex
	// active 活动打洞的映射,用于去重
	active map[peer.ID]struct{}

	// closeMx 关闭状态的读写锁
	closeMx sync.RWMutex
	// closed 是否已关闭
	closed bool

	// tracer 追踪器
	tracer *tracer
	// filter 地址过滤器
	filter AddrFilter
}

// newHolePuncher 创建新的打洞器
// 参数:
//   - h: host.Host dep2p 主机
//   - ids: identify.IDService 身份识别服务
//   - listenAddrs: func() []ma.Multiaddr 返回监听地址的函数
//   - tracer: *tracer 追踪器
//   - filter: AddrFilter 地址过滤器
//
// 返回值:
//   - *holePuncher 新创建的打洞器
func newHolePuncher(h host.Host, ids identify.IDService, listenAddrs func() []ma.Multiaddr, tracer *tracer, filter AddrFilter) *holePuncher {
	hp := &holePuncher{
		host:        h,
		ids:         ids,
		active:      make(map[peer.ID]struct{}),
		tracer:      tracer,
		filter:      filter,
		listenAddrs: listenAddrs,
	}
	hp.ctx, hp.ctxCancel = context.WithCancel(context.Background())
	h.Network().Notify((*netNotifiee)(hp))
	return hp
}

// beginDirectConnect 开始直接连接
// 参数:
//   - p: peer.ID 目标节点ID
//
// 返回值:
//   - error 错误信息
func (hp *holePuncher) beginDirectConnect(p peer.ID) error {
	hp.closeMx.RLock()
	defer hp.closeMx.RUnlock()
	if hp.closed {
		log.Debugf("打洞器已关闭")
		return ErrClosed
	}

	hp.activeMx.Lock()
	defer hp.activeMx.Unlock()
	if _, ok := hp.active[p]; ok {
		log.Debugf("打洞尝试正在进行中: %s", p)
		return ErrHolePunchActive
	}

	hp.active[p] = struct{}{}
	return nil
}

// DirectConnect 尝试与远程节点建立直接连接
// 它首先尝试直接拨号(如果我们有该节点的公共地址),然后通过给定的中继连接协调打洞
// 参数:
//   - p: peer.ID 目标节点ID
//
// 返回值:
//   - error 错误信息
func (hp *holePuncher) DirectConnect(p peer.ID) error {
	if err := hp.beginDirectConnect(p); err != nil {
		log.Debugf("开始直接连接失败: %v", err)
		return err
	}

	defer func() {
		hp.activeMx.Lock()
		delete(hp.active, p)
		hp.activeMx.Unlock()
	}()

	return hp.directConnect(p)
}

// directConnect 执行直接连接
// 参数:
//   - rp: peer.ID 目标节点ID
//
// 返回值:
//   - error 错误信息
func (hp *holePuncher) directConnect(rp peer.ID) error {
	// 检查是否已经有直接连接
	if getDirectConnection(hp.host, rp) != nil {
		log.Debugw("直接连接已存在", "peer", rp)
		return nil
	}
	// 如果直接拨号成功则跳过打洞
	// 仅当我们有远程节点的公共地址时才尝试直接连接
	for _, a := range hp.host.Peerstore().Addrs(rp) {
		if !isRelayAddress(a) && manet.IsPublicAddr(a) {
			forceDirectConnCtx := network.WithForceDirectDial(hp.ctx, "hole-punching")
			dialCtx, cancel := context.WithTimeout(forceDirectConnCtx, dialTimeout)

			tstart := time.Now()
			// 拨号所有地址,包括公共和私有地址
			err := hp.host.Connect(dialCtx, peer.AddrInfo{ID: rp})
			dt := time.Since(tstart)
			cancel()

			if err != nil {
				hp.tracer.DirectDialFailed(rp, dt, err)
				break
			}
			hp.tracer.DirectDialSuccessful(rp, dt)
			log.Debugw("直接连接成功,无需打洞", "peer", rp)
			return nil
		}
	}

	log.Debugw("收到入站代理连接", "peer", rp)

	// 打洞
	for i := 1; i <= maxRetries; i++ {
		addrs, obsAddrs, rtt, err := hp.initiateHolePunch(rp)
		if err != nil {
			log.Debugw("打洞失败", "peer", rp, "error", err)
			hp.tracer.ProtocolError(rp, err)
			return err
		}
		synTime := rtt / 2
		log.Debugf("节点 RTT 为 %s; %s 后开始打洞", rtt, synTime)

		// 等待同步到达对端,然后通过尝试连接在 NAT 中打洞
		timer := time.NewTimer(synTime)
		select {
		case start := <-timer.C:
			pi := peer.AddrInfo{
				ID:    rp,
				Addrs: addrs,
			}
			hp.tracer.StartHolePunch(rp, addrs, rtt)
			hp.tracer.HolePunchAttempt(pi.ID)
			err := holePunchConnect(hp.ctx, hp.host, pi, true)
			dt := time.Since(start)
			hp.tracer.EndHolePunch(rp, dt, err)
			if err == nil {
				log.Debugw("打洞成功", "peer", rp, "time", dt)
				hp.tracer.HolePunchFinished("initiator", i, addrs, obsAddrs, getDirectConnection(hp.host, rp))
				return nil
			}
		case <-hp.ctx.Done():
			timer.Stop()
			return hp.ctx.Err()
		}
		if i == maxRetries {
			hp.tracer.HolePunchFinished("initiator", maxRetries, addrs, obsAddrs, nil)
		}
	}
	return fmt.Errorf("与节点 %s 的所有打洞重试均失败", rp)
}

// initiateHolePunch 打开新的打洞协调流,交换地址并测量 RTT
// 参数:
//   - rp: peer.ID 目标节点ID
//
// 返回值:
//   - []ma.Multiaddr 地址列表
//   - []ma.Multiaddr 观察到的地址列表
//   - time.Duration RTT 时间
//   - error 错误信息
func (hp *holePuncher) initiateHolePunch(rp peer.ID) ([]ma.Multiaddr, []ma.Multiaddr, time.Duration, error) {
	hpCtx := network.WithAllowLimitedConn(hp.ctx, "hole-punch")
	sCtx := network.WithNoDial(hpCtx, "hole-punch")

	str, err := hp.host.NewStream(sCtx, rp, Protocol)
	if err != nil {
		log.Debugf("打开打洞流失败: %v", err)
		return nil, nil, 0, fmt.Errorf("打开打洞流失败: %w", err)
	}
	defer str.Close()

	addr, obsAddr, rtt, err := hp.initiateHolePunchImpl(str)
	if err != nil {
		log.Debugf("打洞初始化失败: %v", err)
		str.Reset()
		return addr, obsAddr, rtt, err
	}
	return addr, obsAddr, rtt, err
}

// initiateHolePunchImpl 实现打洞初始化
// 参数:
//   - str: network.Stream 网络流
//
// 返回值:
//   - []ma.Multiaddr 地址列表
//   - []ma.Multiaddr 观察到的地址列表
//   - time.Duration RTT 时间
//   - error 错误信息
func (hp *holePuncher) initiateHolePunchImpl(str network.Stream) ([]ma.Multiaddr, []ma.Multiaddr, time.Duration, error) {
	if err := str.Scope().SetService(ServiceName); err != nil {
		log.Debugf("将流附加到打洞服务时出错: %s", err)
		return nil, nil, 0, err
	}

	if err := str.Scope().ReserveMemory(maxMsgSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为流预留内存时出错: %s", err)
		return nil, nil, 0, err
	}
	defer str.Scope().ReleaseMemory(maxMsgSize)

	w := pbio.NewDelimitedWriter(str)
	rd := pbio.NewDelimitedReader(str, maxMsgSize)

	str.SetDeadline(time.Now().Add(StreamTimeout))

	// 发送 CONNECT 并开始 RTT 测量
	obsAddrs := removeRelayAddrs(hp.listenAddrs())
	if hp.filter != nil {
		obsAddrs = hp.filter.FilterLocal(str.Conn().RemotePeer(), obsAddrs)
	}
	if len(obsAddrs) == 0 {
		log.Debugf("因为没有公共地址而中止打洞初始化")
		return nil, nil, 0, errors.New("因为没有公共地址而中止打洞初始化")
	}

	start := time.Now()
	if err := w.WriteMsg(&pb.HolePunch{
		Type:     pb.HolePunch_CONNECT.Enum(),
		ObsAddrs: addrsToBytes(obsAddrs),
	}); err != nil {
		log.Debugf("发送 CONNECT 消息失败: %v", err)
		str.Reset()
		return nil, nil, 0, err
	}

	// 等待远程节点的 CONNECT 消息
	var msg pb.HolePunch
	if err := rd.ReadMsg(&msg); err != nil {
		log.Debugf("读取远程节点的 CONNECT 消息失败: %v", err)
		return nil, nil, 0, err
	}
	rtt := time.Since(start)
	if t := msg.GetType(); t != pb.HolePunch_CONNECT {
		log.Debugf("期望 CONNECT 消息,但收到 %s", t)
		return nil, nil, 0, fmt.Errorf("期望 CONNECT 消息,但收到 %s", t)
	}

	addrs := removeRelayAddrs(addrsFromBytes(msg.ObsAddrs))
	if hp.filter != nil {
		addrs = hp.filter.FilterRemote(str.Conn().RemotePeer(), addrs)
	}

	if len(addrs) == 0 {
		log.Debugf("在 CONNECT 中没有收到任何公共地址")
		return nil, nil, 0, errors.New("在 CONNECT 中没有收到任何公共地址")
	}

	if err := w.WriteMsg(&pb.HolePunch{Type: pb.HolePunch_SYNC.Enum()}); err != nil {
		log.Debugf("发送打洞 SYNC 消息失败: %v", err)
		return nil, nil, 0, err
	}
	return addrs, obsAddrs, rtt, nil
}

// Close 关闭打洞器
// 返回值:
//   - error 错误信息
func (hp *holePuncher) Close() error {
	hp.closeMx.Lock()
	hp.closed = true
	hp.closeMx.Unlock()
	hp.ctxCancel()
	hp.refCount.Wait()
	return nil
}

// netNotifiee 网络通知器
type netNotifiee holePuncher

// Connected 处理连接建立事件
// 参数:
//   - _: network.Network 网络对象(未使用)
//   - conn: network.Conn 网络连接
func (nn *netNotifiee) Connected(_ network.Network, conn network.Conn) {
	hs := (*holePuncher)(nn)

	// 如果是入站代理连接则进行打洞
	// 如果我们已经与远程节点有直接连接,这将是空操作
	if conn.Stat().Direction == network.DirInbound && isRelayAddress(conn.RemoteMultiaddr()) {
		hs.refCount.Add(1)
		go func() {
			defer hs.refCount.Done()

			select {
			// 等待身份识别,这将允许我们访问节点的公共和观察到的地址,用于打洞拨号
			case <-hs.ids.IdentifyWait(conn):
			case <-hs.ctx.Done():
				return
			}

			_ = hs.DirectConnect(conn.RemotePeer())
		}()
	}
}

// Disconnected 处理连接断开事件
func (nn *netNotifiee) Disconnected(_ network.Network, v network.Conn) {}

// Listen 处理开始监听事件
func (nn *netNotifiee) Listen(n network.Network, a ma.Multiaddr) {}

// ListenClose 处理停止监听事件
func (nn *netNotifiee) ListenClose(n network.Network, a ma.Multiaddr) {}
