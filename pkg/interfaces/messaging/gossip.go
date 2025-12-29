// Package messaging 定义消息传递相关接口
package messaging

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              GossipRouter 接口
// ============================================================================

// GossipRouter GossipSub 路由器接口
//
// 提供 GossipSub v1.1 协议的核心功能，包括：
// - 消息发布和订阅
// - Mesh 网络管理
// - Peer 评分
// - 控制消息处理
type GossipRouter interface {
	// ==================== 发布订阅 ====================

	// Publish 发布消息到指定主题
	//
	// 消息通过 mesh 网络传播到所有订阅该主题的节点。
	Publish(ctx context.Context, topic string, data []byte) error

	// Subscribe 订阅指定主题
	//
	// 订阅后将接收该主题上的所有消息。
	// 返回订阅句柄用于接收消息和取消订阅。
	Subscribe(topic string) (GossipSubscription, error)

	// Unsubscribe 取消订阅指定主题
	Unsubscribe(topic string) error

	// ==================== Peer 管理 ====================

	// AddPeer 添加 peer 到路由器
	//
	// 当与新 peer 建立连接时调用。
	AddPeer(peerID types.NodeID, protocols []string)

	// RemovePeer 从路由器移除 peer
	//
	// 当与 peer 断开连接时调用。
	RemovePeer(peerID types.NodeID)

	// PeerScore 获取 peer 的当前评分
	PeerScore(peerID types.NodeID) float64

	// ==================== 统计信息 ====================

	// Stats 获取统计信息
	Stats() *types.GossipStats

	// Topics 获取所有订阅的主题
	Topics() []string

	// MeshPeers 获取指定主题的 mesh peers
	MeshPeers(topic string) []types.NodeID

	// ==================== 生命周期 ====================

	// Start 启动路由器
	Start(ctx context.Context) error

	// Stop 停止路由器
	Stop() error
}

// GossipSubscription GossipSub 订阅句柄
type GossipSubscription interface {
	// Topic 返回订阅的主题
	Topic() string

	// Next 获取下一条消息（阻塞）
	Next(ctx context.Context) (*types.Message, error)

	// Cancel 取消订阅
	Cancel()
}

// ============================================================================
//                              MeshManager（v1.1 已删除）
// ============================================================================

// 注意：MeshManager 接口已删除（v1.1 清理）。
// 原因：仅内部使用，internal/core/messaging/gossipsub.MeshManager 结构体提供实现。
// 公共 API 不暴露 Mesh 管理细节。

// ============================================================================
//                              PeerScorer（v1.1 已删除）
// ============================================================================

// 注意：PeerScorer 接口已删除（v1.1 清理）。
// 原因：仅内部使用，internal/core/messaging/gossipsub.PeerScorer 结构体提供实现。
// 公共 API 不暴露评分系统细节。

// ============================================================================
//                              MessageValidator（v1.1 已删除）
// ============================================================================

// 注意：MessageValidator 接口及 ValidationResult 类型已删除（v1.1 清理）。
// 原因：无外部实现/使用，验证逻辑已内置于 Router/MembershipCache。
// 如需自定义验证，请通过 internal/core/messaging/gossipsub 内部扩展。

// ============================================================================
//                              GossipSub 事件
// ============================================================================

// GossipSub 特定事件类型
const (
	// EventGraftReceived 收到 GRAFT 请求
	EventGraftReceived = "gossipsub.graft_received"

	// EventGraftSent 发送 GRAFT 请求
	EventGraftSent = "gossipsub.graft_sent"

	// EventPruneReceived 收到 PRUNE 通知
	EventPruneReceived = "gossipsub.prune_received"

	// EventPruneSent 发送 PRUNE 通知
	EventPruneSent = "gossipsub.prune_sent"

	// EventMeshUpdated mesh 网络更新
	EventMeshUpdated = "gossipsub.mesh_updated"

	// EventPeerScoreUpdated peer 评分更新
	EventPeerScoreUpdated = "gossipsub.peer_score_updated"
)

// GraftEvent GRAFT 事件
type GraftEvent struct {
	Topic     string
	Peer      types.NodeID
	Received  bool // true=收到, false=发送
	Timestamp int64
}

// Type 返回事件类型
func (e GraftEvent) Type() string {
	if e.Received {
		return EventGraftReceived
	}
	return EventGraftSent
}

// PruneEvent PRUNE 事件
type PruneEvent struct {
	Topic     string
	Peer      types.NodeID
	Received  bool // true=收到, false=发送
	Backoff   int64
	Timestamp int64
}

// Type 返回事件类型
func (e PruneEvent) Type() string {
	if e.Received {
		return EventPruneReceived
	}
	return EventPruneSent
}

// MeshUpdatedEvent mesh 更新事件
type MeshUpdatedEvent struct {
	Topic     string
	Added     []types.NodeID
	Removed   []types.NodeID
	MeshSize  int
	Timestamp int64
}

// Type 返回事件类型
func (e MeshUpdatedEvent) Type() string {
	return EventMeshUpdated
}

// PeerScoreUpdatedEvent 评分更新事件
type PeerScoreUpdatedEvent struct {
	Peer      types.NodeID
	OldScore  float64
	NewScore  float64
	Timestamp int64
}

// Type 返回事件类型
func (e PeerScoreUpdatedEvent) Type() string {
	return EventPeerScoreUpdated
}
