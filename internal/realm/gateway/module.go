package gateway

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

// Module Gateway Fx 模块
var Module = fx.Module("realm_gateway",
	fx.Provide(
		NewGatewayFromParams,
		NewRelayServiceFromParams,
		NewConnectionPoolFromParams,
		NewBandwidthLimiterFromParams,
		NewProtocolValidatorFromParams,
		NewRouterAdapterFromParams,
	),
	fx.Invoke(registerLifecycle),
)

// ============================================================================
//
//	配置转换
//
// ============================================================================

// ConfigFromUnified 从统一配置创建 Gateway 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil || !cfg.Realm.EnableGateway {
		return DefaultConfig()
	}
	// 使用统一配置中存在的字段，其他使用默认值
	defaultCfg := DefaultConfig()
	defaultCfg.MaxConcurrent = cfg.Realm.Gateway.MaxPeers * 10 // 最大并发根据节点数计算
	defaultCfg.IdleTimeout = cfg.Realm.Gateway.ConnectionTimeout
	return defaultCfg
}

// ============================================================================
//
//	Fx 参数和结果
//
// ============================================================================

// GatewayParams Gateway 依赖参数
type GatewayParams struct {
	fx.In

	RealmID    string         `name:"realm_id"`
	Host       pkgif.Host     `optional:"true"`
	Auth       interfaces.Authenticator `optional:"true"`
	UnifiedCfg *config.Config `optional:"true"`
}

// GatewayResult Gateway 导出结果
type GatewayResult struct {
	fx.Out

	Gateway interfaces.Gateway
}

// RelayServiceParams RelayService 依赖参数
type RelayServiceParams struct {
	fx.In

	Gateway interfaces.Gateway
}

// RelayServiceResult RelayService 导出结果
type RelayServiceResult struct {
	fx.Out

	RelayService interfaces.RelayService
}

// ConnectionPoolParams ConnectionPool 依赖参数
type ConnectionPoolParams struct {
	fx.In

	Host       pkgif.Host     `optional:"true"`
	UnifiedCfg *config.Config `optional:"true"`
}

// ConnectionPoolResult ConnectionPool 导出结果
type ConnectionPoolResult struct {
	fx.Out

	ConnPool interfaces.ConnectionPool
}

// BandwidthLimiterParams BandwidthLimiter 依赖参数
type BandwidthLimiterParams struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// BandwidthLimiterResult BandwidthLimiter 导出结果
type BandwidthLimiterResult struct {
	fx.Out

	Limiter interfaces.BandwidthLimiter
}

// ProtocolValidatorParams ProtocolValidator 依赖参数
type ProtocolValidatorParams struct {
	fx.In
}

// ProtocolValidatorResult ProtocolValidator 导出结果
type ProtocolValidatorResult struct {
	fx.Out

	Validator interfaces.ProtocolValidator
}

// RouterAdapterParams RouterAdapter 依赖参数
type RouterAdapterParams struct {
	fx.In

	Gateway interfaces.Gateway
}

// RouterAdapterResult RouterAdapter 导出结果
type RouterAdapterResult struct {
	fx.Out

	Adapter interfaces.RouterAdapter
}

// ============================================================================
//
//	构造函数
//
// ============================================================================

// NewGatewayFromParams 从 Fx 参数创建 Gateway
func NewGatewayFromParams(p GatewayParams) (GatewayResult, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	gateway := NewGateway(p.RealmID, p.Host, p.Auth, cfg)

	return GatewayResult{
		Gateway: gateway,
	}, nil
}

// NewRelayServiceFromParams 从 Fx 参数创建 RelayService
func NewRelayServiceFromParams(_ RelayServiceParams) (RelayServiceResult, error) {
	// RelayService 由 Gateway 创建
	// 这里简化处理
	return RelayServiceResult{}, nil
}

// NewConnectionPoolFromParams 从 Fx 参数创建 ConnectionPool
func NewConnectionPoolFromParams(p ConnectionPoolParams) (ConnectionPoolResult, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	pool := NewConnectionPool(p.Host, cfg.MaxConnPerPeer, cfg.MaxConcurrent)

	return ConnectionPoolResult{
		ConnPool: pool,
	}, nil
}

// NewBandwidthLimiterFromParams 从 Fx 参数创建 BandwidthLimiter
func NewBandwidthLimiterFromParams(p BandwidthLimiterParams) (BandwidthLimiterResult, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	limiter := NewBandwidthLimiter(cfg.MaxBandwidth, cfg.BurstSize)

	return BandwidthLimiterResult{
		Limiter: limiter,
	}, nil
}

// NewProtocolValidatorFromParams 从 Fx 参数创建 ProtocolValidator
func NewProtocolValidatorFromParams(_ ProtocolValidatorParams) (ProtocolValidatorResult, error) {
	validator := NewProtocolValidator()

	return ProtocolValidatorResult{
		Validator: validator,
	}, nil
}

// NewRouterAdapterFromParams 从 Fx 参数创建 RouterAdapter
func NewRouterAdapterFromParams(p RouterAdapterParams) (RouterAdapterResult, error) {
	adapter := NewRouterAdapter(p.Gateway)

	return RouterAdapterResult{
		Adapter: adapter,
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
	Gateway interfaces.Gateway
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Gateway.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			if err := input.Gateway.Stop(ctx); err != nil {
				return err
			}
			return input.Gateway.Close()
		},
	})
}
