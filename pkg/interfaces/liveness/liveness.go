// Package liveness 定义节点存活检测接口
//
// Liveness 是 dep2p 的节点存活检测和网络健康维护服务：
// - 节点状态检测（Online/Degraded/Offline/Unknown）
// - 心跳机制
// - 优雅下线（Goodbye 协议）
// - 健康评分
package liveness

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议 ID
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolPing Ping 协议 ID (v1.1 scope: sys)
	ProtocolPing = protocolids.SysPing

	// ProtocolGoodbye Goodbye 协议 ID (v1.1 scope: sys)
	ProtocolGoodbye = protocolids.SysGoodbye

	// ProtocolHeartbeat Heartbeat 协议 ID (v1.1 scope: sys)
	ProtocolHeartbeat = protocolids.SysHeartbeat
)

// ============================================================================
//                              LivenessService 接口
// ============================================================================

// LivenessService 节点存活检测服务接口
//
// 负责检测节点状态、维持心跳和处理优雅下线。
type LivenessService interface {
	// ============================
	// 状态查询
	// ============================

	// PeerStatus 获取节点当前状态
	PeerStatus(nodeID types.NodeID) types.PeerStatus

	// PeerHealth 获取节点健康信息
	PeerHealth(nodeID types.NodeID) *types.PeerHealth

	// AllPeerStatuses 获取所有已知节点的状态
	AllPeerStatuses() map[types.NodeID]types.PeerStatus

	// OnlinePeers 获取所有在线节点
	OnlinePeers() []types.NodeID

	// ============================
	// 心跳检测
	// ============================

	// Ping 对指定节点进行 Ping 检测
	//
	// 返回 RTT（往返时间），如果超时返回错误。
	Ping(ctx context.Context, nodeID types.NodeID) (time.Duration, error)

	// StartHeartbeat 开始对指定节点的心跳检测
	StartHeartbeat(nodeID types.NodeID)

	// StopHeartbeat 停止对指定节点的心跳检测
	StopHeartbeat(nodeID types.NodeID)

	// ============================
	// Goodbye 协议
	// ============================

	// SendGoodbye 发送 Goodbye 消息（优雅下线）
	//
	// 会向所有已连接的邻居节点发送 Goodbye 消息。
	SendGoodbye(ctx context.Context, reason types.GoodbyeReason) error

	// SendGoodbyeTo 向指定节点发送 Goodbye 消息
	SendGoodbyeTo(ctx context.Context, nodeID types.NodeID, reason types.GoodbyeReason) error

	// ============================
	// 状态变更监听
	// ============================

	// OnStatusChange 注册状态变更回调
	//
	// 当节点状态发生变化时调用回调函数。
	OnStatusChange(callback StatusChangeCallback)

	// 注意：RemoveStatusChangeCallback 已删除（v1.1 清理）- 空实现。
	// Go 无法比较函数，无法正确移除回调。

	// ============================
	// 健康评分
	// ============================

	// HealthScore 获取节点健康评分（0-100）
	HealthScore(nodeID types.NodeID) int

	// SetHealthScoreDecay 设置健康评分衰减规则
	SetHealthScoreDecay(decay HealthScoreDecay)

	// ============================
	// 配置
	// ============================

	// SetThresholds 设置检测阈值
	SetThresholds(thresholds types.LivenessThresholds)

	// Thresholds 获取当前阈值
	Thresholds() types.LivenessThresholds

	// ============================
	// 生命周期
	// ============================

	// Start 启动存活检测服务
	Start(ctx context.Context) error

	// Stop 停止存活检测服务
	Stop() error
}

// ============================================================================
//                              回调类型
// ============================================================================

// StatusChangeCallback 状态变更回调函数类型
type StatusChangeCallback func(event types.PeerStatusChangeEvent)

// ============================================================================
//                              健康评分衰减
// ============================================================================

// HealthScoreDecay 健康评分衰减配置
type HealthScoreDecay struct {
	// DecayInterval 衰减间隔
	DecayInterval time.Duration

	// DecayAmount 每次衰减量
	DecayAmount int

	// MinScore 最低分数
	MinScore int

	// RecoveryOnPing Ping 成功时恢复的分数
	RecoveryOnPing int

	// RecoveryOnData 数据传输成功时恢复的分数
	RecoveryOnData int
}

// DefaultHealthScoreDecay 默认健康评分衰减配置
func DefaultHealthScoreDecay() HealthScoreDecay {
	return HealthScoreDecay{
		DecayInterval:  1 * time.Minute,
		DecayAmount:    5,
		MinScore:       0,
		RecoveryOnPing: 10,
		RecoveryOnData: 5,
	}
}

// ============================================================================
//                              PingResult
// ============================================================================

// PingResult Ping 结果
type PingResult struct {
	// NodeID 目标节点
	NodeID types.NodeID

	// RTT 往返时间
	RTT time.Duration

	// Success 是否成功
	Success bool

	// Error 错误信息（如果失败）
	Error error

	// Timestamp Ping 时间
	Timestamp time.Time
}

// ============================================================================
//                              HeartbeatStats
// ============================================================================

// HeartbeatStats 心跳统计
type HeartbeatStats struct {
	// NodeID 节点 ID
	NodeID types.NodeID

	// TotalPings 总 Ping 次数
	TotalPings int

	// SuccessfulPings 成功的 Ping 次数
	SuccessfulPings int

	// FailedPings 失败的 Ping 次数
	FailedPings int

	// LastPingTime 最后一次 Ping 时间
	LastPingTime time.Time

	// LastSuccessTime 最后一次成功时间
	LastSuccessTime time.Time

	// AvgRTT 平均 RTT
	AvgRTT time.Duration

	// MinRTT 最小 RTT
	MinRTT time.Duration

	// MaxRTT 最大 RTT
	MaxRTT time.Duration
}


