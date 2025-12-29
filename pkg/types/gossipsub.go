package types

import "time"

// ============================================================================
//                              GossipSub 控制消息类型
// ============================================================================

// GossipControlType 控制消息类型
type GossipControlType uint8

const (
	// GossipControlIHave 通知有消息（消息 ID 列表）
	GossipControlIHave GossipControlType = iota + 1
	// GossipControlIWant 请求消息
	GossipControlIWant
	// GossipControlGraft 请求加入 mesh
	GossipControlGraft
	// GossipControlPrune 移出 mesh
	GossipControlPrune
)

// String 返回控制消息类型的字符串表示
func (t GossipControlType) String() string {
	switch t {
	case GossipControlIHave:
		return "IHAVE"
	case GossipControlIWant:
		return "IWANT"
	case GossipControlGraft:
		return "GRAFT"
	case GossipControlPrune:
		return "PRUNE"
	default:
		return "UNKNOWN"
	}
}

// ============================================================================
//                              GossipSub RPC 消息
// ============================================================================

// GossipRPC 是 GossipSub 的协议消息，包含订阅、数据和控制消息
type GossipRPC struct {
	// Subscriptions 订阅变更列表
	Subscriptions []GossipSubOpt

	// Messages 数据消息列表
	Messages []*Message

	// Control 控制消息
	Control *GossipControl
}

// GossipSubOpt 订阅选项
type GossipSubOpt struct {
	// Subscribe true 表示订阅，false 表示取消订阅
	Subscribe bool
	// Topic 主题
	Topic string
}

// GossipControl 控制消息
type GossipControl struct {
	// IHave 通知有消息
	IHave []GossipIHave
	// IWant 请求消息
	IWant []GossipIWant
	// Graft 请求加入 mesh
	Graft []GossipGraft
	// Prune 移出 mesh
	Prune []GossipPrune
}

// GossipIHave IHAVE 消息 - 通知 peer 本节点拥有的消息
type GossipIHave struct {
	// Topic 主题
	Topic string
	// MessageIDs 消息 ID 列表
	MessageIDs [][]byte
}

// GossipIWant IWANT 消息 - 请求 peer 发送指定消息
type GossipIWant struct {
	// MessageIDs 请求的消息 ID 列表
	MessageIDs [][]byte
}

// GossipGraft GRAFT 消息 - 请求加入 mesh
type GossipGraft struct {
	// Topic 主题
	Topic string
}

// GossipPrune PRUNE 消息 - 从 mesh 移除
type GossipPrune struct {
	// Topic 主题
	Topic string
	// Peers 推荐的其他 peers（PX - Peer Exchange）
	Peers []GossipPeerInfo
	// Backoff 退避时间（秒）
	Backoff uint64
}

// GossipPeerInfo Peer 信息（用于 PX）
type GossipPeerInfo struct {
	// ID 节点 ID
	ID NodeID
	// SignedPeerRecord 签名的 peer 记录（可选）
	SignedPeerRecord []byte
}

// ============================================================================
//                              GossipSub Peer 状态
// ============================================================================

// GossipPeerState Peer 在 GossipSub 中的状态
type GossipPeerState struct {
	// ID 节点 ID
	ID NodeID

	// Topics peer 订阅的主题集合
	Topics map[string]struct{}

	// Connected 是否已连接
	Connected bool

	// Outbound 是否是出站连接
	Outbound bool

	// FirstSeen 首次发现时间
	FirstSeen time.Time

	// LastSeen 最后活跃时间
	LastSeen time.Time

	// Score 节点评分
	Score float64

	// Behaviours 行为计数器（用于评分）
	Behaviours *GossipPeerBehaviours
}

// GossipPeerBehaviours Peer 行为统计
type GossipPeerBehaviours struct {
	// FirstMessageDeliveries 首次消息投递数（正向）
	FirstMessageDeliveries map[string]float64

	// MeshMessageDeliveries 网格消息投递数
	MeshMessageDeliveries map[string]float64

	// MeshTime mesh 中的累计时间
	MeshTime map[string]time.Duration

	// MeshFailurePenalty mesh 失败惩罚
	MeshFailurePenalty map[string]float64

	// InvalidMessages 无效消息数
	InvalidMessages map[string]float64

	// DuplicateMessages 重复消息数
	DuplicateMessages int

	// BrokenPromises 未履行的 IWANT 请求
	BrokenPromises int

	// IPColocationFactor IP 协同因子
	IPColocationFactor float64
}

