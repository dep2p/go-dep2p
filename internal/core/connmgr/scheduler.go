// Package connmgr 实现连接管理器
//
// scheduler.go 实现拨号调度器，提供：
// - 静态节点（配置的引导节点）
// - 动态节点（发现的节点）
// - 防频繁重连机制
// - 优先级队列
//
// P2 修复完成
package connmgr

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              常量和错误
// ============================================================================

const (
	// DefaultDialTimeout 默认拨号超时
	DefaultDialTimeout = 10 * time.Second

	// DefaultMaxDialAttempts 默认最大拨号尝试次数
	DefaultMaxDialAttempts = 3

	// DefaultBackoffBase 默认退避基数
	DefaultBackoffBase = 5 * time.Second

	// DefaultBackoffMax 默认最大退避时间
	DefaultBackoffMax = 5 * time.Minute

	// DefaultConcurrentDials 默认并发拨号数
	DefaultConcurrentDials = 8

	// DefaultSchedulerCleanupInterval 默认清理间隔
	DefaultSchedulerCleanupInterval = 10 * time.Minute
)

var (
	// ErrSchedulerClosed 调度器已关闭
	ErrSchedulerClosed = errors.New("dial scheduler closed")

	// ErrPeerBanned 节点被禁止
	ErrPeerBanned = errors.New("peer is banned")

	// ErrTooManyAttempts 尝试次数过多
	ErrTooManyAttempts = errors.New("too many dial attempts")

	// ErrInBackoff 正在退避中
	ErrInBackoff = errors.New("peer is in backoff")
)

// ============================================================================
//                              调度器配置
// ============================================================================

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	// DialTimeout 拨号超时
	DialTimeout time.Duration

	// MaxDialAttempts 最大拨号尝试次数（超过后加入黑名单）
	MaxDialAttempts int

	// BackoffBase 退避基数
	BackoffBase time.Duration

	// BackoffMax 最大退避时间
	BackoffMax time.Duration

	// ConcurrentDials 最大并发拨号数
	ConcurrentDials int

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration

	// StaticPeers 静态节点列表
	StaticPeers []StaticPeer
}

// StaticPeer 静态节点
type StaticPeer struct {
	// ID 节点 ID
	ID string

	// Addrs 地址列表
	Addrs []string

	// Permanent 是否永久重连
	Permanent bool
}

// DefaultSchedulerConfig 默认配置
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		DialTimeout:     DefaultDialTimeout,
		MaxDialAttempts: DefaultMaxDialAttempts,
		BackoffBase:     DefaultBackoffBase,
		BackoffMax:      DefaultBackoffMax,
		ConcurrentDials: DefaultConcurrentDials,
		CleanupInterval: DefaultSchedulerCleanupInterval,
	}
}

// ============================================================================
//                              节点状态
// ============================================================================

// PeerPriority 节点优先级
type PeerPriority int

const (
	// PriorityStatic 静态节点（最高优先级）
	PriorityStatic PeerPriority = 0

	// PriorityHigh 高优先级（最近活跃）
	PriorityHigh PeerPriority = 1

	// PriorityNormal 正常优先级
	PriorityNormal PeerPriority = 2

	// PriorityLow 低优先级（多次失败）
	PriorityLow PeerPriority = 3
)

// peerDialState 节点拨号状态
type peerDialState struct {
	// ID 节点 ID
	ID string

	// Addrs 地址列表
	Addrs []string

	// Priority 优先级
	Priority PeerPriority

	// NextDialTime 下次可拨号时间
	NextDialTime time.Time

	// Attempts 尝试次数
	Attempts int

	// LastAttempt 最后尝试时间
	LastAttempt time.Time

	// LastError 最后错误
	LastError error

	// Static 是否为静态节点
	Static bool

	// Permanent 是否永久重连
	Permanent bool

	// Connected 是否已连接
	Connected bool

	// heapIndex 堆索引
	heapIndex int
}

// ============================================================================
//                              优先级队列
// ============================================================================

// dialQueue 拨号队列（最小堆）
type dialQueue []*peerDialState

func (q dialQueue) Len() int { return len(q) }

func (q dialQueue) Less(i, j int) bool {
	// 按 NextDialTime 排序，时间早的优先
	if !q[i].NextDialTime.Equal(q[j].NextDialTime) {
		return q[i].NextDialTime.Before(q[j].NextDialTime)
	}
	// 时间相同按优先级排序
	return q[i].Priority < q[j].Priority
}

func (q dialQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].heapIndex = i
	q[j].heapIndex = j
}

