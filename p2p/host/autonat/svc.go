package autonat

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/p2p/host/autonat/pb"

	"github.com/dep2p/go-dep2p/p2plib/msgio/pbio"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// 流超时时间为60秒
var streamTimeout = 60 * time.Second

const (
	ServiceName = "dep2p.autonat" // 服务名称
	maxMsgSize  = 4096            // 最大消息大小
)

// autoNATService 为其他节点提供NAT自动检测服务
type autoNATService struct {
	instanceLock      sync.Mutex         // 实例锁,用于保护实例状态
	instance          context.CancelFunc // 实例取消函数
	backgroundRunning chan struct{}      // 后台运行状态通道,后台退出时关闭

	config *config // 配置对象

	// 速率限制器
	mx         sync.Mutex      // 互斥锁,用于保护请求计数
	reqs       map[peer.ID]int // 每个节点的请求计数
	globalReqs int             // 全局请求计数
}

// newAutoNATService 创建一个新的AutoNATService实例并附加到主机
// 参数:
//   - c: *config 配置对象
//
// 返回值:
//   - *autoNATService: 返回创建的服务实例
//   - error: 如果发生错误则返回错误信息
func newAutoNATService(c *config) (*autoNATService, error) {
	if c.dialer == nil {
		log.Debugf("无法在没有网络的情况下创建NAT服务")
		return nil, errors.New("无法在没有网络的情况下创建NAT服务")
	}
	return &autoNATService{
		config: c,
		reqs:   make(map[peer.ID]int),
	}, nil
}

// handleStream 处理传入的流
// 参数:
//   - s: network.Stream 网络流对象
func (as *autoNATService) handleStream(s network.Stream) {
	// 设置流的服务范围
	if err := s.Scope().SetService(ServiceName); err != nil {
		log.Debugf("将流附加到autonat服务时出错: %s", err)
		s.Reset()
		return
	}

	// 为流预留内存
	if err := s.Scope().ReserveMemory(maxMsgSize, network.ReservationPriorityAlways); err != nil {
		log.Debugf("为autonat流预留内存时出错: %s", err)
		s.Reset()
		return
	}
	defer s.Scope().ReleaseMemory(maxMsgSize)

	// 设置流的超时时间
	s.SetDeadline(time.Now().Add(streamTimeout))
	defer s.Close()

	pid := s.Conn().RemotePeer()
	log.Debugf("来自 %s 的新流", pid)

	// 创建读写器
	r := pbio.NewDelimitedReader(s, maxMsgSize)
	w := pbio.NewDelimitedWriter(s)

	var req pb.Message
	var res pb.Message

	// 读取请求消息
	err := r.ReadMsg(&req)
	if err != nil {
		log.Debugf("从 %s 读取消息时出错: %s", pid, err.Error())
		s.Reset()
		return
	}

	// 检查消息类型
	t := req.GetType()
	if t != pb.Message_DIAL {
		log.Debugf("来自 %s 的意外消息: %s (%d)", pid, t.String(), t)
		s.Reset()
		return
	}

	// 处理拨号请求
	dr := as.handleDial(pid, s.Conn().RemoteMultiaddr(), req.GetDial().GetPeer())
	res.Type = pb.Message_DIAL_RESPONSE.Enum()
	res.DialResponse = dr

	// 写入响应消息
	err = w.WriteMsg(&res)
	if err != nil {
		log.Debugf("向 %s 写入响应时出错: %s", pid, err.Error())
		s.Reset()
		return
	}
	if as.config.metricsTracer != nil {
		as.config.metricsTracer.OutgoingDialResponse(res.GetDialResponse().GetStatus())
	}
}

