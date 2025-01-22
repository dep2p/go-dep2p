package basichost

import (
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/record"
	"github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/host/autonat"
	"github.com/dep2p/go-dep2p/p2p/host/basic/internal/backoff"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"
	"github.com/dep2p/go-dep2p/p2p/host/pstoremanager"
	"github.com/dep2p/go-dep2p/p2p/host/relaysvc"
	"github.com/dep2p/go-dep2p/p2p/protocol/autonatv2"
	relayv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/relay"
	"github.com/dep2p/go-dep2p/p2p/protocol/holepunch"
	"github.com/dep2p/go-dep2p/p2p/protocol/identify"
	"github.com/dep2p/go-dep2p/p2p/protocol/ping"
	dep2pwebrtc "github.com/dep2p/go-dep2p/p2p/transport/webrtc"
	dep2pwebtransport "github.com/dep2p/go-dep2p/p2p/transport/webtransport"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/dep2p/go-dep2p/p2plib/netroute"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	msmux "github.com/dep2p/go-dep2p/multiformats/multistream"
	logging "github.com/dep2p/log"
)

// 两次地址变更检查的时间间隔
var addrChangeTickrInterval = 5 * time.Second

var log = logging.Logger("host-basic")

var (
	// 协议协商超时的默认值
	DefaultNegotiationTimeout = 10 * time.Second

	// 地址工厂的默认值
	DefaultAddrsFactory = func(addrs []ma.Multiaddr) []ma.Multiaddr { return addrs }
)

// 对等节点记录的最大大小,设为8k以兼容identify的限制
const maxPeerRecordSize = 8 * 1024

// AddrsFactory 函数可以传递给New来覆盖Addrs返回的地址
type AddrsFactory func([]ma.Multiaddr) []ma.Multiaddr

// BasicHost 是host.Host接口的基本实现。这个特定的host实现:
//   - 使用协议多路复用器来复用每个协议的流
//   - 使用身份服务来发送和接收节点信息
//   - 使用nat服务来建立NAT端口映射
type BasicHost struct {
	ctx       context.Context    // 上下文
	ctxCancel context.CancelFunc // 取消函数
	// 确保我们只关闭一次
	closeSync sync.Once
	// 跟踪在关闭前需要等待的资源
	refCount sync.WaitGroup

	network      network.Network                      // 网络层
	psManager    *pstoremanager.PeerstoreManager      // 对等节点存储管理器
	mux          *msmux.MultistreamMuxer[protocol.ID] // 多流复用器
	ids          identify.IDService                   // 身份服务
	hps          *holepunch.Service                   // 打洞服务
	pings        *ping.PingService                    // ping服务
	natmgr       NATManager                           // NAT管理器
	cmgr         connmgr.ConnManager                  // 连接管理器
	eventbus     event.Bus                            // 事件总线
	relayManager *relaysvc.RelayManager               // 中继管理器

	AddrsFactory AddrsFactory // 地址工厂

	negtimeout time.Duration // 协商超时时间

	emitters struct {
		evtLocalProtocolsUpdated event.Emitter // 本地协议更新事件发射器
		evtLocalAddrsUpdated     event.Emitter // 本地地址更新事件发射器
	}

	addrChangeChan chan struct{} // 地址变更通知通道

	addrMu                 sync.RWMutex       // 地址互斥锁
	updateLocalIPv4Backoff backoff.ExpBackoff // IPv4更新退避
	updateLocalIPv6Backoff backoff.ExpBackoff // IPv6更新退避
	filteredInterfaceAddrs []ma.Multiaddr     // 过滤后的接口地址
	allInterfaceAddrs      []ma.Multiaddr     // 所有接口地址

	disableSignedPeerRecord bool                        // 是否禁用签名的对等节点记录
	signKey                 crypto.PrivKey              // 签名密钥
	caBook                  peerstore.CertifiedAddrBook // 认证地址簿

	autoNat autonat.AutoNAT // 自动NAT

	autonatv2 *autonatv2.AutoNAT // 自动NATv2
}

var _ host.Host = (*BasicHost)(nil)

// HostOpts 包含可以传递给NewHost的选项,用于自定义BasicHost的构建
type HostOpts struct {
	// EventBus 设置事件总线。如果省略将构造新的事件总线
	EventBus event.Bus

	// MultistreamMuxer 对BasicHost来说是必需的,如果省略将使用合理的默认值
	MultistreamMuxer *msmux.MultistreamMuxer[protocol.ID]

	// NegotiationTimeout 决定协商流协议时的读写超时。
	// 如果为0或省略,将使用DefaultNegotiationTimeout。
	// 如果小于0,将停用流的超时。
	NegotiationTimeout time.Duration

	// AddrsFactory 持有一个可用于覆盖或过滤Addrs结果的函数。
	// 如果省略,则没有覆盖或过滤,Addrs和AllAddrs的结果相同。
	AddrsFactory AddrsFactory

	// NATManager 负责设置NAT端口映射和发现外部地址。
	// 如果省略,这个功能将被禁用。
	NATManager func(network.Network) NATManager

	// ConnManager 是dep2p连接管理器,用于管理与其他节点的连接
	ConnManager connmgr.ConnManager

	// EnablePing 指示是否实例化ping服务,用于检测节点之间的连通性
	EnablePing bool

	// EnableRelayService 启用circuit v2中继(如果我们可以公开访问)
	// 中继服务允许不能直接连接的节点通过中继节点进行通信
	EnableRelayService bool

	// RelayServiceOpts 是circuit v2中继的选项,用于配置中继服务的行为
	RelayServiceOpts []relayv2.Option

	// UserAgent 设置主机的用户代理字符串,用于标识节点的客户端软件
	UserAgent string

	// ProtocolVersion 设置主机的协议版本,用于确保节点间协议的兼容性
	ProtocolVersion string

	// DisableSignedPeerRecord 在此主机上禁用签名对等节点记录的生成
	// 签名对等节点记录用于验证节点身份和地址信息的真实性
	DisableSignedPeerRecord bool

	// EnableHolePunching 使对等节点能够发起/响应NAT穿透的打洞尝试
	// 打洞技术用于帮助位于NAT后的节点建立直接连接
	EnableHolePunching bool

	// HolePunchingOptions 是打洞服务的选项,用于配置NAT穿透的行为
	HolePunchingOptions []holepunch.Option

	// EnableMetrics 启用指标子系统,用于收集和导出各种性能和行为指标
	EnableMetrics bool

	// PrometheusRegisterer 是用于指标的Prometheus注册器
	// 用于将收集的指标注册到Prometheus监控系统
	PrometheusRegisterer prometheus.Registerer

	// DisableIdentifyAddressDiscovery 禁用使用对等节点在identify中提供的观察地址进行地址发现
	// 地址发现用于了解节点在网络中的可见地址
	DisableIdentifyAddressDiscovery bool

	// EnableAutoNATv2 启用自动NAT检测和穿透的第二版本
	EnableAutoNATv2 bool

	// AutoNATv2Dialer 是用于AutoNATv2服务的拨号器主机
	// 用于执行NAT类型检测和地址可达性测试
	AutoNATv2Dialer host.Host
}

