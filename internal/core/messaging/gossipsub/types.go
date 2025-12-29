// Package gossipsub 实现 GossipSub v1.1 协议
//
// GossipSub 是一种高效的发布订阅协议，通过维护 mesh 网络和 gossip 机制
// 实现 O(log N) 的消息传播复杂度。
//
// 本包使用 pkg/types 中的统一类型定义，包括：
//   - types.Message: 发布订阅消息
//   - types.GossipRPC: GossipSub RPC 消息
//   - types.GossipControl: 控制消息
//   - types.GossipPeerState: Peer 状态
//   - types.GossipTopicState: Topic 状态
//   - types.GossipStats: 统计信息
package gossipsub

import (
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              类型别名 - 使用统一类型
// ============================================================================

// ControlMessageType 控制消息类型（类型别名）
type ControlMessageType = types.GossipControlType

// 控制消息类型常量
const (
	ControlIHave = types.GossipControlIHave
	ControlIWant = types.GossipControlIWant
	ControlGraft = types.GossipControlGraft
	ControlPrune = types.GossipControlPrune
)

// RPC GossipSub RPC 消息（类型别名）
type RPC = types.GossipRPC

// SubOpt 订阅选项（类型别名）
type SubOpt = types.GossipSubOpt

// Message GossipSub 消息（类型别名）
type Message = types.Message

// ControlMessage 控制消息（类型别名）
type ControlMessage = types.GossipControl

// ControlIHaveMessage IHAVE 消息（类型别名）
type ControlIHaveMessage = types.GossipIHave

// ControlIWantMessage IWANT 消息（类型别名）
type ControlIWantMessage = types.GossipIWant

// ControlGraftMessage GRAFT 消息（类型别名）
type ControlGraftMessage = types.GossipGraft

// ControlPruneMessage PRUNE 消息（类型别名）
type ControlPruneMessage = types.GossipPrune

// PeerInfo Peer 信息（类型别名）
type PeerInfo = types.GossipPeerInfo

// PeerState Peer 状态（类型别名）
type PeerState = types.GossipPeerState

// PeerBehaviours Peer 行为统计（类型别名）
type PeerBehaviours = types.GossipPeerBehaviours

// NewPeerBehaviours 创建新的 Peer 行为统计
var NewPeerBehaviours = types.NewGossipPeerBehaviours

// TopicState 主题状态（类型别名）
type TopicState = types.GossipTopicState

// NewTopicState 创建新的主题状态
var NewTopicState = types.NewGossipTopicState

// CacheEntry 消息缓存条目（类型别名）
type CacheEntry = types.GossipCacheEntry

// EventType 事件类型（类型别名）
type EventType = types.GossipEventType

// 事件类型常量
const (
	EventPeerJoined       = types.GossipEventPeerJoined
	EventPeerLeft         = types.GossipEventPeerLeft
	EventGrafted          = types.GossipEventGrafted
	EventPruned           = types.GossipEventPruned
	EventMessageReceived  = types.GossipEventMessageReceived
	EventMessagePublished = types.GossipEventMessagePublished
)

// Event GossipSub 事件（类型别名）
type Event = types.GossipEvent

// Stats GossipSub 统计信息（类型别名）
type Stats = types.GossipStats

// TopicStats 主题统计（类型别名）
type TopicStats = types.GossipTopicStats

// NewStats 创建新的 GossipSub 统计信息
var NewStats = types.NewGossipStats
