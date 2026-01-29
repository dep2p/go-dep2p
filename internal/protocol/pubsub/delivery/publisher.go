// Package delivery 提供可靠消息投递功能
//
// IMPL-NETWORK-RESILIENCE Phase 4: 可靠发布器
package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("protocol/pubsub/delivery")

// ============================================================================
//                              接口定义
// ============================================================================

// Publisher 消息发布器接口
type Publisher interface {
	// Publish 发布消息
	Publish(ctx context.Context, topic string, data []byte) error
}

// AckHandler ACK 处理器接口
type AckHandler interface {
	// SendAck 发送 ACK 确认给指定节点
	SendAck(ctx context.Context, targetPeer string, ack *AckMessage) error
}

// ============================================================================
//                              可靠发布器
// ============================================================================

// ReliablePublisher 可靠消息发布器
//
// IMPL-NETWORK-RESILIENCE Phase 4: 包装底层发布器，提供：
// - 消息队列：网络不可用时缓存消息
// - 自动重发：网络恢复后自动发送队列中的消息
// - 发送状态回调：通知调用方消息的实际发送状态
// - ACK 确认：支持关键节点确认机制
type ReliablePublisher struct {
	mu sync.RWMutex

	// 底层发布器
	underlying Publisher

	// 消息队列
	queue *MessageQueue

	// 配置
	config *PublisherConfig

	// 状态回调
	statusCallbacks   []StatusCallback
	statusCallbacksMu sync.RWMutex

	// 运行状态
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	flushing atomic.Bool

	// 统计
	stats PublisherStats

	// ACK 相关字段
	pendingAcks sync.Map // messageID -> *PendingAck
	ackHandler  AckHandler
	localNodeID string
}

// PublisherConfig 发布器配置
type PublisherConfig struct {
	// QueueConfig 队列配置
	QueueConfig *QueueConfig

	// FlushInterval 自动刷新间隔
	// 默认: 1s
	FlushInterval time.Duration

	// FlushBatchSize 每次刷新的最大消息数
	// 默认: 10
	FlushBatchSize int

	// RetryDelay 重试延迟
	// 默认: 100ms
	RetryDelay time.Duration

	// EnableAck 是否启用 ACK 确认机制
	// 默认: false
	EnableAck bool

	// AckTimeout ACK 超时时间
	// 默认: 5s
	AckTimeout time.Duration

	// AckRetries ACK 重试次数
	// 默认: 3
	AckRetries int

	// CriticalPeers 关键节点列表
	CriticalPeers []string

	// RequireAllAcks 是否要求所有关键节点确认
	// 默认: false
	RequireAllAcks bool
}

// DefaultPublisherConfig 返回默认配置
func DefaultPublisherConfig() *PublisherConfig {
	return &PublisherConfig{
		QueueConfig:    DefaultQueueConfig(),
		FlushInterval:  1 * time.Second,
		FlushBatchSize: 10,
		RetryDelay:     100 * time.Millisecond,
		EnableAck:      false,
		AckTimeout:     5 * time.Second,
		AckRetries:     3,
		CriticalPeers:  nil,
		RequireAllAcks: false,
	}
}

// StatusCallback 状态回调
type StatusCallback func(msgID string, status DeliveryStatus, err error)

// DeliveryStatus 投递状态
type DeliveryStatus int

const (
	// StatusQueued 已入队（等待发送）
	StatusQueued DeliveryStatus = iota

	// StatusSent 已发送（best-effort）
	StatusSent

	// StatusAcked 已确认（收到 ACK）
	StatusAcked

	// StatusFailed 发送失败
	StatusFailed

	// StatusDropped 已丢弃（超过重试次数或队列满）
	StatusDropped

	// StatusPendingAck 等待 ACK 中
	StatusPendingAck
)

// String 返回状态字符串
func (s DeliveryStatus) String() string {
	switch s {
	case StatusQueued:
		return "queued"
	case StatusSent:
		return "sent"
	case StatusAcked:
		return "acked"
	case StatusFailed:
		return "failed"
	case StatusDropped:
		return "dropped"
	case StatusPendingAck:
		return "pending_ack"
	default:
		return "unknown"
	}
}