// NewHost 构造一个新的*BasicHost并通过将其流和连接处理程序附加到给定的inet.Network来激活它
// 参数:
//   - n: network.Network - 网络接口实例
//   - opts: *HostOpts - 主机配置选项
//
// 返回:
//   - *BasicHost - 创建的基础主机实例
//   - error 错误信息
func NewHost(n network.Network, opts *HostOpts) (*BasicHost, error) {
	// 如果选项为空则创建默认选项
	if opts == nil {
		opts = &HostOpts{}
	}
	// 如果事件总线为空则创建新的事件总线
	if opts.EventBus == nil {
		opts.EventBus = eventbus.NewBus()
	}

	// 创建对等节点存储管理器
	psManager, err := pstoremanager.NewPeerstoreManager(n.Peerstore(), opts.EventBus, n)
	if err != nil {
		log.Debugf("创建对等节点存储管理器失败: %v", err)
		return nil, err
	}

	// 创建主机上下文和取消函数
	hostCtx, cancel := context.WithCancel(context.Background())

	// 初始化基础主机结构体
	h := &BasicHost{
		network:                 n,
		psManager:               psManager,
		mux:                     msmux.NewMultistreamMuxer[protocol.ID](),
		negtimeout:              DefaultNegotiationTimeout,
		AddrsFactory:            DefaultAddrsFactory,
		eventbus:                opts.EventBus,
		addrChangeChan:          make(chan struct{}, 1),
		ctx:                     hostCtx,
		ctxCancel:               cancel,
		disableSignedPeerRecord: opts.DisableSignedPeerRecord,
	}

	// 更新本地IP地址
	h.updateLocalIpAddr()

	// 创建本地协议更新事件发射器
	if h.emitters.evtLocalProtocolsUpdated, err = h.eventbus.Emitter(&event.EvtLocalProtocolsUpdated{}, eventbus.Stateful); err != nil {
		log.Debugf("创建本地协议更新事件发射器失败: %v", err)
		return nil, err
	}
	// 创建本地地址更新事件发射器
	if h.emitters.evtLocalAddrsUpdated, err = h.eventbus.Emitter(&event.EvtLocalAddressesUpdated{}, eventbus.Stateful); err != nil {
		log.Debugf("创建本地地址更新事件发射器失败: %v", err)
		return nil, err
	}

	// 如果未禁用签名对等记录,则处理证书相关逻辑
	if !h.disableSignedPeerRecord {
		// 获取认证地址簿
		cab, ok := peerstore.GetCertifiedAddrBook(n.Peerstore())
		if !ok {
			log.Debugf("peerstore应该也是一个认证地址簿")
			return nil, errors.New("peerstore应该也是一个认证地址簿")
		}
		h.caBook = cab

		// 获取主机私钥
		h.signKey = h.Peerstore().PrivKey(h.ID())
		if h.signKey == nil {
			log.Debugf("无法访问主机密钥")
			return nil, errors.New("无法访问主机密钥")
		}

		// 为自身创建并持久化签名对等记录
		rec := peer.PeerRecordFromAddrInfo(peer.AddrInfo{
			ID:    h.ID(),
			Addrs: h.Addrs(),
		})
		ev, err := record.Seal(rec, h.signKey)
		if err != nil {
			log.Debugf("为自身创建签名记录失败: %v", err)
			return nil, err
		}
		if _, err := cab.ConsumePeerRecord(ev, peerstore.PermanentAddrTTL); err != nil {
			log.Debugf("将签名记录持久化到peerstore失败: %v", err)
			return nil, err
		}
	}

	// 设置多流复用器
	if opts.MultistreamMuxer != nil {
		h.mux = opts.MultistreamMuxer
	}

	// 配置身份识别选项
	idOpts := []identify.Option{
		identify.UserAgent(opts.UserAgent),
		identify.ProtocolVersion(opts.ProtocolVersion),
	}

	// 根据配置添加额外的身份识别选项
	if h.disableSignedPeerRecord {
		idOpts = append(idOpts, identify.DisableSignedPeerRecord())
	}
	if opts.EnableMetrics {
		idOpts = append(idOpts,
			identify.WithMetricsTracer(
				identify.NewMetricsTracer(identify.WithRegisterer(opts.PrometheusRegisterer))))
	}
	if opts.DisableIdentifyAddressDiscovery {
		idOpts = append(idOpts, identify.DisableObservedAddrManager())
	}

	// 创建身份识别服务
	h.ids, err = identify.NewIDService(h, idOpts...)
	if err != nil {
		log.Debugf("创建Identify服务失败: %v", err)
		return nil, err
	}

	// 配置打洞服务
	if opts.EnableHolePunching {
		if opts.EnableMetrics {
			hpOpts := []holepunch.Option{
				holepunch.WithMetricsTracer(holepunch.NewMetricsTracer(holepunch.WithRegisterer(opts.PrometheusRegisterer)))}
			opts.HolePunchingOptions = append(hpOpts, opts.HolePunchingOptions...)
		}

		// 创建打洞服务
		h.hps, err = holepunch.NewService(h, h.ids, func() []ma.Multiaddr {
			addrs := h.AllAddrs()
			if opts.AddrsFactory != nil {
				addrs = slices.Clone(opts.AddrsFactory(addrs))
			}
			// 合并观察到的地址
			addrs = append(addrs, h.ids.OwnObservedAddrs()...)
			addrs = ma.Unique(addrs)
			return slices.DeleteFunc(addrs, func(a ma.Multiaddr) bool { return !manet.IsPublicAddr(a) })
		}, opts.HolePunchingOptions...)
		if err != nil {
			log.Debugf("创建打洞服务失败: %v", err)
			return nil, err
		}
	}

	// 设置协商超时时间
	if uint64(opts.NegotiationTimeout) != 0 {
		h.negtimeout = opts.NegotiationTimeout
	}

	// 设置地址工厂
	if opts.AddrsFactory != nil {
		h.AddrsFactory = opts.AddrsFactory
	}

	// 设置NAT管理器
	if opts.NATManager != nil {
		h.natmgr = opts.NATManager(n)
	}

	// 设置连接管理器
	if opts.ConnManager == nil {
		h.cmgr = &connmgr.NullConnMgr{}
	} else {
		h.cmgr = opts.ConnManager
		n.Notify(h.cmgr.Notifee())
	}

	// 配置中继服务
	if opts.EnableRelayService {
		if opts.EnableMetrics {
			metricsOpt := []relayv2.Option{
				relayv2.WithMetricsTracer(
					relayv2.NewMetricsTracer(relayv2.WithRegisterer(opts.PrometheusRegisterer)))}
			opts.RelayServiceOpts = append(metricsOpt, opts.RelayServiceOpts...)
		}
		h.relayManager = relaysvc.NewRelayManager(h, opts.RelayServiceOpts...)
	}

	// 启用ping服务
	if opts.EnablePing {
		h.pings = ping.NewPingService(h)
	}

	// 配置AutoNATv2服务
	if opts.EnableAutoNATv2 {
		var mt autonatv2.MetricsTracer
		if opts.EnableMetrics {
			mt = autonatv2.NewMetricsTracer(opts.PrometheusRegisterer)
		}
		h.autonatv2, err = autonatv2.New(h, opts.AutoNATv2Dialer, autonatv2.WithMetricsTracer(mt))
		if err != nil {
			log.Debugf("创建autonatv2失败: %v", err)
			return nil, err
		}
	}

	// 设置流处理器
	n.SetStreamHandler(h.newStreamHandler)

	// 注册网络监听地址变更通知
	listenHandler := func(network.Network, ma.Multiaddr) {
		h.SignalAddressChange()
	}
	n.Notify(&network.NotifyBundle{
		ListenF:      listenHandler,
		ListenCloseF: listenHandler,
	})

	return h, nil
}

