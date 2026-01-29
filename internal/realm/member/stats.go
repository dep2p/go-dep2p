package member

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              统计信息
// ============================================================================

// StatsCollector 统计收集器
type StatsCollector struct {
	mu sync.RWMutex

	manager *Manager

	// 统计数据
	totalAdded   int64
	totalRemoved int64
	totalSyncs   int64
	lastSyncTime time.Time
}

// NewStatsCollector 创建统计收集器
func NewStatsCollector(manager *Manager) *StatsCollector {
	return &StatsCollector{
		manager: manager,
	}
}

// RecordAdded 记录添加
func (s *StatsCollector) RecordAdded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalAdded++
}

// RecordRemoved 记录删除
func (s *StatsCollector) RecordRemoved() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRemoved++
}

// RecordSync 记录同步
func (s *StatsCollector) RecordSync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalSyncs++
	s.lastSyncTime = time.Now()
}

// GetStats 获取统计信息
func (s *StatsCollector) GetStats() *interfaces.Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &interfaces.Stats{
		LastSyncTime: s.lastSyncTime,
	}

	// 从管理器获取当前统计
	if s.manager != nil {
		managerStats := s.manager.GetStats()
		stats.TotalCount = managerStats.TotalCount
		stats.OnlineCount = managerStats.OnlineCount
		stats.AdminCount = managerStats.AdminCount
		stats.RelayCount = managerStats.RelayCount
		stats.CacheHitRate = managerStats.CacheHitRate
	}

	return stats
}

// GetTotalAdded 获取总添加数
func (s *StatsCollector) GetTotalAdded() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalAdded
}

// GetTotalRemoved 获取总删除数
func (s *StatsCollector) GetTotalRemoved() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalRemoved
}

// GetTotalSyncs 获取总同步次数
func (s *StatsCollector) GetTotalSyncs() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalSyncs
}

// Reset 重置统计
func (s *StatsCollector) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalAdded = 0
	s.totalRemoved = 0
	s.totalSyncs = 0
	s.lastSyncTime = time.Time{}
}
