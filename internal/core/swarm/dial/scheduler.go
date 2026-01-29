package dial

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("swarm/dial")

// ============================================================================
//                              配置
// ============================================================================

// Config 拨号调度器配置
type Config struct {
	// MaxDialedConns 最大已拨号连接数
	MaxDialedConns int

	// MaxActiveDials 最大同时拨号数
	MaxActiveDials int

	// DialHistoryExpiration 拨号历史过期时间（秒）
	DialHistoryExpiration int

	// StaticReconnectDelay 静态节点重连延迟（秒）
	StaticReconnectDelay int

	// DialTimeout 单次拨号超时
	DialTimeout time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MaxDialedConns:        50,
		MaxActiveDials:        16,
		DialHistoryExpiration: 30,
		StaticReconnectDelay:  5,
		DialTimeout:           10 * time.Second,
	}
}

// ============================================================================
//                              PeerInfo
// ============================================================================

// PeerInfo 节点信息
type PeerInfo struct {
	// ID 节点 ID
	ID string

	// Addrs 节点地址列表
	Addrs []string
}

// ============================================================================
//                              Scheduler 实现
// ============================================================================

// Scheduler 拨号调度器
type Scheduler struct {
	config Config

	// 连接建立回调
	setupFunc func(ctx context.Context, peerID string, addrs []string) error

	// 路径健康管理器（可选，用于优先选择健康路径）
	pathHealthManager pkgif.PathHealthManager

	// 静态节点（总是尝试保持连接）
	static     map[string]*dialTask
	staticPool []*dialTask
	staticMu   sync.RWMutex

	// 动态节点
	dynamicCh chan PeerInfo

	// 正在拨号
	dialing map[string]*dialTask
	dialMu  sync.RWMutex

	// 已连接
	peers  map[string]struct{}
	peerMu sync.RWMutex

	// 拨号历史（防止频繁重连）
	history *dialHistory

	// 状态
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// dialTask 拨号任务
type dialTask struct {
	node      PeerInfo
	isStatic  bool
	poolIndex int // 在 staticPool 中的索引
}

// NewScheduler 创建拨号调度器
func NewScheduler(config Config, setupFunc func(ctx context.Context, peerID string, addrs []string) error) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		config:     config,
		setupFunc:  setupFunc,
		static:     make(map[string]*dialTask),
		staticPool: make([]*dialTask, 0),
		dynamicCh:  make(chan PeerInfo, 100),
		dialing:    make(map[string]*dialTask),
		peers:      make(map[string]struct{}),
		history:    newDialHistory(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// AddStatic 添加静态节点
func (s *Scheduler) AddStatic(node PeerInfo) {
	s.staticMu.Lock()
	defer s.staticMu.Unlock()

	if _, exists := s.static[node.ID]; exists {
		return
	}

	task := &dialTask{
		node:      node,
		isStatic:  true,
		poolIndex: -1,
	}
	s.static[node.ID] = task

	// 如果节点未连接且不在拨号中，加入静态池
	s.peerMu.RLock()
	_, connected := s.peers[node.ID]
	s.peerMu.RUnlock()

	s.dialMu.RLock()
	_, dialing := s.dialing[node.ID]
	s.dialMu.RUnlock()

	if !connected && !dialing {
		s.addToStaticPool(task)
	}

	logger.Debug("添加静态节点", "peer", node.ID)
}

// RemoveStatic 移除静态节点
func (s *Scheduler) RemoveStatic(id string) {
	s.staticMu.Lock()
	defer s.staticMu.Unlock()

	task := s.static[id]
	if task == nil {
		return
	}

	delete(s.static, id)
	if task.poolIndex >= 0 {
		s.removeFromStaticPool(task.poolIndex)
	}

	logger.Debug("移除静态节点", "peer", id)
}

// AddDynamic 添加动态节点
func (s *Scheduler) AddDynamic(node PeerInfo) {
	select {
	case s.dynamicCh <- node:
	case <-s.ctx.Done():
	default:
		// 通道已满，丢弃
	}
}

// PeerAdded 通知连接建立
func (s *Scheduler) PeerAdded(id string) {
	s.peerMu.Lock()
	s.peers[id] = struct{}{}
	s.peerMu.Unlock()

	// 从静态池移除
	s.staticMu.Lock()
	if task := s.static[id]; task != nil && task.poolIndex >= 0 {
		s.removeFromStaticPool(task.poolIndex)
	}
	s.staticMu.Unlock()

	// 从拨号中移除
	s.dialMu.Lock()
	delete(s.dialing, id)
	s.dialMu.Unlock()

	logger.Debug("节点已连接", "peer", id)
}

// PeerRemoved 通知连接断开
func (s *Scheduler) PeerRemoved(id string) {
	s.peerMu.Lock()
	delete(s.peers, id)
	s.peerMu.Unlock()

	// 如果是静态节点，重新加入静态池
	s.staticMu.Lock()
	task := s.static[id]
	s.staticMu.Unlock()

	if task != nil && task.poolIndex < 0 {
		// 延迟重连
		go func() {
			time.Sleep(time.Duration(s.config.StaticReconnectDelay) * time.Second)
			if atomic.LoadInt32(&s.running) == 1 {
				s.staticMu.Lock()
				if task := s.static[id]; task != nil && task.poolIndex < 0 {
					s.peerMu.RLock()
					_, connected := s.peers[id]
					s.peerMu.RUnlock()
					if !connected {
						s.addToStaticPool(task)
					}
				}
				s.staticMu.Unlock()
			}
		}()
	}

	logger.Debug("节点已断开", "peer", id)
}

// Start 启动调度器
func (s *Scheduler) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil // 已经运行
	}

	// 使用新的 context
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go s.loop()

	logger.Info("拨号调度器已启动")
	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		return nil // 已经停止
	}

	s.cancel()
	s.wg.Wait()

	logger.Info("拨号调度器已停止")
	return nil
}

