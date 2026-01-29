package geoip

import (
	"go.uber.org/fx"
)

// Module GeoIP 模块
var Module = fx.Module("geoip",
	fx.Provide(
		NewResolverFromConfig,
	),
)

// NewResolverFromConfig 从配置创建解析器
func NewResolverFromConfig() Resolver {
	return NewRegionResolver(DefaultConfig())
}

// Params GeoIP 参数
type Params struct {
	fx.In

	Config Config `optional:"true"`
}

// Result GeoIP 结果
type Result struct {
	fx.Out

	Resolver Resolver
}

// ProvideResolver 提供解析器
func ProvideResolver(params Params) Result {
	config := params.Config
	if config.CacheSize == 0 {
		config = DefaultConfig()
	}
	return Result{
		Resolver: NewRegionResolver(config),
	}
}