// updateLocalIpAddr 更新主机的本地IP地址
func (h *BasicHost) updateLocalIpAddr() {
	h.addrMu.Lock()
	defer h.addrMu.Unlock()

	// 清空现有地址
	h.filteredInterfaceAddrs = nil
	h.allInterfaceAddrs = nil

	// 尝试使用默认的IPv4/IPv6地址
	if r, err := netroute.New(); err != nil {
		log.Debugf("构建路由表失败: %v", err)
	} else {
		// 获取本地IPv4地址
		var localIPv4 net.IP
		var ran bool
		err, ran = h.updateLocalIPv4Backoff.Run(func() error {
			_, _, localIPv4, err = r.Route(net.IPv4zero)
			log.Debugf("获取本地IPv4地址: %v", err)
			return err
		})

		if ran && err != nil {
			log.Debugf("获取本地IPv4地址失败: %v", err)
		} else if ran && localIPv4.IsGlobalUnicast() {
			maddr, err := manet.FromIP(localIPv4)
			if err == nil {
				h.filteredInterfaceAddrs = append(h.filteredInterfaceAddrs, maddr)
			}
		}

		// 获取本地IPv6地址
		var localIPv6 net.IP
		err, ran = h.updateLocalIPv6Backoff.Run(func() error {
			_, _, localIPv6, err = r.Route(net.IPv6unspecified)
			log.Debugf("获取本地IPv6地址: %v", err)
			return err
		})

		if ran && err != nil {
			log.Debugf("获取本地IPv6地址失败: %v", err)
		} else if ran && localIPv6.IsGlobalUnicast() {
			maddr, err := manet.FromIP(localIPv6)
			if err == nil {
				h.filteredInterfaceAddrs = append(h.filteredInterfaceAddrs, maddr)
			}
		}
	}

	// 解析接口地址
	ifaceAddrs, err := manet.InterfaceMultiaddrs()
	if err != nil {
		log.Debugf("解析接口地址失败: %v", err)
		// 添加回环地址作为备选
		h.filteredInterfaceAddrs = append(h.filteredInterfaceAddrs, manet.IP4Loopback, manet.IP6Loopback)
		h.allInterfaceAddrs = h.filteredInterfaceAddrs
		return
	}

	// 处理所有接口地址
	for _, addr := range ifaceAddrs {
		// 跳过链路本地地址
		if !manet.IsIP6LinkLocal(addr) {
			h.allInterfaceAddrs = append(h.allInterfaceAddrs, addr)
		}
	}

	// 如果没有获取到过滤后的接口地址,使用所有地址
	if len(h.filteredInterfaceAddrs) == 0 {
		h.filteredInterfaceAddrs = h.allInterfaceAddrs
	} else {
		// 只添加回环地址
		for _, addr := range h.allInterfaceAddrs {
			if manet.IsIPLoopback(addr) {
				h.filteredInterfaceAddrs = append(h.filteredInterfaceAddrs, addr)
			}
		}
	}
}

