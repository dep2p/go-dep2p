// Package pubsub 实现发布订阅协议
package pubsub

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
)

// topic 实现 Topic 接口
type topic struct {
	name   string
	ps     *Service
	gossip *gossipSub

	mu            sync.RWMutex
	subscriptions []*topicSubscription
	eventHandlers []*topicEventHandler
	peers         map[string]bool // 主题中的节点
	closed        bool

	// 消息去重：防止同一消息被多次投递给订阅者
	deliveredMsgs   map[string]struct{}
	deliveredMsgsMu sync.Mutex
}

// 确保实现接口
var _ interfaces.Topic = (*topic)(nil)

// newTopic 创建主题
func newTopic(name string, ps *Service, gossip *gossipSub) *topic {
	return &topic{
		name:          name,
		ps:            ps,
		gossip:        gossip,
		peers:         make(map[string]bool),
		deliveredMsgs: make(map[string]struct{}),
	}
}

// String 返回主题名称
func (t *topic) String() string {
	return t.name
}

// Publish 发布消息
func (t *topic) Publish(ctx context.Context, data []byte, opts ...interfaces.PublishOption) error {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		logger.Warn("尝试发布到已关闭的主题", "topic", t.name)
		return ErrTopicClosed
	}
	peerCount := len(t.peers)
	t.mu.RUnlock()

	// 应用选项
	options := &interfaces.PublishOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// 检查就绪条件
	if options.Ready != nil {
		if err := options.Ready(); err != nil {
			logger.Warn("发布就绪检查失败", "topic", t.name, "error", err)
			return fmt.Errorf("ready check failed: %w", err)
		}
	} else {
		// P3 修复：默认等待 Mesh 就绪，避免首次消息丢失
		if err := t.waitForMeshReady(ctx, 2*time.Second); err != nil {
			logger.Warn("Mesh 未就绪，发布失败", "topic", t.name, "error", err)
			return err
		}
	}

	// 创建消息
	msg := &pb.Message{
		From:  []byte(t.ps.host.ID()),
		Data:  data,
		Topic: t.name,
		Seqno: generateSeqno(),
	}

	// 验证消息
	if err := t.ps.validator.Validate(ctx, t.ps.host.ID(), msg); err != nil {
		logger.Warn("消息验证失败", "topic", t.name, "error", err)
		return err
	}

	// P1-1: 记录发送时间用于 E2E 延迟分析
	sendTimeNano := time.Now().UnixNano()
	msgID := messageID(string(msg.From), msg.Seqno)

	logger.Debug("发布消息",
		"topic", t.name,
		"msgID", msgID[:16],
		"dataSize", len(data),
		"peerCount", peerCount,
		"sendTimeNano", sendTimeNano)

	// 通过 GossipSub 发布
	err := t.gossip.Publish(ctx, t.name, msg)
	if err != nil {
		logger.Warn("消息发布失败", "topic", t.name, "msgID", msgID[:16], "error", err)
	} else {
		logger.Debug("消息发布成功", "topic", t.name, "msgID", msgID[:16])
	}
	return err
}

// waitForMeshReady 等待 Mesh 就绪
//
// P3 修复：当 Mesh 为空时短暂等待，避免首次消息丢失。
// 如果超过等待时间仍无 Mesh 节点，返回 ErrNoConnectedPeers。
func (t *topic) waitForMeshReady(ctx context.Context, timeout time.Duration) error {
	if t.gossip == nil {
		return ErrNoConnectedPeers
	}

	// 如果已就绪，直接返回
	if t.gossip.mesh.Count(t.name) > 0 {
		return nil
	}

	if timeout <= 0 {
		return ErrNoConnectedPeers
	}

	timer := time.NewTimer(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			if t.gossip.mesh.Count(t.name) > 0 {
				return nil
			}
			return ErrNoConnectedPeers
		case <-ticker.C:
			if t.gossip.mesh.Count(t.name) > 0 {
				return nil
			}
		}
	}
}

// Subscribe 订阅主题
func (t *topic) Subscribe(opts ...interfaces.SubscribeOption) (interfaces.TopicSubscription, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, ErrTopicClosed
	}

	// 应用选项
	options := &interfaces.SubscribeOptions{
		BufferSize: 32, // 默认缓冲区大小
	}
	for _, opt := range opts {
		opt(options)
	}

	// 创建订阅
	sub := newTopicSubscription(t, options.BufferSize)
	t.subscriptions = append(t.subscriptions, sub)

	return sub, nil
}

