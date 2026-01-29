package metrics

import (
	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
)

// Config 指标配置
type Config struct {
	// Enabled 是否启用指标收集
	Enabled bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Enabled: true,
	}
}

// ConfigFromUnified 从统一配置创建指标配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil {
		return DefaultConfig()
	}
	return Config{
		Enabled: cfg.Resource.EnableResourceManager, // 指标收集与资源管理器关联
	}
}

// Params Metrics 依赖参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// Module 是 metrics 的 Fx 模块
var Module = fx.Module("metrics",
	fx.Provide(
		fx.Annotate(
			NewBandwidthCounterFromParams,
			fx.As(new(Reporter)),
		),
	),
)

// NewBandwidthCounterFromParams 从参数创建 BandwidthCounter
func NewBandwidthCounterFromParams(p Params) *BandwidthCounter {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	if !cfg.Enabled {
		return nil
	}
	return NewBandwidthCounter()
}
