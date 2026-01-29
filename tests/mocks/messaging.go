package mocks

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// MockMessaging 模拟消息服务（简化版）
//
// 注意：这不是完整的 interfaces.Messaging 实现，
// 仅用于测试场景中记录消息发送行为。
type MockMessaging struct {
	// 消息存储
	SentMessages []SentMessage

	// 可覆盖的方法
	SendFunc  func(ctx context.Context, peerID string, protocol string, data []byte) ([]byte, error)
	CloseFunc func() error
}

// SentMessage 记录发送的消息
type SentMessage struct {
	PeerID   string
	Protocol string
	Data     []byte
}

// NewMockMessaging 创建带有默认值的 MockMessaging
func NewMockMessaging() *MockMessaging {
	return &MockMessaging{
		SentMessages: make([]SentMessage, 0),
	}
}

// Send 发送消息
func (m *MockMessaging) Send(ctx context.Context, peerID string, protocol string, data []byte) ([]byte, error) {
	m.SentMessages = append(m.SentMessages, SentMessage{PeerID: peerID, Protocol: protocol, Data: data})
	if m.SendFunc != nil {
		return m.SendFunc(ctx, peerID, protocol, data)
	}
	return []byte("mock-response"), nil
}

// SendAsync 异步发送消息（简化实现）
func (m *MockMessaging) SendAsync(ctx context.Context, peerID string, protocol string, data []byte) (<-chan *interfaces.Response, error) {
	ch := make(chan *interfaces.Response, 1)
	go func() {
		resp, err := m.Send(ctx, peerID, protocol, data)
		ch <- &interfaces.Response{Data: resp, Error: err}
		close(ch)
	}()
	return ch, nil
}

// SetHandler 设置消息处理器（空实现）
func (m *MockMessaging) SetHandler(_ string, _ interfaces.MessageHandler) {}

// RemoveHandler 移除消息处理器（空实现）
func (m *MockMessaging) RemoveHandler(_ string) {}

// Close 关闭服务
func (m *MockMessaging) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// ============================================================================
// MockPubSub
// ============================================================================

// MockPubSub 模拟发布订阅服务（简化版）
type MockPubSub struct {
	// 主题存储
	Topics map[string]*MockTopic

	// 可覆盖的方法
	JoinFunc      func(topic string) (*MockTopic, error)
	GetTopicsFunc func() []string
	CloseFunc     func() error
}

// NewMockPubSub 创建带有默认值的 MockPubSub
func NewMockPubSub() *MockPubSub {
	return &MockPubSub{
		Topics: make(map[string]*MockTopic),
	}
}

// Join 加入主题
func (m *MockPubSub) Join(topic string) (*MockTopic, error) {
	if m.JoinFunc != nil {
		return m.JoinFunc(topic)
	}
	t := NewMockTopic(topic)
	m.Topics[topic] = t
	return t, nil
}

// GetTopics 获取所有主题
func (m *MockPubSub) GetTopics() []string {
	if m.GetTopicsFunc != nil {
		return m.GetTopicsFunc()
	}
	topics := make([]string, 0, len(m.Topics))
	for t := range m.Topics {
		topics = append(topics, t)
	}
	return topics
}

// Close 关闭服务
func (m *MockPubSub) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// ============================================================================
// MockTopic
// ============================================================================

// MockTopic 模拟主题（简化版）
type MockTopic struct {
	// 主题属性
	TopicName   string
	Closed      bool
	Published   [][]byte
	Subscribers []*MockPubSubSubscription

	// 可覆盖的方法
	PublishFunc   func(ctx context.Context, data []byte) error
	SubscribeFunc func() (*MockPubSubSubscription, error)
	CloseFunc     func() error
}

// NewMockTopic 创建带有默认值的 MockTopic
func NewMockTopic(name string) *MockTopic {
	return &MockTopic{
		TopicName: name,
		Published: make([][]byte, 0),
	}
}

// Publish 发布消息
func (m *MockTopic) Publish(ctx context.Context, data []byte) error {
	m.Published = append(m.Published, data)
	if m.PublishFunc != nil {
		return m.PublishFunc(ctx, data)
	}
	return nil
}

// Subscribe 订阅主题
func (m *MockTopic) Subscribe() (*MockPubSubSubscription, error) {
	if m.SubscribeFunc != nil {
		return m.SubscribeFunc()
	}
	sub := &MockPubSubSubscription{TopicName: m.TopicName, Messages: make(chan *interfaces.Message, 100)}
	m.Subscribers = append(m.Subscribers, sub)
	return sub, nil
}

// Close 关闭主题
func (m *MockTopic) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	m.Closed = true
	return nil
}

// ListPeers 获取订阅者列表（简化实现）
func (m *MockTopic) ListPeers() []string {
	return nil
}

// String 返回主题名
func (m *MockTopic) String() string {
	return m.TopicName
}

// ============================================================================
// MockPubSubSubscription
// ============================================================================

// MockPubSubSubscription 模拟 PubSub 订阅
type MockPubSubSubscription struct {
	TopicName string
	Messages  chan *interfaces.Message
	Cancelled bool
}

// Next 获取下一条消息
func (m *MockPubSubSubscription) Next(ctx context.Context) (*interfaces.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-m.Messages:
		return msg, nil
	}
}

// Topic 返回主题名
func (m *MockPubSubSubscription) Topic() string {
	return m.TopicName
}

// Cancel 取消订阅
func (m *MockPubSubSubscription) Cancel() {
	if !m.Cancelled {
		m.Cancelled = true
		close(m.Messages)
	}
}

// Close 关闭订阅
func (m *MockPubSubSubscription) Close() error {
	m.Cancel()
	return nil
}
