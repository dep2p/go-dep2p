package autonat

import (
	"context"
	"math/rand"
	"slices"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	logging "github.com/dep2p/log"
)

// log 用于记录host-autonat相关的日志
var log = logging.Logger("host-autonat")

// maxConfidence 定义最大置信度
const maxConfidence = 3

// AmbientAutoNAT 实现了环境NAT自动发现
type AmbientAutoNAT struct {
	host host.Host // dep2p主机实例

	*config // 配置信息

	ctx               context.Context    // 上下文
	ctxCancel         context.CancelFunc // Close调用时关闭
	backgroundRunning chan struct{}      // 后台goroutine退出时关闭

	inboundConn   chan network.Conn                    // 入站连接通道
	dialResponses chan error                           // 拨号响应通道
	observations  chan network.Reachability            // 用于测试autonat服务的观察通道
	status        atomic.Pointer[network.Reachability] // 当前状态的原子指针
	// confidence 反映了NAT状态为私有的置信度
	// 单次回拨可能因为与NAT无关的原因失败
	// 如果小于3,则可能会联系多个autoNAT节点进行回拨
	// 如果只知道一个autoNAT节点,则每次失败都会增加置信度直到达到3
	confidence    int                   // NAT状态置信度
	lastInbound   time.Time             // 最后一次入站连接时间
	lastProbe     time.Time             // 最后一次探测时间
	recentProbes  map[peer.ID]time.Time // 最近探测的节点记录
	pendingProbes int                   // 待处理的探测数量
	ourAddrs      map[string]struct{}   // 我们的地址集合

	service *autoNATService // autoNAT服务实例

	emitReachabilityChanged event.Emitter      // 可达性变更事件发射器
	subscriber              event.Subscription // 事件订阅
}

// StaticAutoNAT 是一个简单的AutoNAT实现,用于单一NAT状态场景
type StaticAutoNAT struct {
	host         host.Host            // dep2p主机实例
	reachability network.Reachability // 可达性状态
	service      *autoNATService      // autoNAT服务实例
}

