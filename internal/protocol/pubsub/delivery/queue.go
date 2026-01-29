// Package delivery 提供可靠消息投递功能
//
// IMPL-NETWORK-RESILIENCE Phase 4: 消息队列
package delivery

import (
	"container/list"
	"sync"
	"time"
)

// ============================================================================
//                              消息队列
// ============================================================================

// QueuedMessage 队列消息
type QueuedMessage struct {
	// ID 消息唯一标识
	ID string

	// Topic 消息主题
	Topic string

	// Data 消息内容
	Data []byte

	// QueuedAt 入队时间
	QueuedAt time.Time

	// Attempts 发送尝试次数
	Attempts int

	// LastAttempt 最后尝试时间
	LastAttempt time.Time
}

// MessageQueue 消息队列
//
// IMPL-NETWORK-RESILIENCE: 用于在网络不可用时缓存消息，
// 网络恢复后自动重发。
type MessageQueue struct {
	mu sync.RWMutex

	// 队列数据（使用链表实现 FIFO）
	queue *list.List

	// 索引（用于快速查找）
	index map[string]*list.Element

	// 配置
	maxSize     int           // 最大消息数
	maxAge      time.Duration // 消息最大存活时间
	maxAttempts int           // 最大重试次数

	// 统计
	totalEnqueued int64
	totalDequeued int64
	totalDropped  int64
	totalExpired  int64
}

// QueueConfig 队列配置
type QueueConfig struct {
	// MaxSize 最大消息数
	// 默认: 1000
	MaxSize int

	// MaxAge 消息最大存活时间
	// 默认: 5m
	MaxAge time.Duration

	// MaxAttempts 最大重试次数
	// 默认: 3
	MaxAttempts int
}

// DefaultQueueConfig 返回默认配置
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		MaxSize:     1000,
		MaxAge:      5 * time.Minute,
		MaxAttempts: 3,
	}
}

// NewMessageQueue 创建消息队列
func NewMessageQueue(config *QueueConfig) *MessageQueue {
	if config == nil {
		config = DefaultQueueConfig()
	}

	return &MessageQueue{
		queue:       list.New(),
		index:       make(map[string]*list.Element),
		maxSize:     config.MaxSize,
		maxAge:      config.MaxAge,
		maxAttempts: config.MaxAttempts,
	}
}

// Enqueue 入队消息
//
// 如果队列已满，会移除最旧的消息。
// 返回是否成功入队（如果消息已存在则返回 false）
func (q *MessageQueue) Enqueue(msg *QueuedMessage) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 检查是否已存在
	if _, exists := q.index[msg.ID]; exists {
		return false
	}

	// 设置入队时间
	if msg.QueuedAt.IsZero() {
		msg.QueuedAt = time.Now()
	}

	// 如果队列已满，移除最旧的消息
	for q.queue.Len() >= q.maxSize {
		oldest := q.queue.Front()
		if oldest != nil {
			oldMsg := oldest.Value.(*QueuedMessage)
			delete(q.index, oldMsg.ID)
			q.queue.Remove(oldest)
			q.totalDropped++
		}
	}

	// 入队
	elem := q.queue.PushBack(msg)
	q.index[msg.ID] = elem
	q.totalEnqueued++

	return true
}

// Dequeue 出队消息
//
// 返回队列头部的消息，如果队列为空返回 nil
func (q *MessageQueue) Dequeue() *QueuedMessage {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 清理过期消息
	q.cleanupExpiredLocked()

	// 获取队首
	front := q.queue.Front()
	if front == nil {
		return nil
	}

	msg := front.Value.(*QueuedMessage)
	delete(q.index, msg.ID)
	q.queue.Remove(front)
	q.totalDequeued++

	return msg
}

// Peek 查看队首消息（不移除）
func (q *MessageQueue) Peek() *QueuedMessage {
	q.mu.RLock()
	defer q.mu.RUnlock()

	front := q.queue.Front()
	if front == nil {
		return nil
	}

	return front.Value.(*QueuedMessage)
}

// Remove 移除指定消息
func (q *MessageQueue) Remove(msgID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	elem, exists := q.index[msgID]
	if !exists {
		return false
	}

	delete(q.index, msgID)
	q.queue.Remove(elem)
	q.totalDequeued++

	return true
}

// Contains 检查消息是否在队列中
func (q *MessageQueue) Contains(msgID string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	_, exists := q.index[msgID]
	return exists
}

// Len 返回队列长度
func (q *MessageQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.queue.Len()
}

// IsEmpty 检查队列是否为空
func (q *MessageQueue) IsEmpty() bool {
	return q.Len() == 0
}

// Clear 清空队列
func (q *MessageQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.queue = list.New()
	q.index = make(map[string]*list.Element)
}

// GetAll 获取所有消息（用于批量处理）
func (q *MessageQueue) GetAll() []*QueuedMessage {
	q.mu.RLock()
	defer q.mu.RUnlock()

	messages := make([]*QueuedMessage, 0, q.queue.Len())
	for elem := q.queue.Front(); elem != nil; elem = elem.Next() {
		messages = append(messages, elem.Value.(*QueuedMessage))
	}

	return messages
}

// Stats 返回队列统计
func (q *MessageQueue) Stats() QueueStats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return QueueStats{
		CurrentSize:   q.queue.Len(),
		MaxSize:       q.maxSize,
		TotalEnqueued: q.totalEnqueued,
		TotalDequeued: q.totalDequeued,
		TotalDropped:  q.totalDropped,
		TotalExpired:  q.totalExpired,
	}
}

// QueueStats 队列统计
type QueueStats struct {
	CurrentSize   int
	MaxSize       int
	TotalEnqueued int64
	TotalDequeued int64
	TotalDropped  int64
	TotalExpired  int64
}

// ============================================================================
//                              内部方法
// ============================================================================

// cleanupExpiredLocked 清理过期消息（需持有锁）
func (q *MessageQueue) cleanupExpiredLocked() {
	if q.maxAge <= 0 {
		return
	}

	now := time.Now()
	threshold := now.Add(-q.maxAge)

	// 从队首开始清理（最旧的在前面）
	for {
		front := q.queue.Front()
		if front == nil {
			break
		}

		msg := front.Value.(*QueuedMessage)
		if msg.QueuedAt.After(threshold) {
			// 消息未过期，停止清理
			break
		}

		// 移除过期消息
		delete(q.index, msg.ID)
		q.queue.Remove(front)
		q.totalExpired++
	}
}

// IncrementAttempts 增加尝试次数
func (q *MessageQueue) IncrementAttempts(msgID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	elem, exists := q.index[msgID]
	if !exists {
		return false
	}

	msg := elem.Value.(*QueuedMessage)
	msg.Attempts++
	msg.LastAttempt = time.Now()

	// 如果超过最大尝试次数，移除
	if msg.Attempts >= q.maxAttempts {
		delete(q.index, msgID)
		q.queue.Remove(elem)
		q.totalDropped++
		return false
	}

	return true
}
