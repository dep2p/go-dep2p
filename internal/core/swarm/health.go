// Package swarm 实现网络连接管理
//
// 本文件实现 Liveness 健康检查器，作为断开检测的兜底机制。
package swarm

import (
	"context"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var healthLogger = log.Logger("swarm/health")

// ============================================================================
//                              配置常量
// ============================================================================

const (
	// LivenessPingInterval Ping 检查间隔
	LivenessPingInterval = 30 * time.Second

	// LivenessPingTimeout 单次 Ping 超时
	LivenessPingTimeout = 5 * time.Second

	// LivenessMaxFailures 最大连续失败次数
	// 超过此次数后认为节点断开
	LivenessMaxFailures = 3

	// LivenessCheckBatchSize 每批检查的节点数量
	// 避免同时检查太多节点造成网络拥塞
	LivenessCheckBatchSize = 10
)

// ============================================================================
//                              健康检查器
// ============================================================================

// HealthChecker Liveness 健康检查器
//
// 作为断开检测的兜底机制（第四层），定期检查所有连接的节点。
// 如果节点连续 N 次 Ping 失败，触发断开处理。
//
// 设计目标：
//   - 检测延迟：< 2 分钟（30s * 3 次失败 + 处理时间）
//   - 资源消耗：低（批量检查、间隔较长）
//   - 角色：兜底机制，不作为主要检测手段
type HealthChecker struct {
	mu sync.RWMutex

	// 配置
	pingInterval time.Duration
	pingTimeout  time.Duration
	maxFailures  int

	// 依赖
	swarm    pkgif.Swarm
	liveness pkgif.Liveness

	// 状态跟踪
	failureCounts map[string]int       // peer_id -> 连续失败次数
	lastCheckTime map[string]time.Time // peer_id -> 上次检查时间

	// 回调
	onPeerUnhealthy func(peerID string, failures int)

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(swarm pkgif.Swarm, liveness pkgif.Liveness) *HealthChecker {
	return &HealthChecker{
		pingInterval:  LivenessPingInterval,
		pingTimeout:   LivenessPingTimeout,
		maxFailures:   LivenessMaxFailures,
		swarm:         swarm,
		liveness:      liveness,
		failureCounts: make(map[string]int),
		lastCheckTime: make(map[string]time.Time),
	}
}

// SetConfig 设置配置参数
func (hc *HealthChecker) SetConfig(pingInterval, pingTimeout time.Duration, maxFailures int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if pingInterval > 0 {
		hc.pingInterval = pingInterval
	}
	if pingTimeout > 0 {
		hc.pingTimeout = pingTimeout
	}
	if maxFailures > 0 {
		hc.maxFailures = maxFailures
	}
}

// SetOnPeerUnhealthy 设置节点不健康回调
func (hc *HealthChecker) SetOnPeerUnhealthy(callback func(peerID string, failures int)) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.onPeerUnhealthy = callback
}

// Start 启动健康检查器
func (hc *HealthChecker) Start(ctx context.Context) error {
	hc.mu.Lock()
	hc.ctx, hc.cancel = context.WithCancel(ctx)
	hc.mu.Unlock()

	// 启动检查循环
	go hc.checkLoop()

	healthLogger.Info("健康检查器已启动",
		"pingInterval", hc.pingInterval,
		"pingTimeout", hc.pingTimeout,
		"maxFailures", hc.maxFailures)

	return nil
}

// Stop 停止健康检查器
func (hc *HealthChecker) Stop(_ context.Context) error {
	hc.mu.Lock()
	if hc.cancel != nil {
		hc.cancel()
	}
	hc.mu.Unlock()

	healthLogger.Info("健康检查器已停止")
	return nil
}

// checkLoop 健康检查循环
func (hc *HealthChecker) checkLoop() {
	ticker := time.NewTicker(hc.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-hc.ctx.Done():
			return
		case <-ticker.C:
			hc.checkAllConnections()
		}
	}
}

