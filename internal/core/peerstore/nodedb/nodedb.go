package nodedb

import (
	"net"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/nodedb")

// ============================================================================
//                              类型定义
// ============================================================================

// NodeRecord 节点记录
type NodeRecord struct {
	// ID 节点 ID
	ID string

	// IP 节点 IP 地址
	IP net.IP

	// UDP UDP 端口
	UDP int

	// TCP TCP 端口
	TCP int

	// Addrs 节点多地址列表
	Addrs []string

	// LastPong 最后 Pong 时间
	LastPong time.Time

	// LastSeen 最后活跃时间
	LastSeen time.Time

	// FailedDials 连续拨号失败次数
	FailedDials int

	// LastDial 最后拨号时间
	LastDial time.Time
}

// Clone 克隆节点记录
func (r *NodeRecord) Clone() *NodeRecord {
	clone := &NodeRecord{
		ID:          r.ID,
		UDP:         r.UDP,
		TCP:         r.TCP,
		LastPong:    r.LastPong,
		LastSeen:    r.LastSeen,
		FailedDials: r.FailedDials,
		LastDial:    r.LastDial,
	}
	if r.IP != nil {
		clone.IP = make(net.IP, len(r.IP))
		copy(clone.IP, r.IP)
	}
	if len(r.Addrs) > 0 {
		clone.Addrs = make([]string, len(r.Addrs))
		copy(clone.Addrs, r.Addrs)
	}
	return clone
}

// ============================================================================
//                              配置
// ============================================================================

// Config 节点数据库配置
type Config struct {
	// MaxNodes 最大节点数
	// 默认 10000
	MaxNodes int

	// NodeExpiry 节点过期时间
	// 超过此时间未活跃的节点会被清理
	// 默认 7 天
	NodeExpiry time.Duration

	// CleanupInterval 清理间隔
	// 默认 1 小时
	CleanupInterval time.Duration

	// MaxFailedDials 最大连续拨号失败次数
	// 超过此次数后节点优先级降低
	// 默认 5
	MaxFailedDials int
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MaxNodes:        10000,
		NodeExpiry:      7 * 24 * time.Hour,
		CleanupInterval: 1 * time.Hour,
		MaxFailedDials:  5,
	}
}

// ============================================================================
//                              NodeDB 接口
// ============================================================================

// NodeDB 节点数据库接口
type NodeDB interface {
	// UpdateNode 更新节点信息
	UpdateNode(node *NodeRecord) error

	// GetNode 获取节点信息
	GetNode(id string) *NodeRecord

	// RemoveNode 删除节点
	RemoveNode(id string) error

	// QuerySeeds 查询种子节点
	// count: 返回的最大节点数
	// maxAge: 节点的最大年龄（超过此时间的节点不返回）
	QuerySeeds(count int, maxAge time.Duration) []*NodeRecord

	// LastPongReceived 获取最后 Pong 时间
	LastPongReceived(id string) time.Time

	// UpdateLastPong 更新最后 Pong 时间
	UpdateLastPong(id string, t time.Time) error

	// UpdateDialAttempt 更新拨号尝试
	UpdateDialAttempt(id string, success bool) error

	// Size 返回节点数量
	Size() int

	// Close 关闭数据库
	Close() error
}

// ============================================================================
//                              Memory NodeDB 实现
// ============================================================================

// MemoryDB 内存节点数据库实现
type MemoryDB struct {
	config Config

	// 节点记录
	nodes map[string]*NodeRecord

	// 生命周期
	ctx    chan struct{}
	closed bool

	mu sync.RWMutex
}

var _ NodeDB = (*MemoryDB)(nil)

// NewMemoryDB 创建内存节点数据库
func NewMemoryDB(config Config) *MemoryDB {
	if config.MaxNodes == 0 {
		config.MaxNodes = DefaultConfig().MaxNodes
	}
	if config.NodeExpiry == 0 {
		config.NodeExpiry = DefaultConfig().NodeExpiry
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = DefaultConfig().CleanupInterval
	}
	if config.MaxFailedDials == 0 {
		config.MaxFailedDials = DefaultConfig().MaxFailedDials
	}

	db := &MemoryDB{
		config: config,
		nodes:  make(map[string]*NodeRecord),
		ctx:    make(chan struct{}),
	}

	// 启动清理协程
	go db.cleanupLoop()

	logger.Info("内存节点数据库已创建", "maxNodes", config.MaxNodes)
	return db
}

// UpdateNode 更新节点信息
func (db *MemoryDB) UpdateNode(node *NodeRecord) error {
	if node == nil || node.ID == "" {
		return ErrInvalidNode
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrDatabaseClosed
	}

	// 检查容量限制
	if len(db.nodes) >= db.config.MaxNodes {
		// 清理过期节点
		db.cleanupExpiredLocked()
		if len(db.nodes) >= db.config.MaxNodes {
			// 删除最旧的节点
			db.removeOldestLocked()
		}
	}

	// 更新或插入
	existing := db.nodes[node.ID]
	if existing != nil {
		// 更新现有记录
		if node.IP != nil {
			existing.IP = node.IP
		}
		if node.UDP > 0 {
			existing.UDP = node.UDP
		}
		if node.TCP > 0 {
			existing.TCP = node.TCP
		}
		if len(node.Addrs) > 0 {
			existing.Addrs = node.Addrs
		}
		if !node.LastSeen.IsZero() {
			existing.LastSeen = node.LastSeen
		}
		if !node.LastPong.IsZero() {
			existing.LastPong = node.LastPong
		}
	} else {
		// 插入新记录
		now := time.Now()
		if node.LastSeen.IsZero() {
			node.LastSeen = now
		}
		db.nodes[node.ID] = node.Clone()
	}

	return nil
}

