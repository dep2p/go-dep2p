// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 PubSub 接口，提供发布/订阅消息模式。
package interfaces

import (
	"context"
)

// PubSub 定义发布订阅服务接口
//
// PubSub 提供基于 GossipSub 的发布/订阅功能。
type PubSub interface {
	// Join 加入主题
	Join(topic string, opts ...TopicOption) (Topic, error)

	// GetTopics 获取所有已加入的主题
	GetTopics() []string

	// ListPeers 列出指定主题的所有节点
	ListPeers(topic string) []string

	// Close 关闭服务
	Close() error
}

// Topic 定义主题接口
type Topic interface {
	// String 返回主题名称
	String() string

	// Publish 发布消息
	Publish(ctx context.Context, data []byte, opts ...PublishOption) error

	// Subscribe 订阅主题
	Subscribe(opts ...SubscribeOption) (TopicSubscription, error)

	// EventHandler 注册事件处理器
	EventHandler(opts ...TopicEventHandlerOption) (TopicEventHandler, error)

	// ListPeers 列出此主题的所有节点
	ListPeers() []string

	// Close 关闭主题
	Close() error
}

// TopicSubscription 定义主题订阅接口
type TopicSubscription interface {
	// Next 获取下一条消息
	Next(ctx context.Context) (*Message, error)

	// Cancel 取消订阅
	Cancel()
}

// TopicEventHandler 定义主题事件处理器
type TopicEventHandler interface {
	// NextPeerEvent 获取下一个节点事件
	NextPeerEvent(ctx context.Context) (PeerEvent, error)

	// Cancel 取消事件处理
	Cancel()
}

// Message 定义消息结构
type Message struct {
	// From 发送方节点 ID
	From string

	// Data 消息数据
	Data []byte

	// Topic 消息所属主题
	Topic string

	// Seqno 序列号
	Seqno []byte

	// ID 消息唯一标识
	ID string

	// ReceivedFrom 接收自哪个节点
	ReceivedFrom string

	// P1-1: 消息追踪字段（用于 E2E 延迟分析）

	// SentTimeNano 发送时间戳（纳秒，Unix 时间）
	// 由发送方设置，接收方可用于计算 E2E 延迟
	SentTimeNano int64

	// RecvTimeNano 接收时间戳（纳秒，Unix 时间）
	// 由接收方设置，用于计算 E2E 延迟
	RecvTimeNano int64
}

// PeerEvent 节点事件
type PeerEvent struct {
	// Type 事件类型
	Type PeerEventType

	// Peer 相关节点 ID
	Peer string
}

// PeerEventType 节点事件类型
type PeerEventType int

const (
	// PeerJoin 节点加入
	PeerJoin PeerEventType = iota
	// PeerLeave 节点离开
	PeerLeave
)

// TopicOption 主题选项
type TopicOption func(*TopicOptions)

// TopicOptions 主题选项集合
type TopicOptions struct {
	// WithRelay 是否启用中继
	WithRelay bool
}

// PublishOption 发布选项
type PublishOption func(*PublishOptions)

// PublishOptions 发布选项集合
type PublishOptions struct {
	// Ready 发布前的就绪检查
	Ready func() error
}

// SubscribeOption 订阅选项
type SubscribeOption func(*SubscribeOptions)

// SubscribeOptions 订阅选项集合
type SubscribeOptions struct {
	// BufferSize 缓冲区大小
	BufferSize int
}

// TopicEventHandlerOption 事件处理器选项
type TopicEventHandlerOption func(*TopicEventHandlerOptions)

// TopicEventHandlerOptions 事件处理器选项集合
type TopicEventHandlerOptions struct{}
