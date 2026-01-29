package routing

import (
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              路由策略实现
// ============================================================================

// PolicyEvaluator 策略评估器
type PolicyEvaluator struct {
	// 可以添加配置参数
}

// NewPolicyEvaluator 创建策略评估器
func NewPolicyEvaluator() *PolicyEvaluator {
	return &PolicyEvaluator{}
}

// ============================================================================
//                              策略评估
// ============================================================================

// EvaluateRoute 评估路由
func (pe *PolicyEvaluator) EvaluateRoute(route *interfaces.Route, policy interfaces.RoutingPolicy) float64 {
	if route == nil {
		return 0
	}

	switch policy {
	case interfaces.PolicyLowestLatency:
		return pe.evaluateLatency(route)

	case interfaces.PolicyLeastHops:
		return pe.evaluateHops(route)

	case interfaces.PolicyLoadBalance:
		return pe.evaluateLoadBalance(route)

	case interfaces.PolicyMixed:
		return pe.evaluateMixed(route)

	default:
		return 0
	}
}

// evaluateLatency 评估延迟
func (pe *PolicyEvaluator) evaluateLatency(route *interfaces.Route) float64 {
	// 延迟越低，评分越高
	if route.Latency == 0 {
		return 1.0
	}

	// 100ms 作为基准
	baseLatency := 100.0 // milliseconds
	latencyMs := float64(route.Latency.Milliseconds())

	score := baseLatency / (baseLatency + latencyMs)
	return score
}

// evaluateHops 评估跳数
func (pe *PolicyEvaluator) evaluateHops(route *interfaces.Route) float64 {
	// 跳数越少，评分越高
	if route.Hops == 0 {
		return 1.0
	}

	// 10 跳作为基准
	baseHops := 10.0
	hops := float64(route.Hops)

	score := baseHops / (baseHops + hops)
	return score
}

// evaluateLoadBalance 评估负载均衡
func (pe *PolicyEvaluator) evaluateLoadBalance(route *interfaces.Route) float64 {
	// 使用现有的 Score 字段
	return route.Score
}

// evaluateMixed 混合评估
func (pe *PolicyEvaluator) evaluateMixed(route *interfaces.Route) float64 {
	// 综合评分：延迟 50% + 跳数 30% + 负载 20%
	latencyScore := pe.evaluateLatency(route)
	hopsScore := pe.evaluateHops(route)
	loadScore := route.Score // 假设已包含负载信息

	mixed := latencyScore*0.5 + hopsScore*0.3 + loadScore*0.2
	return mixed
}

// ============================================================================
//                              策略比较
// ============================================================================

// CompareRoutes 比较两条路由
func (pe *PolicyEvaluator) CompareRoutes(route1, route2 *interfaces.Route, policy interfaces.RoutingPolicy) int {
	score1 := pe.EvaluateRoute(route1, policy)
	score2 := pe.EvaluateRoute(route2, policy)

	if score1 > score2 {
		return 1
	} else if score1 < score2 {
		return -1
	}
	return 0
}

// SelectBestRoute 选择最佳路由
func (pe *PolicyEvaluator) SelectBestRoute(routes []*interfaces.Route, policy interfaces.RoutingPolicy) *interfaces.Route {
	if len(routes) == 0 {
		return nil
	}

	best := routes[0]
	bestScore := pe.EvaluateRoute(best, policy)

	for _, route := range routes[1:] {
		score := pe.EvaluateRoute(route, policy)
		if score > bestScore {
			bestScore = score
			best = route
		}
	}

	return best
}
