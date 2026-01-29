// Package addrmgmt 提供地址管理协议的实现
package addrmgmt

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              Scheduler 配置
// ============================================================================

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration

	// NotifyTimeout 通知超时
	NotifyTimeout time.Duration

	// MaxNeighbors 最大邻居数
	MaxNeighbors int
}

// DefaultSchedulerConfig 默认配置
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		RefreshInterval: 30 * time.Minute,
		CleanupInterval: 10 * time.Minute,
		NotifyTimeout:   10 * time.Second,
		MaxNeighbors:    50,
	}
}

// ============================================================================
//                              Scheduler 实现
// ============================================================================

// Scheduler 地址管理调度器
//
// 负责：
// - 定期刷新本地地址记录
// - 向邻居发送地址刷新通知
// - 清理过期的地址记录
type Scheduler struct {
	config SchedulerConfig

	// 依赖
	localID string
	handler *Handler

	// 本地地址记录
	localRecord   *AddressRecord
	localRecordMu sync.RWMutex

	// 签名函数（可选，由外部注入）
	signRecord func(record *AddressRecord) error

	// 邻居连接获取函数
	getNeighbors func() []string
	openStream   func(ctx context.Context, peerID string, protocolID string) (interfaces.Stream, error)

	// 运行状态
	running int32
	ctxMu   sync.RWMutex // BUG FIX #B32: 保护 ctx 和 cancel 字段
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewScheduler 创建调度器
func NewScheduler(
	config SchedulerConfig,
	localID string,
	handler *Handler,
) *Scheduler {
	return &Scheduler{
		config:  config,
		localID: localID,
		handler: handler,
	}
}

// SetSignFunction 设置签名函数
func (s *Scheduler) SetSignFunction(sign func(record *AddressRecord) error) {
	s.signRecord = sign
}

// SetNeighborFuncs 设置邻居相关函数
func (s *Scheduler) SetNeighborFuncs(
	getNeighbors func() []string,
	openStream func(ctx context.Context, peerID string, protocolID string) (interfaces.Stream, error),
) {
	s.getNeighbors = getNeighbors
	s.openStream = openStream
}

// Start 启动调度器
func (s *Scheduler) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil
	}

	// BUG FIX #B32: 使用锁保护 ctx 和 cancel 的写入
	s.ctxMu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.ctxMu.Unlock()

	// 启动刷新循环
	go s.refreshLoop()

	// 启动清理循环
	go s.cleanupLoop()

	logger.Info("地址管理调度器已启动",
		"refreshInterval", s.config.RefreshInterval,
		"cleanupInterval", s.config.CleanupInterval)

	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		return nil
	}

	// BUG FIX #B32: 使用锁保护 cancel 的读取
	s.ctxMu.RLock()
	cancel := s.cancel
	s.ctxMu.RUnlock()

	if cancel != nil {
		cancel()
	}

	logger.Info("地址管理调度器已停止")
	return nil
}

// ============================================================================
//                              地址刷新
// ============================================================================

// UpdateLocalAddrs 更新本地地址
func (s *Scheduler) UpdateLocalAddrs(addrs []string) {
	s.localRecordMu.Lock()

	if s.localRecord == nil {
		// 创建新记录
		s.localRecord = NewAddressRecord(s.localID, addrs, time.Hour)
	} else {
		// 更新现有记录
		s.localRecord.UpdateAddresses(addrs)
	}

	// 签名记录
	if s.signRecord != nil {
		if err := s.signRecord(s.localRecord); err != nil {
			logger.Error("签名地址记录失败", "err", err)
		}
	}

	record := s.localRecord.Clone()
	s.localRecordMu.Unlock()

	// 通知邻居
	go s.notifyNeighbors(record)
}

// GetLocalRecord 获取本地地址记录
func (s *Scheduler) GetLocalRecord() *AddressRecord {
	s.localRecordMu.RLock()
	defer s.localRecordMu.RUnlock()
	if s.localRecord == nil {
		return nil
	}
	return s.localRecord.Clone()
}

// refreshLoop 刷新循环
func (s *Scheduler) refreshLoop() {
	// BUG FIX #B32: 使用锁保护 ctx 的读取
	s.ctxMu.RLock()
	ctx := s.ctx
	s.ctxMu.RUnlock()

	if ctx == nil {
		return
	}

	ticker := time.NewTicker(s.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshLocalRecord()
		}
	}
}

// refreshLocalRecord 刷新本地记录
func (s *Scheduler) refreshLocalRecord() {
	s.localRecordMu.Lock()
	if s.localRecord == nil {
		s.localRecordMu.Unlock()
		return
	}

	// 递增序列号
	s.localRecord.Sequence++
	s.localRecord.Timestamp = time.Now()

	// 重新签名
	if s.signRecord != nil {
		if err := s.signRecord(s.localRecord); err != nil {
			logger.Error("签名地址记录失败", "err", err)
		}
	}

	record := s.localRecord.Clone()
	s.localRecordMu.Unlock()

	// 通知邻居
	s.notifyNeighbors(record)
}

