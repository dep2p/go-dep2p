// Package pathhealth 提供路径健康管理功能
package pathhealth

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/pathhealth")

// Manager 路径健康管理器
type Manager struct {
	mu sync.RWMutex

	// 配置
	config *Config

	// 按 Peer 组织的路径
	// peerID -> addr -> *Path
	peerPaths map[string]map[string]*Path

	// 运行状态
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// 确保实现接口
var _ interfaces.PathHealthManager = (*Manager)(nil)

// NewManager 创建路径健康管理器
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}
	_ = config.Validate()

	return &Manager{
		config:    config,
		peerPaths: make(map[string]map[string]*Path),
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动管理器
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.ctx != nil {
		m.mu.Unlock()
		return nil // 已启动
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	// 启动清理协程
	m.wg.Add(1)
	go m.cleanupLoop()

	logger.Info("路径健康管理器已启动")
	return nil
}

// Stop 停止管理器
func (m *Manager) Stop() error {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()

	m.wg.Wait()
	logger.Info("路径健康管理器已停止")
	return nil
}

// cleanupLoop 清理过期路径的协程
func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.PathExpiry / 2)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupExpiredPaths()
		}
	}
}

// cleanupExpiredPaths 清理过期路径
func (m *Manager) cleanupExpiredPaths() {
	m.mu.Lock()
	defer m.mu.Unlock()

	expiry := time.Now().Add(-m.config.PathExpiry)

	for peerID, paths := range m.peerPaths {
		for addr, path := range paths {
			if path.GetLastSeen().Before(expiry) {
				delete(paths, addr)
				logger.Debug("清理过期路径",
					"peer", peerID[:min(8, len(peerID))],
					"addr", addr)
			}
		}

		// 如果 Peer 没有路径了，清理 Peer
		if len(paths) == 0 {
			delete(m.peerPaths, peerID)
		}
	}
}

// ============================================================================
//                              路径观察
// ============================================================================

// ObservePeerAddrs 观察 Peer 的地址列表
func (m *Manager) ObservePeerAddrs(peerID string, addrs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	paths, ok := m.peerPaths[peerID]
	if !ok {
		paths = make(map[string]*Path)
		m.peerPaths[peerID] = paths
	}

	for _, addr := range addrs {
		if _, exists := paths[addr]; !exists {
			pathType := DetectPathType(addr)
			paths[addr] = NewPath(addr, pathType, m.config)
			logger.Debug("发现新路径",
				"peer", peerID[:min(8, len(peerID))],
				"addr", addr,
				"type", pathType.String())
		}
	}

	// 限制每个 Peer 的路径数量
	m.enforceMaxPaths(peerID)
}

// enforceMaxPaths 限制路径数量
func (m *Manager) enforceMaxPaths(peerID string) {
	paths, ok := m.peerPaths[peerID]
	if !ok || len(paths) <= m.config.MaxPathsPerPeer {
		return
	}

	// 按评分排序，删除评分最差的
	pathList := make([]*Path, 0, len(paths))
	for _, p := range paths {
		pathList = append(pathList, p)
	}

	sort.Slice(pathList, func(i, j int) bool {
		return pathList[i].CalculateScore() > pathList[j].CalculateScore()
	})

	// 删除超出限制的路径
	for i := m.config.MaxPathsPerPeer; i < len(pathList); i++ {
		delete(paths, pathList[i].GetAddr())
	}
}

// ReportProbe 报告探测结果
func (m *Manager) ReportProbe(peerID string, addr string, rtt time.Duration, err error) {
	path := m.getOrCreatePath(peerID, addr)
	if path == nil {
		return
	}

	path.RecordProbe(rtt, err)

	if err != nil {
		logger.Debug("路径探测失败",
			"peer", peerID[:min(8, len(peerID))],
			"addr", addr,
			"error", err)
	} else {
		logger.Debug("路径探测成功",
			"peer", peerID[:min(8, len(peerID))],
			"addr", addr,
			"rtt", rtt)
	}
}

// ReportHandshake 报告握手结果
func (m *Manager) ReportHandshake(peerID string, addr string, rtt time.Duration, err error) {
	// 握手结果与探测结果处理相同
	m.ReportProbe(peerID, addr, rtt, err)
}

// getOrCreatePath 获取或创建路径
func (m *Manager) getOrCreatePath(peerID string, addr string) *Path {
	m.mu.Lock()
	defer m.mu.Unlock()

	paths, ok := m.peerPaths[peerID]
	if !ok {
		paths = make(map[string]*Path)
		m.peerPaths[peerID] = paths
	}

	path, ok := paths[addr]
	if !ok {
		pathType := DetectPathType(addr)
		path = NewPath(addr, pathType, m.config)
		paths[addr] = path
	}

	return path
}

// ============================================================================
//                              路径查询
// ============================================================================

// GetPathStats 获取特定路径的统计信息
func (m *Manager) GetPathStats(peerID string, addr string) *interfaces.PathStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	paths, ok := m.peerPaths[peerID]
	if !ok {
		return nil
	}

	path, ok := paths[addr]
	if !ok {
		return nil
	}

	return path.ToStats()
}

// GetPeerPaths 获取 Peer 的所有路径
func (m *Manager) GetPeerPaths(peerID string) []*interfaces.PathStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	paths, ok := m.peerPaths[peerID]
	if !ok {
		return nil
	}

	result := make([]*interfaces.PathStats, 0, len(paths))
	for _, path := range paths {
		result = append(result, path.ToStats())
	}

	// 按评分排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score < result[j].Score
	})

	return result
}