// loop 主循环
func (s *Scheduler) loop() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// 定期清理历史
	cleanupTicker := time.NewTicker(10 * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return

		case <-ticker.C:
			s.processDialTasks()

		case node := <-s.dynamicCh:
			s.handleDynamicNode(node)

		case <-cleanupTicker.C:
			s.history.cleanup()
		}
	}
}

// processDialTasks 处理拨号任务
func (s *Scheduler) processDialTasks() {
	// 检查拨号槽位
	freeSlots := s.freeDialSlots()
	if freeSlots <= 0 {
		return
	}

	// 优先处理静态节点
	s.staticMu.Lock()
	for freeSlots > 0 && len(s.staticPool) > 0 {
		task := s.staticPool[0]
		s.removeFromStaticPoolLocked(0)
		s.staticMu.Unlock()

		s.startDial(task)
		freeSlots--

		s.staticMu.Lock()
	}
	s.staticMu.Unlock()
}

// handleDynamicNode 处理动态节点
func (s *Scheduler) handleDynamicNode(node PeerInfo) {
	// 检查是否已连接
	s.peerMu.RLock()
	_, connected := s.peers[node.ID]
	s.peerMu.RUnlock()

	if connected {
		return
	}

	// 检查是否在拨号中
	s.dialMu.RLock()
	_, dialing := s.dialing[node.ID]
	s.dialMu.RUnlock()

	if dialing {
		return
	}

	// 检查拨号历史
	if s.history.contains(node.ID) {
		return
	}

	// 检查拨号槽位
	if s.freeDialSlots() <= 0 {
		return
	}

	task := &dialTask{
		node:     node,
		isStatic: false,
	}
	s.startDial(task)
}

// startDial 启动拨号
func (s *Scheduler) startDial(task *dialTask) {
	s.dialMu.Lock()
	if _, exists := s.dialing[task.node.ID]; exists {
		s.dialMu.Unlock()
		return
	}
	s.dialing[task.node.ID] = task
	s.dialMu.Unlock()

	// 添加到历史
	expiry := time.Now().Add(time.Duration(s.config.DialHistoryExpiration) * time.Second)
	s.history.add(task.node.ID, expiry)

	// 启动拨号 goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.dial(task)
	}()
}

// dial 执行拨号
func (s *Scheduler) dial(task *dialTask) {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.DialTimeout)
	defer cancel()

	// 使用路径健康管理器优化地址顺序
	addrs := s.rankAddrsWithHealth(task.node.ID, task.node.Addrs)
	if len(addrs) == 0 {
		logger.Debug("无可用地址",
			"peer", task.node.ID,
			"originalAddrs", len(task.node.Addrs))
		// 从拨号中移除
		s.dialMu.Lock()
		delete(s.dialing, task.node.ID)
		s.dialMu.Unlock()
		return
	}

	err := s.setupFunc(ctx, task.node.ID, addrs)
	if err != nil {
		logger.Debug("拨号失败",
			"peer", task.node.ID,
			"err", err)
	}

	// 从拨号中移除
	s.dialMu.Lock()
	delete(s.dialing, task.node.ID)
	s.dialMu.Unlock()
}