// notifyNeighbors 通知邻居地址变化
func (s *Scheduler) notifyNeighbors(record *AddressRecord) {
	if s.getNeighbors == nil || s.openStream == nil {
		return
	}

	neighbors := s.getNeighbors()
	if len(neighbors) == 0 {
		return
	}

	// 限制通知数量
	if len(neighbors) > s.config.MaxNeighbors {
		neighbors = neighbors[:s.config.MaxNeighbors]
	}

	var wg sync.WaitGroup
	for _, peerID := range neighbors {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			s.notifyPeer(id, record)
		}(peerID)
	}

	// 等待所有通知完成（带超时）
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(s.config.NotifyTimeout):
		logger.Debug("邻居通知超时")
	}
}

// notifyPeer 通知单个节点
func (s *Scheduler) notifyPeer(peerID string, record *AddressRecord) {
	// BUG FIX #B32: 使用锁保护 ctx 的读取
	s.ctxMu.RLock()
	parentCtx := s.ctx
	s.ctxMu.RUnlock()

	if parentCtx == nil {
		return
	}

	ctx, cancel := context.WithTimeout(parentCtx, s.config.NotifyTimeout)
	defer cancel()

	stream, err := s.openStream(ctx, peerID, ProtocolID)
	if err != nil {
		logger.Debug("打开流失败",
			"peer", peerID,
			"err", err)
		return
	}
	defer func() { _ = stream.Close() }()

	if err := s.handler.SendRefreshNotify(ctx, stream, record); err != nil {
		logger.Debug("发送刷新通知失败",
			"peer", peerID,
			"err", err)
		return
	}

	logger.Debug("已通知邻居地址更新",
		"peer", peerID,
		"addrs", len(record.Addresses))
}

// ============================================================================
//                              清理
// ============================================================================

// cleanupLoop 清理循环
func (s *Scheduler) cleanupLoop() {
	// BUG FIX #B32: 使用锁保护 ctx 的读取
	s.ctxMu.RLock()
	ctx := s.ctx
	s.ctxMu.RUnlock()

	if ctx == nil {
		return
	}

	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupRecords()
		}
	}
}

// cleanupRecords 清理过期记录
func (s *Scheduler) cleanupRecords() {
	count := s.handler.CleanExpired()
	if count > 0 {
		logger.Debug("清理过期地址记录", "count", count)
	}
}

// ============================================================================
//                              地址查询
// ============================================================================

// QueryPeerAddrs 查询节点地址
//
// 首先检查本地缓存，如果没有则向邻居查询。
func (s *Scheduler) QueryPeerAddrs(ctx context.Context, targetID string) ([]string, error) {
	// 检查本地缓存
	record := s.handler.GetRecord(targetID)
	if record != nil && !record.IsExpired() {
		return record.Addresses, nil
	}

	// 向邻居查询
	return s.queryFromNeighbors(ctx, targetID)
}

// queryFromNeighbors 从邻居查询地址
func (s *Scheduler) queryFromNeighbors(ctx context.Context, targetID string) ([]string, error) {
	if s.getNeighbors == nil || s.openStream == nil {
		return nil, nil
	}

	neighbors := s.getNeighbors()
	if len(neighbors) == 0 {
		return nil, nil
	}

	// 并行查询（最多 3 个邻居）
	maxQueries := 3
	if len(neighbors) < maxQueries {
		maxQueries = len(neighbors)
	}

	type queryResult struct {
		addrs []string
		err   error
	}

	resultCh := make(chan queryResult, maxQueries)

	for i := 0; i < maxQueries; i++ {
		go func(peerID string) {
			addrs, err := s.queryPeer(ctx, peerID, targetID)
			resultCh <- queryResult{addrs: addrs, err: err}
		}(neighbors[i])
	}

	// 等待第一个成功的结果
	for i := 0; i < maxQueries; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-resultCh:
			if result.err == nil && len(result.addrs) > 0 {
				return result.addrs, nil
			}
		}
	}

	return nil, nil
}

// queryPeer 向单个节点查询
func (s *Scheduler) queryPeer(ctx context.Context, peerID, targetID string) ([]string, error) {
	stream, err := s.openStream(ctx, peerID, ProtocolID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()

	record, err := s.handler.QueryPeer(ctx, stream, targetID)
	if err != nil {
		return nil, err
	}

	if record == nil {
		return nil, nil
	}

	// 缓存结果
	s.handler.recordsMu.Lock()
	s.handler.records[targetID] = record
	s.handler.recordsMu.Unlock()

	return record.Addresses, nil
}
