package dns

import (
	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Module DNS 发现模块
var Module = fx.Module("discovery_dns",
	fx.Provide(
		NewFromParams,
	),
)

// Params DNS 依赖参数
type Params struct {
	fx.In

	Host       pkgif.Host     `optional:"true"`
	UnifiedCfg *config.Config `optional:"true"`
}

// Result DNS 导出结果
type Result struct {
	fx.Out

	DNS       *Discoverer
	Discovery pkgif.Discovery `group:"discoveries"`
}

// ConfigFromUnified 从统一配置创建 DNS 配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil || !cfg.Discovery.EnableDNS {
		// 返回默认配置
		return DefaultConfig()
	}
	return Config{
		Domains:         nil, // 域名列表从其他来源获取
		Timeout:         cfg.Discovery.DNS.Timeout.Duration(),
		CacheTTL:        cfg.Discovery.DNS.CacheTTL.Duration(),
		MaxDepth:        3, // 默认值
		CustomResolver:  cfg.Discovery.DNS.ResolverURL,
		RefreshInterval: cfg.Discovery.DNS.Timeout.Duration() * 30, // 刷新间隔为超时的30倍
	}
}

// NewFromParams 从 Fx 参数创建 Discoverer
func NewFromParams(p Params) (Result, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	discoverer := NewDiscoverer(cfg)

	return Result{
		DNS:       discoverer,
		Discovery: discoverer,
	}, nil
}