// SetPathHealthManager 设置路径健康管理器
//
// 设置后，拨号时会：
//   - 按路径健康度排序地址
//   - 跳过死亡路径
func (s *Scheduler) SetPathHealthManager(manager pkgif.PathHealthManager) {
	s.pathHealthManager = manager
}

// rankAddrsWithHealth 使用路径健康管理器排序地址并过滤死亡路径
func (s *Scheduler) rankAddrsWithHealth(peerID string, addrs []string) []string {
	if s.pathHealthManager == nil {
		return addrs
	}

	// 使用路径健康管理器排序
	rankedAddrs := s.pathHealthManager.RankAddrs(peerID, addrs)

	// 过滤掉死亡路径
	var healthyAddrs []string
	for _, addr := range rankedAddrs {
		stats := s.pathHealthManager.GetPathStats(peerID, addr)
		if stats == nil {
			// 未知路径，允许尝试
			healthyAddrs = append(healthyAddrs, addr)
			continue
		}
		if stats.State == pkgif.PathStateDead {
			logger.Debug("跳过死亡路径",
				"peer", peerID,
				"addr", addr,
				"consecutiveFailures", stats.ConsecutiveFailures)
			continue
		}
		healthyAddrs = append(healthyAddrs, addr)
	}

	return healthyAddrs
}

// freeDialSlots 返回空闲拨号槽位
func (s *Scheduler) freeDialSlots() int {
	s.peerMu.RLock()
	peerCount := len(s.peers)
	s.peerMu.RUnlock()

	s.dialMu.RLock()
	dialingCount := len(s.dialing)
	s.dialMu.RUnlock()

	slots := (s.config.MaxDialedConns - peerCount) * 2
	if slots > s.config.MaxActiveDials {
		slots = s.config.MaxActiveDials
	}
	return slots - dialingCount
}

// addToStaticPool 添加到静态池
func (s *Scheduler) addToStaticPool(task *dialTask) {
	if task.poolIndex >= 0 {
		return // 已在池中
	}
	s.staticPool = append(s.staticPool, task)
	task.poolIndex = len(s.staticPool) - 1
}

// removeFromStaticPool 从静态池移除
func (s *Scheduler) removeFromStaticPool(idx int) {
	s.removeFromStaticPoolLocked(idx)
}

// removeFromStaticPoolLocked 从静态池移除（需要持有锁）
func (s *Scheduler) removeFromStaticPoolLocked(idx int) {
	if idx < 0 || idx >= len(s.staticPool) {
		return
	}
	task := s.staticPool[idx]
	end := len(s.staticPool) - 1
	if idx != end {
		s.staticPool[idx] = s.staticPool[end]
		s.staticPool[idx].poolIndex = idx
	}
	s.staticPool[end] = nil
	s.staticPool = s.staticPool[:end]
	task.poolIndex = -1
}

// ============================================================================
//                              Stats
// ============================================================================

// Stats 返回调度器统计信息
func (s *Scheduler) Stats() SchedulerStats {
	s.staticMu.RLock()
	staticCount := len(s.static)
	staticPoolCount := len(s.staticPool)
	s.staticMu.RUnlock()

	s.dialMu.RLock()
	dialingCount := len(s.dialing)
	s.dialMu.RUnlock()

	s.peerMu.RLock()
	peerCount := len(s.peers)
	s.peerMu.RUnlock()

	return SchedulerStats{
		StaticNodes:    staticCount,
		StaticPoolSize: staticPoolCount,
		DialingCount:   dialingCount,
		ConnectedPeers: peerCount,
		HistorySize:    s.history.size(),
		FreeDialSlots:  s.freeDialSlots(),
	}
}

// SchedulerStats 调度器统计
type SchedulerStats struct {
	StaticNodes    int
	StaticPoolSize int
	DialingCount   int
	ConnectedPeers int
	HistorySize    int
	FreeDialSlots  int
}

// ============================================================================
//                              拨号历史
// ============================================================================

// dialHistory 拨号历史，防止频繁重连
type dialHistory struct {
	items map[string]time.Time
	mu    sync.RWMutex
}

func newDialHistory() *dialHistory {
	return &dialHistory{
		items: make(map[string]time.Time),
	}
}

func (h *dialHistory) add(key string, expiry time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items[key] = expiry
}

func (h *dialHistory) contains(key string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	expiry, exists := h.items[key]
	if !exists {
		return false
	}
	return time.Now().Before(expiry)
}

func (h *dialHistory) cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for key, expiry := range h.items {
		if now.After(expiry) {
			delete(h.items, key)
		}
	}
}

func (h *dialHistory) size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.items)
}