func (q *dialQueue) Push(x interface{}) {
	n := len(*q)
	item := x.(*peerDialState)
	item.heapIndex = n
	*q = append(*q, item)
}

func (q *dialQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.heapIndex = -1
	*q = old[0 : n-1]
	return item
}

// ============================================================================
//                              Scheduler 实现
// ============================================================================

// Scheduler 拨号调度器
type Scheduler struct {
	config SchedulerConfig
	host   interfaces.Host

	mu      sync.RWMutex
	queue   dialQueue                 // 拨号队列
	peers   map[string]*peerDialState // ID -> state
	banned  map[string]time.Time      // 被禁止的节点
	dialing map[string]bool           // 正在拨号的节点

	// 控制
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	dialSem   chan struct{} // 并发控制信号量
	wakeCh    chan struct{} // 唤醒调度循环
}

// NewScheduler 创建拨号调度器
func NewScheduler(host interfaces.Host, config SchedulerConfig) *Scheduler {
	s := &Scheduler{
		config:  config,
		host:    host,
		queue:   make(dialQueue, 0),
		peers:   make(map[string]*peerDialState),
		banned:  make(map[string]time.Time),
		dialing: make(map[string]bool),
		dialSem: make(chan struct{}, config.ConcurrentDials),
		wakeCh:  make(chan struct{}, 1),
	}
	heap.Init(&s.queue)

	// 添加静态节点
	for _, sp := range config.StaticPeers {
		s.addStaticPeer(sp)
	}

	return s
}

// addStaticPeer 添加静态节点
func (s *Scheduler) addStaticPeer(sp StaticPeer) {
	state := &peerDialState{
		ID:           sp.ID,
		Addrs:        sp.Addrs,
		Priority:     PriorityStatic,
		NextDialTime: time.Now(),
		Static:       true,
		Permanent:    sp.Permanent,
		heapIndex:    -1,
	}
	s.peers[sp.ID] = state
	heap.Push(&s.queue, state)
}

// Start 启动调度器
func (s *Scheduler) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 启动调度循环
	s.wg.Add(1)
	go s.scheduleLoop()

	// 启动清理循环
	s.wg.Add(1)
	go s.cleanupLoop()

	logger.Info("拨号调度器已启动",
		"concurrentDials", s.config.ConcurrentDials,
		"staticPeers", len(s.config.StaticPeers))

	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	logger.Info("拨号调度器已停止")
	return nil
}

// ============================================================================
//                              公共方法
// ============================================================================

// AddPeer 添加节点到拨号队列
func (s *Scheduler) AddPeer(peerID string, addrs []string, priority PeerPriority) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否被禁止
	if banTime, banned := s.banned[peerID]; banned {
		if time.Now().Before(banTime) {
			return
		}
		delete(s.banned, peerID)
	}

	// 检查是否已存在
	if state, exists := s.peers[peerID]; exists {
		// 更新地址
		if len(addrs) > 0 {
			state.Addrs = addrs
		}
		return
	}

	// 创建新状态
	state := &peerDialState{
		ID:           peerID,
		Addrs:        addrs,
		Priority:     priority,
		NextDialTime: time.Now(),
		heapIndex:    -1,
	}
	s.peers[peerID] = state
	heap.Push(&s.queue, state)

	// 唤醒调度循环
	s.wake()
}

// RemovePeer 从队列移除节点
func (s *Scheduler) RemovePeer(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.peers[peerID]
	if !exists {
		return
	}

	// 不移除静态节点
	if state.Static {
		return
	}

	// 从堆中移除
	if state.heapIndex >= 0 {
		heap.Remove(&s.queue, state.heapIndex)
	}
	delete(s.peers, peerID)
}

// BanPeer 禁止节点
func (s *Scheduler) BanPeer(peerID string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.banned[peerID] = time.Now().Add(duration)

	// 从队列移除（除非是永久静态节点）
	if state, exists := s.peers[peerID]; exists {
		if !state.Permanent {
			if state.heapIndex >= 0 {
				heap.Remove(&s.queue, state.heapIndex)
			}
			delete(s.peers, peerID)
		}
	}
}

// NotifyConnected 通知连接成功
func (s *Scheduler) NotifyConnected(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.peers[peerID]
	if !exists {
		return
	}

	state.Connected = true
	state.Attempts = 0
	state.LastError = nil

	// 从队列移除（已连接不需要重拨）
	if state.heapIndex >= 0 {
		heap.Remove(&s.queue, state.heapIndex)
	}
}

