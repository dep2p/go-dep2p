// Package config 定义 Realm 配置常量
//
// 本文件集中定义快速断开检测相关的所有配置常量。
// 修改这些常量可以调整断开检测的行为。
package config

import "time"

// ============================================================================
//                              传输层配置（Layer 1）
// ============================================================================

const (
	// QUICKeepAlivePeriod QUIC Keep-Alive 周期
	// 每隔此时间发送 Keep-Alive 帧以维持连接
	QUICKeepAlivePeriod = 3 * time.Second

	// QUICMaxIdleTimeout QUIC 最大空闲超时
	// 超过此时间未收到任何数据包则认为连接断开
	QUICMaxIdleTimeout = 6 * time.Second
)

// ============================================================================
//                              MemberLeave 配置（Layer 2）
// ============================================================================

const (
	// MemberLeaveWaitBeforeClose 发送 MemberLeave 后等待时间
	// 确保消息被传播后再关闭连接
	MemberLeaveWaitBeforeClose = 50 * time.Millisecond

	// MemberLeaveTimestampValidity MemberLeave 时间戳有效期
	// 超过此时间的消息会被拒绝
	MemberLeaveTimestampValidity = 30 * time.Second
)

// ============================================================================
//                              见证人配置（Layer 3）
// ============================================================================

const (
	// WitnessMaxBroadcastDelay 最大广播延迟
	// 随机延迟 0-500ms，避免多个节点同时广播
	WitnessMaxBroadcastDelay = 500 * time.Millisecond

	// WitnessConfirmationTimeout 确认超时时间
	// 超过此时间未收到足够确认则根据已有投票决定
	WitnessConfirmationTimeout = 2 * time.Second

	// WitnessFastPathMemberThreshold 快速路径成员数阈值
	// 成员数小于此值时可使用快速路径（单票 AGREE 无反对即确认）
	WitnessFastPathMemberThreshold = 10

	// WitnessReportExpiry 见证报告过期时间
	// 超过此时间的报告会被忽略
	WitnessReportExpiry = 10 * time.Second

	// WitnessRateLimitPerMinute 每分钟最大报告数
	// 每个目标节点每分钟最多触发此数量的见证报告
	WitnessRateLimitPerMinute = 10
)

// ============================================================================
//                              Liveness 兜底配置（Layer 4）
// ============================================================================

const (
	// LivenessPingInterval Ping 检查间隔
	// 每隔此时间对所有连接执行健康检查
	LivenessPingInterval = 30 * time.Second

	// LivenessPingTimeout 单次 Ping 超时
	LivenessPingTimeout = 5 * time.Second

	// LivenessMaxFailures 最大连续失败次数
	// 连续失败此次数后认为节点断开
	LivenessMaxFailures = 3
)

// ============================================================================
//                              防误判配置（Layer 5）
// ============================================================================

const (
	// ReconnectGracePeriod 重连宽限期
	// 断开后在此时间内重连不会触发成员移除
	ReconnectGracePeriod = 15 * time.Second

	// MaxGracePeriodExtensions 最大宽限期延长次数
	// 在宽限期内收到通信可延长宽限期
	MaxGracePeriodExtensions = 2

	// DisconnectProtection 断开保护期
	// 成员被移除后在此时间内拒绝重新添加
	DisconnectProtection = 30 * time.Second

	// FlapWindowDuration 震荡检测时间窗口
	FlapWindowDuration = 60 * time.Second

	// FlapThreshold 震荡阈值
	// 在时间窗口内超过此次数的状态转换被视为震荡
	FlapThreshold = 3

	// FlappingRecovery 震荡恢复时间
	// 被标记为震荡后需稳定此时间才能解除
	FlappingRecovery = 5 * time.Minute
)

// ============================================================================
//                              Relay 见证配置
// ============================================================================

const (
	// RelayBatchAnomalyThreshold Relay 批量异常阈值
	// 在时间窗口内超过此数量的断开报告触发异常抑制
	RelayBatchAnomalyThreshold = 5

	// RelayBatchAnomalyWindow Relay 批量异常检测时间窗口
	RelayBatchAnomalyWindow = 60 * time.Second
)

// ============================================================================
//                              检测延迟目标（参考）
// ============================================================================

// 这些是设计目标，不是配置常量：
//   - 优雅断开检测延迟: < 100ms
//   - 直连非优雅断开检测: < 10s
//   - 中继非优雅断开检测: < 15s
//   - 误判率（分区场景）: < 5%