// GetNode 获取节点信息
func (db *MemoryDB) GetNode(id string) *NodeRecord {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if db.closed {
		return nil
	}

	node := db.nodes[id]
	if node == nil {
		return nil
	}
	return node.Clone()
}

// RemoveNode 删除节点
func (db *MemoryDB) RemoveNode(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrDatabaseClosed
	}

	delete(db.nodes, id)
	return nil
}

// QuerySeeds 查询种子节点
func (db *MemoryDB) QuerySeeds(count int, maxAge time.Duration) []*NodeRecord {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if db.closed {
		return nil
	}

	now := time.Now()
	var candidates []*NodeRecord

	for _, node := range db.nodes {
		age := now.Sub(node.LastSeen)
		if age <= maxAge {
			// 检查失败拨号次数
			if node.FailedDials < db.config.MaxFailedDials {
				candidates = append(candidates, node.Clone())
			}
		}
	}

	// 按最后活跃时间排序（降序），失败次数少的优先
	sort.Slice(candidates, func(i, j int) bool {
		// 首先按失败次数排序
		if candidates[i].FailedDials != candidates[j].FailedDials {
			return candidates[i].FailedDials < candidates[j].FailedDials
		}
		// 然后按最后活跃时间排序
		return candidates[i].LastSeen.After(candidates[j].LastSeen)
	})

	if count > len(candidates) {
		count = len(candidates)
	}
	if count <= 0 {
		return nil
	}

	return candidates[:count]
}

// LastPongReceived 获取最后 Pong 时间
func (db *MemoryDB) LastPongReceived(id string) time.Time {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if db.closed {
		return time.Time{}
	}

	node := db.nodes[id]
	if node == nil {
		return time.Time{}
	}
	return node.LastPong
}

// UpdateLastPong 更新最后 Pong 时间
func (db *MemoryDB) UpdateLastPong(id string, t time.Time) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrDatabaseClosed
	}

	node := db.nodes[id]
	if node == nil {
		// 创建新节点记录
		node = &NodeRecord{
			ID:       id,
			LastPong: t,
			LastSeen: t,
		}
		db.nodes[id] = node
	} else {
		node.LastPong = t
		if t.After(node.LastSeen) {
			node.LastSeen = t
		}
		// 收到 Pong 表示节点活跃，重置失败计数
		node.FailedDials = 0
	}

	return nil
}

// UpdateDialAttempt 更新拨号尝试
func (db *MemoryDB) UpdateDialAttempt(id string, success bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return ErrDatabaseClosed
	}

	node := db.nodes[id]
	if node == nil {
		return nil // 节点不存在，忽略
	}

	node.LastDial = time.Now()
	if success {
		node.FailedDials = 0
		node.LastSeen = time.Now()
	} else {
		node.FailedDials++
	}

	return nil
}

// Size 返回节点数量
func (db *MemoryDB) Size() int {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return len(db.nodes)
}

// Close 关闭数据库
func (db *MemoryDB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return nil
	}

	db.closed = true
	close(db.ctx)
	db.nodes = nil

	logger.Info("内存节点数据库已关闭")
	return nil
}

// cleanupLoop 清理循环
func (db *MemoryDB) cleanupLoop() {
	ticker := time.NewTicker(db.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-db.ctx:
			return
		case <-ticker.C:
			db.cleanup()
		}
	}
}

// cleanup 清理过期节点
func (db *MemoryDB) cleanup() {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return
	}

	db.cleanupExpiredLocked()
}

// cleanupExpiredLocked 清理过期节点（需要持有锁）
func (db *MemoryDB) cleanupExpiredLocked() {
	now := time.Now()
	count := 0
	for id, node := range db.nodes {
		age := now.Sub(node.LastSeen)
		if age > db.config.NodeExpiry {
			delete(db.nodes, id)
			count++
		}
	}
	if count > 0 {
		logger.Debug("清理过期节点", "count", count)
	}
}

// removeOldestLocked 删除最旧的节点（需要持有锁）
func (db *MemoryDB) removeOldestLocked() {
	if len(db.nodes) == 0 {
		return
	}

	var oldestID string
	var oldestTime time.Time
	first := true

	for id, node := range db.nodes {
		if first || node.LastSeen.Before(oldestTime) {
			oldestID = id
			oldestTime = node.LastSeen
			first = false
		}
	}

	if !first {
		delete(db.nodes, oldestID)
		logger.Debug("删除最旧节点", "id", oldestID)
	}
}

// ============================================================================
//                              统计
// ============================================================================

// Stats 返回数据库统计
func (db *MemoryDB) Stats() DBStats {
	db.mu.RLock()
	defer db.mu.RUnlock()

	stats := DBStats{
		TotalNodes: len(db.nodes),
	}

	now := time.Now()
	for _, node := range db.nodes {
		if now.Sub(node.LastSeen) <= 1*time.Hour {
			stats.ActiveNodes++
		}
		if node.FailedDials >= db.config.MaxFailedDials {
			stats.UnreachableNodes++
		}
	}

	return stats
}

// DBStats 数据库统计
type DBStats struct {
	TotalNodes       int
	ActiveNodes      int // 1 小时内活跃
	UnreachableNodes int // 超过最大失败次数
}

// ============================================================================
//                              错误定义
// ============================================================================

// Error 节点数据库错误
type Error struct {
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

var (
	ErrInvalidNode    = &Error{Message: "invalid node record"}
	ErrDatabaseClosed = &Error{Message: "database is closed"}
)
