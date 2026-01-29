// Package pubsub 实现发布订阅协议
package pubsub

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/internal/protocol/pubsub/delivery"
	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                   P1 修复：可靠投递集成（delivery 模块）
// ============================================================================

// ReliableConfig 可靠投递配置
type ReliableConfig struct {
	// Enabled 是否启用可靠投递
	Enabled bool

	// PublisherConfig 底层可靠发布器配置
	PublisherConfig *delivery.PublisherConfig
}

// DefaultReliableConfig 返回默认可靠投递配置（彻底重构：返回值类型）
func DefaultReliableConfig() ReliableConfig {
	return ReliableConfig{
		Enabled:         false,
		PublisherConfig: delivery.DefaultPublisherConfig(),
	}
}

// WithReliableDelivery 启用可靠投递（彻底重构：直接修改内嵌配置）
//
// 启用后，消息会通过队列缓冲，支持自动重发和状态回调。
// 适用于网络不稳定或需要确保消息送达的场景。
func WithReliableDelivery(enabled bool) Option {
	return func(c *Config) {
		c.ReliableDelivery.Enabled = enabled
		if enabled && c.ReliableDelivery.PublisherConfig == nil {
			c.ReliableDelivery.PublisherConfig = delivery.DefaultPublisherConfig()
		}
	}
}

// WithReliableDeliveryConfig 设置可靠投递详细配置（彻底重构版本）
func WithReliableDeliveryConfig(cfg ReliableConfig) Option {
	return func(c *Config) {
		c.ReliableDelivery = cfg
	}
}

// ============================================================================
//                              可靠 Topic 包装器
// ============================================================================

// reliableTopic 可靠投递 Topic 包装器
//
// 包装底层 Topic，通过 ReliablePublisher 发送消息。
type reliableTopic struct {
	underlying *topic
	publisher  *delivery.ReliablePublisher
	topicName  string
}

// newReliableTopic 创建可靠 Topic
func newReliableTopic(t *topic, publisherConfig *delivery.PublisherConfig) (*reliableTopic, error) {
	// 创建适配器
	adapter := &topicPublisherAdapter{
		topic: t,
	}

	// 创建可靠发布器
	publisher := delivery.NewReliablePublisher(adapter, publisherConfig)

	return &reliableTopic{
		underlying: t,
		publisher:  publisher,
		topicName:  t.name,
	}, nil
}

// 确保实现接口
var _ interfaces.Topic = (*reliableTopic)(nil)

// String 返回主题名称
func (rt *reliableTopic) String() string {
	return rt.underlying.String()
}

// Publish 发布消息（通过可靠发布器）
func (rt *reliableTopic) Publish(ctx context.Context, data []byte, _ ...interfaces.PublishOption) error {
	// 通过可靠发布器发送
	return rt.publisher.Publish(ctx, rt.topicName, data)
}

// Subscribe 订阅主题
func (rt *reliableTopic) Subscribe(opts ...interfaces.SubscribeOption) (interfaces.TopicSubscription, error) {
	return rt.underlying.Subscribe(opts...)
}

// EventHandler 注册事件处理器
func (rt *reliableTopic) EventHandler(opts ...interfaces.TopicEventHandlerOption) (interfaces.TopicEventHandler, error) {
	return rt.underlying.EventHandler(opts...)
}

// ListPeers 列出主题中的节点
func (rt *reliableTopic) ListPeers() []string {
	return rt.underlying.ListPeers()
}

// Close 关闭主题
func (rt *reliableTopic) Close() error {
	// 先停止可靠发布器
	rt.publisher.Stop()
	// 再关闭底层主题
	return rt.underlying.Close()
}

// ============================================================================
//                        Topic Publisher 适配器
// ============================================================================

// topicPublisherAdapter 将 Topic 适配为 delivery.Publisher 接口
type topicPublisherAdapter struct {
	topic *topic
}

// 确保实现接口
var _ delivery.Publisher = (*topicPublisherAdapter)(nil)

// Publish 发布消息
func (a *topicPublisherAdapter) Publish(ctx context.Context, _ string, data []byte) error {
	// 直接调用底层 topic 的 Publish 方法
	return a.topic.Publish(ctx, data)
}

// ============================================================================
//                        可靠发布器管理
// ============================================================================

// ReliableTopicManager 可靠 Topic 管理器
//
// 管理可靠 Topic 的创建和生命周期。
type ReliableTopicManager struct {
	mu       sync.RWMutex
	topics   map[string]*reliableTopic
	config   *delivery.PublisherConfig
	statusCb delivery.StatusCallback
}

// NewReliableTopicManager 创建可靠 Topic 管理器
func NewReliableTopicManager(config *delivery.PublisherConfig) *ReliableTopicManager {
	if config == nil {
		config = delivery.DefaultPublisherConfig()
	}
	return &ReliableTopicManager{
		topics: make(map[string]*reliableTopic),
		config: config,
	}
}

// SetStatusCallback 设置全局状态回调
func (m *ReliableTopicManager) SetStatusCallback(cb delivery.StatusCallback) {
	m.statusCb = cb
}

// WrapTopic 包装 Topic 为可靠 Topic
func (m *ReliableTopicManager) WrapTopic(t *topic) (interfaces.Topic, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已包装
	if rt, exists := m.topics[t.name]; exists {
		return rt, nil
	}

	// 创建可靠 Topic
	rt, err := newReliableTopic(t, m.config)
	if err != nil {
		return nil, err
	}

	// 设置状态回调
	if m.statusCb != nil {
		rt.publisher.OnStatusChange(m.statusCb)
	}

	// 启动可靠发布器（使用后台 context）
	if err := rt.publisher.Start(context.Background()); err != nil {
		return nil, err
	}

	m.topics[t.name] = rt
	return rt, nil
}

// Close 关闭所有可靠 Topic
func (m *ReliableTopicManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, rt := range m.topics {
		rt.publisher.Stop()
	}
	m.topics = make(map[string]*reliableTopic)
	return nil
}

// GetStats 获取所有可靠 Topic 的统计信息
func (m *ReliableTopicManager) GetStats() map[string]delivery.PublisherStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]delivery.PublisherStats)
	for name, rt := range m.topics {
		stats[name] = rt.publisher.GetStats()
	}
	return stats
}
