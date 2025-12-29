// Package messaging 定义消息传递相关接口
//
// 消息模块提供高级通信模式，包括：
// - 请求-响应模式
// - 发布-订阅模式
// - 查询模式
package messaging

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              MessagingService 接口
// ============================================================================

// MessagingService 消息服务接口
//
// 提供三种核心通信模式：请求响应、发布订阅、查询。
type MessagingService interface {
	// ==================== 请求响应模式 ====================

	// Request 发送请求并等待响应
	//
	// 向目标节点发送请求数据，阻塞等待响应。
	// 适用于一对一的同步通信场景。
	Request(ctx context.Context, nodeID types.NodeID, protocol types.ProtocolID, data []byte) ([]byte, error)

	// Send 发送通知，不等待响应
	//
	// 向目标节点发送数据，不期待响应。
	// 适用于单向通知场景。
	Send(ctx context.Context, nodeID types.NodeID, protocol types.ProtocolID, data []byte) error

	// SetRequestHandler 注册请求处理器
	//
	// 当收到指定协议的请求时调用处理器。
	SetRequestHandler(protocol types.ProtocolID, handler RequestHandler)

	// SetNotifyHandler 注册通知处理器
	//
	// 当收到指定协议的通知时调用处理器。
	SetNotifyHandler(protocol types.ProtocolID, handler NotifyHandler)

	// ==================== 发布订阅模式 ====================

	// Publish 发布消息到主题
	//
	// 将消息广播给所有订阅该主题的节点。
	// 使用 Gossip 协议实现高效的消息传播。
	Publish(ctx context.Context, topic string, data []byte) error

	// Subscribe 订阅主题
	//
	// 订阅指定主题，接收该主题上的所有消息。
	// 返回订阅句柄，可用于取消订阅。
	Subscribe(ctx context.Context, topic string) (Subscription, error)

	// TopicPeers 获取订阅指定 topic 的所有已知 peers
	//
	// 返回通过 GossipSub 协议发现的订阅者（本节点视角）。
	// 与 libp2p pubsub.ListPeers(topic) 语义一致。
	TopicPeers(topic string) []types.NodeID

	// MeshPeers 获取指定 topic 的 mesh peers
	//
	// 返回 mesh 网络中的 peers（约 D=6 个），用于第一跳消息传播。
	MeshPeers(topic string) []types.NodeID

	// ==================== 查询模式 ====================

	// Query 发布查询并等待一个响应
	//
	// 向已连接的节点发送查询，返回第一个响应。
	// 适用于已知连接的节点查询场景。
	Query(ctx context.Context, topic string, query []byte) ([]byte, error)

	// QueryAll 发布查询并等待多个响应
	//
	// 向已连接的节点发送查询，收集多个响应。
	// 可通过选项控制最大响应数和超时时间。
	QueryAll(ctx context.Context, topic string, query []byte, opts QueryOptions) ([]QueryResponse, error)

	// PublishQuery 通过 Pub-Sub 广播查询
	//
	// 将查询消息广播到主题的所有订阅者，订阅者通过点对点响应。
	// 相比 Query/QueryAll，可以查询到非直接连接的节点。
	//
	// 流程：
	//   1. 发布带 ReplyTo 字段的查询消息到主题
	//   2. 订阅者收到消息后调用 QueryHandler
	//   3. 有结果的订阅者通过 Request/Response 回复到 ReplyTo 节点
	//   4. 发起方收集响应
	//
	// 参数：
	//   - topic: 查询主题
	//   - query: 查询数据
	//   - opts: 查询选项（超时、最大响应数等）
	PublishQuery(ctx context.Context, topic string, query []byte, opts QueryOptions) ([]QueryResponse, error)

	// SetQueryHandler 注册查询处理器
	//
	// 当收到指定主题的查询时调用处理器。
	// 处理器返回 (响应数据, 是否响应)。
	SetQueryHandler(topic string, handler QueryHandler)

	// ==================== 生命周期 ====================

	// Start 启动消息服务
	Start(ctx context.Context) error

	// Stop 停止消息服务
	Stop() error
}

// ============================================================================
//                              处理器类型
// ============================================================================

// RequestHandler 请求处理函数
//
// 处理请求并返回响应。
type RequestHandler func(req *Request) *Response

// NotifyHandler 通知处理函数
//
// 处理单向通知。
type NotifyHandler func(data []byte, from types.NodeID)

// QueryHandler 查询处理函数
//
// 处理查询并决定是否响应。
// 返回值: (响应数据, 是否响应)
type QueryHandler func(query []byte, from types.NodeID) ([]byte, bool)