// PublisherStats 发布器统计
type PublisherStats struct {
	TotalPublished  int64 // 总发布数
	TotalQueued     int64 // 总入队数
	TotalSent       int64 // 成功发送数
	TotalFailed     int64 // 失败数
	TotalDropped    int64 // 丢弃数
	QueueSize       int   // 当前队列大小
	TotalAcked      int64 // 收到 ACK 确认数
	TotalAckTimeout int64 // ACK 超时数
	PendingAckCount int   // 当前等待 ACK 数
}

// NewReliablePublisher 创建可靠发布器
func NewReliablePublisher(underlying Publisher, config *PublisherConfig) *ReliablePublisher {
	if config == nil {
		config = DefaultPublisherConfig()
	}

	return &ReliablePublisher{
		underlying:      underlying,
		queue:           NewMessageQueue(config.QueueConfig),
		config:          config,
		statusCallbacks: make([]StatusCallback, 0),
	}
}

// SetUnderlyingPublisher 设置底层发布器
func (p *ReliablePublisher) SetUnderlyingPublisher(publisher Publisher) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.underlying = publisher
}

// SetAckHandler 设置 ACK 处理器
func (p *ReliablePublisher) SetAckHandler(handler AckHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ackHandler = handler
}

// SetLocalNodeID 设置本地节点 ID
func (p *ReliablePublisher) SetLocalNodeID(nodeID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.localNodeID = nodeID
}