// checkAllConnections 检查所有连接
func (hc *HealthChecker) checkAllConnections() {
	if hc.swarm == nil {
		return
	}

	// 获取所有已连接的节点
	peers := hc.swarm.Peers()
	if len(peers) == 0 {
		return
	}

	healthLogger.Debug("开始批量健康检查", "peerCount", len(peers))

	// 批量检查，避免同时发送太多 Ping
	for i := 0; i < len(peers); i += LivenessCheckBatchSize {
		end := i + LivenessCheckBatchSize
		if end > len(peers) {
			end = len(peers)
		}

		batch := peers[i:end]
		hc.checkBatchPeers(batch)

		// 批次间休息，避免网络拥塞
		if end < len(peers) {
			select {
			case <-hc.ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

// checkBatchPeers 检查一批节点
func (hc *HealthChecker) checkBatchPeers(peers []string) {
	var wg sync.WaitGroup

	for _, peerID := range peers {
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()
			hc.checkPeer(pid)
		}(peerID)
	}

	wg.Wait()
}

// checkPeer 检查单个节点
func (hc *HealthChecker) checkPeer(peerID string) {
	// 更新检查时间
	hc.mu.Lock()
	hc.lastCheckTime[peerID] = time.Now()
	hc.mu.Unlock()

	// 执行 Ping
	healthy := hc.pingPeer(peerID)

	hc.mu.Lock()
	defer hc.mu.Unlock()

	if healthy {
		// 重置失败计数
		if hc.failureCounts[peerID] > 0 {
			healthLogger.Debug("节点恢复健康",
				"peerID", truncateID(peerID, 8),
				"previousFailures", hc.failureCounts[peerID])
		}
		delete(hc.failureCounts, peerID)
		return
	}

	// 增加失败计数
	hc.failureCounts[peerID]++
	failures := hc.failureCounts[peerID]

	healthLogger.Debug("节点 Ping 失败",
		"peerID", truncateID(peerID, 8),
		"failures", failures,
		"maxFailures", hc.maxFailures)

	// 检查是否达到最大失败次数
	if failures >= hc.maxFailures {
		healthLogger.Warn("节点连续失败达到阈值",
			"peerID", truncateID(peerID, 8),
			"failures", failures)

		// 调用回调
		if hc.onPeerUnhealthy != nil {
			go hc.onPeerUnhealthy(peerID, failures)
		}

		// 重置计数（避免重复触发）
		delete(hc.failureCounts, peerID)
	}
}

// pingPeer 执行 Ping 检查
func (hc *HealthChecker) pingPeer(peerID string) bool {
	if hc.liveness == nil {
		// 如果没有 Liveness 服务，检查连接状态
		return hc.checkConnectedness(peerID)
	}

	ctx, cancel := context.WithTimeout(hc.ctx, hc.pingTimeout)
	defer cancel()

	// 使用 Liveness 服务执行 Ping
	_, err := hc.liveness.Ping(ctx, peerID)
	if err != nil {
		healthLogger.Debug("Ping 失败",
			"peerID", truncateID(peerID, 8),
			"err", err)
		return false
	}

	return true
}

// checkConnectedness 检查连接状态（降级方案）
func (hc *HealthChecker) checkConnectedness(peerID string) bool {
	if hc.swarm == nil {
		return false
	}

	// 检查是否仍然连接
	connectedness := hc.swarm.Connectedness(peerID)
	return connectedness == pkgif.Connected
}

// ============================================================================
//                              状态查询
// ============================================================================

// GetFailureCount 获取节点的失败计数
func (hc *HealthChecker) GetFailureCount(peerID string) int {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.failureCounts[peerID]
}

// GetLastCheckTime 获取节点的上次检查时间
func (hc *HealthChecker) GetLastCheckTime(peerID string) (time.Time, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	t, ok := hc.lastCheckTime[peerID]
	return t, ok
}

// GetUnhealthyPeers 获取所有不健康的节点
func (hc *HealthChecker) GetUnhealthyPeers() []string {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	threshold := hc.maxFailures / 2 // 超过一半失败次数认为不健康
	if threshold < 1 {
		threshold = 1
	}

	var unhealthy []string
	for peerID, failures := range hc.failureCounts {
		if failures >= threshold {
			unhealthy = append(unhealthy, peerID)
		}
	}

	return unhealthy
}

// ResetPeer 重置节点的失败计数（例如重新连接后）
func (hc *HealthChecker) ResetPeer(peerID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.failureCounts, peerID)
}