// ============================================================================
//                              请求/响应类型别名
// ============================================================================

// Request 请求消息（类型别名，实际定义在 types 包）
type Request = types.Request

// Response 响应消息（类型别名，实际定义在 types 包）
type Response = types.Response

// QueryResponse 查询响应（类型别名，实际定义在 types 包）
type QueryResponse = types.QueryResponse

// ============================================================================
//                              订阅接口
// ============================================================================

// Subscription 订阅句柄接口
type Subscription interface {
	// Topic 返回订阅的主题
	Topic() string

	// Messages 返回消息通道
	Messages() <-chan *Message

	// Cancel 取消订阅
	Cancel()

	// IsActive 是否仍然活跃
	IsActive() bool
}

// Message 发布订阅消息（类型别名，实际定义在 types 包）
type Message = types.Message

// ============================================================================
//                              查询选项
// ============================================================================

// QueryOptions 查询选项
type QueryOptions struct {
	// MaxResponses 最大响应数量
	// 0 表示无限制
	MaxResponses int

	// Timeout 超时时间
	// 到达超时时间后返回已收集的响应
	Timeout time.Duration

	// MinResponses 最小响应数量
	// 在收集到足够响应前不会提前返回
	MinResponses int
}

// DefaultQueryOptions 返回默认查询选项
func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		MaxResponses: 10,
		Timeout:      5 * time.Second,
		MinResponses: 1,
	}
}

// ============================================================================
//                              状态码定义
// ============================================================================

// 状态码
const (
	// StatusOK 成功
	StatusOK uint16 = 200

	// StatusBadRequest 请求格式错误
	StatusBadRequest uint16 = 400

	// StatusUnauthorized 未授权
	StatusUnauthorized uint16 = 401

	// StatusForbidden 禁止访问
	StatusForbidden uint16 = 403

	// StatusNotFound 未找到
	StatusNotFound uint16 = 404

	// StatusTimeout 请求超时
	StatusTimeout uint16 = 408

	// StatusInternalError 内部错误
	StatusInternalError uint16 = 500

	// StatusServiceUnavail 服务不可用
	StatusServiceUnavail uint16 = 503
)

// ============================================================================
//                              事件
// ============================================================================

// 消息服务事件类型
const (
	// EventMessageReceived 收到消息
	EventMessageReceived = "messaging.message_received"

	// EventMessagePublished 消息已发布
	EventMessagePublished = "messaging.message_published"

	// EventSubscribed 订阅成功
	EventSubscribed = "messaging.subscribed"

	// EventUnsubscribed 取消订阅
	EventUnsubscribed = "messaging.unsubscribed"
)

// MessageReceivedEvent 收到消息事件
type MessageReceivedEvent struct {
	Topic     string
	From      types.NodeID
	Size      int
	Timestamp time.Time
}

// Type 返回事件类型
func (e MessageReceivedEvent) Type() string {
	return EventMessageReceived
}

// ============================================================================
//                              配置
// ============================================================================

// Config 消息服务配置
type Config struct {
	// MaxMessageSize 最大消息大小
	MaxMessageSize int

	// MessageTTL 消息 TTL
	MessageTTL time.Duration

	// DeDuplicateTTL 去重缓存 TTL
	DeDuplicateTTL time.Duration

	// FloodPublish 是否使用洪泛发布
	FloodPublish bool

	// HeartbeatInterval Gossip 心跳间隔
	HeartbeatInterval time.Duration

	// HistoryLength 历史消息缓存长度
	HistoryLength int

	// HistoryGossip 每次心跳发送的历史消息数
	HistoryGossip int

	// GossipSub GossipSub 特定配置
	GossipSub GossipSubConfig
}

