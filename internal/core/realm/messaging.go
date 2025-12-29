// Package realm 提供 Realm 管理实现
//
// v1.1: 采用严格单 Realm 模型，不支持跨 Realm 通信
package realm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RealmMessaging 实现
// ============================================================================

// RealmMessagingWrapper Realm 感知的消息服务包装器
//
// 确保 Pub-Sub 消息在 Realm 内隔离
type RealmMessagingWrapper struct {
	manager   *Manager
	messaging messagingif.MessagingService

	// 订阅管理
	subscriptions map[string]*realmSubscription
	subMu         sync.RWMutex

	// 运行状态
	closed int32
}

// realmSubscription Realm 订阅
type realmSubscription struct {
	id        string
	realmID   types.RealmID
	topic     string
	messages  chan *realmif.RealmMessage
	baseSub   messagingif.Subscription
	active    int32
	cancelFn  context.CancelFunc
}

// NewRealmMessagingWrapper 创建 Realm 消息包装器
func NewRealmMessagingWrapper(manager *Manager, messaging messagingif.MessagingService) *RealmMessagingWrapper {
	return &RealmMessagingWrapper{
		manager:       manager,
		messaging:     messaging,
		subscriptions: make(map[string]*realmSubscription),
	}
}

// PublishToRealm 在 Realm 内发布消息
func (m *RealmMessagingWrapper) PublishToRealm(ctx context.Context, realmID types.RealmID, topic string, data []byte) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return fmt.Errorf("messaging wrapper closed")
	}

	if m.manager == nil {
		return ErrNotMember
	}

	// 检查是否是 Realm 成员
	// v1.1: 使用 IsMemberOf
	if !m.manager.IsMemberOf(realmID) {
		return ErrNotMember
	}

	// 构建 Realm 隔离的主题
	realmTopic := m.buildRealmTopic(realmID, topic)

	log.Debug("发布到 Realm",
		"realm", string(realmID),
		"topic", topic,
		"size", len(data))

	// 发布消息
	return m.messaging.Publish(ctx, realmTopic, data)
}

// SubscribeInRealm 在 Realm 内订阅主题
func (m *RealmMessagingWrapper) SubscribeInRealm(ctx context.Context, realmID types.RealmID, topic string) (realmif.RealmSubscription, error) {
	if atomic.LoadInt32(&m.closed) == 1 {
		return nil, fmt.Errorf("messaging wrapper closed")
	}

	if m.manager == nil {
		return nil, ErrNotMember
	}

	// 检查是否是 Realm 成员
	// v1.1: 使用 IsMemberOf
	if !m.manager.IsMemberOf(realmID) {
		return nil, ErrNotMember
	}

	// 构建 Realm 隔离的主题
	realmTopic := m.buildRealmTopic(realmID, topic)

	// 订阅底层消息服务
	baseSub, err := m.messaging.Subscribe(ctx, realmTopic)
	if err != nil {
		return nil, err
	}

	// 创建取消上下文
	subCtx, cancelFn := context.WithCancel(ctx)

	// 创建 Realm 订阅
	subID := fmt.Sprintf("%s:%s:%d", realmID, topic, atomic.AddInt32(&m.closed, 0))
	sub := &realmSubscription{
		id:       subID,
		realmID:  realmID,
		topic:    topic,
		messages: make(chan *realmif.RealmMessage, 100),
		baseSub:  baseSub,
		active:   1,
		cancelFn: cancelFn,
	}

	// 存储订阅
	m.subMu.Lock()
	m.subscriptions[subID] = sub
	m.subMu.Unlock()

	// 启动消息转发协程
	go m.forwardMessages(subCtx, sub)

	log.Debug("订阅 Realm 主题",
		"realm", string(realmID),
		"topic", topic)

	return sub, nil
}

// forwardMessages 转发消息
func (m *RealmMessagingWrapper) forwardMessages(ctx context.Context, sub *realmSubscription) {
	defer func() {
		// 清理
		atomic.StoreInt32(&sub.active, 0)
		close(sub.messages)

		m.subMu.Lock()
		delete(m.subscriptions, sub.id)
		m.subMu.Unlock()
	}()

	baseMsgs := sub.baseSub.Messages()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-baseMsgs:
			if !ok {
				return
			}

			// 检查发送者是否是 Realm 成员
			if m.manager != nil {
				members := m.manager.RealmPeers(sub.realmID)
				isMember := false
				for _, member := range members {
					if member == msg.From {
						isMember = true
						break
					}
				}

				if !isMember {
					// 非成员消息，丢弃
					log.Debug("丢弃非成员消息",
						"from", msg.From.ShortString(),
						"realm", string(sub.realmID))
					continue
				}
			}

			// 转换为 Realm 消息
			realmMsg := &realmif.RealmMessage{
				RealmID: sub.realmID,
				Topic:   sub.topic,
				From:    msg.From,
				Data:    msg.Data,
			}

			// 发送到订阅通道
			select {
			case sub.messages <- realmMsg:
			default:
				// 通道满，丢弃
				log.Warn("订阅通道已满，丢弃消息",
					"realm", string(sub.realmID),
					"topic", sub.topic)
			}
		}
	}
}

// buildRealmTopic 构建 Realm 隔离的主题
func (m *RealmMessagingWrapper) buildRealmTopic(realmID types.RealmID, topic string) string {
	return fmt.Sprintf("realm/%s/%s", realmID, topic)
}

// Close 关闭包装器
func (m *RealmMessagingWrapper) Close() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return nil
	}

	// 取消所有订阅
	m.subMu.Lock()
	for _, sub := range m.subscriptions {
		if sub.cancelFn != nil {
			sub.cancelFn()
		}
		sub.baseSub.Cancel()
	}
	m.subscriptions = make(map[string]*realmSubscription)
	m.subMu.Unlock()

	return nil
}

// 确保实现接口
var _ realmif.RealmMessaging = (*RealmMessagingWrapper)(nil)

// ============================================================================
//                              realmSubscription 实现
// ============================================================================

// Messages 返回消息通道
func (s *realmSubscription) Messages() <-chan *realmif.RealmMessage {
	return s.messages
}

// Cancel 取消订阅
func (s *realmSubscription) Cancel() {
	if atomic.CompareAndSwapInt32(&s.active, 1, 0) {
		if s.cancelFn != nil {
			s.cancelFn()
		}
		if s.baseSub != nil {
			s.baseSub.Cancel()
		}
	}
}