// Start 启动主机中的后台任务
// 启动 PSManager、增加引用计数、启动 ID 服务、启动 autonat 服务(如果存在)、启动后台任务
func (h *BasicHost) Start() {
	// 启动 PSManager
	h.psManager.Start()
	// 增加引用计数
	h.refCount.Add(1)
	// 启动 ID 服务
	h.ids.Start()
	// 如果存在 autonat v2 服务,则启动它
	if h.autonatv2 != nil {
		err := h.autonatv2.Start()
		if err != nil {
			log.Errorf("autonat v2 启动失败: %v", err)
		}
	}
	// 启动后台任务
	go h.background()
}

// newStreamHandler 处理网络中远程打开的流
// 参数:
//   - s: network.Stream 网络流对象
func (h *BasicHost) newStreamHandler(s network.Stream) {
	// 记录开始时间
	before := time.Now()

	// 如果设置了协商超时时间,则设置流的截止时间
	if h.negtimeout > 0 {
		if err := s.SetDeadline(time.Now().Add(h.negtimeout)); err != nil {
			log.Debugf("设置流截止时间失败: %v", err)
			s.Reset()
			return
		}
	}

	// 协商协议
	protoID, handle, err := h.Mux().Negotiate(s)
	took := time.Since(before)
	if err != nil {
		if err == io.EOF {
			logf := log.Debugf
			if took > time.Second*10 {
				logf = log.Warnf
			}
			logf("协议 EOF: %s (耗时 %s)", s.Conn().RemotePeer(), took)
		} else {
			log.Debugf("协议多路复用失败: %s (耗时 %s, id:%s, 远程节点:%s, 远程地址:%v)", err, took, s.ID(), s.Conn().RemotePeer(), s.Conn().RemoteMultiaddr())
		}
		s.Reset()
		return
	}

	// 清除流的截止时间
	if h.negtimeout > 0 {
		if err := s.SetDeadline(time.Time{}); err != nil {
			log.Debugf("重置流截止时间失败: ", err)
			s.Reset()
			return
		}
	}

	// 设置流的协议
	if err := s.SetProtocol(protoID); err != nil {
		log.Debugf("设置流协议失败: %s", err)
		s.Reset()
		return
	}

	log.Debugf("协商完成: %s (耗时 %s)", protoID, took)

	// 处理流
	handle(protoID, s)
}

// SignalAddressChange 通知主机需要检查监听地址是否最近发生变化
// 警告: 此接口不稳定,可能在未来版本中移除
func (h *BasicHost) SignalAddressChange() {
	select {
	case h.addrChangeChan <- struct{}{}:
	default:
	}
}

// makeUpdatedAddrEvent 创建地址更新事件
// 参数:
//   - prev: []ma.Multiaddr 先前的地址列表
//   - current: []ma.Multiaddr 当前的地址列表
//
// 返回:
//   - *event.EvtLocalAddressesUpdated 地址更新事件
func (h *BasicHost) makeUpdatedAddrEvent(prev, current []ma.Multiaddr) *event.EvtLocalAddressesUpdated {
	// 如果前后地址都为空,返回 nil
	if prev == nil && current == nil {
		return nil
	}
	// 创建地址映射
	prevmap := make(map[string]ma.Multiaddr, len(prev))
	currmap := make(map[string]ma.Multiaddr, len(current))
	evt := &event.EvtLocalAddressesUpdated{Diffs: true}
	addrsAdded := false

	// 构建地址映射
	for _, addr := range prev {
		prevmap[string(addr.Bytes())] = addr
	}
	for _, addr := range current {
		currmap[string(addr.Bytes())] = addr
	}

	// 检查新增和保持的地址
	for _, addr := range currmap {
		_, ok := prevmap[string(addr.Bytes())]
		updated := event.UpdatedAddress{Address: addr}
		if ok {
			updated.Action = event.Maintained
		} else {
			updated.Action = event.Added
			addrsAdded = true
		}
		evt.Current = append(evt.Current, updated)
		delete(prevmap, string(addr.Bytes()))
	}

	// 检查删除的地址
	for _, addr := range prevmap {
		updated := event.UpdatedAddress{Action: event.Removed, Address: addr}
		evt.Removed = append(evt.Removed, updated)
	}

	// 如果没有地址变化,返回 nil
	if !addrsAdded && len(evt.Removed) == 0 {
		return nil
	}

	// 如果地址发生变化且未禁用签名对等记录,则创建新的签名对等记录
	if !h.disableSignedPeerRecord {
		sr, err := h.makeSignedPeerRecord(current)
		if err != nil {
			log.Debugf("从当前地址集创建签名对等记录失败, err=%s", err)
			return nil
		}
		evt.SignedPeerRecord = sr
	}

	return evt
}

// makeSignedPeerRecord 创建签名的对等记录
// 参数:
//   - addrs: []ma.Multiaddr 地址列表
//
// 返回:
//   - *record.Envelope 签名的对等记录
//   - error 错误信息
func (h *BasicHost) makeSignedPeerRecord(addrs []ma.Multiaddr) (*record.Envelope, error) {
	// 限制当前地址长度以确保签名对等记录不被拒绝
	peerRecordSize := 64 // HostID
	k, err := h.signKey.Raw()
	if err != nil {
		peerRecordSize += 2 * len(k) // 1 用于签名, 1 用于公钥
	}
	// 裁剪地址列表以保持签名对等记录大小合适
	addrs = trimHostAddrList(addrs, maxPeerRecordSize-peerRecordSize-256) // 256 B 的缓冲区
	rec := peer.PeerRecordFromAddrInfo(peer.AddrInfo{
		ID:    h.ID(),
		Addrs: addrs,
	})
	return record.Seal(rec, h.signKey)
}