// EventHandler 注册事件处理器
func (t *topic) EventHandler(_ ...interfaces.TopicEventHandlerOption) (interfaces.TopicEventHandler, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, ErrTopicClosed
	}

	// 创建事件处理器
	handler := newTopicEventHandler(t)
	t.eventHandlers = append(t.eventHandlers, handler)

	return handler, nil
}

// ListPeers 列出此主题的所有节点
func (t *topic) ListPeers() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	peers := make([]string, 0, len(t.peers))
	for peerID := range t.peers {
		peers = append(peers, peerID)
	}
	return peers
}

// Close 关闭主题
func (t *topic) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true

	// 取消所有订阅
	for _, sub := range t.subscriptions {
		sub.Cancel()
	}
	t.subscriptions = nil

	// 取消所有事件处理器
	for _, handler := range t.eventHandlers {
		handler.Cancel()
	}
	t.eventHandlers = nil

	// 从 GossipSub 离开主题
	if t.gossip != nil {
		t.gossip.Leave(t.name)
	}

	return nil
}

// deliverMessage 投递消息给所有订阅者
//
// 消息去重：每个消息 ID 只投递一次，防止重复投递。
func (t *topic) deliverMessage(msg *interfaces.Message) {
	// 消息去重检查
	t.deliveredMsgsMu.Lock()
	if _, exists := t.deliveredMsgs[msg.ID]; exists {
		t.deliveredMsgsMu.Unlock()
		logger.Debug("跳过重复投递", "topic", t.name, "msgID", msg.ID[:16])
		return
	}
	t.deliveredMsgs[msg.ID] = struct{}{}
	// 限制缓存大小，防止内存泄漏
	if len(t.deliveredMsgs) > 10000 {
		// 简单策略：清空重建
		t.deliveredMsgs = make(map[string]struct{})
	}
	t.deliveredMsgsMu.Unlock()

	// P1-1: 记录消息投递时间用于 E2E 延迟分析
	msgIDShort := msg.ID
	if len(msgIDShort) > 16 {
		msgIDShort = msgIDShort[:16]
	}
	logger.Debug("投递消息给订阅者",
		"topic", t.name,
		"msgID", msgIDShort,
		"from", truncatePeerID(msg.From, 8),
		"recvTimeNano", msg.RecvTimeNano)

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, sub := range t.subscriptions {
		if !sub.isCancelled() {
			sub.pushMessage(msg)
		}
	}
}

// truncatePeerID 安全截断 PeerID
func truncatePeerID(peerID string, maxLen int) string {
	if len(peerID) <= maxLen {
		return peerID
	}
	return peerID[:maxLen]
}

// notifyPeerJoin 通知节点加入
func (t *topic) notifyPeerJoin(peerID string) {
	t.mu.Lock()
	_, alreadyIn := t.peers[peerID]
	t.peers[peerID] = true
	peerCount := len(t.peers)
	t.mu.Unlock()

	if !alreadyIn {
		peerShort := peerID
		if len(peerShort) > 8 {
			peerShort = peerShort[:8]
		}
		logger.Info("Peer 加入主题",
			"topic", t.name,
			"peer", peerShort,
			"totalPeers", peerCount)
	}

	event := interfaces.PeerEvent{
		Type: interfaces.PeerJoin,
		Peer: peerID,
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, handler := range t.eventHandlers {
		if !handler.isCancelled() {
			handler.pushEvent(event)
		}
	}
}

// notifyPeerLeave 通知节点离开
func (t *topic) notifyPeerLeave(peerID string) {
	t.mu.Lock()
	_, wasIn := t.peers[peerID]
	delete(t.peers, peerID)
	peerCount := len(t.peers)
	t.mu.Unlock()

	if wasIn {
		peerShort := peerID
		if len(peerShort) > 8 {
			peerShort = peerShort[:8]
		}
		logger.Info("Peer 离开主题",
			"topic", t.name,
			"peer", peerShort,
			"remainingPeers", peerCount)
	}

	event := interfaces.PeerEvent{
		Type: interfaces.PeerLeave,
		Peer: peerID,
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, handler := range t.eventHandlers {
		if !handler.isCancelled() {
			handler.pushEvent(event)
		}
	}
}
