// Package discovery 提供节点发现模块的实现
//
// 发现模块负责：
// - DHT 分布式发现
// - mDNS 本地发现
// - Bootstrap 引导
// - Realm 感知的节点过滤
package discovery

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/internal/core/discovery/bootstrap"
	"github.com/dep2p/go-dep2p/internal/core/discovery/dht"
	"github.com/dep2p/go-dep2p/internal/core/discovery/dns"
	"github.com/dep2p/go-dep2p/internal/core/discovery/mdns"
	"github.com/dep2p/go-dep2p/internal/core/discovery/rendezvous"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("discovery")

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Identity 节点身份（用于 mDNS 广播本节点信息）
	Identity identityif.Identity `name:"identity"`

	// Transport 传输服务
	Transport transportif.Transport `name:"transport"`

	// Config 配置（可选）
	Config *discoveryif.Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// DiscoveryService (internal) 发现服务（pkg/interfaces/discovery）
	// 供 Relay/Realm 等内部模块注入使用。
	DiscoveryService discoveryif.DiscoveryService `name:"discovery"`

	// EndpointDiscoveryService (api) 发现服务（pkg/interfaces/endpoint）
	// 供 Endpoint 注入使用（node.Discovery()）。
	EndpointDiscoveryService endpoint.DiscoveryService `name:"discovery"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	// 检查必需的依赖
	if input.Transport == nil {
		return ModuleOutput{}, fmt.Errorf("transport is required")
	}

	config := discoveryif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 注意：不要在构造期依赖 realm_manager，否则会形成 discovery -> realm -> endpoint -> discovery 的 Fx 依赖环。
	// RealmManager 会在 wireRealmManager Invoke 阶段（可选）注入。
	service := NewDiscoveryService(input.Transport, nil, config)

	// Bootstrap 发现器已被移除（v1.2）
	//
	// 原因：Bootstrap discoverer 需要 Connector 接口，但在 Fx 构造期无法获取 endpoint。
	// 解决方案：DHT 已原生支持 BootstrapPeers，会自动添加到路由表并通过 FIND_NODE 扩散。
	//
	// Bootstrap 流程现在是：
	// 1. config.BootstrapPeers → discoveryif.Config.BootstrapPeers → dht.Config.BootstrapPeers
	// 2. DHT.Start() → DHT.Bootstrap() → 添加到路由表 → FIND_NODE 自动扩散
	//
	// 如果需要 bootstrap 发现器的状态查询能力，可以通过 DHT 路由表查询。
	if len(config.BootstrapPeers) > 0 {
		log.Info("Bootstrap peers 配置",
			"count", len(config.BootstrapPeers),
			"note", "将由 DHT 直接处理 bootstrap 流程")
	}

	// 注册 mDNS 发现器（如果启用且是服务器模式）
	if config.EnableMDNS {
		mdnsConfig := mdns.DefaultConfig()
		mdnsConfig.ServiceTag = config.MDNSServiceTag
		if mdnsConfig.ServiceTag == "" {
			mdnsConfig.ServiceTag = "_dep2p._udp"
		}

		// 获取本地节点信息
		localID := input.Identity.ID()
		// 本地地址将在 Listen 后通过 UpdateLocalAddrs 更新
		var localAddrs []string

		mdnsDiscoverer := mdns.NewDiscoverer(mdnsConfig, localID, localAddrs)

		// 设置 mDNS 发现回调，转发给 DiscoveryService
		mdnsDiscoverer.SetOnPeerDiscovered(func(peer discoveryif.PeerInfo) {
			service.NotifyPeerDiscovered(peer)
		})

		service.RegisterDiscoverer("mdns", mdnsDiscoverer)
		log.Info("注册 mDNS 发现器",
			"service_tag", mdnsConfig.ServiceTag,
			"local_id", localID.ShortString())
	}

	// 注册 DHT 发现器
	if config.EnableDHT {
		// RealmID 可在运行时由 Realm 模块注入 RealmManager 后更新。
		// 这里构造期不依赖 realm_manager，避免 Fx 依赖环。
		var realmID types.RealmID

		dhtConfig := dht.DefaultConfig()
		dhtConfig.Mode = config.DHTMode
		dhtConfig.BootstrapPeers = config.BootstrapPeers

		dhtDiscoverer := dht.NewDHT(dhtConfig, nil, realmID)
		service.RegisterDiscoverer("dht", dhtDiscoverer)
		// T1 修复：同时注册 DHT 为 announcer，使 Announce("relay") 能触发 AddProvider
		service.RegisterAnnouncer("dht", dhtDiscoverer)
		log.Info("注册 DHT 发现器", "mode", dhtModeString(config.DHTMode))
	}

	// 注册 DNS 发现器
	if config.EnableDNS && len(config.DNSDomains) > 0 {
		dnsConfig := dns.DiscovererConfig{
			Domains:        config.DNSDomains,
			Timeout:        config.DNSTimeout,
			MaxDepth:       config.DNSMaxDepth,
			CacheTTL:       config.DNSCacheTTL,
			CustomResolver: config.DNSResolver,
		}
		dnsDiscoverer := dns.NewDiscoverer(dnsConfig)
		// DNS 发现只支持基于命名空间的发现
		service.RegisterNamespaceDiscoverer("dns", dnsDiscoverer)
		log.Info("注册 DNS 发现器", "domains", config.DNSDomains)
	}

	return ModuleOutput{
		DiscoveryService:         service,
		EndpointDiscoveryService: &endpointDiscoveryAdapter{svc: service},
	}, nil
}

// dhtModeString 返回 DHT 模式的字符串表示
func dhtModeString(m discoveryif.DHTMode) string {
	switch m {
	case discoveryif.DHTModeAuto:
		return "auto"
	case discoveryif.DHTModeServer:
		return "server"
	case discoveryif.DHTModeClient:
		return "client"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("discovery",
		fx.Provide(ProvideServices),
		fx.Invoke(wireRealmManager),
		fx.Invoke(wireDHTNetwork),
		fx.Invoke(registerRendezvous),
		fx.Invoke(registerBootstrapAnnouncer),
		fx.Invoke(registerLifecycle),
	)
}

type realmWireInput struct {
	fx.In
	Discovery discoveryif.DiscoveryService `name:"discovery"`
	RealmMgr  realmif.RealmManager         `name:"realm_manager" optional:"true"`
}

func wireRealmManager(input realmWireInput) {
	if input.RealmMgr == nil {
		return
	}
	if setter, ok := input.Discovery.(interface{ SetRealmManager(realmif.RealmManager) }); ok {
		setter.SetRealmManager(input.RealmMgr)
	}
}

// dhtWireInput DHT 网络层注入输入
type dhtWireInput struct {
	fx.In

	Discovery discoveryif.DiscoveryService `name:"discovery"`
	Endpoint  endpoint.Endpoint            `name:"endpoint" optional:"true"`
	Identity  identityif.Identity          `name:"identity" optional:"true"`
	Config    *discoveryif.Config          `optional:"true"`
}

// wireDHTNetwork 注入 DHT 网络层
//
// 在 Fx Invoke 阶段，Endpoint 已经构造完成，可以安全地注入给 DHT。
// 同时注册 DHT 协议处理器。
//
// 设计说明：
//
//	为避免 "DHT→Connect→DHT FindPeer" 递归依赖，此函数：
//	1. 将 Endpoint 的 AddressBook 注入到 NetworkAdapter
//	2. 将 DHT 的 RoutingTable 注入到 NetworkAdapter
//	3. 将 Endpoint 注入到 DiscoveryService（用于 FindPeer 本地优先查找）
//	这样 DHT RPC 拨号时优先使用已知地址，而非触发 discovery 回调。
//
// 参考：docs/01-design/protocols/network/01-discovery.md#312-冷启动与拨号闭环
func wireDHTNetwork(input dhtWireInput) {
	if input.Endpoint == nil {
		log.Debug("跳过 DHT 网络层注入: Endpoint 未启用")
		return
	}

	cfg := discoveryif.DefaultConfig()
	if input.Config != nil {
		cfg = *input.Config
	}
	if !cfg.EnableDHT {
		log.Debug("跳过 DHT 网络层注入: DHT 未启用")
		return
	}

	// 获取 DiscoveryService 内部的 DHT 实例
	svc, ok := input.Discovery.(*DiscoveryService)
	if !ok {
		log.Warn("无法获取 DiscoveryService 实例")
		return
	}

	// 注入 Endpoint 到 DiscoveryService（用于 FindPeer 本地优先查找）
	svc.SetEndpoint(input.Endpoint)

	discoverer := svc.GetDiscoverer("dht")
	if discoverer == nil {
		log.Warn("DHT 发现器未注册")
		return
	}

	dhtInstance, ok := discoverer.(*dht.DHT)
	if !ok {
		log.Warn("DHT 发现器类型不匹配")
		return
	}

	// 创建网络适配器
	adapter := dht.NewNetworkAdapter(input.Endpoint)

	// 注入 AddressBook（避免递归发现）
	// 将 Endpoint 的 AddressBook 适配为 dht.AddressBookGetter
	if ab := input.Endpoint.AddressBook(); ab != nil {
		adapter.SetAddressBook(&addressBookAdapter{ab: ab})

		// 同时注入 AddressBook 到 DHT，用于将发现的地址写入全局 AddressBook
		dhtInstance.SetAddressBook(&addressBookWriterAdapter{ab: ab})
	}

	// 注入网络层
	dhtInstance.SetNetwork(adapter)

	// 注入 RoutingTable 到 NetworkAdapter（在 SetNetwork 之后，因为 DHT 需要先设置 network）
	adapter.SetRoutingTable(dhtInstance.InternalRoutingTable())

	// 注入身份（用于签名 PeerRecord）
	if input.Identity != nil {
		dhtInstance.SetIdentity(&identityAdapter{input.Identity})
	}

	// 创建协议处理器
	handler := dht.NewHandler(dhtInstance)

	// 注册 DHT 协议处理器
	input.Endpoint.SetProtocolHandler(
		endpoint.ProtocolID(dht.ProtocolID),
		handler.HandlerFunc(),
	)

	log.Info("DHT 网络层已注入",
		"protocol", string(dht.ProtocolID),
		"localID", input.Endpoint.ID().ShortString(),
		"hasIdentity", input.Identity != nil,
		"hasAddressBook", input.Endpoint.AddressBook() != nil)
}

// addressBookAdapter 将 endpoint.AddressBook 适配为 dht.AddressBookGetter
type addressBookAdapter struct {
	ab endpoint.AddressBook
}

func (a *addressBookAdapter) Get(nodeID types.NodeID) []endpoint.Address {
	return a.ab.Get(endpoint.NodeID(nodeID))
}

// addressBookWriterAdapter 将 endpoint.AddressBook 适配为 dht.AddressBookWriter
type addressBookWriterAdapter struct {
	ab endpoint.AddressBook
}

func (a *addressBookWriterAdapter) Add(nodeID types.NodeID, addrs ...string) {
	// 将字符串地址转换为 endpoint.Address
	parsedAddrs := make([]endpoint.Address, 0, len(addrs))
	for _, addrStr := range addrs {
		if addr, err := parseAddressString(addrStr); err == nil {
			parsedAddrs = append(parsedAddrs, addr)
		}
	}
	if len(parsedAddrs) > 0 {
		a.ab.Add(endpoint.NodeID(nodeID), parsedAddrs...)
	}
}

// parseAddressString 解析地址字符串为 endpoint.Address
func parseAddressString(s string) (endpoint.Address, error) {
	return address.Parse(s)
}

// identityAdapter 将 identityif.Identity 适配为 dht.IdentityWithPubKey
//
// T3 修复：添加 PubKeyBytes() 方法支持签名验证
type identityAdapter struct {
	identity identityif.Identity
}

func (a *identityAdapter) ID() types.NodeID {
	return types.NodeID(a.identity.ID())
}

func (a *identityAdapter) Sign(data []byte) ([]byte, error) {
	return a.identity.Sign(data)
}

func (a *identityAdapter) PubKeyBytes() []byte {
	if a.identity == nil {
		return nil
	}
	pubKey := a.identity.PublicKey()
	if pubKey == nil {
		return nil
	}
	return pubKey.Bytes()
}

// registerBootstrapAnnouncer 注册 Bootstrap 通告器
//
// Layer1 修复：使 Bootstrap 节点注册到 DHT，可被动态发现。
type bootstrapAnnouncerInput struct {
	fx.In

	Discovery discoveryif.DiscoveryService `name:"discovery"`
	Endpoint  endpoint.Endpoint            `name:"endpoint" optional:"true"`
	Config    *discoveryif.Config          `optional:"true"`
}

func registerBootstrapAnnouncer(input bootstrapAnnouncerInput) {
	if input.Endpoint == nil {
		return
	}

	cfg := discoveryif.DefaultConfig()
	if input.Config != nil {
		cfg = *input.Config
	}

	// 只有当 ServeBootstrap=true 时才注册 Bootstrap 通告器
	if !cfg.ServeBootstrap {
		return
	}

	// 创建 Bootstrap 通告器
	announcer := bootstrap.NewAnnouncer(
		input.Discovery,
		input.Endpoint,
		input.Endpoint.ID(),
		true, // isServer=true，启动时会通告到 DHT
	)

	// 在 DiscoveryService 中注册 announcer
	if svc, ok := input.Discovery.(*DiscoveryService); ok {
		svc.RegisterAnnouncer("bootstrap", &bootstrapAnnouncerAdapter{announcer})
		log.Info("注册 Bootstrap 通告器（将注册到 DHT sys/bootstrap）")
	}
}

// bootstrapAnnouncerAdapter 适配 bootstrap.Announcer 到 discoveryif.Announcer
type bootstrapAnnouncerAdapter struct {
	announcer *bootstrap.Announcer
}

func (a *bootstrapAnnouncerAdapter) Announce(ctx context.Context, _ string) error {
	return a.announcer.Advertise(ctx)
}

func (a *bootstrapAnnouncerAdapter) AnnounceWithTTL(ctx context.Context, _ string, _ time.Duration) error {
	return a.announcer.Advertise(ctx)
}

func (a *bootstrapAnnouncerAdapter) StopAnnounce(_ string) error {
	return a.announcer.Stop()
}

// rendezvousInput 将 Rendezvous 注册从 ProvideServices 挪到 Invoke，避免 discovery <-> endpoint 构造期依赖环。
type rendezvousInput struct {
	fx.In

	Discovery discoveryif.DiscoveryService `name:"discovery"`
	Endpoint  endpoint.Endpoint            `name:"endpoint" optional:"true"`

	Config *discoveryif.Config `optional:"true"`
}

func registerRendezvous(input rendezvousInput) {
	if input.Endpoint == nil {
		return
	}

	cfg := discoveryif.DefaultConfig()
	if input.Config != nil {
		cfg = *input.Config
	}
	if !cfg.EnableRendezvous {
		return
	}

	// 创建 Rendezvous 发现器（客户端）
	rvConfig := rendezvous.DefaultDiscovererConfig()
	if cfg.RendezvousTTL > 0 {
		rvConfig.DefaultTTL = cfg.RendezvousTTL
	}
	for _, peer := range cfg.RendezvousPoints {
		rvConfig.Points = append(rvConfig.Points, peer.ID)
	}

	rvDiscoverer := rendezvous.NewDiscoverer(
		input.Endpoint,
		input.Endpoint.ID(),
		rvConfig,
	)

	// DiscoveryService 接口内部实现支持 RegisterNamespaceDiscoverer（细粒度接口）
	// 这里使用 type assert，避免扩展公共接口。
	if reg, ok := input.Discovery.(interface {
		RegisterNamespaceDiscoverer(name string, discoverer discoveryif.NamespaceDiscoverer)
	}); ok {
		reg.RegisterNamespaceDiscoverer("rendezvous", rvDiscoverer)
		log.Info("注册 Rendezvous 发现器", "points", len(rvConfig.Points))
	}

	if cfg.ServeRendezvous {
		pointConfig := rendezvous.DefaultPointConfig()
		if cfg.RendezvousMaxRegistrations > 0 {
			pointConfig.MaxRegistrations = cfg.RendezvousMaxRegistrations
		}
		if cfg.RendezvousMaxNamespaces > 0 {
			pointConfig.MaxNamespaces = cfg.RendezvousMaxNamespaces
		}
		if cfg.RendezvousMaxTTL > 0 {
			pointConfig.MaxTTL = cfg.RendezvousMaxTTL
		}
		if cfg.RendezvousCleanupInterval > 0 {
			pointConfig.CleanupInterval = cfg.RendezvousCleanupInterval
		}

		rvPoint := rendezvous.NewPoint(input.Endpoint, pointConfig)
		if setter, ok := input.Discovery.(interface {
			SetRendezvousPoint(point discoveryif.RendezvousPoint)
		}); ok {
			setter.SetRendezvousPoint(rvPoint)
			log.Info("注册 Rendezvous 服务点",
				"max_registrations", pointConfig.MaxRegistrations,
				"max_namespaces", pointConfig.MaxNamespaces,
			)
		}
	}
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC               fx.Lifecycle
	DiscoveryService discoveryif.DiscoveryService `name:"discovery"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("发现服务启动")
			return input.DiscoveryService.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			log.Info("发现服务停止")
			return input.DiscoveryService.Stop()
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	Version     = "1.0.0"
	Name        = "discovery"
	Description = "节点发现模块，提供 DHT、mDNS、Bootstrap、Rendezvous 和 DNS 发现能力"
)
