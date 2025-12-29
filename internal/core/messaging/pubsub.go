// Package messaging 提供消息服务模块的实现
package messaging

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/messaging/gossipsub"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              发布订阅模式
// ============================================================================

// Publish 发布消息到主题
//
// 使用 GossipSub 协议实现高效的消息传播：
// - 订阅的主题：发送给 mesh peers
// - 未订阅的主题：发送给 fanout peers
// - O(log N) 轮传播，带宽效率高
func (s *MessagingService) Publish(ctx context.Context, topic string, data []byte) error {
	if atomic.LoadInt32(&s.closed) == 1 {
		return ErrServiceClosed
	}

	// 使用 GossipSub 路由器发布
	if s.gossipRouter != nil {
		return s.gossipRouter.Publish(ctx, topic, data)
	}

	// 降级到简单洪泛模式（兼容性）
	return s.publishFlood(ctx, topic, data)
}

// publishFlood 洪泛发布（降级模式）
func (s *MessagingService) publishFlood(ctx context.Context, topic string, data []byte) error {
	// 检查 endpoint 是否可用
	if s.endpoint == nil {
		return ErrNoConnection
	}

	// 生成消息 ID
	msgID := make([]byte, 16)
	if _, err := rand.Read(msgID); err != nil {
		return fmt.Errorf("生成消息 ID 失败: %w", err)
	}

	msg := &types.Message{
		ID:    msgID,
		Topic: topic,
		From:  s.endpoint.ID(),
		Data:  data,
	}

	// 标记为已见
	s.markSeen(msgID)

	// 本地分发
	s.deliverLocal(msg)

	// 向所有连接的节点广播
	for _, conn := range s.endpoint.Connections() {
		go s.publishToConn(ctx, conn, msg)
	}

	log.Debug("消息已发布（洪泛模式）",
		"topic", topic,
		"size", len(data))

	return nil
}

// publishToConn 向连接发布消息
func (s *MessagingService) publishToConn(ctx context.Context, conn endpoint.Connection, msg *types.Message) {
	stream, err := conn.OpenStream(ctx, ProtocolPubsub)
	if err != nil {
		return
	}
	defer func() { _ = stream.Close() }()

	// 写入消息
	_ = writeMessage(stream, msg) // 发送失败在流关闭时会处理
}

// deliverLocal 本地分发消息
func (s *MessagingService) deliverLocal(msg *types.Message) {
	// 检查是否为查询消息（通过编码格式识别）
	if queryID, replyTo, queryData, isQuery := decodeQueryMessage(msg.Data); isQuery {
		// 处理查询消息
		s.handleIncomingQuery(msg.Topic, queryID, replyTo, queryData, msg.From)
		return
	}

	// 普通消息分发给订阅者
	s.subMu.RLock()
	subs := s.subscriptions[msg.Topic]
	s.subMu.RUnlock()

	for _, sub := range subs {
		if atomic.LoadInt32(&sub.active) == 1 {
			select {
			case sub.messages <- msg:
			default:
				// 通道满，丢弃消息
				log.Warn("订阅通道已满，丢弃消息",
					"topic", msg.Topic)
			}
		}
	}
}

// handleIncomingQuery 处理收到的查询消息
func (s *MessagingService) handleIncomingQuery(topic string, queryID string, replyTo types.NodeID, queryData []byte, from types.NodeID) {
	// 检查 endpoint 是否可用
	if s.endpoint == nil {
		return
	}

	// 不处理自己发送的查询
	if from == s.endpoint.ID() {
		return
	}

	// 查找处理器
	s.handlerMu.RLock()
	handler := s.queryHandlers[topic]
	s.handlerMu.RUnlock()

	if handler == nil {
		return
	}

	// 调用处理器
	responseData, shouldRespond := handler(queryData, from)
	if !shouldRespond {
		return
	}

	// 发送响应到 ReplyTo 节点
	go s.sendQueryResponse(replyTo, queryID, responseData)
}

// sendQueryResponse 发送查询响应
func (s *MessagingService) sendQueryResponse(replyTo types.NodeID, queryID string, data []byte) {
	// 检查 endpoint 是否可用
	if s.endpoint == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取到 ReplyTo 节点的连接
	var conn endpoint.Connection
	for _, c := range s.endpoint.Connections() {
		if c.RemoteID() == replyTo {
			conn = c
			break
		}
	}

	if conn == nil {
		log.Debug("无法发送查询响应：无连接",
			"replyTo", replyTo.ShortString(),
			"queryID", queryID)
		return
	}

	// 打开响应流
	stream, err := conn.OpenStream(ctx, ProtocolQueryResponse)
	if err != nil {
		log.Debug("打开查询响应流失败",
			"replyTo", replyTo.ShortString(),
			"err", err)
		return
	}
	defer func() { _ = stream.Close() }()

	// 写入查询 ID
	if err := writeString(stream, queryID); err != nil {
		return
	}

	// 写入响应数据
	if err := writeBytes(stream, data); err != nil {
		return
	}

	log.Debug("查询响应已发送",
		"queryID", queryID,
		"replyTo", replyTo.ShortString())
}