// NewGossipPeerBehaviours 创建新的 Peer 行为统计
func NewGossipPeerBehaviours() *GossipPeerBehaviours {
	return &GossipPeerBehaviours{
		FirstMessageDeliveries: make(map[string]float64),
		MeshMessageDeliveries:  make(map[string]float64),
		MeshTime:               make(map[string]time.Duration),
		MeshFailurePenalty:     make(map[string]float64),
		InvalidMessages:        make(map[string]float64),
	}
}

// ============================================================================
//                              GossipSub Topic 状态
// ============================================================================

// GossipTopicState 主题状态
type GossipTopicState struct {
	// Topic 主题名称
	Topic string

	// Mesh mesh 中的 peers
	Mesh map[NodeID]struct{}

	// Fanout 非订阅主题的发布目标
	Fanout map[NodeID]struct{}

	// FanoutLastPub fanout 最后发布时间
	FanoutLastPub time.Time

	// Peers 所有订阅该主题的 peers
	Peers map[NodeID]struct{}

	// Subscribed 本节点是否订阅该主题
	Subscribed bool

	// MessageCount 消息计数（统计用）
	MessageCount uint64
}

// NewGossipTopicState 创建新的主题状态
func NewGossipTopicState(topic string) *GossipTopicState {
	return &GossipTopicState{
		Topic:  topic,
		Mesh:   make(map[NodeID]struct{}),
		Fanout: make(map[NodeID]struct{}),
		Peers:  make(map[NodeID]struct{}),
	}
}

// ============================================================================
//                              GossipSub 缓存条目
// ============================================================================

// GossipCacheEntry 消息缓存条目
type GossipCacheEntry struct {
	// Message 消息
	Message *Message

	// ReceivedFrom 接收来源
	ReceivedFrom NodeID

	// ReceivedAt 接收时间
	ReceivedAt time.Time

	// Validated 是否已验证
	Validated bool

	// Valid 验证结果
	Valid bool
}

// ============================================================================
//                              GossipSub 事件类型
// ============================================================================

// GossipEventType 事件类型
type GossipEventType int

const (
	// GossipEventPeerJoined peer 加入主题
	GossipEventPeerJoined GossipEventType = iota
	// GossipEventPeerLeft peer 离开主题
	GossipEventPeerLeft
	// GossipEventGrafted peer 加入 mesh
	GossipEventGrafted
	// GossipEventPruned peer 离开 mesh
	GossipEventPruned
	// GossipEventMessageReceived 收到消息
	GossipEventMessageReceived
	// GossipEventMessagePublished 发布消息
	GossipEventMessagePublished
)

// String 返回事件类型的字符串表示
func (t GossipEventType) String() string {
	switch t {
	case GossipEventPeerJoined:
		return "PEER_JOINED"
	case GossipEventPeerLeft:
		return "PEER_LEFT"
	case GossipEventGrafted:
		return "GRAFTED"
	case GossipEventPruned:
		return "PRUNED"
	case GossipEventMessageReceived:
		return "MESSAGE_RECEIVED"
	case GossipEventMessagePublished:
		return "MESSAGE_PUBLISHED"
	default:
		return "UNKNOWN"
	}
}

// GossipEvent GossipSub 事件
type GossipEvent struct {
	// Type 事件类型
	Type GossipEventType
	// Topic 主题
	Topic string
	// Peer 相关 peer
	Peer NodeID
	// Message 相关消息（如果有）
	Message *Message
	// Timestamp 事件时间
	Timestamp time.Time
}

// ============================================================================
//                              GossipSub 统计信息
// ============================================================================

// GossipStats GossipSub 统计信息
type GossipStats struct {
	// TopicStats 各主题统计
	TopicStats map[string]*GossipTopicStats

	// TotalMessagesReceived 总接收消息数
	TotalMessagesReceived uint64

	// TotalMessagesPublished 总发布消息数
	TotalMessagesPublished uint64

	// TotalDuplicates 总重复消息数
	TotalDuplicates uint64

	// TotalPeers 总 peer 数
	TotalPeers int

	// MeshPeers 各主题 mesh peer 数
	MeshPeers map[string]int
}

// GossipTopicStats 主题统计
type GossipTopicStats struct {
	// Topic 主题
	Topic string

	// MessagesReceived 接收消息数
	MessagesReceived uint64

	// MessagesPublished 发布消息数
	MessagesPublished uint64

	// MeshPeerCount mesh peer 数
	MeshPeerCount int

	// PeerCount 订阅 peer 数
	PeerCount int
}

// NewGossipStats 创建新的 GossipSub 统计信息
func NewGossipStats() *GossipStats {
	return &GossipStats{
		TopicStats: make(map[string]*GossipTopicStats),
		MeshPeers:  make(map[string]int),
	}
}