// handleDial 处理拨号请求
// 参数:
//   - p: peer.ID 请求节点ID
//   - obsaddr: ma.Multiaddr 观察到的地址
//   - mpi: *pb.Message_PeerInfo 节点信息
//
// 返回值:
//   - *pb.Message_DialResponse: 返回拨号响应
func (as *autoNATService) handleDial(p peer.ID, obsaddr ma.Multiaddr, mpi *pb.Message_PeerInfo) *pb.Message_DialResponse {
	// 验证节点信息
	if mpi == nil {
		log.Debugf("缺少节点信息")
		return newDialResponseError(pb.Message_E_BAD_REQUEST, "缺少节点信息")
	}

	// 验证节点ID
	mpid := mpi.GetId()
	if mpid != nil {
		mp, err := peer.IDFromBytes(mpid)
		if err != nil {
			log.Debugf("无效的节点ID: %v", err)
			return newDialResponseError(pb.Message_E_BAD_REQUEST, "无效的节点ID")
		}

		if mp != p {
			log.Debugf("节点ID不匹配")
			return newDialResponseError(pb.Message_E_BAD_REQUEST, "节点ID不匹配")
		}
	}

	// 初始化地址列表
	addrs := make([]ma.Multiaddr, 0, as.config.maxPeerAddresses)
	seen := make(map[string]struct{})

	// 检查远程地址是否被阻止
	if as.config.dialPolicy.skipDial(obsaddr) {
		if as.config.metricsTracer != nil {
			as.config.metricsTracer.OutgoingDialRefused(dial_blocked)
		}
		return newDialResponseError(pb.Message_E_DIAL_REFUSED, "拒绝拨号到具有被阻止观察地址的节点")
	}

	// 获取节点的IP地址
	hostIP, _ := ma.SplitFirst(obsaddr)
	switch hostIP.Protocol().Code {
	case ma.P_IP4, ma.P_IP6:
	default:
		return newDialResponseError(pb.Message_E_INTERNAL_ERROR, "需要IP地址")
	}

	// 添加观察到的地址
	addrs = append(addrs, obsaddr)
	seen[obsaddr.String()] = struct{}{}

	// 处理其他地址
	for _, maddr := range mpi.GetAddrs() {
		addr, err := ma.NewMultiaddrBytes(maddr)
		if err != nil {
			log.Debugf("解析多地址时出错: %s", err.Error())
			continue
		}

		// 出于安全考虑,只拨号观察到的IP地址
		if ip, rest := ma.SplitFirst(addr); !ip.Equal(hostIP) {
			switch ip.Protocol().Code {
			case ma.P_IP4, ma.P_IP6:
			default:
				log.Debugf("跳过非IP地址: %s", addr.String())
				continue
			}
			addr = hostIP
			if rest != nil {
				addr = addr.Encapsulate(rest)
			}
		}

		// 检查是否应该跳过此地址
		if as.config.dialPolicy.skipDial(addr) {
			log.Debugf("跳过地址: %s", addr.String())
			continue
		}

		str := addr.String()
		_, ok := seen[str]
		if ok {
			log.Debugf("地址已存在: %s", addr.String())
			continue
		}

		addrs = append(addrs, addr)
		seen[str] = struct{}{}

		if len(addrs) >= as.config.maxPeerAddresses {
			break
		}
	}

	// 检查是否有可用地址
	if len(addrs) == 0 {
		if as.config.metricsTracer != nil {
			as.config.metricsTracer.OutgoingDialRefused(no_valid_address)
		}
		log.Debugf("没有可拨号的地址")
		return newDialResponseError(pb.Message_E_DIAL_REFUSED, "没有可拨号的地址")
	}

	return as.doDial(peer.AddrInfo{ID: p, Addrs: addrs})
}

// doDial 执行实际的拨号操作
// 参数:
//   - pi: peer.AddrInfo 要拨号的节点信息
//
// 返回值:
//   - *pb.Message_DialResponse: 返回拨号响应
func (as *autoNATService) doDial(pi peer.AddrInfo) *pb.Message_DialResponse {
	// 速率限制检查
	as.mx.Lock()
	count := as.reqs[pi.ID]
	if count >= as.config.throttlePeerMax || (as.config.throttleGlobalMax > 0 &&
		as.globalReqs >= as.config.throttleGlobalMax) {
		as.mx.Unlock()
		if as.config.metricsTracer != nil {
			as.config.metricsTracer.OutgoingDialRefused(rate_limited)
		}
		return newDialResponseError(pb.Message_E_DIAL_REFUSED, "拨号次数过多")
	}
	as.reqs[pi.ID] = count + 1
	as.globalReqs++
	as.mx.Unlock()

	// 创建超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), as.config.dialTimeout)
	defer cancel()

	// 清理并添加临时地址
	as.config.dialer.Peerstore().ClearAddrs(pi.ID)
	as.config.dialer.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.TempAddrTTL)

	defer func() {
		as.config.dialer.Peerstore().ClearAddrs(pi.ID)
		as.config.dialer.Peerstore().RemovePeer(pi.ID)
	}()

	// 执行拨号
	conn, err := as.config.dialer.DialPeer(ctx, pi.ID)
	if err != nil {
		log.Debugf("拨号 %s 时出错: %s", pi.ID, err.Error())
		<-ctx.Done()
		return newDialResponseError(pb.Message_E_DIAL_ERROR, "拨号失败")
	}

	ra := conn.RemoteMultiaddr()
	as.config.dialer.ClosePeer(pi.ID)
	return newDialResponseOK(ra)
}

// Enable 如果服务未运行则启用autoNAT服务
func (as *autoNATService) Enable() {
	as.instanceLock.Lock()
	defer as.instanceLock.Unlock()
	if as.instance != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	as.instance = cancel
	as.backgroundRunning = make(chan struct{})
	as.config.host.SetStreamHandler(AutoNATProto, as.handleStream)

	go as.background(ctx)
}

// Disable 如果服务正在运行则禁用autoNAT服务
func (as *autoNATService) Disable() {
	as.instanceLock.Lock()
	defer as.instanceLock.Unlock()
	if as.instance != nil {
		as.config.host.RemoveStreamHandler(AutoNATProto)
		as.instance()
		as.instance = nil
		<-as.backgroundRunning
	}
}

// Close 关闭autoNAT服务
// 返回值:
//   - error: 如果发生错误则返回错误信息
func (as *autoNATService) Close() error {
	as.Disable()
	return as.config.dialer.Close()
}

// background 运行后台任务
// 参数:
//   - ctx: context.Context 上下文对象
func (as *autoNATService) background(ctx context.Context) {
	defer close(as.backgroundRunning)

	// 创建定时器
	timer := time.NewTimer(as.config.throttleResetPeriod)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			// 重置请求计数
			as.mx.Lock()
			as.reqs = make(map[peer.ID]int)
			as.globalReqs = 0
			as.mx.Unlock()
			// 添加随机抖动
			jitter := rand.Float32() * float32(as.config.throttleResetJitter)
			timer.Reset(as.config.throttleResetPeriod + time.Duration(int64(jitter)))
		case <-ctx.Done():
			return
		}
	}
}