// background 后台任务
// 定期检查地址变化并发出地址更新事件
func (h *BasicHost) background() {
	// 完成时减少引用计数
	defer h.refCount.Done()
	var lastAddrs []ma.Multiaddr

	// 发出地址变化事件的函数
	emitAddrChange := func(currentAddrs []ma.Multiaddr, lastAddrs []ma.Multiaddr) {
		changeEvt := h.makeUpdatedAddrEvent(lastAddrs, currentAddrs)
		if changeEvt == nil {
			return
		}
		// 如果未禁用签名对等记录,则将其存储在对等存储中
		if !h.disableSignedPeerRecord {
			if _, err := h.caBook.ConsumePeerRecord(changeEvt.SignedPeerRecord, peerstore.PermanentAddrTTL); err != nil {
				log.Errorf("在对等存储中持久化签名对等记录失败, err=%s", err)
				return
			}
		}
		// 更新对等存储中的主机地址
		removedAddrs := make([]ma.Multiaddr, 0, len(changeEvt.Removed))
		for _, ua := range changeEvt.Removed {
			removedAddrs = append(removedAddrs, ua.Address)
		}
		h.Peerstore().SetAddrs(h.ID(), currentAddrs, peerstore.PermanentAddrTTL)
		h.Peerstore().SetAddrs(h.ID(), removedAddrs, 0)

		// 发出地址变化事件
		if err := h.emitters.evtLocalAddrsUpdated.Emit(*changeEvt); err != nil {
			log.Warnf("发出地址变更事件失败: %s", err)
		}
	}

	// 创建定时器,定期检查地址变化
	ticker := time.NewTicker(addrChangeTickrInterval)
	defer ticker.Stop()

	// 主循环
	for {
		// 如果有监听地址,则更新本地 IP 地址
		if len(h.network.ListenAddresses()) > 0 {
			h.updateLocalIpAddr()
		}
		// 获取当前地址并检查变化
		curr := h.Addrs()
		emitAddrChange(curr, lastAddrs)
		lastAddrs = curr

		// 等待下一次检查
		select {
		case <-ticker.C:
		case <-h.addrChangeChan:
		case <-h.ctx.Done():
			return
		}
	}
}

// ID 返回与此主机关联的(本地)对等点ID
// 返回:
//   - peer.ID 本地对等点ID
func (h *BasicHost) ID() peer.ID {
	return h.Network().LocalPeer()
}

// Peerstore 返回主机的对等点地址和密钥存储库
// 返回:
//   - peerstore.Peerstore 对等点存储库
func (h *BasicHost) Peerstore() peerstore.Peerstore {
	return h.Network().Peerstore()
}

// Network 返回主机的网络接口
// 返回:
//   - network.Network 网络接口
func (h *BasicHost) Network() network.Network {
	return h.network
}

// Mux 返回用于将传入流多路复用到协议处理程序的多路复用器
// 返回:
//   - protocol.Switch 协议多路复用器
func (h *BasicHost) Mux() protocol.Switch {
	return h.mux
}

// IDService 返回身份识别服务
// 返回:
//   - identify.IDService 身份识别服务
func (h *BasicHost) IDService() identify.IDService {
	return h.ids
}

// EventBus 返回事件总线
// 返回:
//   - event.Bus 事件总线
func (h *BasicHost) EventBus() event.Bus {
	return h.eventbus
}

// SetStreamHandler 在主机的多路复用器上设置协议处理程序
// 等同于: host.Mux().SetHandler(proto, handler)
// (线程安全)
// 参数:
//   - pid: protocol.ID 协议ID
//   - handler: network.StreamHandler 流处理程序
func (h *BasicHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	// 添加处理程序到多路复用器
	h.Mux().AddHandler(pid, func(p protocol.ID, rwc io.ReadWriteCloser) error {
		is := rwc.(network.Stream)
		handler(is)
		return nil
	})
	// 发出本地协议更新事件
	h.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Added: []protocol.ID{pid},
	})
}

// SetStreamHandlerMatch 使用匹配函数在主机的多路复用器上设置协议处理程序
// 参数:
//   - pid: protocol.ID 协议ID
//   - m: func(protocol.ID) bool 协议匹配函数
//   - handler: network.StreamHandler 流处理程序
func (h *BasicHost) SetStreamHandlerMatch(pid protocol.ID, m func(protocol.ID) bool, handler network.StreamHandler) {
	// 添加带匹配函数的处理程序
	h.Mux().AddHandlerWithFunc(pid, m, func(p protocol.ID, rwc io.ReadWriteCloser) error {
		is := rwc.(network.Stream)
		handler(is)
		return nil
	})
	// 发出本地协议更新事件
	h.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Added: []protocol.ID{pid},
	})
}

// RemoveStreamHandler 移除流处理程序
// 参数:
//   - pid: protocol.ID 要移除的协议ID
func (h *BasicHost) RemoveStreamHandler(pid protocol.ID) {
	// 从多路复用器移除处理程序
	h.Mux().RemoveHandler(pid)
	// 发出本地协议更新事件
	h.emitters.evtLocalProtocolsUpdated.Emit(event.EvtLocalProtocolsUpdated{
		Removed: []protocol.ID{pid},
	})
}

