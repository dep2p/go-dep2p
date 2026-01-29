package routing

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("realm/routing")

// ============================================================================
//                              路由器实现
// ============================================================================

// Router 路由器
type Router struct {
	mu sync.RWMutex

	// 配置
	realmID string
	config  *Config

	// 组件
	table        interfaces.RouteTable
	pathFinder   interfaces.PathFinder
	loadBalancer interfaces.LoadBalancer
	prober       interfaces.LatencyProber
	cache        *RouteCache
	gateway      interfaces.GatewayAdapter
	dht          pkgif.DHT

	// 指标
	metrics *Metrics

	// 状态
	started atomic.Bool
	closed  atomic.Bool

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewRouter 创建路由器
func NewRouter(realmID string, dht pkgif.DHT, config *Config) *Router {
	if config == nil {
		config = DefaultConfig()
	}

	table := NewRouteTable(realmID)
	cache := NewRouteCache(config.CacheSize, config.CacheTTL)
	prober := NewLatencyProber(nil)
	loadBalancer := NewLoadBalancer()
	pathFinder := NewPathFinder(table, prober)
	gateway := NewGatewayAdapter(nil)
	metrics := NewMetrics()

	return &Router{
		realmID:      realmID,
		config:       config,
		table:        table,
		pathFinder:   pathFinder,
		loadBalancer: loadBalancer,
		prober:       prober,
		cache:        cache,
		gateway:      gateway,
		dht:          dht,
		metrics:      metrics,
	}
}

// ============================================================================
//                              路由查找
// ============================================================================

// FindRoute 查找路由
func (r *Router) FindRoute(ctx context.Context, targetPeerID string) (*interfaces.Route, error) {
	if !r.started.Load() {
		return nil, ErrNotStarted
	}

	if r.closed.Load() {
		return nil, ErrRouterClosed
	}

	logger.Debug("查找路由", "targetPeerID", targetPeerID[:8])
	r.metrics.RecordQuery()

	// 1. 检查缓存
	if route, ok := r.cache.Get(targetPeerID); ok {
		r.metrics.RecordCacheHit()
		logger.Debug("路由缓存命中", "targetPeerID", targetPeerID[:8])
		return route, nil
	}
	r.metrics.RecordCacheMiss()
	logger.Debug("路由缓存未命中", "targetPeerID", targetPeerID[:8])

	// 2. 检查是否可以直连
	if node, err := r.table.GetNode(targetPeerID); err == nil && node.IsReachable {
		route := &interfaces.Route{
			TargetPeerID: targetPeerID,
			NextHop:      targetPeerID,
			Path:         []string{r.realmID, targetPeerID},
			Latency:      node.Latency,
			Hops:         1,
			Score:        r.calculateScore(node.Latency, 1, 0),
			CreatedAt:    time.Now(),
		}

		// 缓存路由
		r.cache.Set(route)
		r.metrics.RecordSuccess()

		return route, nil
	}

	// 3. 查找多跳路径
	path, err := r.pathFinder.FindShortestPath(ctx, r.realmID, targetPeerID)
	if err != nil {
		r.metrics.RecordFailure()
		logger.Warn("查找路由失败", "targetPeerID", targetPeerID[:8], "error", err)
		return nil, err
	}

	// 4. 构造路由
	route := &interfaces.Route{
		TargetPeerID: targetPeerID,
		NextHop:      path.Nodes[1],
		Path:         path.Nodes,
		Latency:      path.TotalLatency,
		Hops:         path.Hops,
		Score:        r.pathFinder.ScorePath(path),
		CreatedAt:    time.Now(),
	}

	// 缓存路由
	r.cache.Set(route)
	r.metrics.RecordSuccess()
	logger.Debug("路由查找成功", "targetPeerID", targetPeerID[:8], "hops", route.Hops, "latency", route.Latency)

	return route, nil
}

// FindRoutes 查找多条路由
func (r *Router) FindRoutes(ctx context.Context, targetPeerID string, count int) ([]*interfaces.Route, error) {
	if !r.started.Load() {
		return nil, ErrNotStarted
	}

	if r.closed.Load() {
		return nil, ErrRouterClosed
	}

	// 查找多条路径
	paths, err := r.pathFinder.FindMultiplePaths(ctx, r.realmID, targetPeerID, count)
	if err != nil {
		return nil, err
	}

	// 转换为路由
	routes := make([]*interfaces.Route, 0, len(paths))
	for _, path := range paths {
		if len(path.Nodes) < 2 {
			continue
		}

		route := &interfaces.Route{
			TargetPeerID: targetPeerID,
			NextHop:      path.Nodes[1],
			Path:         path.Nodes,
			Latency:      path.TotalLatency,
			Hops:         path.Hops,
			Score:        r.pathFinder.ScorePath(path),
			CreatedAt:    time.Now(),
		}

		routes = append(routes, route)
	}

	return routes, nil
}

// SelectBestRoute 选择最佳路由
func (r *Router) SelectBestRoute(ctx context.Context, routes []*interfaces.Route, policy interfaces.RoutingPolicy) (*interfaces.Route, error) {
	if len(routes) == 0 {
		return nil, ErrRouteNotFound
	}

	switch policy {
	case interfaces.PolicyLowestLatency:
		return r.selectLowestLatency(routes), nil

	case interfaces.PolicyLeastHops:
		return r.selectLeastHops(routes), nil

	case interfaces.PolicyLoadBalance:
		return r.selectLoadBalanced(ctx, routes)

	case interfaces.PolicyMixed:
		return r.selectMixed(routes), nil

	default:
		return routes[0], nil
	}
}

// InvalidateRoute 使路由失效
func (r *Router) InvalidateRoute(peerID string) {
	r.cache.Invalidate(peerID)
	r.pathFinder.InvalidatePath(peerID)
}

// GetRouteTable 获取路由表
func (r *Router) GetRouteTable() interfaces.RouteTable {
	return r.table
}

// ============================================================================
//                              路由选择策略
// ============================================================================

// selectLowestLatency 选择最低延迟路由
func (r *Router) selectLowestLatency(routes []*interfaces.Route) *interfaces.Route {
	best := routes[0]
	for _, route := range routes[1:] {
		if route.Latency < best.Latency {
			best = route
		}
	}
	return best
}

// selectLeastHops 选择最少跳数路由
func (r *Router) selectLeastHops(routes []*interfaces.Route) *interfaces.Route {
	best := routes[0]
	for _, route := range routes[1:] {
		if route.Hops < best.Hops {
			best = route
		}
	}
	return best
}

// selectLoadBalanced 选择负载均衡路由
func (r *Router) selectLoadBalanced(ctx context.Context, routes []*interfaces.Route) (*interfaces.Route, error) {
	// 收集候选节点
	candidates := make([]*interfaces.RouteNode, 0, len(routes))
	for _, route := range routes {
		if node, err := r.table.GetNode(route.NextHop); err == nil {
			candidates = append(candidates, node)
		}
	}

	// 使用负载均衡器选择
	selected, err := r.loadBalancer.SelectNode(ctx, candidates)
	if err != nil {
		return nil, err
	}

	// 找到对应的路由
	for _, route := range routes {
		if route.NextHop == selected.PeerID {
			return route, nil
		}
	}

	return routes[0], nil
}

// selectMixed 混合策略选择
func (r *Router) selectMixed(routes []*interfaces.Route) *interfaces.Route {
	// 使用 Score 字段（已综合考虑延迟、跳数、负载）
	best := routes[0]
	for _, route := range routes[1:] {
		if route.Score > best.Score {
			best = route
		}
	}
	return best
}

// calculateScore 计算路由评分
func (r *Router) calculateScore(latency time.Duration, hops int, load float64) float64 {
	// 归一化
	latencyScore := 1.0 / (1.0 + float64(latency.Milliseconds())/100.0)
	hopsScore := 1.0 / (1.0 + float64(hops))
	loadScore := 1.0 - load

	// 加权计算
	score := latencyScore*r.config.PathScoreWeight.Latency +
		hopsScore*r.config.PathScoreWeight.Hops +
		loadScore*r.config.PathScoreWeight.Load

	return score
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动路由器
func (r *Router) Start(_ context.Context) error {
	if r.started.Load() {
		return ErrAlreadyStarted
	}

	if r.closed.Load() {
		return ErrRouterClosed
	}

	// 使用 context.Background() 保证后台循环不受上层 ctx 取消的影响
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.started.Store(true)

	// 启动延迟探测器
	if r.prober != nil {
		r.prober.Start(r.ctx)
	}

	// 启动路由表刷新
	go r.refreshLoop()

	return nil
}

// Stop 停止路由器
func (r *Router) Stop(ctx context.Context) error {
	if !r.started.Load() {
		return ErrNotStarted
	}

	r.started.Store(false)

	if r.cancel != nil {
		r.cancel()
	}

	// 停止延迟探测器
	if r.prober != nil {
		r.prober.Stop(ctx)
	}

	return nil
}

// Close 关闭路由器
func (r *Router) Close() error {
	if r.closed.Load() {
		return nil
	}

	r.closed.Store(true)

	if r.started.Load() {
		ctx := context.Background()
		r.Stop(ctx)
	}

	// 清理缓存
	if r.cache != nil {
		r.cache.Clear()
	}

	return nil
}

// refreshLoop 路由表刷新循环
func (r *Router) refreshLoop() {
	ticker := time.NewTicker(r.config.TableRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.refreshRouteTable()

		case <-r.ctx.Done():
			return
		}
	}
}

// refreshRouteTable 刷新路由表
func (r *Router) refreshRouteTable() {
	// 从 DHT 获取路由表更新
	if r.dht != nil {
		dhtTable := r.dht.RoutingTable()
		if dhtTable != nil {
			// 同步 DHT 路由表到本地路由表
			// 简化实现：清理过期节点
			if rt, ok := r.table.(*RouteTable); ok {
				rt.CleanupExpired()
			}
		}
	}
}

// 确保实现接口
var _ interfaces.Router = (*Router)(nil)