// GossipSubConfig GossipSub 特定配置
type GossipSubConfig struct {
	// D 目标 mesh 度数（期望的 mesh peer 数量）
	D int

	// Dlo mesh 度数下限
	Dlo int

	// Dhi mesh 度数上限
	Dhi int

	// Dlazy 懒惰发布 peer 数量（用于 gossip）
	Dlazy int

	// Dscore 评分阈值内的 peer 数量
	Dscore int

	// Dout 出站连接的期望数量
	Dout int

	// FanoutTTL fanout map 过期时间
	FanoutTTL time.Duration

	// GossipFactor gossip 发送比例（0-1）
	GossipFactor float64

	// OpportunisticGraftThreshold 机会性 GRAFT 的评分阈值
	OpportunisticGraftThreshold float64

	// OpportunisticGraftTicks 机会性 GRAFT 的心跳间隔数
	OpportunisticGraftTicks int

	// OpportunisticGraftPeers 机会性 GRAFT 的 peer 数
	OpportunisticGraftPeers int

	// PruneBackoff PRUNE 后的退避时间
	PruneBackoff time.Duration

	// UnsubscribeBackoff 取消订阅后的退避时间
	UnsubscribeBackoff time.Duration

	// ConnectorQueueSize 连接器队列大小
	ConnectorQueueSize int

	// MaxPendingConnections 最大挂起连接数
	MaxPendingConnections int

	// GraftFloodThreshold GRAFT 洪泛检测阈值
	GraftFloodThreshold time.Duration

	// MaxIHaveLength 单个 IHAVE 消息的最大消息 ID 数
	MaxIHaveLength int

	// MaxIHaveMessages 单个 RPC 中的最大 IHAVE 消息数
	MaxIHaveMessages int

	// IWantFollowupTime IWANT 请求的跟进时间
	IWantFollowupTime time.Duration

	// SlowHeartbeatWarning 慢心跳警告阈值
	SlowHeartbeatWarning float64

	// SeenMessagesTTL 已见消息缓存 TTL
	SeenMessagesTTL time.Duration

	// ValidateQueueSize 验证队列大小
	ValidateQueueSize int

	// ValidateConcurrency 验证并发数
	ValidateConcurrency int

	// ValidateThrottle 验证节流时间
	ValidateThrottle time.Duration

	// Scoring 评分配置
	Scoring *GossipScoreConfig
}

// GossipScoreConfig GossipSub 评分配置
type GossipScoreConfig struct {
	// GossipThreshold gossip 阈值（低于此值不向该 peer 发送 gossip）
	GossipThreshold float64

	// PublishThreshold 发布阈值（低于此值不向该 peer 发布消息）
	PublishThreshold float64

	// GraylistThreshold 灰名单阈值（低于此值加入灰名单）
	GraylistThreshold float64

	// AcceptPXThreshold 接受 PX 阈值（高于此值才接受该 peer 的 PX）
	AcceptPXThreshold float64

	// DecayInterval 评分衰减间隔
	DecayInterval time.Duration

	// DecayToZero 衰减到零的阈值
	DecayToZero float64

	// RetainScore 保留评分的时间（peer 断开后）
	RetainScore time.Duration

	// AppSpecificWeight 应用特定评分权重
	AppSpecificWeight float64

	// AppSpecificScore 应用特定评分函数
	AppSpecificScore func(peerID string) float64

	// IPColocationFactorWeight IP 协同因子权重
	IPColocationFactorWeight float64

	// IPColocationFactorThreshold IP 协同因子阈值
	IPColocationFactorThreshold int

	// IPColocationFactorWhitelist IP 白名单
	IPColocationFactorWhitelist map[string]struct{}

	// BehaviourPenaltyWeight 行为惩罚权重
	BehaviourPenaltyWeight float64

	// BehaviourPenaltyDecay 行为惩罚衰减
	BehaviourPenaltyDecay float64

	// BehaviourPenaltyThreshold 行为惩罚阈值
	BehaviourPenaltyThreshold float64

	// TopicScoreParams 主题特定评分参数
	TopicScoreParams map[string]*TopicScoreConfig
}

