package resourcemgr

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Config 资源管理器配置
type Config struct {
	Limits *pkgif.LimitConfig
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Limits: DefaultLimitConfig(),
	}
}

// ConfigFromUnified 从统一配置创建资源管理器配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil || !cfg.Resource.EnableResourceManager {
		return DefaultConfig()
	}

	return Config{
		Limits: &pkgif.LimitConfig{
			System: pkgif.Limit{
				Conns:   cfg.Resource.System.MaxConnections,
				Streams: cfg.Resource.System.MaxStreams,
				Memory:  cfg.Resource.System.MaxMemory,
				FD:      cfg.Resource.System.MaxFD,
			},
			Transient: pkgif.Limit{
				Conns:   cfg.Resource.System.MaxConnections / 10, // 临时连接为系统的 1/10
				Streams: cfg.Resource.System.MaxStreams / 10,
				Memory:  cfg.Resource.System.MaxMemory / 10,
			},
			PeerDefault: pkgif.Limit{
				Conns:   cfg.Resource.Peer.MaxConnectionsPerPeer,
				Streams: cfg.Resource.Peer.MaxStreamsPerPeer,
				Memory:  cfg.Resource.Peer.MaxMemoryPerPeer,
			},
		},
	}
}

// Params ResourceManager 依赖参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// Module 是 resourcemgr 的 Fx 模块
var Module = fx.Module("resourcemgr",
	fx.Provide(
		fx.Annotate(
			ProvideResourceManager,
			fx.As(new(pkgif.ResourceManager)),
		),
	),
	fx.Invoke(registerLifecycle),
)

// ProvideResourceManager 提供 ResourceManager 实例
func ProvideResourceManager(p Params) (pkgif.ResourceManager, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	return NewResourceManager(cfg.Limits)
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(lc fx.Lifecycle, rm pkgif.ResourceManager) {
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// ResourceManager 无需启动操作
			return nil
		},
		OnStop: func(_ context.Context) error {
			// 关闭资源管理器
			return rm.Close()
		},
	})
}
