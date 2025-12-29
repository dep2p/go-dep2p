// Package realm 定义 Realm 相关接口
package realm

import (
	"context"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Messaging 消息服务接口
// ============================================================================

// Messaging 消息服务接口（Layer 3）
//
// 从 Realm 获取，用于点对点消息发送和请求/响应模式。
// 所有协议 ID 由框架自动添加 Realm 前缀。
//
// 示例:
//
//	messaging := realm.Messaging()
//	err := messaging.Send(ctx, peerID, data)
//	resp, err := messaging.Request(ctx, peerID, data)
type Messaging interface {
	// ============================
	// 发送消息
	// ============================

	// Send 发送消息（使用默认协议）
	//
	// 使用框架内置的 messaging 协议发送消息。
	// 协议 ID 自动添加 Realm 前缀。
	Send(ctx context.Context, to types.NodeID, data []byte) error

	// SendWithProtocol 发送消息（指定协议）
	//
	// 用户只需写相对协议名（如 "chat/1.0.0"），
	// 框架自动转换为 "/dep2p/app/<realmID>/chat/1.0.0"
	SendWithProtocol(ctx context.Context, to types.NodeID, protocol string, data []byte) error

	// ============================
	// 请求/响应
	// ============================

	// Request 发送请求并等待响应（使用默认协议）
	Request(ctx context.Context, to types.NodeID, data []byte) ([]byte, error)

	// RequestWithProtocol 发送请求并等待响应（指定协议）
	RequestWithProtocol(ctx context.Context, to types.NodeID, protocol string, data []byte) ([]byte, error)

	// ============================
	// 消息处理器注册
	// ============================

	// OnMessage 注册默认消息处理器
	OnMessage(handler MessageHandler)

	// OnRequest 注册默认请求处理器
	OnRequest(handler RequestHandler)

	// OnProtocol 注册自定义协议处理器
	//
	// 用户只需写相对协议名（如 "chat/1.0.0"），
	// 框架自动添加 Realm 前缀。
	OnProtocol(protocol string, handler ProtocolHandler)
}

// MessageHandler 消息处理函数类型
type MessageHandler func(from types.NodeID, data []byte)

// RequestHandler 请求处理函数类型
// 返回响应数据和错误
type RequestHandler func(from types.NodeID, data []byte) ([]byte, error)

// ProtocolHandler 协议处理函数类型
type ProtocolHandler func(from types.NodeID, protocol string, data []byte) ([]byte, error)

// ============================================================================
//                              PubSub 发布订阅服务接口
// ============================================================================

// PubSub 发布订阅服务接口（Layer 3）
//
// 从 Realm 获取，用于主题订阅和消息广播。
// 所有 topic 由框架自动添加 Realm 前缀。
//
// 示例:
//
//	pubsub := realm.PubSub()
//	topic, err := pubsub.Join(ctx, "blocks")
//	sub, err := topic.Subscribe()
type PubSub interface {
	// Join 加入主题
	//
	// 用户只需写相对主题名（如 "blocks"），
	// 框架自动转换为 "/dep2p/app/<realmID>/blocks"
	Join(ctx context.Context, topicName string) (Topic, error)

	// Topics 返回已加入的所有主题
	Topics() []Topic
}

// Topic 主题对象接口
//
// 代表一个 PubSub 主题，用于发布消息和订阅。
type Topic interface {
	// Name 返回主题名称（用户定义的相对名称）
	Name() string

	// FullName 返回完整主题名称（含 Realm 前缀）
	FullName() string

	// Publish 发布消息到主题
	Publish(ctx context.Context, data []byte) error

	// Subscribe 订阅主题消息
	Subscribe() (Subscription, error)

	// Peers 返回主题内的节点列表
	Peers() []types.NodeID

	// Leave 离开主题
	Leave() error
}

// Subscription 订阅句柄接口
//
// 代表一个主题订阅，用于接收消息。
type Subscription interface {
	// Messages 返回消息通道
	//
	// 当订阅被取消或主题被 Leave 后，通道会被关闭。
	Messages() <-chan *PubSubMessage

	// Cancel 取消订阅
	Cancel()
}

// PubSubMessage PubSub 消息
type PubSubMessage struct {
	// From 发送者节点 ID
	From types.NodeID

	// Topic 主题名称（用户定义的相对名称）
	Topic string

	// Data 消息数据
	Data []byte

	// ReceivedAt 接收时间
	ReceivedAt time.Time
}

// ============================================================================
//                              RealmDiscoveryService Realm 内发现服务接口
// ============================================================================

// RealmDiscoveryService Realm 内发现服务接口（Layer 3）
//
// 从 Realm 获取，用于发现 Realm 内的其他成员。
//
// 示例:
//
//	discovery := realm.Discovery()
//	peers, err := discovery.FindPeers(ctx)
type RealmDiscoveryService interface {
	// FindPeers 发现 Realm 内的其他成员
	FindPeers(ctx context.Context, opts ...FindOption) ([]types.NodeID, error)

	// FindPeersWithService 发现提供特定服务的节点
	FindPeersWithService(ctx context.Context, service string) ([]types.NodeID, error)

	// Advertise 通告自己提供某项服务
	Advertise(ctx context.Context, service string) error

	// StopAdvertise 停止通告服务
	StopAdvertise(service string) error

	// Watch 监听成员变化事件
	Watch(ctx context.Context) (<-chan MemberEvent, error)
}

// FindOption 发现选项函数类型
type FindOption func(*FindOptions)

// FindOptions 发现选项
type FindOptions struct {
	// Limit 最大返回数量
	Limit int

	// Timeout 超时时间
	Timeout time.Duration
}

// WithFindLimit 设置最大返回数量
func WithFindLimit(limit int) FindOption {
	return func(o *FindOptions) {
		o.Limit = limit
	}
}

// WithFindTimeout 设置超时时间
func WithFindTimeout(timeout time.Duration) FindOption {
	return func(o *FindOptions) {
		o.Timeout = timeout
	}
}

// MemberEvent 成员事件
type MemberEvent struct {
	// Type 事件类型
	Type MemberEventType

	// NodeID 相关节点
	NodeID types.NodeID

	// Timestamp 事件时间
	Timestamp time.Time
}

// MemberEventType 成员事件类型
type MemberEventType int

const (
	// MemberJoined 成员加入
	MemberJoined MemberEventType = iota

	// MemberLeft 成员离开
	MemberLeft

	// MemberUpdated 成员信息更新
	MemberUpdated
)

// ============================================================================
//                              StreamManager 流管理服务接口
// ============================================================================

// StreamManager 流管理服务接口（Layer 3）
//
// 从 Realm 获取，用于自定义协议的流式通信。
// 所有协议 ID 由框架自动添加 Realm 前缀。
//
// 示例:
//
//	streams := realm.Streams()
//	stream, err := streams.Open(ctx, peerID, "file-transfer/1.0.0")
type StreamManager interface {
	// Open 打开流
	//
	// 用户只需写相对协议名（如 "file-transfer/1.0.0"），
	// 框架自动转换为 "/dep2p/app/<realmID>/file-transfer/1.0.0"
	Open(ctx context.Context, to types.NodeID, protocol string) (Stream, error)

	// SetHandler 注册协议处理器
	//
	// 用户只需写相对协议名，框架自动添加 Realm 前缀。
	SetHandler(protocol string, handler StreamHandler)

	// RemoveHandler 移除协议处理器
	RemoveHandler(protocol string)
}

// Stream 流接口
//
// 代表一个双向流式连接。
type Stream interface {
	io.ReadWriteCloser

	// Protocol 返回协议 ID（完整路径）
	Protocol() types.ProtocolID

	// RemotePeer 返回对端节点 ID
	RemotePeer() types.NodeID

	// SetDeadline 设置读写超时
	SetDeadline(t time.Time) error

	// SetReadDeadline 设置读超时
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline 设置写超时
	SetWriteDeadline(t time.Time) error
}

// StreamHandler 流处理函数类型
type StreamHandler func(stream Stream)

// ============================================================================
//                              RealmRelayService Realm 中继服务接口
// ============================================================================

// RealmRelayService Realm 中继服务接口（Layer 3）
//
// 从 Realm 获取，用于管理 Realm 内的中继连接。
// Realm Relay 与 System Relay 不同，仅服务于同一 Realm 的成员。
//
// 示例:
//
//	relay := realm.Relay()
//	err := relay.Serve(ctx)  // 成为中继
//	relays, err := relay.FindRelays(ctx)  // 发现中继
type RealmRelayService interface {
	// ============================
	// 成为中继
	// ============================

	// Serve 声明自己愿意为 Realm 提供中继服务
	Serve(ctx context.Context, opts ...RelayServeOption) error

	// StopServing 停止提供中继服务
	StopServing() error

	// IsServing 是否正在提供中继服务
	IsServing() bool

	// ============================
	// 发现中继
	// ============================

	// FindRelays 发现 Realm 内的可用中继节点
	FindRelays(ctx context.Context) ([]types.NodeID, error)

	// ============================
	// 使用中继
	// ============================

	// Reserve 预留一个中继槽位（用于接收入站连接）
	Reserve(ctx context.Context, relay types.NodeID) (Reservation, error)

	// ============================
	// 统计
	// ============================

	// Stats 获取中继使用统计
	Stats() RelayStats
}

// RelayServeOption 中继服务选项函数类型
type RelayServeOption func(*RelayServeOptions)

// RelayServeOptions 中继服务选项
type RelayServeOptions struct {
	// BandwidthLimit 带宽限制（字节/秒）
	BandwidthLimit int64

	// MaxConnections 最大连接数
	MaxConnections int

	// MaxDuration 最大连接时长
	MaxDuration time.Duration
}

// WithRelayBandwidthLimit 设置带宽限制
func WithRelayBandwidthLimit(bytesPerSec int64) RelayServeOption {
	return func(o *RelayServeOptions) {
		o.BandwidthLimit = bytesPerSec
	}
}

// WithRelayMaxConnections 设置最大连接数
func WithRelayMaxConnections(n int) RelayServeOption {
	return func(o *RelayServeOptions) {
		o.MaxConnections = n
	}
}

// WithRelayMaxDuration 设置最大连接时长
func WithRelayMaxDuration(d time.Duration) RelayServeOption {
	return func(o *RelayServeOptions) {
		o.MaxDuration = d
	}
}

// Reservation 中继预留接口
type Reservation interface {
	// Relay 返回预留的中继节点
	Relay() types.NodeID

	// Addrs 返回可以告诉其他人的中继地址
	Addrs() []string

	// Expiry 返回预留过期时间
	Expiry() time.Time

	// Refresh 刷新预留（延长有效期）
	Refresh(ctx context.Context) error

	// Close 释放预留
	Close() error
}

// RelayStats 中继统计
type RelayStats struct {
	// ============================
	// 作为中继时的统计
	// ============================

	// RelayedConnections 已中继的连接数
	RelayedConnections int64

	// RelayedBytes 已中继的字节数
	RelayedBytes int64

	// ActiveRelayedConnections 当前活跃的中继连接数
	ActiveRelayedConnections int

	// ============================
	// 使用中继时的统计
	// ============================

	// ConnectionsViaRelay 通过中继的连接数
	ConnectionsViaRelay int64

	// BytesViaRelay 通过中继的字节数
	BytesViaRelay int64
}

