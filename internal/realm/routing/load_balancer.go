package routing

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              负载均衡器
// ============================================================================

// LoadBalancer 负载均衡器
type LoadBalancer struct {
	mu sync.RWMutex

	// 节点负载
	loads map[string]*interfaces.NodeLoad

	// 配置
	overloadThreshold float64

	// 轮询权重
	weights map[string]float64
}

// NewLoadBalancer 创建负载均衡器
func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		loads:             make(map[string]*interfaces.NodeLoad),
		overloadThreshold: 0.8,
		weights:           make(map[string]float64),
	}
}

// ============================================================================
//                              节点选择
// ============================================================================

// SelectNode 选择节点（加权轮询 + 最少连接）
func (lb *LoadBalancer) SelectNode(_ context.Context, candidates []*interfaces.RouteNode) (*interfaces.RouteNode, error) {
	if len(candidates) == 0 {
		return nil, ErrNodeNotFound
	}

	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// 过滤过载节点
	available := make([]*interfaces.RouteNode, 0, len(candidates))
	for _, node := range candidates {
		if !lb.isNodeOverloaded(node.PeerID) {
			available = append(available, node)
		}
	}

	if len(available) == 0 {
		// 所有节点都过载，选择负载最低的
		return lb.selectLeastLoaded(candidates), nil
	}

	// 选择负载最低的节点
	return lb.selectLeastLoaded(available), nil
}

// selectLeastLoaded 选择负载最低的节点
func (lb *LoadBalancer) selectLeastLoaded(nodes []*interfaces.RouteNode) *interfaces.RouteNode {
	if len(nodes) == 0 {
		return nil
	}

	best := nodes[0]
	minLoad := lb.getNodeLoadScore(best.PeerID)

	for _, node := range nodes[1:] {
		loadScore := lb.getNodeLoadScore(node.PeerID)
		if loadScore < minLoad {
			minLoad = loadScore
			best = node
		}
	}

	return best
}

// getNodeLoadScore 获取节点负载评分
func (lb *LoadBalancer) getNodeLoadScore(peerID string) float64 {
	load, ok := lb.loads[peerID]
	if !ok {
		return 0.0
	}

	// 综合评分：连接数 + 带宽 + CPU
	connScore := float64(load.ConnectionCount) / 100.0
	bwScore := float64(load.BandwidthUsage) / 1000000.0 // MB
	cpuScore := load.CPUUsage

	return connScore*0.4 + bwScore*0.3 + cpuScore*0.3
}

// isNodeOverloaded 检查节点是否过载
func (lb *LoadBalancer) isNodeOverloaded(peerID string) bool {
	load, ok := lb.loads[peerID]
	if !ok {
		return false
	}

	// 检查 CPU 过载
	if load.CPUUsage > lb.overloadThreshold {
		return true
	}

	// 检查连接数过载
	if load.ConnectionCount > 1000 {
		return true
	}

	return false
}

// ============================================================================
//                              负载管理
// ============================================================================

// ReportLoad 报告节点负载
func (lb *LoadBalancer) ReportLoad(peerID string, load *interfaces.NodeLoad) error {
	if load == nil {
		return ErrInvalidNode
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	load.LastUpdated = time.Now()
	lb.loads[peerID] = load

	// 更新权重
	lb.updateWeight(peerID, load)

	return nil
}

// GetLoad 获取节点负载
func (lb *LoadBalancer) GetLoad(peerID string) (*interfaces.NodeLoad, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	load, ok := lb.loads[peerID]
	if !ok {
		return nil, ErrNodeNotFound
	}

	return load, nil
}

// IsOverloaded 检查节点是否过载
func (lb *LoadBalancer) IsOverloaded(peerID string) bool {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	return lb.isNodeOverloaded(peerID)
}

// updateWeight 更新节点权重
func (lb *LoadBalancer) updateWeight(peerID string, _ *interfaces.NodeLoad) {
	// 权重 = 容量 / (当前负载 + 1)
	loadScore := lb.getNodeLoadScore(peerID)
	weight := 1.0 / (loadScore + 1.0)

	lb.weights[peerID] = weight
}

// ============================================================================
//                              统计信息
// ============================================================================

// GetStats 获取负载均衡统计
func (lb *LoadBalancer) GetStats() *interfaces.LoadBalancerStats {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	stats := &interfaces.LoadBalancerStats{
		TotalNodes: len(lb.loads),
	}

	totalLoad := 0.0
	loadScores := make([]float64, 0, len(lb.loads))

	for peerID := range lb.loads {
		loadScore := lb.getNodeLoadScore(peerID)
		loadScores = append(loadScores, loadScore)
		totalLoad += loadScore

		if lb.isNodeOverloaded(peerID) {
			stats.OverloadedNodes++
		}
	}

	// 计算平均负载
	if len(loadScores) > 0 {
		stats.AverageLoad = totalLoad / float64(len(loadScores))

		// 计算负载方差
		variance := 0.0
		for _, score := range loadScores {
			diff := score - stats.AverageLoad
			variance += diff * diff
		}
		stats.LoadVariance = math.Sqrt(variance / float64(len(loadScores)))
	}

	return stats
}

// CleanupStale 清理过期负载信息
func (lb *LoadBalancer) CleanupStale(maxAge time.Duration) int {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	now := time.Now()
	removed := 0

	for peerID, loadInfo := range lb.loads {
		if now.Sub(loadInfo.LastUpdated) > maxAge {
			delete(lb.loads, peerID)
			delete(lb.weights, peerID)
			removed++
		}
	}

	return removed
}

// 确保实现接口
var _ interfaces.LoadBalancer = (*LoadBalancer)(nil)