// NewStream 打开到给定对等点p的新流,并使用给定的协议ID写入p2p/协议头
// 如果没有到p的连接,则尝试创建一个。如果ProtocolID为"",则不写入头
// (线程安全)
// 参数:
//   - ctx: context.Context 上下文
//   - p: peer.ID 目标对等点ID
//   - pids: ...protocol.ID 协议ID列表
//
// 返回:
//   - network.Stream 新建的流
//   - error 错误信息
func (h *BasicHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (str network.Stream, strErr error) {
	// 检查上下文是否有截止时间,如果没有且协商超时>0,则添加超时
	if _, ok := ctx.Deadline(); !ok {
		if h.negtimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, h.negtimeout)
			defer cancel()
		}
	}

	// 如果调用者没有禁用拨号,则尝试连接对等点
	if nodial, _ := network.GetNoDial(ctx); !nodial {
		err := h.Connect(ctx, peer.AddrInfo{ID: p})
		if err != nil {
			log.Debugf("连接对等点失败: %v", err)
			return nil, err
		}
	}

	// 创建新的流
	s, err := h.Network().NewStream(network.WithNoDial(ctx, "already dialed"), p)
	if err != nil {
		if errors.Is(err, network.ErrNoConn) {
			log.Debugf("连接失败")
			return nil, errors.New("连接失败")
		}
		log.Debugf("打开流失败: %v", err)
		return nil, err
	}
	defer func() {
		if strErr != nil && s != nil {
			s.Reset()
		}
	}()

	// 等待连接上正在进行的身份识别完成
	select {
	case <-h.ids.IdentifyWait(s.Conn()):
	case <-ctx.Done():
		log.Debugf("身份识别未能完成: %v", ctx.Err())
		return nil, ctx.Err()
	}

	// 获取首选协议
	pref, err := h.preferredProtocol(p, pids)
	if err != nil {
		log.Debugf("获取首选协议失败: %v", err)
		return nil, err
	}

	// 如果有首选协议,直接使用
	if pref != "" {
		if err := s.SetProtocol(pref); err != nil {
			log.Debugf("设置协议失败: %v", err)
			return nil, err
		}
		lzcon := msmux.NewMSSelect(s, pref)
		return &streamWrapper{
			Stream: s,
			rw:     lzcon,
		}, nil
	}

	// 在后台协商协议,遵守上下文
	var selected protocol.ID
	errCh := make(chan error, 1)
	go func() {
		selected, err = msmux.SelectOneOf(pids, s)
		errCh <- err
	}()
	select {
	case err = <-errCh:
		if err != nil {
			log.Debugf("协商协议失败: %v", err)
			return nil, err
		}
	case <-ctx.Done():
		s.Reset()
		// wait for `SelectOneOf` to error out because of resetting the stream.
		<-errCh
		log.Debugf("协商协议失败: %v", ctx.Err())
		return nil, ctx.Err()
	}

	// 设置选定的协议
	if err := s.SetProtocol(selected); err != nil {
		log.Debugf("设置协议失败: %v", err)
		return nil, err
	}
	_ = h.Peerstore().AddProtocols(p, selected) // 将协议添加到对等存储不是关键操作
	return s, nil
}

// preferredProtocol 获取对等节点支持的首选协议
// 参数:
//   - p: peer.ID 对等节点ID
//   - pids: []protocol.ID 协议ID列表
//
// 返回:
//   - protocol.ID 首选协议ID
//   - error 错误信息
func (h *BasicHost) preferredProtocol(p peer.ID, pids []protocol.ID) (protocol.ID, error) {
	// 从对等存储中获取支持的协议列表
	supported, err := h.Peerstore().SupportsProtocols(p, pids...)
	if err != nil {
		log.Debugf("获取支持的协议列表失败: %v", err)
		return "", err
	}

	// 如果有支持的协议,返回第一个
	var out protocol.ID
	if len(supported) > 0 {
		out = supported[0]
	}
	return out, nil
}

// Connect 确保本主机与给定对等节点ID之间存在连接
// 如果没有活动连接,Connect将调用h.Network.Dial,并阻塞直到连接建立或返回错误
// Connect会将pi中的地址吸收到其内部对等存储中
// 它还将解析所有/dns4、/dns6和/dnsaddr地址
// 参数:
//   - ctx: context.Context 上下文
//   - pi: peer.AddrInfo 对等节点信息
//
// 返回:
//   - error 错误信息
func (h *BasicHost) Connect(ctx context.Context, pi peer.AddrInfo) error {
	// 将地址添加到对等存储中
	h.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.TempAddrTTL)

	// 获取是否强制直连和是否允许有限连接的标志
	forceDirect, _ := network.GetForceDirectDial(ctx)
	canUseLimitedConn, _ := network.GetAllowLimitedConn(ctx)

	// 如果不是强制直连,检查连接状态
	if !forceDirect {
		connectedness := h.Network().Connectedness(pi.ID)
		// 如果已连接或(允许有限连接且连接有限),则返回
		if connectedness == network.Connected || (canUseLimitedConn && connectedness == network.Limited) {
			return nil
		}
	}

	// 拨号连接对等节点
	return h.dialPeer(ctx, pi.ID)
}

// dialPeer 打开与对等节点的连接,并确保连接建立后进行身份识别
// 参数:
//   - ctx: context.Context 上下文
//   - p: peer.ID 对等节点ID
//
// 返回:
//   - error 错误信息
func (h *BasicHost) dialPeer(ctx context.Context, p peer.ID) error {
	// 记录拨号日志
	log.Debugf("主机 %s 正在拨号 %s", h.ID(), p)

	// 拨号连接对等节点
	c, err := h.Network().DialPeer(ctx, p)
	if err != nil {
		log.Debugf("拨号失败: %v", err)
		return err
	}

	// TODO: 考虑移除这部分? 一方面,这很好因为我们可以假设当这个返回时像agent版本这样的东西通常已经设置好了。
	// 另一方面,我们实际上并不_真的_需要等待这个。
	//
	// 这主要是为了保持现有行为。
	select {
	case <-h.ids.IdentifyWait(c):
	case <-ctx.Done():
		log.Debugf("身份识别未能完成: %v", ctx.Err())
		return ctx.Err()
	}

	// 记录拨号完成日志
	log.Debugf("主机 %s 完成拨号 %s", h.ID(), p)
	return nil
}

// ConnManager 返回连接管理器
// 返回:
//   - connmgr.ConnManager 连接管理器接口
func (h *BasicHost) ConnManager() connmgr.ConnManager {
	// 返回连接管理器
	return h.cmgr
}

// Addrs 返回监听地址
// 输出与 AllAddrs 相同,但经过 AddrsFactory 处理
// 当与 AutoRelay 一起使用时,如果主机不能公开访问,这将只包含主机的私有地址、中继地址,而没有公共地址
// 返回:
//   - []ma.Multiaddr 监听地址列表
func (h *BasicHost) Addrs() []ma.Multiaddr {
	// 创建副本,消费者可以修改切片元素
	addrs := slices.Clone(h.AddrsFactory(h.AllAddrs()))
	// 为用户通过地址工厂提供的地址添加证书哈希
	return h.addCertHashes(ma.Unique(addrs))
}

