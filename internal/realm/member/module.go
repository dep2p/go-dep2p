package member

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//
//	Fx 模块定义
//
// ============================================================================

// Module Member Fx 模块
var Module = fx.Module("realm_member",
	fx.Provide(
		NewManagerFromParams,
		NewCacheFromParams,
		NewStoreFromParams,
		NewSynchronizerFromParams,
		NewHeartbeatMonitorFromParams,
	),
	fx.Invoke(registerLifecycle),
)

// ============================================================================
//
//	配置转换
//
// ============================================================================

// ConfigFromUnified 从统一配置创建 Member 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil || !cfg.Realm.EnableMember {
		return DefaultConfig()
	}
	// 使用统一配置中存在的字段，其他使用默认值
	defaultCfg := DefaultConfig()
	defaultCfg.HeartbeatInterval = cfg.Realm.Member.HeartbeatInterval
	defaultCfg.HeartbeatTimeout = cfg.Realm.Member.HeartbeatTimeout
	return defaultCfg
}

// ============================================================================
//
//	Fx 参数和结果
//
// ============================================================================

// ManagerParams Manager 依赖参数
type ManagerParams struct {
	fx.In

	RealmID    string                 `name:"realm_id"`
	Cache      interfaces.MemberCache `optional:"true"`
	Store      interfaces.MemberStore `optional:"true"`
	EventBus   pkgif.EventBus         `optional:"true"`
	UnifiedCfg *config.Config         `optional:"true"`
}

// ManagerResult Manager 导出结果
type ManagerResult struct {
	fx.Out

	Manager interfaces.MemberManager
}

// CacheParams Cache 依赖参数
type CacheParams struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// CacheResult Cache 导出结果
type CacheResult struct {
	fx.Out

	Cache interfaces.MemberCache
}

// StoreParams Store 依赖参数
type StoreParams struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// StoreResult Store 导出结果
type StoreResult struct {
	fx.Out

	Store interfaces.MemberStore
}

// SynchronizerParams Synchronizer 依赖参数
type SynchronizerParams struct {
	fx.In

	Manager *Manager  `optional:"true"`
	DHT     pkgif.DHT `optional:"true"`
}

// SynchronizerResult Synchronizer 导出结果
type SynchronizerResult struct {
	fx.Out

	Synchronizer interfaces.MemberSynchronizer
}

// HeartbeatMonitorParams HeartbeatMonitor 依赖参数
type HeartbeatMonitorParams struct {
	fx.In

	Manager    *Manager       `optional:"true"`
	Host       pkgif.Host     `optional:"true"`
	UnifiedCfg *config.Config `optional:"true"`
}

// HeartbeatMonitorResult HeartbeatMonitor 导出结果
type HeartbeatMonitorResult struct {
	fx.Out

	Monitor interfaces.HeartbeatMonitor
}

// ============================================================================
//
//	构造函数
//
// ============================================================================

// NewManagerFromParams 从 Fx 参数创建 Manager
func NewManagerFromParams(p ManagerParams) (ManagerResult, error) {
	manager := NewManager(p.RealmID, p.Cache, p.Store, p.EventBus)

	return ManagerResult{
		Manager: manager,
	}, nil
}

// NewCacheFromParams 从 Fx 参数创建 Cache
func NewCacheFromParams(p CacheParams) (CacheResult, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	cache := NewCache(cfg.CacheTTL, cfg.CacheSize)

	return CacheResult{
		Cache: cache,
	}, nil
}

// NewStoreFromParams 从 Fx 参数创建 Store
func NewStoreFromParams(p StoreParams) (StoreResult, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	store, err := NewStore(cfg.StorePath, cfg.StoreType)
	if err != nil {
		return StoreResult{}, err
	}

	return StoreResult{
		Store: store,
	}, nil
}

// NewSynchronizerFromParams 从 Fx 参数创建 Synchronizer
func NewSynchronizerFromParams(p SynchronizerParams) (SynchronizerResult, error) {
	sync := NewSynchronizer(p.Manager, p.DHT)

	return SynchronizerResult{
		Synchronizer: sync,
	}, nil
}

// NewHeartbeatMonitorFromParams 从 Fx 参数创建 HeartbeatMonitor
func NewHeartbeatMonitorFromParams(p HeartbeatMonitorParams) (HeartbeatMonitorResult, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	monitor := NewHeartbeatMonitor(
		p.Manager,
		p.Host,
		cfg.HeartbeatInterval,
		cfg.HeartbeatRetries,
	)

	return HeartbeatMonitorResult{
		Monitor: monitor,
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
	LC      fx.Lifecycle
	Manager interfaces.MemberManager
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 启动 Manager
			return input.Manager.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			// 停止 Manager
			if err := input.Manager.Stop(ctx); err != nil {
				return err
			}
			return input.Manager.Close()
		},
	})
}
