package rendezvous

import (
	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Module Rendezvous 模块
var Module = fx.Module("discovery_rendezvous",
	fx.Provide(
		NewFromParams,
	),
)

// Params Rendezvous 依赖参数
type Params struct {
	fx.In

	Host       pkgif.Host     // 移除 name 标签
	UnifiedCfg *config.Config `optional:"true"`
}

// Result Rendezvous 导出结果
type Result struct {
	fx.Out

	Rendezvous *Discoverer
	Discovery  pkgif.Discovery `group:"discoveries"`
}

// ConfigFromUnified 从统一配置创建 Rendezvous 配置
func ConfigFromUnified(cfg *config.Config) DiscovererConfig {
	if cfg == nil || !cfg.Discovery.EnableRendezvous {
		// 返回默认配置
		return DefaultDiscovererConfig()
	}
	return DiscovererConfig{
		Points:          nil, // 从其他地方获取
		DefaultTTL:      cfg.Discovery.Rendezvous.DefaultTTL.Duration(),
		RenewalInterval: cfg.Discovery.Rendezvous.DefaultTTL.Duration() / 2, // 续约间隔为 TTL 的一半
		DiscoverTimeout: cfg.Discovery.Rendezvous.QueryInterval.Duration(),
		RegisterTimeout: cfg.Discovery.Rendezvous.QueryInterval.Duration(),
		MaxRetries:      3,
		RetryInterval:   cfg.Discovery.Rendezvous.QueryInterval.Duration() / 6,
	}
}

// NewFromParams 从 Fx 参数创建 Discoverer
func NewFromParams(p Params) (Result, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	discoverer := NewDiscoverer(p.Host, cfg)

	return Result{
		Rendezvous: discoverer,
		Discovery:  discoverer,
	}, nil
}