// NormalizeMultiaddr 返回适合进行相等性检查的多地址
// 如果多地址是 webtransport 组件,它会移除证书哈希
// 参数:
//   - addr: ma.Multiaddr 需要规范化的多地址
//
// 返回:
//   - ma.Multiaddr 规范化后的多地址
func (h *BasicHost) NormalizeMultiaddr(addr ma.Multiaddr) ma.Multiaddr {
	// 检查是否为 webtransport 地址
	ok, n := dep2pwebtransport.IsWebtransportMultiaddr(addr)
	if !ok {
		// 检查是否为 webrtc 地址
		ok, n = dep2pwebrtc.IsWebRTCDirectMultiaddr(addr)
	}
	if ok && n > 0 {
		// 移除最后 n 个组件
		out := addr
		for i := 0; i < n; i++ {
			out, _ = ma.SplitLast(out)
		}
		return out
	}
	return addr
}

// p2pCircuitAddr 定义 p2p-circuit 地址常量
var p2pCircuitAddr = ma.StringCast("/p2p-circuit")

// AllAddrs 返回主机正在监听的所有地址,除了电路地址
// 返回:
//   - []ma.Multiaddr 所有监听地址列表
func (h *BasicHost) AllAddrs() []ma.Multiaddr {
	// 获取监听地址列表
	listenAddrs := h.Network().ListenAddresses()
	if len(listenAddrs) == 0 {
		return nil
	}

	// 获取过滤后的接口地址和所有接口地址
	h.addrMu.RLock()
	filteredIfaceAddrs := h.filteredInterfaceAddrs
	allIfaceAddrs := h.allInterfaceAddrs
	h.addrMu.RUnlock()

	// 遍历所有未解析的监听地址,仅解析主要接口以避免广播太多地址
	finalAddrs := make([]ma.Multiaddr, 0, 8)
	if resolved, err := manet.ResolveUnspecifiedAddresses(listenAddrs, filteredIfaceAddrs); err != nil {
		// 如果没有监听任何地址,或者监听 IPv6 地址但只有 IPv4 接口地址,就会发生这种情况
		log.Debugf("解析监听地址失败: %v", err)
	} else {
		finalAddrs = append(finalAddrs, resolved...)
	}

	// 去重
	finalAddrs = ma.Unique(finalAddrs)

	// 如果有 NAT 映射就使用它们
	if h.natmgr != nil && h.natmgr.HasDiscoveredNAT() {
		// 遍历所有监听地址
		for _, listen := range listenAddrs {
			// 获取 NAT 映射地址
			extMaddr := h.natmgr.GetMapping(listen)
			if extMaddr == nil {
				// 未映射,继续下一个
				continue
			}

			// 如果路由器报告了一个合理的地址
			if !manet.IsIPUnspecified(extMaddr) {
				// 添加映射的地址
				finalAddrs = append(finalAddrs, extMaddr)
			} else {
				log.Warn("NAT 设备报告了一个未指定的 IP 作为其外部地址")
			}

			// 检查路由器是否给了我们一个可路由的公共地址
			if manet.IsPublicAddr(extMaddr) {
				// 如果是公共地址,继续下一个
				continue
			}

			// 如果不是公共地址,以防路由器给我们错误的地址或我们在双重 NAT 后面
			// 也添加观察到的地址
			resolved, err := manet.ResolveUnspecifiedAddress(listen, allIfaceAddrs)
			if err != nil {
				// 如果尝试解析 /ip6/::/... 而没有任何 IPv6 接口地址,就会发生这种情况
				continue
			}

			// 遍历解析后的地址
			for _, addr := range resolved {
				// 获取观察到的地址
				observed := h.ids.ObservedAddrsFor(addr)

				if len(observed) == 0 {
					continue
				}

				// 从外部多地址中删除 IP
				_, extMaddrNoIP := ma.SplitFirst(extMaddr)

				// 遍历观察到的地址
				for _, obsMaddr := range observed {
					// 提取公共观察地址
					ip, _ := ma.SplitFirst(obsMaddr)
					if ip == nil || !manet.IsPublicAddr(ip) {
						continue
					}

					// 将公共 IP 与外部地址组合
					finalAddrs = append(finalAddrs, ma.Join(ip, extMaddrNoIP))
				}
			}
		}
	} else {
		// 如果没有 NAT 映射,使用观察到的地址
		var observedAddrs []ma.Multiaddr
		if h.ids != nil {
			observedAddrs = h.ids.OwnObservedAddrs()
		}
		finalAddrs = append(finalAddrs, observedAddrs...)
	}

	// 去重
	finalAddrs = ma.Unique(finalAddrs)

	// 从列表中删除 /p2p-circuit 地址
	finalAddrs = slices.DeleteFunc(finalAddrs, func(a ma.Multiaddr) bool {
		return a.Equal(p2pCircuitAddr)
	})

	// 为使用标识发现的 /webrtc-direct、/webtransport 等地址添加证书哈希
	finalAddrs = h.addCertHashes(finalAddrs)
	return finalAddrs
}

// addCertHashes 为地址添加证书哈希
// 参数:
//   - addrs: []ma.Multiaddr 需要添加证书哈希的地址列表
//
// 返回:
//   - []ma.Multiaddr 添加证书哈希后的地址列表
func (h *BasicHost) addCertHashes(addrs []ma.Multiaddr) []ma.Multiaddr {
	// 定义获取传输的接口
	type transportForListeninger interface {
		TransportForListening(a ma.Multiaddr) transport.Transport
	}

	// 定义添加证书哈希的接口
	type addCertHasher interface {
		AddCertHashes(m ma.Multiaddr) (ma.Multiaddr, bool)
	}

	// 尝试将网络转换为 transportForListeninger 接口
	s, ok := h.Network().(transportForListeninger)
	if !ok {
		log.Debugf("网络不支持transportForListeninger接口")
		return addrs
	}

	// 复制地址切片,因为将要修改它
	addrsOld := addrs
	addrs = make([]ma.Multiaddr, len(addrsOld))
	copy(addrs, addrsOld)

	// 遍历所有地址
	for i, addr := range addrs {
		// 检查是否为 webtransport 或 webrtc 地址
		wtOK, wtN := dep2pwebtransport.IsWebtransportMultiaddr(addr)
		webrtcOK, webrtcN := dep2pwebrtc.IsWebRTCDirectMultiaddr(addr)
		if (wtOK && wtN == 0) || (webrtcOK && webrtcN == 0) {
			// 获取传输
			t := s.TransportForListening(addr)
			tpt, ok := t.(addCertHasher)
			if !ok {
				continue
			}
			// 添加证书哈希
			addrWithCerthash, added := tpt.AddCertHashes(addr)
			if !added {
				log.Debugf("无法向多地址添加证书哈希: %s", addr)
				continue
			}
			addrs[i] = addrWithCerthash
		}
	}
	return addrs
}