// TopicScoreConfig 主题特定评分配置
type TopicScoreConfig struct {
	// TopicWeight 主题权重
	TopicWeight float64

	// TimeInMeshWeight 在 mesh 中时间权重
	TimeInMeshWeight float64

	// TimeInMeshQuantum mesh 时间量子
	TimeInMeshQuantum time.Duration

	// TimeInMeshCap mesh 时间上限
	TimeInMeshCap float64

	// FirstMessageDeliveriesWeight 首次消息投递权重
	FirstMessageDeliveriesWeight float64

	// FirstMessageDeliveriesDecay 首次消息投递衰减
	FirstMessageDeliveriesDecay float64

	// FirstMessageDeliveriesCap 首次消息投递上限
	FirstMessageDeliveriesCap float64

	// MeshMessageDeliveriesWeight mesh 消息投递权重（负值表示惩罚）
	MeshMessageDeliveriesWeight float64

	// MeshMessageDeliveriesDecay mesh 消息投递衰减
	MeshMessageDeliveriesDecay float64

	// MeshMessageDeliveriesThreshold mesh 消息投递阈值
	MeshMessageDeliveriesThreshold float64

	// MeshMessageDeliveriesCap mesh 消息投递上限
	MeshMessageDeliveriesCap float64

	// MeshMessageDeliveriesActivation mesh 消息投递激活时间
	MeshMessageDeliveriesActivation time.Duration

	// MeshMessageDeliveriesWindow mesh 消息投递窗口
	MeshMessageDeliveriesWindow time.Duration

	// MeshFailurePenaltyWeight mesh 失败惩罚权重
	MeshFailurePenaltyWeight float64

	// MeshFailurePenaltyDecay mesh 失败惩罚衰减
	MeshFailurePenaltyDecay float64

	// InvalidMessageDeliveriesWeight 无效消息投递权重（负值）
	InvalidMessageDeliveriesWeight float64

	// InvalidMessageDeliveriesDecay 无效消息投递衰减
	InvalidMessageDeliveriesDecay float64
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MaxMessageSize:    1 << 20, // 1 MB
		MessageTTL:        time.Minute,
		DeDuplicateTTL:    time.Minute,
		FloodPublish:      false,
		HeartbeatInterval: time.Second,
		HistoryLength:     5,
		HistoryGossip:     3,
		GossipSub:         DefaultGossipSubConfig(),
	}
}

// DefaultGossipSubConfig 返回默认 GossipSub 配置
func DefaultGossipSubConfig() GossipSubConfig {
	return GossipSubConfig{
		D:                           6,
		Dlo:                         4,
		Dhi:                         12,
		Dlazy:                       6,
		Dscore:                      4,
		Dout:                        2,
		FanoutTTL:                   60 * time.Second,
		GossipFactor:                0.25,
		OpportunisticGraftThreshold: 1.0,
		OpportunisticGraftTicks:     60,
		OpportunisticGraftPeers:     2,
		PruneBackoff:                time.Minute,
		UnsubscribeBackoff:          10 * time.Second,
		ConnectorQueueSize:          32,
		MaxPendingConnections:       128,
		GraftFloodThreshold:         10 * time.Millisecond,
		MaxIHaveLength:              5000,
		MaxIHaveMessages:            10,
		IWantFollowupTime:           3 * time.Second,
		SlowHeartbeatWarning:        0.1,
		SeenMessagesTTL:             2 * time.Minute,
		ValidateQueueSize:           32,
		ValidateConcurrency:         4,
		ValidateThrottle:            100 * time.Millisecond,
		Scoring:                     DefaultGossipScoreConfig(),
	}
}

// DefaultGossipScoreConfig 返回默认评分配置
func DefaultGossipScoreConfig() *GossipScoreConfig {
	return &GossipScoreConfig{
		GossipThreshold:              -500,
		PublishThreshold:             -1000,
		GraylistThreshold:            -2500,
		AcceptPXThreshold:            100,
		DecayInterval:                time.Second,
		DecayToZero:                  0.01,
		RetainScore:                  time.Hour,
		AppSpecificWeight:            1.0,
		AppSpecificScore:             nil, // 可由应用层设置
		IPColocationFactorWeight:     -1.0,
		IPColocationFactorThreshold:  6,
		IPColocationFactorWhitelist:  make(map[string]struct{}),
		BehaviourPenaltyWeight:       -1.0,
		BehaviourPenaltyDecay:        0.999,
		BehaviourPenaltyThreshold:    0.0,
		TopicScoreParams:             make(map[string]*TopicScoreConfig),
	}
}

// DefaultTopicScoreConfig 返回默认主题评分配置
func DefaultTopicScoreConfig() *TopicScoreConfig {
	return &TopicScoreConfig{
		TopicWeight:                     1.0,
		TimeInMeshWeight:                0.0027,
		TimeInMeshQuantum:               time.Second,
		TimeInMeshCap:                   3600,
		FirstMessageDeliveriesWeight:    1.0,
		FirstMessageDeliveriesDecay:     0.9997,
		FirstMessageDeliveriesCap:       2000,
		MeshMessageDeliveriesWeight:     0.0,
		MeshMessageDeliveriesDecay:      0.999,
		MeshMessageDeliveriesThreshold:  0,
		MeshMessageDeliveriesCap:        0,
		MeshMessageDeliveriesActivation: 0,
		MeshMessageDeliveriesWindow:     0,
		MeshFailurePenaltyWeight:        0.0,
		MeshFailurePenaltyDecay:         0.999,
		InvalidMessageDeliveriesWeight:  -1000.0,
		InvalidMessageDeliveriesDecay:   0.9997,
	}
}
