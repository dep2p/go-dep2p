package coordinator

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//
//	Fx 模块定义
//
// ============================================================================

// Module Coordinator Fx 模块
var Module = fx.Module("discovery_coordinator",
	fx.Provide(
		NewFromParams,
		ProvidePeerFinder,
		ProvideAddressAnnouncer,
	),
	fx.Invoke(
		registerLifecycle,
		registerPeerFinderLifecycle,
		registerAnnouncerLifecycle,
	),
)

// ============================================================================
//
//	Fx 参数和结果
//
// ============================================================================

// Params Coordinator 依赖参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`

	// 从 group:"discoveries" 收集所有发现器
	Discoveries []pkgif.Discovery `group:"discoveries"`

	// 命名发现器（可选）
	DHT        pkgif.Discovery `name:"dht" optional:"true"`
	Bootstrap  pkgif.Discovery `name:"bootstrap" optional:"true"`
	MDNS       pkgif.Discovery `name:"mdns" optional:"true"`
	Rendezvous pkgif.Discovery `name:"rendezvous" optional:"true"`
	DNS        pkgif.Discovery `name:"dns" optional:"true"`
}

// Result Coordinator 导出结果
type Result struct {
	fx.Out

	Coordinator *Coordinator
	Discovery   pkgif.Discovery // 无名称，作为主 Discovery 入口供其他模块使用
}

// ============================================================================
//
//	配置转换
//
// ============================================================================

// ConfigFromUnified 从统一配置创建 Coordinator 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}
	// Coordinator 配置比较简单，直接使用默认值
	// 主要依赖子发现器的配置
	return DefaultConfig()
}

// ============================================================================
//
//	构造函数
//
// ============================================================================

// NewFromParams 从 Fx 参数创建 Coordinator
func NewFromParams(p Params) (Result, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return Result{}, err
	}

	// 创建协调器
	coord := NewCoordinator(cfg)

	// 注册从 group 收集的发现器
	for i, discovery := range p.Discoveries {
		if discovery != nil {
			name := "discovery_" + string(rune('0'+i))
			coord.RegisterDiscovery(name, discovery)
		}
	}

	// 注册命名发现器（如果存在）
	if p.DHT != nil {
		coord.RegisterDiscovery("dht", p.DHT)
	}
	if p.Bootstrap != nil {
		coord.RegisterDiscovery("bootstrap", p.Bootstrap)
	}
	if p.MDNS != nil {
		coord.RegisterDiscovery("mdns", p.MDNS)
	}
	if p.Rendezvous != nil {
		coord.RegisterDiscovery("rendezvous", p.Rendezvous)
	}
	if p.DNS != nil {
		coord.RegisterDiscovery("dns", p.DNS)
	}

	return Result{
		Coordinator: coord,
		Discovery:   coord,
	}, nil
}

// ============================================================================
//
//	生命周期管理
//
// ============================================================================

// lifecycleInput Lifecycle 注册输入
type lifecycleInput struct {
	fx.In
	LC          fx.Lifecycle
	Coordinator *Coordinator
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 启动 Coordinator
			return input.Coordinator.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			// 停止 Coordinator
			return input.Coordinator.Stop(ctx)
		},
	})
}

// ============================================================================
//
//	PeerFinder 集成
//
// ============================================================================

// PeerFinderParams PeerFinder 依赖参数
type PeerFinderParams struct {
	fx.In

	Coordinator *Coordinator
	Peerstore   pkgif.Peerstore `optional:"true"`
	Host        pkgif.Host      `optional:"true"`
	Swarm       pkgif.Swarm     `optional:"true"`
}

// ProvidePeerFinder 创建 PeerFinder
func ProvidePeerFinder(p PeerFinderParams) *PeerFinder {
	pf := NewPeerFinder(DefaultPeerFinderConfig())

	// 设置依赖
	if p.Peerstore != nil {
		pf.SetPeerstore(p.Peerstore)
	}
	if p.Host != nil {
		pf.SetHost(p.Host)
	}
	if p.Swarm != nil {
		pf.SetSwarm(p.Swarm)
	}

	// 注册 Coordinator 中的所有发现源
	for _, name := range p.Coordinator.ListDiscoveries() {
		if d := p.Coordinator.GetDiscovery(name); d != nil {
			pf.RegisterDiscovery(name, d)
		}
	}

	return pf
}

// peerFinderLifecycleInput PeerFinder 生命周期输入
type peerFinderLifecycleInput struct {
	fx.In
	LC         fx.Lifecycle
	PeerFinder *PeerFinder
}

// registerPeerFinderLifecycle 注册 PeerFinder 生命周期
func registerPeerFinderLifecycle(input peerFinderLifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.PeerFinder.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return input.PeerFinder.Close()
		},
	})
}

// ============================================================================
//
//	AddressAnnouncer 集成
//
// ============================================================================

// AddressAnnouncerParams AddressAnnouncer 依赖参数
type AddressAnnouncerParams struct {
	fx.In

	Coordinator *Coordinator
	Host        pkgif.Host `optional:"true"`
}

// ProvideAddressAnnouncer 创建 AddressAnnouncer
func ProvideAddressAnnouncer(p AddressAnnouncerParams) *AddressAnnouncer {
	aa := NewAddressAnnouncer(DefaultAddressAnnouncerConfig())

	// 设置 Host
	if p.Host != nil {
		aa.SetHost(p.Host)
	}

	// 注册 Coordinator 中的所有发现源
	for _, name := range p.Coordinator.ListDiscoveries() {
		if d := p.Coordinator.GetDiscovery(name); d != nil {
			aa.RegisterDiscovery(name, d)
		}
	}

	return aa
}

// announcerLifecycleInput AddressAnnouncer 生命周期输入
type announcerLifecycleInput struct {
	fx.In
	LC       fx.Lifecycle
	Announcer *AddressAnnouncer
}

// registerAnnouncerLifecycle 注册 AddressAnnouncer 生命周期
func registerAnnouncerLifecycle(input announcerLifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Announcer.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return input.Announcer.Close()
		},
	})
}