// trimHostAddrList 修剪主机地址列表
// 参数:
//   - addrs: []ma.Multiaddr 需要修剪的地址列表
//   - maxSize: int 最大允许的总字节大小
//
// 返回:
//   - []ma.Multiaddr 修剪后的地址列表
func trimHostAddrList(addrs []ma.Multiaddr, maxSize int) []ma.Multiaddr {
	// 计算总大小
	totalSize := 0
	for _, a := range addrs {
		totalSize += len(a.Bytes())
	}
	if totalSize <= maxSize {
		return addrs
	}

	// 计算地址得分的函数
	score := func(addr ma.Multiaddr) int {
		var res int
		// 公共地址得分最高
		if manet.IsPublicAddr(addr) {
			res |= 1 << 12
		} else if !manet.IsIPLoopback(addr) {
			// 非回环地址次之
			res |= 1 << 11
		}
		// 根据协议类型计算权重
		var protocolWeight int
		ma.ForEach(addr, func(c ma.Component) bool {
			switch c.Protocol().Code {
			case ma.P_QUIC_V1:
				protocolWeight = 5
			case ma.P_TCP:
				protocolWeight = 4
			case ma.P_WSS:
				protocolWeight = 3
			case ma.P_WEBTRANSPORT:
				protocolWeight = 2
			case ma.P_WEBRTC_DIRECT:
				protocolWeight = 1
			case ma.P_P2P:
				return false
			}
			return true
		})
		res |= 1 << protocolWeight
		return res
	}

	// 按得分排序
	slices.SortStableFunc(addrs, func(a, b ma.Multiaddr) int {
		return score(b) - score(a) // b-a 用于反向排序
	})

	// 保留总大小在限制内的地址
	totalSize = 0
	for i, a := range addrs {
		totalSize += len(a.Bytes())
		if totalSize > maxSize {
			addrs = addrs[:i]
			break
		}
	}
	return addrs
}

// SetAutoNat 设置主机的 autonat 服务
// 参数:
//   - a: autonat.AutoNAT autonat 服务实例
func (h *BasicHost) SetAutoNat(a autonat.AutoNAT) {
	h.addrMu.Lock()
	defer h.addrMu.Unlock()
	if h.autoNat == nil {
		h.autoNat = a
	}
}

// GetAutoNat 返回主机的 AutoNAT 服务(如果启用了 AutoNAT)
// 返回:
//   - autonat.AutoNAT AutoNAT 服务实例
func (h *BasicHost) GetAutoNat() autonat.AutoNAT {
	h.addrMu.Lock()
	defer h.addrMu.Unlock()
	return h.autoNat
}

// Close 关闭主机的服务(网络等)
// 返回:
//   - error 关闭过程中的错误
func (h *BasicHost) Close() error {
	// 使用 sync.Once 确保只关闭一次
	h.closeSync.Do(func() {
		// 取消上下文
		h.ctxCancel()
		// 关闭 NAT 管理器
		if h.natmgr != nil {
			h.natmgr.Close()
		}
		// 关闭连接管理器
		if h.cmgr != nil {
			h.cmgr.Close()
		}
		// 关闭身份服务
		if h.ids != nil {
			h.ids.Close()
		}
		// 关闭 AutoNAT 服务
		if h.autoNat != nil {
			h.autoNat.Close()
		}
		// 关闭中继管理器
		if h.relayManager != nil {
			h.relayManager.Close()
		}
		// 关闭主机协议服务
		if h.hps != nil {
			h.hps.Close()
		}
		// 关闭 AutoNAT v2 服务
		if h.autonatv2 != nil {
			h.autonatv2.Close()
		}

		// 关闭事件发射器
		_ = h.emitters.evtLocalProtocolsUpdated.Close()
		_ = h.emitters.evtLocalAddrsUpdated.Close()

		// 关闭网络
		if err := h.network.Close(); err != nil {
			log.Debugf("swarm 关闭失败: %v", err)
		}

		// 关闭协议管理器
		h.psManager.Close()
		// 关闭对等存储
		if h.Peerstore() != nil {
			h.Peerstore().Close()
		}

		// 等待所有引用计数归零
		h.refCount.Wait()

		// 关闭资源管理器
		if h.Network().ResourceManager() != nil {
			h.Network().ResourceManager().Close()
		}
	})

	return nil
}

// streamWrapper 包装网络流,提供自定义的读写接口
type streamWrapper struct {
	network.Stream
	rw io.ReadWriteCloser
}

// Read 从包装的读写器读取数据
// 参数:
//   - b: []byte 用于存储读取数据的缓冲区
//
// 返回:
//   - int 读取的字节数
//   - error 读取过程中的错误
func (s *streamWrapper) Read(b []byte) (int, error) {
	return s.rw.Read(b)
}

// Write 向包装的读写器写入数据
// 参数:
//   - b: []byte 要写入的数据
//
// 返回:
//   - int 写入的字节数
//   - error 写入过程中的错误
func (s *streamWrapper) Write(b []byte) (int, error) {
	return s.rw.Write(b)
}

// Close 关闭包装的读写器
// 返回:
//   - error 关闭过程中的错误
func (s *streamWrapper) Close() error {
	return s.rw.Close()
}

// CloseWrite 关闭流的写入端
// 返回:
//   - error 关闭过程中的错误
func (s *streamWrapper) CloseWrite() error {
	// 在关闭之前刷新握手,但忽略错误
	// 另一端可能已经关闭了他们的读取端
	if flusher, ok := s.rw.(interface{ Flush() error }); ok {
		_ = flusher.Flush()
	}
	return s.Stream.CloseWrite()
}