// New 创建一个新的NAT自动发现系统并附加到主机
// 参数:
//   - h: host.Host 主机实例
//   - options: ...Option 可选的配置选项
//
// 返回值:
//   - AutoNAT: 返回创建的AutoNAT实例
//   - error: 如果发生错误则返回错误信息
func New(h host.Host, options ...Option) (AutoNAT, error) {
	var err error
	// 创建新的配置
	conf := new(config)
	conf.host = h
	conf.dialPolicy.host = h

	// 应用默认配置
	if err = defaults(conf); err != nil {
		log.Debugf("应用默认配置失败: %v", err)
		return nil, err
	}
	// 设置地址获取函数
	if conf.addressFunc == nil {
		if aa, ok := h.(interface{ AllAddrs() []ma.Multiaddr }); ok {
			conf.addressFunc = aa.AllAddrs
		} else {
			conf.addressFunc = h.Addrs
		}
	}

	// 应用选项配置
	for _, o := range options {
		if err = o(conf); err != nil {
			log.Debugf("应用配置选项失败: %v", err)
			return nil, err
		}
	}
	// 创建可达性变更事件发射器
	emitReachabilityChanged, _ := h.EventBus().Emitter(new(event.EvtLocalReachabilityChanged), eventbus.Stateful)

	// 创建autoNAT服务
	var service *autoNATService
	if (!conf.forceReachability || conf.reachability == network.ReachabilityPublic) && conf.dialer != nil {
		service, err = newAutoNATService(conf)
		if err != nil {
			log.Debugf("创建autoNAT服务失败: %v", err)
			return nil, err
		}
		service.Enable()
	}

	// 如果强制设置可达性,返回静态AutoNAT
	if conf.forceReachability {
		emitReachabilityChanged.Emit(event.EvtLocalReachabilityChanged{Reachability: conf.reachability})

		return &StaticAutoNAT{
			host:         h,
			reachability: conf.reachability,
			service:      service,
		}, nil
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	// 创建AmbientAutoNAT实例
	as := &AmbientAutoNAT{
		ctx:               ctx,
		ctxCancel:         cancel,
		backgroundRunning: make(chan struct{}),
		host:              h,
		config:            conf,
		inboundConn:       make(chan network.Conn, 5),
		dialResponses:     make(chan error, 1),
		observations:      make(chan network.Reachability, 1),

		emitReachabilityChanged: emitReachabilityChanged,
		service:                 service,
		recentProbes:            make(map[peer.ID]time.Time),
		ourAddrs:                make(map[string]struct{}),
	}
	// 设置初始可达性状态为未知
	reachability := network.ReachabilityUnknown
	as.status.Store(&reachability)

	// 订阅事件
	subscriber, err := as.host.EventBus().Subscribe(
		[]any{new(event.EvtLocalAddressesUpdated), new(event.EvtPeerIdentificationCompleted)},
		eventbus.Name("autonat"),
	)
	if err != nil {
		log.Debugf("订阅事件失败: %v", err)
		return nil, err
	}
	as.subscriber = subscriber

	// 启动后台任务
	go as.background()

	return as, nil
}

// Status 返回AutoNAT观察到的可达性状态
// 返回值:
//   - network.Reachability: 返回当前的可达性状态
func (as *AmbientAutoNAT) Status() network.Reachability {
	s := as.status.Load()
	return *s
}

// emitStatus 发出状态变更事件
func (as *AmbientAutoNAT) emitStatus() {
	status := *as.status.Load()
	as.emitReachabilityChanged.Emit(event.EvtLocalReachabilityChanged{Reachability: status})
	if as.metricsTracer != nil {
		as.metricsTracer.ReachabilityStatus(status)
	}
}

// ipInList 检查给定地址的IP是否在地址列表中
// 参数:
//   - candidate: ma.Multiaddr 待检查的地址
//   - list: []ma.Multiaddr 地址列表
//
// 返回值:
//   - bool: 如果IP在列表中返回true,否则返回false
func ipInList(candidate ma.Multiaddr, list []ma.Multiaddr) bool {
	candidateIP, _ := manet.ToIP(candidate)
	for _, i := range list {
		if ip, err := manet.ToIP(i); err == nil && ip.Equal(candidateIP) {
			return true
		}
	}
	return false
}

// background 运行后台任务处理NAT探测和状态更新
func (as *AmbientAutoNAT) background() {
	defer close(as.backgroundRunning)
	// 等待节点上线并建立一些连接后再开始自动检测
	delay := as.config.bootDelay

	// 获取订阅通道
	subChan := as.subscriber.Out()
	defer as.subscriber.Close()
	defer as.emitReachabilityChanged.Close()

	// 创建地址变更回退定时器
	// TODO: 事件未正确发出是一个bug,这是一个临时解决方案
	addrChangeTicker := time.NewTicker(30 * time.Minute)
	defer addrChangeTicker.Stop()

	// 创建探测定时器
	timer := time.NewTimer(delay)
	defer timer.Stop()
	timerRunning := true
	forceProbe := false

	// 主循环处理各种事件
	for {
		select {
		case conn := <-as.inboundConn:
			// 处理入站连接
			localAddrs := as.host.Addrs()
			if manet.IsPublicAddr(conn.RemoteMultiaddr()) &&
				!ipInList(conn.RemoteMultiaddr(), localAddrs) {
				as.lastInbound = time.Now()
			}
		case <-addrChangeTicker.C:
			// 地址变更时安排新的探测
		case e := <-subChan:
			// 处理事件
			switch e := e.(type) {
			case event.EvtPeerIdentificationCompleted:
				if proto, err := as.host.Peerstore().SupportsProtocols(e.Peer, AutoNATProto); err == nil && len(proto) > 0 {
					forceProbe = true
				}
			case event.EvtLocalAddressesUpdated:
				// 地址更新时安排新的探测
			default:
				log.Debugf("未知的事件类型: %T", e)
			}
		case obs := <-as.observations:
			// 记录观察结果
			as.recordObservation(obs)
			continue
		case err, ok := <-as.dialResponses:
			// 处理拨号响应
			if !ok {
				return
			}
			as.pendingProbes--
			if IsDialRefused(err) {
				forceProbe = true
			} else {
				as.handleDialResponse(err)
			}
		case <-timer.C:
			// 定时器触发,执行探测
			timerRunning = false
			forceProbe = false
			as.lastProbe = time.Now()
			peer := as.getPeerToProbe()
			as.tryProbe(peer)
		case <-as.ctx.Done():
			return
		}
		// 地址更新时,如果置信度最大则降低一级
		hasNewAddr := as.checkAddrs()
		if hasNewAddr && as.confidence == maxConfidence {
			as.confidence--
		}

		// 重置定时器
		if timerRunning && !timer.Stop() {
			<-timer.C
		}
		timer.Reset(as.scheduleProbe(forceProbe))
		timerRunning = true
	}
}

// checkAddrs 检查地址是否有更新
// 返回值:
//   - hasNewAddr: bool - 是否有新地址
func (as *AmbientAutoNAT) checkAddrs() (hasNewAddr bool) {
	// 获取当前地址列表
	currentAddrs := as.addressFunc()
	// 检查是否有新地址
	hasNewAddr = slices.ContainsFunc(currentAddrs, func(a ma.Multiaddr) bool {
		_, ok := as.ourAddrs[string(a.Bytes())]
		return !ok
	})
	// 清空当前地址映射
	clear(as.ourAddrs)
	// 重新填充公共地址
	for _, a := range currentAddrs {
		if !manet.IsPublicAddr(a) {
			continue
		}
		as.ourAddrs[string(a.Bytes())] = struct{}{}
	}
	return hasNewAddr
}

// scheduleProbe 计算下一次探测的时间间隔
// 参数:
//   - forceProbe: bool - 是否强制探测
//
// 返回值:
//   - time.Duration - 下一次探测的时间间隔
func (as *AmbientAutoNAT) scheduleProbe(forceProbe bool) time.Duration {
	// 获取当前时间
	now := time.Now()
	// 获取当前状态
	currentStatus := *as.status.Load()
	// 设置默认探测间隔
	nextProbeAfter := as.config.refreshInterval
	// 检查是否收到入站连接
	receivedInbound := as.lastInbound.After(as.lastProbe)

	switch {
	case forceProbe && currentStatus == network.ReachabilityUnknown:
		// 如果强制探测且状态未知,快速重试
		nextProbeAfter = 2 * time.Second
	case currentStatus == network.ReachabilityUnknown,
		as.confidence < maxConfidence,
		currentStatus != network.ReachabilityPublic && receivedInbound:
		// 以下情况快速重试:
		// 1. 状态未知
		// 2. 置信度不足
		// 3. 非公开状态但收到入站连接
		nextProbeAfter = as.config.retryInterval
	case currentStatus == network.ReachabilityPublic && receivedInbound:
		// 公开状态且收到入站连接,延长间隔
		nextProbeAfter *= 2
		nextProbeAfter = min(nextProbeAfter, maxRefreshInterval)
	}

	// 计算下次探测时间
	nextProbeTime := as.lastProbe.Add(nextProbeAfter)
	if nextProbeTime.Before(now) {
		nextProbeTime = now
	}
	// 更新指标
	if as.metricsTracer != nil {
		as.metricsTracer.NextProbeTime(nextProbeTime)
	}

	return nextProbeTime.Sub(now)
}

// handleDialResponse 处理拨号响应并更新当前状态
// 参数:
//   - dialErr: error - 拨号错误
func (as *AmbientAutoNAT) handleDialResponse(dialErr error) {
	// 根据错误类型确定可达性
	var observation network.Reachability
	switch {
	case dialErr == nil:
		observation = network.ReachabilityPublic
	case IsDialError(dialErr):
		observation = network.ReachabilityPrivate
	default:
		observation = network.ReachabilityUnknown
	}

	// 记录观察结果
	as.recordObservation(observation)
}

// recordObservation 更新 NAT 状态和置信度
// 参数:
//   - observation: network.Reachability - 观察到的可达性状态
func (as *AmbientAutoNAT) recordObservation(observation network.Reachability) {
	// 获取当前状态
	currentStatus := *as.status.Load()

	if observation == network.ReachabilityPublic {
		changed := false
		if currentStatus != network.ReachabilityPublic {
			// 从其他状态快速切换到公开状态
			log.Debugf("NAT 状态为公开")

			// 状态改变,置信度归零
			as.confidence = 0
			if as.service != nil {
				as.service.Enable()
			}
			changed = true
		} else if as.confidence < maxConfidence {
			as.confidence++
		}
		as.status.Store(&observation)
		if changed {
			as.emitStatus()
		}
	} else if observation == network.ReachabilityPrivate {
		if currentStatus != network.ReachabilityPrivate {
			if as.confidence > 0 {
				as.confidence--
			} else {
				log.Debugf("NAT 状态为私有")

				// 状态改变,置信度归零
				as.confidence = 0
				as.status.Store(&observation)
				if as.service != nil {
					as.service.Disable()
				}
				as.emitStatus()
			}
		} else if as.confidence < maxConfidence {
			as.confidence++
			as.status.Store(&observation)
		}
	} else if as.confidence > 0 {
		// 不直接切换到未知状态,先降低置信度
		as.confidence--
	} else {
		log.Debugf("NAT 状态未知")
		as.status.Store(&observation)
		if currentStatus != network.ReachabilityUnknown {
			if as.service != nil {
				as.service.Enable()
			}
			as.emitStatus()
		}
	}
	// 更新指标
	if as.metricsTracer != nil {
		as.metricsTracer.ReachabilityStatusConfidence(as.confidence)
	}
}

// tryProbe 尝试对指定节点进行探测
// 参数:
//   - p: peer.ID - 目标节点 ID
func (as *AmbientAutoNAT) tryProbe(p peer.ID) {
	// 检查是否可以进行探测
	if p == "" || as.pendingProbes > 5 {
		return
	}
	// 获取节点信息
	info := as.host.Peerstore().PeerInfo(p)
	as.recentProbes[p] = time.Now()
	as.pendingProbes++
	// 异步执行探测
	go as.probe(&info)
}

// probe 执行实际的探测操作
// 参数:
//   - pi: *peer.AddrInfo - 目标节点信息
func (as *AmbientAutoNAT) probe(pi *peer.AddrInfo) {
	// 创建 AutoNAT 客户端
	cli := NewAutoNATClient(as.host, as.config.addressFunc, as.metricsTracer)
	// 创建超时上下文
	ctx, cancel := context.WithTimeout(as.ctx, as.config.requestTimeout)
	defer cancel()

	// 执行回拨
	err := cli.DialBack(ctx, pi.ID)
	log.Debugf("通过节点 %s 完成回拨: err: %s", pi.ID, err)

	// 发送拨号响应
	select {
	case as.dialResponses <- err:
	case <-as.ctx.Done():
		return
	}
}

// getPeerToProbe 获取下一个要探测的节点
// 返回值:
//   - peer.ID - 选中的节点 ID
func (as *AmbientAutoNAT) getPeerToProbe() peer.ID {
	// 获取所有连接的节点
	peers := as.host.Network().Peers()
	if len(peers) == 0 {
		return ""
	}

	// 清理过期的探测记录
	fixedNow := time.Now()
	for k, v := range as.recentProbes {
		if fixedNow.Sub(v) > as.throttlePeerPeriod {
			delete(as.recentProbes, k)
		}
	}

	// 随机打乱节点顺序
	for n := len(peers); n > 0; n-- {
		randIndex := rand.Intn(n)
		peers[n-1], peers[randIndex] = peers[randIndex], peers[n-1]
	}

	// 选择合适的节点
	for _, p := range peers {
		info := as.host.Peerstore().PeerInfo(p)
		// 排除不支持 autonat 协议的节点
		if proto, err := as.host.Peerstore().SupportsProtocols(p, AutoNATProto); len(proto) == 0 || err != nil {
			continue
		}

		// 检查是否需要跳过该节点
		if as.config.dialPolicy.skipPeer(info.Addrs) {
			continue
		}
		return p
	}

	return ""
}

// Close 关闭 AutoNAT 服务
// 返回值:
//   - error 关闭过程中的错误
func (as *AmbientAutoNAT) Close() error {
	as.ctxCancel()
	if as.service != nil {
		return as.service.Close()
	}
	<-as.backgroundRunning
	return nil
}

// Status 返回静态 AutoNAT 的可达性状态
// 返回值:
//   - network.Reachability - 当前的可达性状态
func (s *StaticAutoNAT) Status() network.Reachability {
	return s.reachability
}

// Close 关闭静态 AutoNAT 服务
// 返回值:
//   - error 关闭过程中的错误
func (s *StaticAutoNAT) Close() error {
	if s.service != nil {
		return s.service.Close()
	}
	return nil
}