// Subscribe 订阅主题
//
// 使用 GossipSub 协议订阅：
// - 自动加入主题的 mesh 网络
// - 维护 D=6 个 mesh peers
// - 通过 IHAVE/IWANT 机制确保消息可靠性
func (s *MessagingService) Subscribe(_ context.Context, topic string) (messagingif.Subscription, error) {
	if atomic.LoadInt32(&s.closed) == 1 {
		return nil, ErrServiceClosed
	}

	// 使用 GossipSub 路由器订阅
	if s.gossipRouter != nil {
		msgChan, cancel, err := s.gossipRouter.Subscribe(topic)
		if err != nil {
			return nil, err
		}
		return &gossipSubscriptionHandle{
			topic:    topic,
			messages: msgChan,
			cancel:   cancel,
			active:   1,
		}, nil
	}

	// 降级到简单订阅模式
	return s.subscribeSimple(topic)
}

// subscribeSimple 简单订阅（降级模式）
func (s *MessagingService) subscribeSimple(topic string) (messagingif.Subscription, error) {
	subID := make([]byte, 8)
	if _, err := rand.Read(subID); err != nil {
		return nil, fmt.Errorf("生成订阅 ID 失败: %w", err)
	}

	sub := &subscription{
		id:       fmt.Sprintf("%x", subID),
		topic:    topic,
		messages: make(chan *types.Message, 100),
		active:   1,
	}

	s.subMu.Lock()
	s.subscriptions[topic] = append(s.subscriptions[topic], sub)
	s.subMu.Unlock()

	log.Debug("订阅主题（简单模式）", "topic", topic)

	return &subscriptionHandle{sub: sub, service: s}, nil
}

// handlePubsubStream 处理发布订阅流
func (s *MessagingService) handlePubsubStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	// 读取消息
	msg, err := readMessage(stream)
	if err != nil {
		return
	}

	// 检查去重
	if s.hasSeen(msg.ID) {
		return
	}
	s.markSeen(msg.ID)

	// 本地分发
	s.deliverLocal(msg)

	// 转发给其他节点（Gossip）
	if s.config.FloodPublish && s.endpoint != nil {
		for _, conn := range s.endpoint.Connections() {
			if conn.RemoteID() != msg.From {
				go s.publishToConn(context.Background(), conn, msg)
			}
		}
	}
}

// ============================================================================
//                              Subscription 实现
// ============================================================================

// subscriptionHandle 简单订阅句柄
type subscriptionHandle struct {
	sub     *subscription
	service *MessagingService
}

func (h *subscriptionHandle) Topic() string {
	return h.sub.topic
}

func (h *subscriptionHandle) Messages() <-chan *types.Message {
	return h.sub.messages
}

func (h *subscriptionHandle) Cancel() {
	if atomic.CompareAndSwapInt32(&h.sub.active, 1, 0) {
		close(h.sub.messages)

		// 从服务中移除
		h.service.subMu.Lock()
		subs := h.service.subscriptions[h.sub.topic]
		for i, s := range subs {
			if s.id == h.sub.id {
				h.service.subscriptions[h.sub.topic] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		h.service.subMu.Unlock()
	}
}

func (h *subscriptionHandle) IsActive() bool {
	return atomic.LoadInt32(&h.sub.active) == 1
}

// ============================================================================
//                              GossipSub 订阅句柄
// ============================================================================

// gossipSubscriptionHandle GossipSub 订阅句柄
type gossipSubscriptionHandle struct {
	topic      string
	messages   <-chan *gossipsub.Message
	cancel     func()
	active     int32
	outChan    chan *types.Message
	initOnce   sync.Once
}

func (h *gossipSubscriptionHandle) Topic() string {
	return h.topic
}

func (h *gossipSubscriptionHandle) Messages() <-chan *types.Message {
	// 使用 sync.Once 确保只初始化一次转换 goroutine
	h.initOnce.Do(func() {
		h.outChan = make(chan *types.Message, 100)
		go func() {
			defer close(h.outChan)
			for msg := range h.messages {
				if atomic.LoadInt32(&h.active) == 0 {
					return
				}
				// 转换消息类型
				typesMsg := &types.Message{
					ID:        msg.ID,
					Topic:     msg.Topic,
					From:      msg.From,
					Data:      msg.Data,
					Timestamp: msg.Timestamp,
					Sequence:  msg.Sequence,
				}
				select {
				case h.outChan <- typesMsg:
				default:
					// 通道满，丢弃
				}
			}
		}()
	})
	return h.outChan
}

func (h *gossipSubscriptionHandle) Cancel() {
	if atomic.CompareAndSwapInt32(&h.active, 1, 0) {
		if h.cancel != nil {
			h.cancel()
		}
	}
}

func (h *gossipSubscriptionHandle) IsActive() bool {
	return atomic.LoadInt32(&h.active) == 1
}