// SetCriticalPeers 设置关键节点列表
func (p *ReliablePublisher) SetCriticalPeers(peers []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.CriticalPeers = peers
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动发布器
func (p *ReliablePublisher) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.ctx != nil {
		p.mu.Unlock()
		return ErrAlreadyStarted
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.mu.Unlock()

	// 启动自动刷新
	p.wg.Add(1)
	go p.flushLoop()

	logger.Info("可靠发布器已启动")
	return nil
}

// Stop 停止发布器
func (p *ReliablePublisher) Stop() error {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Unlock()

	p.wg.Wait()

	logger.Info("可靠发布器已停止")
	return nil
}

// ============================================================================
//                              发布接口
// ============================================================================

// Publish 发布消息
//
// 尝试直接发送，如果失败则入队等待重试。
func (p *ReliablePublisher) Publish(ctx context.Context, topic string, data []byte) error {
	atomic.AddInt64(&p.stats.TotalPublished, 1)

	// 生成消息 ID
	msgID := generateMessageID(topic, data)

	// 尝试直接发送（需要读锁保护 underlying 字段）
	p.mu.RLock()
	underlying := p.underlying
	p.mu.RUnlock()

	if underlying != nil {
		if err := underlying.Publish(ctx, topic, data); err == nil {
			// 发送成功
			atomic.AddInt64(&p.stats.TotalSent, 1)
			p.notifyStatus(msgID, StatusSent, nil)
			return nil
		}
		// 发送失败，入队
	}

	// 入队
	msg := &QueuedMessage{
		ID:       msgID,
		Topic:    topic,
		Data:     data,
		QueuedAt: time.Now(),
	}

	if p.queue.Enqueue(msg) {
		atomic.AddInt64(&p.stats.TotalQueued, 1)
		p.notifyStatus(msgID, StatusQueued, nil)
		logger.Debug("消息已入队",
			"msg_id", msgID,
			"topic", topic,
			"queue_size", p.queue.Len())
		return nil
	}

	// 队列已满
	atomic.AddInt64(&p.stats.TotalDropped, 1)
	p.notifyStatus(msgID, StatusDropped, ErrQueueFull)
	return ErrQueueFull
}

// PublishQueued 仅入队（不尝试直接发送）
func (p *ReliablePublisher) PublishQueued(topic string, data []byte) (string, error) {
	msgID := generateMessageID(topic, data)

	msg := &QueuedMessage{
		ID:       msgID,
		Topic:    topic,
		Data:     data,
		QueuedAt: time.Now(),
	}

	if p.queue.Enqueue(msg) {
		atomic.AddInt64(&p.stats.TotalQueued, 1)
		p.notifyStatus(msgID, StatusQueued, nil)
		return msgID, nil
	}

	return "", ErrQueueFull
}

// PublishWithAck 发布消息并等待 ACK
func (p *ReliablePublisher) PublishWithAck(ctx context.Context, topic string, data []byte) (*AckResult, error) {
	// 检查 ACK 是否启用
	if !p.config.EnableAck {
		return nil, ErrAckDisabled
	}

	// 检查关键节点
	if len(p.config.CriticalPeers) == 0 {
		return nil, ErrNoCriticalPeers
	}

	atomic.AddInt64(&p.stats.TotalPublished, 1)

	// 生成消息 ID
	msgID := generateMessageID(topic, data)

	// 创建 PendingAck
	pending := NewPendingAck(
		msgID,
		topic,
		data,
		p.config.CriticalPeers,
		p.config.RequireAllAcks,
	)

	// 注册到 pendingAcks
	p.pendingAcks.Store(msgID, pending)
	defer p.pendingAcks.Delete(msgID)

	// 构建带 ACK 请求的消息
	p.mu.RLock()
	localNodeID := p.localNodeID
	p.mu.RUnlock()

	ackReq := &AckRequest{
		MessageID:   msgID,
		RequesterID: localNodeID,
		Topic:       topic,
		Timestamp:   time.Now(),
	}

	wrappedData, err := PrependAckRequest(ackReq, data)
	if err != nil {
		return nil, err
	}

	// 发送消息（需要读锁保护 underlying 字段）
	p.mu.RLock()
	underlying := p.underlying
	p.mu.RUnlock()

	if underlying != nil {
		if err := underlying.Publish(ctx, topic, wrappedData); err != nil {
			atomic.AddInt64(&p.stats.TotalFailed, 1)
			p.notifyStatus(msgID, StatusFailed, err)
			return nil, err
		}
	}

	// 通知状态: 等待 ACK
	p.notifyStatus(msgID, StatusPendingAck, nil)

	// 等待 ACK 或超时
	return p.waitForAcks(ctx, pending)
}

// HandleAck 处理收到的 ACK 消息
func (p *ReliablePublisher) HandleAck(ack *AckMessage) bool {
	if ack == nil {
		return false
	}

	// 查找对应的 PendingAck
	value, ok := p.pendingAcks.Load(ack.MessageID)
	if !ok {
		logger.Debug("收到未知消息的 ACK",
			"msg_id", ack.MessageID,
			"acker", ack.AckerID)
		return false
	}

	pending := value.(*PendingAck)

	// 添加 ACK
	complete := pending.AddAck(ack.AckerID)

	logger.Debug("收到 ACK",
		"msg_id", ack.MessageID,
		"acker", ack.AckerID,
		"complete", complete)

	// 如果已完成，通知等待者
	if complete {
		select {
		case <-pending.Done:
			// 已关闭
		default:
			close(pending.Done)
		}
	}

	return true
}

// SendAck 发送 ACK 响应
func (p *ReliablePublisher) SendAck(ctx context.Context, targetPeer, msgID, topic string) error {
	p.mu.RLock()
	handler := p.ackHandler
	localNodeID := p.localNodeID
	p.mu.RUnlock()

	if handler == nil {
		return &DeliveryError{Message: "ack handler not set"}
	}

	ack := &AckMessage{
		MessageID: msgID,
		AckerID:   localNodeID,
		Topic:     topic,
		Timestamp: time.Now(),
	}

	return handler.SendAck(ctx, targetPeer, ack)
}

// ProcessIncomingMessage 处理收到的消息
func (p *ReliablePublisher) ProcessIncomingMessage(ctx context.Context, data []byte) ([]byte, error) {
	// 提取 ACK 请求
	ackReq, payload, err := ExtractAckRequest(data)
	if err != nil {
		return data, err
	}

	// 如果有 ACK 请求，发送 ACK
	if ackReq != nil {
		go func() {
			if err := p.SendAck(ctx, ackReq.RequesterID, ackReq.MessageID, ackReq.Topic); err != nil {
				logger.Debug("发送 ACK 失败",
					"msg_id", ackReq.MessageID,
					"target", ackReq.RequesterID,
					"err", err)
			}
		}()
	}

	return payload, nil
}

// ============================================================================
//                              队列管理
// ============================================================================

// FlushQueue 刷新队列（发送所有待发送消息）
func (p *ReliablePublisher) FlushQueue(ctx context.Context) int {
	if !p.flushing.CompareAndSwap(false, true) {
		return 0 // 已在刷新中
	}
	defer p.flushing.Store(false)

	var sentCount int
	batchSize := p.config.FlushBatchSize

	for i := 0; i < batchSize; i++ {
		select {
		case <-ctx.Done():
			return sentCount
		default:
		}

		msg := p.queue.Dequeue()
		if msg == nil {
			break // 队列已空
		}

		// 尝试发送（需要读锁保护 underlying 字段）
		p.mu.RLock()
		underlying := p.underlying
		p.mu.RUnlock()

		if underlying != nil {
			if err := underlying.Publish(ctx, msg.Topic, msg.Data); err == nil {
				atomic.AddInt64(&p.stats.TotalSent, 1)
				p.notifyStatus(msg.ID, StatusSent, nil)
				sentCount++
				continue
			}
		}

		// 发送失败，更新尝试次数
		msg.Attempts++
		msg.LastAttempt = time.Now()

		// 检查是否超过最大重试次数
		if msg.Attempts >= p.config.QueueConfig.MaxAttempts {
			// 超过重试次数，丢弃
			atomic.AddInt64(&p.stats.TotalDropped, 1)
			p.notifyStatus(msg.ID, StatusDropped, ErrMaxRetries)
		} else {
			// 重新入队
			if p.queue.Enqueue(msg) {
				atomic.AddInt64(&p.stats.TotalFailed, 1)
				p.notifyStatus(msg.ID, StatusFailed, nil)
			} else {
				// 队列已满，丢弃
				atomic.AddInt64(&p.stats.TotalDropped, 1)
				p.notifyStatus(msg.ID, StatusDropped, ErrQueueFull)
			}
		}
	}

	if sentCount > 0 {
		logger.Debug("队列刷新完成",
			"sent", sentCount,
			"remaining", p.queue.Len())
	}

	return sentCount
}

// QueueSize 返回队列大小
func (p *ReliablePublisher) QueueSize() int {
	return p.queue.Len()
}

// IsQueueEmpty 检查队列是否为空
func (p *ReliablePublisher) IsQueueEmpty() bool {
	return p.queue.IsEmpty()
}

// ClearQueue 清空队列
func (p *ReliablePublisher) ClearQueue() {
	p.queue.Clear()
}

// ============================================================================
//                              回调和统计
// ============================================================================

// OnStatusChange 注册状态变更回调
func (p *ReliablePublisher) OnStatusChange(callback StatusCallback) {
	p.statusCallbacksMu.Lock()
	defer p.statusCallbacksMu.Unlock()
	p.statusCallbacks = append(p.statusCallbacks, callback)
}

// GetStats 获取统计信息
func (p *ReliablePublisher) GetStats() PublisherStats {
	stats := PublisherStats{
		TotalPublished:  atomic.LoadInt64(&p.stats.TotalPublished),
		TotalQueued:     atomic.LoadInt64(&p.stats.TotalQueued),
		TotalSent:       atomic.LoadInt64(&p.stats.TotalSent),
		TotalFailed:     atomic.LoadInt64(&p.stats.TotalFailed),
		TotalDropped:    atomic.LoadInt64(&p.stats.TotalDropped),
		QueueSize:       p.queue.Len(),
		TotalAcked:      atomic.LoadInt64(&p.stats.TotalAcked),
		TotalAckTimeout: atomic.LoadInt64(&p.stats.TotalAckTimeout),
		PendingAckCount: p.GetPendingAckCount(),
	}
	return stats
}

// GetPendingAckCount 获取等待 ACK 的消息数
func (p *ReliablePublisher) GetPendingAckCount() int {
	count := 0
	p.pendingAcks.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// ============================================================================
//                              内部方法
// ============================================================================

// flushLoop 自动刷新循环
func (p *ReliablePublisher) flushLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if !p.queue.IsEmpty() {
				p.FlushQueue(p.ctx)
			}
		}
	}
}

