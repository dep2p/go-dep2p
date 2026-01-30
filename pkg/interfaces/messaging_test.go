package interfaces_test

import (
	"context"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// Mock 实现
// ============================================================================

// MockMessaging 模拟 Messaging 接口实现
type MockMessaging struct {
	sent [][]byte
}

func NewMockMessaging() *MockMessaging {
	return &MockMessaging{
		sent: make([][]byte, 0),
	}
}

func (m *MockMessaging) Send(ctx context.Context, peerID string, protocol string, data []byte) ([]byte, error) {
	m.sent = append(m.sent, data)
	return []byte("response"), nil
}

func (m *MockMessaging) SendAsync(ctx context.Context, peerID string, protocol string, data []byte) (<-chan *interfaces.Response, error) {
	ch := make(chan *interfaces.Response, 1)
	ch <- &interfaces.Response{Data: []byte("response")}
	close(ch)
	return ch, nil
}

func (m *MockMessaging) RegisterHandler(protocol string, handler interfaces.MessageHandler) error {
	return nil
}

func (m *MockMessaging) UnregisterHandler(protocol string) error {
	return nil
}

func (m *MockMessaging) Close() error {
	return nil
}

func (m *MockMessaging) Broadcast(ctx context.Context, protocol string, data []byte) *interfaces.BroadcastResult {
	return &interfaces.BroadcastResult{
		TotalCount:   1,
		SuccessCount: 1,
		FailedCount:  0,
	}
}

func (m *MockMessaging) BroadcastAsync(ctx context.Context, protocol string, data []byte) <-chan interfaces.SendResult {
	ch := make(chan interfaces.SendResult, 1)
	ch <- interfaces.SendResult{}
	close(ch)
	return ch
}

func (m *MockMessaging) SendToMany(ctx context.Context, peers []string, protocol string, data []byte) []interfaces.SendResult {
	results := make([]interfaces.SendResult, len(peers))
	for i, peerID := range peers {
		results[i] = interfaces.SendResult{PeerID: peerID}
	}
	return results
}

// MockPubSub 模拟 PubSub 接口实现
type MockPubSub struct {
	topics map[string]bool
}

func NewMockPubSub() *MockPubSub {
	return &MockPubSub{
		topics: make(map[string]bool),
	}
}

func (m *MockPubSub) Join(topic string, opts ...interfaces.TopicOption) (interfaces.Topic, error) {
	m.topics[topic] = true
	return NewMockTopic(topic), nil
}

func (m *MockPubSub) GetTopics() []string {
	topics := make([]string, 0, len(m.topics))
	for topic := range m.topics {
		topics = append(topics, topic)
	}
	return topics
}

func (m *MockPubSub) ListPeers(topic string) []string {
	return []string{}
}

func (m *MockPubSub) Close() error {
	return nil
}

// MockTopic 模拟 Topic 接口实现
type MockTopic struct {
	name   string
	closed bool
}

func NewMockTopic(name string) *MockTopic {
	return &MockTopic{name: name}
}

func (m *MockTopic) String() string {
	return m.name
}

func (m *MockTopic) Publish(ctx context.Context, data []byte, opts ...interfaces.PublishOption) error {
	return nil
}

func (m *MockTopic) Subscribe(opts ...interfaces.SubscribeOption) (interfaces.TopicSubscription, error) {
	return nil, nil
}

func (m *MockTopic) EventHandler(opts ...interfaces.TopicEventHandlerOption) (interfaces.TopicEventHandler, error) {
	return nil, nil
}

func (m *MockTopic) ListPeers() []string {
	return []string{}
}

func (m *MockTopic) Close() error {
	m.closed = true
	return nil
}

// ============================================================================
// 接口契约测试
// ============================================================================

// TestMessagingInterface 验证 Messaging 接口存在
func TestMessagingInterface(t *testing.T) {
	var _ interfaces.Messaging = (*MockMessaging)(nil)
}

// TestMessaging_Send 测试 Send 方法
func TestMessaging_Send(t *testing.T) {
	msg := NewMockMessaging()
	peer := "test-peer"
	data := []byte("hello")

	resp, err := msg.Send(context.Background(), peer, "/test/1.0.0", data)
	if err != nil {
		t.Errorf("Send() failed: %v", err)
	}

	if len(resp) == 0 {
		t.Error("Send() returned empty response")
	}

	if len(msg.sent) != 1 {
		t.Errorf("Send() sent %d messages, want 1", len(msg.sent))
	}
}

// TestPubSubInterface 验证 PubSub 接口存在
func TestPubSubInterface(t *testing.T) {
	var _ interfaces.PubSub = (*MockPubSub)(nil)
}

// TestPubSub_Join 测试 Join 方法
func TestPubSub_Join(t *testing.T) {
	ps := NewMockPubSub()

	topic, err := ps.Join("test-topic")
	if err != nil {
		t.Errorf("Join() failed: %v", err)
	}

	if topic == nil {
		t.Error("Join() returned nil topic")
	}
}

// TestTopicInterface 验证 Topic 接口存在
func TestTopicInterface(t *testing.T) {
	var _ interfaces.Topic = (*MockTopic)(nil)
}

// TestTopic_Publish 测试 Publish 方法
func TestTopic_Publish(t *testing.T) {
	topic := NewMockTopic("test")

	err := topic.Publish(context.Background(), []byte("data"))
	if err != nil {
		t.Errorf("Publish() failed: %v", err)
	}
}
