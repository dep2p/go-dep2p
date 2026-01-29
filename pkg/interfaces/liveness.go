// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Liveness 接口，提供存活检测服务。
package interfaces

import (
	"context"
	"time"
)

// Liveness 定义存活检测服务接口
//
// Liveness 监控节点的存活状态和网络健康。
type Liveness interface {
	// Ping 发送 ping 并测量 RTT
	Ping(ctx context.Context, peerID string) (time.Duration, error)

	// Check 检查节点是否存活
	Check(ctx context.Context, peerID string) (bool, error)

	// Watch 监控节点状态变化
	Watch(peerID string) (<-chan LivenessEvent, error)

	// Unwatch 停止监控节点
	Unwatch(peerID string) error

	// GetStatus 获取节点存活状态
	GetStatus(peerID string) LivenessStatus

	// Start 启动存活检测服务
	Start(ctx context.Context) error

	// Stop 停止存活检测服务
	Stop(ctx context.Context) error
}

// LivenessEvent 存活事件
type LivenessEvent struct {
	// PeerID 节点 ID
	PeerID string

	// Type 事件类型
	Type LivenessEventType

	// Status 当前状态
	Status LivenessStatus

	// Timestamp 事件时间
	Timestamp time.Time

	// RTT 往返时间（仅在 Pong 事件时有效）
	RTT time.Duration
}

// LivenessEventType 存活事件类型
type LivenessEventType int

const (
	// LivenessEventPong 收到 pong 响应
	LivenessEventPong LivenessEventType = iota
	// LivenessEventTimeout 超时
	LivenessEventTimeout
	// LivenessEventUp 节点上线
	LivenessEventUp
	// LivenessEventDown 节点下线
	LivenessEventDown
)

// LivenessStatus 存活状态
type LivenessStatus struct {
	// Alive 是否存活
	Alive bool

	// LastSeen 最后一次确认存活的时间
	LastSeen time.Time

	// LastRTT 最后一次 RTT
	LastRTT time.Duration

	// AvgRTT 平均 RTT（基于滑动窗口）
	AvgRTT time.Duration

	// MinRTT 最小 RTT（历史最优）
	MinRTT time.Duration

	// MaxRTT 最大 RTT（历史最差）
	MaxRTT time.Duration

	// FailCount 连续失败次数
	FailCount int

	// TotalPings 总 Ping 次数
	TotalPings int

	// SuccessCount 成功次数
	SuccessCount int

	// SuccessRate 成功率 (0.0 - 1.0)
	SuccessRate float64
}

// LivenessConfig 存活检测配置
type LivenessConfig struct {
	// Interval 检测间隔
	Interval time.Duration

	// Timeout 单次检测超时
	Timeout time.Duration

	// FailThreshold 判定下线的失败阈值
	FailThreshold int
}

// DefaultLivenessConfig 返回默认配置
func DefaultLivenessConfig() *LivenessConfig {
	return &LivenessConfig{
		Interval:      30 * time.Second,
		Timeout:       5 * time.Second,
		FailThreshold: 3,
	}
}