// NotifyDisconnected 通知断开连接
func (s *Scheduler) NotifyDisconnected(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.peers[peerID]
	if !exists {
		return
	}

	state.Connected = false

	// 静态/永久节点重新加入队列
	if state.Static || state.Permanent {
		state.NextDialTime = time.Now().Add(s.config.BackoffBase)
		if state.heapIndex < 0 {
			heap.Push(&s.queue, state)
		} else {
			heap.Fix(&s.queue, state.heapIndex)
		}
		s.wake()
	}
}

// GetStats 获取统计信息
func (s *Scheduler) GetStats() SchedulerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := SchedulerStats{
		QueueSize:    len(s.queue),
		TotalPeers:   len(s.peers),
		BannedPeers:  len(s.banned),
		DialingPeers: len(s.dialing),
	}

	for _, state := range s.peers {
		if state.Connected {
			stats.ConnectedPeers++
		}
		if state.Static {
			stats.StaticPeers++
		}
	}

	return stats
}

// SchedulerStats 调度器统计
type SchedulerStats struct {
	QueueSize      int
	TotalPeers     int
	ConnectedPeers int
	StaticPeers    int
	BannedPeers    int
	DialingPeers   int
}

// ============================================================================
//                              内部方法
// ============================================================================

// scheduleLoop 调度循环
func (s *Scheduler) scheduleLoop() {
	defer s.wg.Done()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.wakeCh:
			s.processDials()
		case <-timer.C:
			s.processDials()
		}

		// 重置定时器
		s.mu.RLock()
		if len(s.queue) > 0 {
			next := s.queue[0].NextDialTime
			wait := time.Until(next)
			if wait < 0 {
				wait = 0
			}
			timer.Reset(wait)
		} else {
			timer.Reset(time.Second)
		}
		s.mu.RUnlock()
	}
}

// processDials 处理拨号
func (s *Scheduler) processDials() {
	now := time.Now()

	for {
		s.mu.Lock()

		// 检查队列
		if len(s.queue) == 0 {
			s.mu.Unlock()
			return
		}

		// 检查队首
		state := s.queue[0]
		if state.NextDialTime.After(now) {
			s.mu.Unlock()
			return
		}

		// 检查是否正在拨号
		if s.dialing[state.ID] {
			s.mu.Unlock()
			return
		}

		// 弹出队首
		heap.Pop(&s.queue)
		s.dialing[state.ID] = true
		s.mu.Unlock()

		// 获取信号量
		select {
		case s.dialSem <- struct{}{}:
		case <-s.ctx.Done():
			return
		}

		// 异步拨号
		go s.dialPeer(state)
	}
}

// dialPeer 拨号节点
func (s *Scheduler) dialPeer(state *peerDialState) {
	defer func() {
		<-s.dialSem // 释放信号量
	}()

	ctx, cancel := context.WithTimeout(s.ctx, s.config.DialTimeout)
	defer cancel()

	// 执行拨号
	err := s.host.Connect(ctx, state.ID, state.Addrs)

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.dialing, state.ID)
	state.LastAttempt = time.Now()

	if err != nil {
		state.Attempts++
		state.LastError = err

		logger.Debug("拨号失败",
			"peer", state.ID,
			"attempts", state.Attempts,
			"error", err)

		// 检查是否超过最大尝试次数
		if state.Attempts >= s.config.MaxDialAttempts && !state.Permanent {
			// 禁止一段时间
			s.banned[state.ID] = time.Now().Add(s.config.BackoffMax)
			delete(s.peers, state.ID)
			return
		}

		// 计算退避时间
		backoff := s.calculateBackoff(state.Attempts)
		state.NextDialTime = time.Now().Add(backoff)

		// 降低优先级
		if state.Priority < PriorityLow {
			state.Priority++
		}

		// 重新入队
		heap.Push(&s.queue, state)
	} else {
		// 连接成功
		state.Connected = true
		state.Attempts = 0
		state.LastError = nil

		logger.Debug("拨号成功", "peer", state.ID)
	}
}

// calculateBackoff 计算退避时间
func (s *Scheduler) calculateBackoff(attempts int) time.Duration {
	backoff := s.config.BackoffBase
	for i := 1; i < attempts; i++ {
		backoff *= 2
		if backoff > s.config.BackoffMax {
			backoff = s.config.BackoffMax
			break
		}
	}
	return backoff
}

// cleanupLoop 清理循环
func (s *Scheduler) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup 清理过期数据
func (s *Scheduler) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// 清理过期的禁止列表
	for id, banTime := range s.banned {
		if now.After(banTime) {
			delete(s.banned, id)
		}
	}

	logger.Debug("清理完成", "banned", len(s.banned))
}

// wake 唤醒调度循环
func (s *Scheduler) wake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}