// notifyStatus 通知状态变更
func (p *ReliablePublisher) notifyStatus(msgID string, status DeliveryStatus, err error) {
	p.statusCallbacksMu.RLock()
	callbacks := make([]StatusCallback, len(p.statusCallbacks))
	copy(callbacks, p.statusCallbacks)
	p.statusCallbacksMu.RUnlock()

	for _, cb := range callbacks {
		go cb(msgID, status, err)
	}
}

// waitForAcks 等待 ACK 完成或超时
func (p *ReliablePublisher) waitForAcks(ctx context.Context, pending *PendingAck) (*AckResult, error) {
	timeout := p.config.AckTimeout
	maxRetries := p.config.AckRetries

	for attempt := 0; attempt <= maxRetries; attempt++ {
		pending.Attempts = attempt

		// 创建超时上下文
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)

		select {
		case <-timeoutCtx.Done():
			cancel()
			if ctx.Err() != nil {
				// 外部上下文取消
				atomic.AddInt64(&p.stats.TotalFailed, 1)
				p.notifyStatus(pending.MessageID, StatusFailed, ctx.Err())
				return pending.GetResult(), ctx.Err()
			}
			// 超时，尝试重试
			if attempt < maxRetries {
				atomic.AddInt64(&p.stats.TotalAckTimeout, 1)
				logger.Debug("ACK 超时，准备重试",
					"msg_id", pending.MessageID,
					"attempt", attempt+1,
					"max_retries", maxRetries)

				// 重发消息
				if err := p.retryPublish(ctx, pending); err != nil {
					logger.Debug("重发消息失败",
						"msg_id", pending.MessageID,
						"err", err)
				}
				continue
			}
			// 超过重试次数
			atomic.AddInt64(&p.stats.TotalAckTimeout, 1)
			atomic.AddInt64(&p.stats.TotalFailed, 1)
			p.notifyStatus(pending.MessageID, StatusFailed, ErrAckTimeout)
			result := pending.GetResult()
			result.Error = ErrAckTimeout
			return result, ErrAckTimeout

		case <-pending.Done:
			cancel()
			// ACK 完成
			atomic.AddInt64(&p.stats.TotalAcked, 1)
			atomic.AddInt64(&p.stats.TotalSent, 1)
			p.notifyStatus(pending.MessageID, StatusAcked, nil)
			return pending.GetResult(), nil
		}
	}

	// 不应到达这里
	return pending.GetResult(), ErrAckTimeout
}