// GetBestPath 获取 Peer 的最佳路径
func (m *Manager) GetBestPath(peerID string) *interfaces.PathStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	paths, ok := m.peerPaths[peerID]
	if !ok || len(paths) == 0 {
		return nil
	}

	var bestPath *Path
	var bestScore = 1e10

	for _, path := range paths {
		score := path.CalculateScore()
		if score < bestScore {
			bestScore = score
			bestPath = path
		}
	}

	if bestPath == nil {
		return nil
	}

	return bestPath.ToStats()
}

// ============================================================================
//                              路径排序
// ============================================================================

// RankAddrs 对地址列表按路径健康度排序
func (m *Manager) RankAddrs(peerID string, addrs []string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	paths, ok := m.peerPaths[peerID]

	// 创建地址评分映射
	type addrScore struct {
		addr  string
		score float64
	}

	scores := make([]addrScore, len(addrs))
	for i, addr := range addrs {
		var score = 1e6 // 未知路径的默认评分

		if ok {
			if path, exists := paths[addr]; exists {
				score = path.CalculateScore()
			}
		}

		scores[i] = addrScore{addr: addr, score: score}
	}

	// 按评分排序（越低越好）
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score < scores[j].score
	})

	// 提取排序后的地址
	result := make([]string, len(addrs))
	for i, s := range scores {
		result[i] = s.addr
	}

	return result
}

// ============================================================================
//                              切换决策
// ============================================================================

// ShouldSwitch 判断是否应该切换路径
func (m *Manager) ShouldSwitch(peerID string, currentPath interfaces.PathID) interfaces.SwitchDecision {
	m.mu.RLock()
	defer m.mu.RUnlock()

	paths, ok := m.peerPaths[peerID]
	if !ok || len(paths) == 0 {
		return interfaces.SwitchDecision{
			ShouldSwitch: false,
			Reason:       interfaces.SwitchReasonNone,
		}
	}

	// 查找当前路径
	var currentPathObj *Path
	for _, path := range paths {
		if path.GetPathID() == currentPath {
			currentPathObj = path
			break
		}
	}

	// 当前路径不存在或已死亡
	if currentPathObj == nil || currentPathObj.GetState() == interfaces.PathStateDead {
		// 查找最佳可用路径
		bestPath := m.findBestUsablePath(paths)
		if bestPath != nil {
			return interfaces.SwitchDecision{
				ShouldSwitch: true,
				Reason:       interfaces.SwitchReasonCurrentDead,
				TargetPath:   bestPath.GetPathID(),
				CurrentScore: 1e9,
				TargetScore:  bestPath.CalculateScore(),
			}
		}
		return interfaces.SwitchDecision{
			ShouldSwitch: false,
			Reason:       interfaces.SwitchReasonNone,
		}
	}

	currentScore := currentPathObj.CalculateScore()

	// 查找更好的路径
	bestPath := m.findBestUsablePath(paths)
	if bestPath == nil || bestPath.GetPathID() == currentPath {
		return interfaces.SwitchDecision{
			ShouldSwitch: false,
			Reason:       interfaces.SwitchReasonNone,
			CurrentScore: currentScore,
		}
	}

	bestScore := bestPath.CalculateScore()

	// 检查是否显著更好（超过滞后阈值）
	improvement := (currentScore - bestScore) / currentScore
	if improvement < m.config.SwitchHysteresis {
		return interfaces.SwitchDecision{
			ShouldSwitch: false,
			Reason:       interfaces.SwitchReasonNone,
			CurrentScore: currentScore,
			TargetScore:  bestScore,
		}
	}

	// 检查目标路径是否稳定
	if !bestPath.IsStable(m.config.StabilityWindow) {
		return interfaces.SwitchDecision{
			ShouldSwitch: false,
			Reason:       interfaces.SwitchReasonNone,
			CurrentScore: currentScore,
			TargetScore:  bestScore,
		}
	}

	return interfaces.SwitchDecision{
		ShouldSwitch: true,
		Reason:       interfaces.SwitchReasonBetterPath,
		TargetPath:   bestPath.GetPathID(),
		CurrentScore: currentScore,
		TargetScore:  bestScore,
	}
}

// findBestUsablePath 查找最佳可用路径
func (m *Manager) findBestUsablePath(paths map[string]*Path) *Path {
	var bestPath *Path
	var bestScore = 1e10

	for _, path := range paths {
		if !path.GetState().IsUsable() {
			continue
		}

		score := path.CalculateScore()
		if score < bestScore {
			bestScore = score
			bestPath = path
		}
	}

	return bestPath
}

// ============================================================================
//                              事件通知
// ============================================================================

// OnNetworkChange 网络变更通知
func (m *Manager) OnNetworkChange(_ context.Context, reason string) {
	logger.Info("收到网络变更通知", "reason", reason)

	// 网络变更后，将所有路径标记为可疑
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, paths := range m.peerPaths {
		for _, path := range paths {
			// 重置连续失败计数，给路径一个重新证明自己的机会
			path.mu.Lock()
			if path.state != interfaces.PathStateDead {
				path.consecutiveFailures = 0
			}
			path.mu.Unlock()
		}
	}
}

// ============================================================================
//                              管理
// ============================================================================

// RemovePeer 移除 Peer 的所有路径信息
func (m *Manager) RemovePeer(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.peerPaths, peerID)
	logger.Debug("移除 Peer 路径信息", "peer", peerID[:min(8, len(peerID))])
}

// Reset 重置所有状态
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.peerPaths = make(map[string]map[string]*Path)
	logger.Info("路径健康管理器已重置")
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
