package routing

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

// Module Routing Fx 模块
var Module = fx.Module("realm_routing",
	fx.Provide(
		NewRouterFromParams,
		NewRouteTableFromParams,
		NewPathFinderFromParams,
		NewLoadBalancerFromParams,
		NewLatencyProberFromParams,
	),
	fx.Invoke(registerLifecycle),
)

// ============================================================================
//
//	配置转换
//
// ============================================================================

// ConfigFromUnified 从统一配置创建 Routing 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil || !cfg.Realm.EnableRouting {
		return DefaultConfig()
	}
	// 使用统一配置中存在的字段，其他使用默认值
	defaultCfg := DefaultConfig()
	defaultCfg.CacheSize = cfg.Realm.Routing.MaxRoutes // 路由缓存大小
	defaultCfg.CacheTTL = cfg.Realm.Routing.DefaultTTL // 缓存 TTL
	return defaultCfg
}

// ============================================================================
//
//	Fx 参数和结果
//
// ============================================================================

// RouterParams Router 依赖参数
type RouterParams struct {
	fx.In

	RealmID    string         `name:"realm_id"`
	DHT        pkgif.DHT      `optional:"true"`
	UnifiedCfg *config.Config `optional:"true"`
}

// RouterResult Router 导出结果
type RouterResult struct {
	fx.Out

	Router interfaces.Router
}

// RouteTableParams RouteTable 依赖参数
type RouteTableParams struct {
	fx.In

	LocalPeerID string `name:"local_peer_id"`
}

// RouteTableResult RouteTable 导出结果
type RouteTableResult struct {
	fx.Out

	RouteTable interfaces.RouteTable
}

// PathFinderParams PathFinder 依赖参数
type PathFinderParams struct {
	fx.In

	RouteTable interfaces.RouteTable    `optional:"true"`
	Prober     interfaces.LatencyProber `optional:"true"`
}

// PathFinderResult PathFinder 导出结果
type PathFinderResult struct {
	fx.Out

	PathFinder interfaces.PathFinder
}

// LoadBalancerParams LoadBalancer 依赖参数
type LoadBalancerParams struct {
	fx.In
}

// LoadBalancerResult LoadBalancer 导出结果
type LoadBalancerResult struct {
	fx.Out

	LoadBalancer interfaces.LoadBalancer
}

// LatencyProberParams LatencyProber 依赖参数
type LatencyProberParams struct {
	fx.In

	Host pkgif.Host `optional:"true"`
}

// LatencyProberResult LatencyProber 导出结果
type LatencyProberResult struct {
	fx.Out

	Prober interfaces.LatencyProber
}

// ============================================================================
//
//	构造函数
//
// ============================================================================

// NewRouterFromParams 从 Fx 参数创建 Router
func NewRouterFromParams(p RouterParams) (RouterResult, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	router := NewRouter(p.RealmID, p.DHT, cfg)

	return RouterResult{
		Router: router,
	}, nil
}

// NewRouteTableFromParams 从 Fx 参数创建 RouteTable
func NewRouteTableFromParams(p RouteTableParams) (RouteTableResult, error) {
	table := NewRouteTable(p.LocalPeerID)

	return RouteTableResult{
		RouteTable: table,
	}, nil
}

// NewPathFinderFromParams 从 Fx 参数创建 PathFinder
func NewPathFinderFromParams(p PathFinderParams) (PathFinderResult, error) {
	finder := NewPathFinder(p.RouteTable, p.Prober)

	return PathFinderResult{
		PathFinder: finder,
	}, nil
}

// NewLoadBalancerFromParams 从 Fx 参数创建 LoadBalancer
func NewLoadBalancerFromParams(_ LoadBalancerParams) (LoadBalancerResult, error) {
	balancer := NewLoadBalancer()

	return LoadBalancerResult{
		LoadBalancer: balancer,
	}, nil
}

// NewLatencyProberFromParams 从 Fx 参数创建 LatencyProber
func NewLatencyProberFromParams(p LatencyProberParams) (LatencyProberResult, error) {
	prober := NewLatencyProber(p.Host)

	return LatencyProberResult{
		Prober: prober,
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
	LC     fx.Lifecycle
	Router interfaces.Router
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Router.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			if err := input.Router.Stop(ctx); err != nil {
				return err
			}
			return input.Router.Close()
		},
	})
}