// retryPublish 重发消息
func (p *ReliablePublisher) retryPublish(ctx context.Context, pending *PendingAck) error {
	// 读取需要的字段（需要读锁保护）
	p.mu.RLock()
	underlying := p.underlying
	localNodeID := p.localNodeID
	p.mu.RUnlock()

	if underlying == nil {
		return ErrNoUnderlying
	}

	// 构建带 ACK 请求的消息
	ackReq := &AckRequest{
		MessageID:   pending.MessageID,
		RequesterID: localNodeID,
		Topic:       pending.Topic,
		Timestamp:   time.Now(),
	}

	wrappedData, err := PrependAckRequest(ackReq, pending.Data)
	if err != nil {
		return err
	}

	return underlying.Publish(ctx, pending.Topic, wrappedData)
}

// generateMessageID 生成消息 ID
func generateMessageID(topic string, data []byte) string {
	h := sha256.New()
	h.Write([]byte(topic))
	h.Write(data)
	h.Write([]byte(time.Now().String()))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// CleanupPendingAcks 清理过期的 PendingAck
func (p *ReliablePublisher) CleanupPendingAcks(maxAge time.Duration) int {
	now := time.Now()
	cleaned := 0

	p.pendingAcks.Range(func(key, value interface{}) bool {
		pending := value.(*PendingAck)

		if now.Sub(pending.CreatedAt) > maxAge {
			p.pendingAcks.Delete(key)
			cleaned++

			// 关闭 Done channel（如果还没关闭）
			select {
			case <-pending.Done:
			default:
				close(pending.Done)
			}
		}

		return true
	})

	if cleaned > 0 {
		logger.Debug("清理过期 PendingAck",
			"cleaned", cleaned)
	}

	return cleaned
}
