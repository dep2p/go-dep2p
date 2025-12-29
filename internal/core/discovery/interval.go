// Package discovery 提供节点发现模块的实现
package discovery

import (
	"sync"
	"time"
)

// ============================================================================
//                              动态发现间隔
// ============================================================================

// IntervalConfig 发现间隔配置
type IntervalConfig struct {
	// BaseInterval 基础间隔（默认 30s）
	BaseInterval time.Duration

	// MinInterval 最小间隔（默认 5s）
	MinInterval time.Duration

	// MaxInterval 最大间隔（默认 5min）
	MaxInterval time.Duration

	// TargetPeerCount 目标节点数（默认 50）
	TargetPeerCount int
}

// DefaultIntervalConfig 默认发现间隔配置
func DefaultIntervalConfig() IntervalConfig {
	return IntervalConfig{
		BaseInterval:    30 * time.Second,
		MinInterval:     5 * time.Second,
		MaxInterval:     5 * time.Minute,
		TargetPeerCount: 50,
	}
}

// DynamicInterval 动态发现间隔计算器
type DynamicInterval struct {
	config IntervalConfig

	// 状态
	currentInterval time.Duration
	lastCheck       time.Time
	peerHistory     []int // 历史节点数记录

	// 紧急恢复
	emergency *EmergencyRecovery

	mu sync.RWMutex
}

// NewDynamicInterval 创建动态间隔计算器
func NewDynamicInterval(config IntervalConfig) *DynamicInterval {
	return &DynamicInterval{
		config:          config,
		currentInterval: config.BaseInterval,
		peerHistory:     make([]int, 0, 10),
		emergency:       NewEmergencyRecovery(),
	}
}

// Calculate 计算发现间隔
//
// 根据当前节点数与目标节点数的比例调整间隔：
//   - ratio < 0.3: 紧急模式，使用最小间隔
//   - ratio < 0.5: 加速模式，使用基础间隔的一半
//   - ratio > 0.9: 减速模式，使用基础间隔的两倍
//   - 其他: 使用基础间隔
func (d *DynamicInterval) Calculate(currentPeers int) time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 记录历史
	d.recordPeerCount(currentPeers)

	// 检查紧急恢复
	if d.emergency.Check(currentPeers, d.peerHistory) {
		d.currentInterval = d.config.MinInterval
		return d.currentInterval
	}

	// 计算比例
	if d.config.TargetPeerCount == 0 {
		d.currentInterval = d.config.BaseInterval
		return d.currentInterval
	}

	ratio := float64(currentPeers) / float64(d.config.TargetPeerCount)

	var interval time.Duration
	switch {
	case ratio < 0.3:
		// 紧急模式：连接数严重不足
		interval = d.config.MinInterval
	case ratio < 0.5:
		// 加速模式
		interval = d.config.BaseInterval / 2
	case ratio > 0.9:
		// 减速模式：已接近目标
		interval = d.config.BaseInterval * 2
	default:
		interval = d.config.BaseInterval
	}

	// 约束在允许范围内
	if interval < d.config.MinInterval {
		interval = d.config.MinInterval
	}
	if interval > d.config.MaxInterval {
		interval = d.config.MaxInterval
	}

	d.currentInterval = interval
	d.lastCheck = time.Now()

	return interval
}

// CurrentInterval 获取当前间隔
func (d *DynamicInterval) CurrentInterval() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentInterval
}

// recordPeerCount 记录节点数历史
func (d *DynamicInterval) recordPeerCount(count int) {
	d.peerHistory = append(d.peerHistory, count)

	// 保留最近 10 条记录
	if len(d.peerHistory) > 10 {
		d.peerHistory = d.peerHistory[1:]
	}
}

// IsEmergency 是否处于紧急恢复模式
func (d *DynamicInterval) IsEmergency() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.emergency.InRecoveryMode()
}

// ============================================================================
//                              紧急恢复机制
// ============================================================================

// EmergencyRecovery 紧急恢复机制
//
// 触发条件:
//  1. 连接数在 5 分钟内下降超过 50%
//  2. 连续 3 次发现失败
//  3. 检测到网络分区
//  4. 核心邻居数量 < 3
type EmergencyRecovery struct {
	// 状态
	triggered      bool
	triggerTime    time.Time
	triggerReason  EmergencyReason
	originalPeers  int
	recoveryMode   bool
	failureCount   int

	mu sync.RWMutex
}

// EmergencyReason 紧急恢复触发原因
type EmergencyReason int

const (
	// EmergencyReasonNone 无紧急情况
	EmergencyReasonNone EmergencyReason = iota

	// EmergencyReasonPeerDrop 节点数骤降
	EmergencyReasonPeerDrop

	// EmergencyReasonDiscoveryFailed 发现连续失败
	EmergencyReasonDiscoveryFailed

	// EmergencyReasonPartition 网络分区
	EmergencyReasonPartition

	// EmergencyReasonLowNeighbors 核心邻居不足
	EmergencyReasonLowNeighbors
)

// String 返回原因的字符串表示
func (r EmergencyReason) String() string {
	switch r {
	case EmergencyReasonNone:
		return "none"
	case EmergencyReasonPeerDrop:
		return "peer_drop"
	case EmergencyReasonDiscoveryFailed:
		return "discovery_failed"
	case EmergencyReasonPartition:
		return "partition"
	case EmergencyReasonLowNeighbors:
		return "low_neighbors"
	default:
		return "unknown"
	}
}

// NewEmergencyRecovery 创建紧急恢复机制
func NewEmergencyRecovery() *EmergencyRecovery {
	return &EmergencyRecovery{}
}

// Check 检查是否需要触发紧急恢复
func (e *EmergencyRecovery) Check(currentPeers int, history []int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 如果已经在恢复模式，检查是否可以退出
	if e.recoveryMode {
		return e.checkRecoveryComplete(currentPeers)
	}

	// 检测连接数骤降
	if e.checkPeerDrop(currentPeers, history) {
		e.trigger(EmergencyReasonPeerDrop, currentPeers)
		return true
	}

	// 检测核心邻居不足
	if currentPeers < 3 {
		e.trigger(EmergencyReasonLowNeighbors, currentPeers)
		return true
	}

	return false
}

// checkPeerDrop 检测连接数骤降
func (e *EmergencyRecovery) checkPeerDrop(currentPeers int, history []int) bool {
	if len(history) == 0 {
		return false
	}

	// 获取 5 分钟前的节点数（假设每次检查间隔 30 秒，10 条记录约 5 分钟）
	oldPeers := history[0]
	if oldPeers == 0 {
		return false
	}

	// 如果下降超过 50%
	ratio := float64(currentPeers) / float64(oldPeers)
	return ratio < 0.5
}

// trigger 触发紧急恢复
func (e *EmergencyRecovery) trigger(reason EmergencyReason, currentPeers int) {
	e.triggered = true
	e.triggerTime = time.Now()
	e.triggerReason = reason
	e.originalPeers = currentPeers
	e.recoveryMode = true
}

// checkRecoveryComplete 检查恢复是否完成
func (e *EmergencyRecovery) checkRecoveryComplete(currentPeers int) bool {
	// 恢复条件：
	// 1. 节点数恢复到触发前的水平
	// 2. 或者已经超过 10 分钟（超时）

	if currentPeers >= e.originalPeers*2 || currentPeers >= 10 {
		e.reset()
		return false
	}

	if time.Since(e.triggerTime) > 10*time.Minute {
		e.reset()
		return false
	}

	return true // 继续恢复模式
}

// reset 重置紧急恢复状态
func (e *EmergencyRecovery) reset() {
	e.triggered = false
	e.triggerTime = time.Time{}
	e.triggerReason = EmergencyReasonNone
	e.originalPeers = 0
	e.recoveryMode = false
	e.failureCount = 0
}

// InRecoveryMode 是否处于恢复模式
func (e *EmergencyRecovery) InRecoveryMode() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.recoveryMode
}

// RecordDiscoveryFailure 记录发现失败
func (e *EmergencyRecovery) RecordDiscoveryFailure() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.failureCount++

	// 连续 3 次失败触发紧急恢复
	if e.failureCount >= 3 && !e.recoveryMode {
		e.trigger(EmergencyReasonDiscoveryFailed, 0)
	}
}

// RecordDiscoverySuccess 记录发现成功
func (e *EmergencyRecovery) RecordDiscoverySuccess() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.failureCount = 0
}

// TriggerReason 获取触发原因
func (e *EmergencyRecovery) TriggerReason() EmergencyReason {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.triggerReason
}

// ============================================================================
//                              恢复动作
// ============================================================================

// RecoveryActions 紧急恢复时应执行的动作
type RecoveryActions struct {
	// 发现间隔设为最小值
	UseMinInterval bool

	// 并行连接多个 Bootstrap 节点
	ConnectBootstrap bool

	// 主动发起 DHT 查找
	AggressiveDHTLookup bool

	// 持续直到连接数恢复到 LowWater
	ContinueUntilLowWater bool
}

// GetRecoveryActions 获取恢复动作
func (e *EmergencyRecovery) GetRecoveryActions() RecoveryActions {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.recoveryMode {
		return RecoveryActions{}
	}

	return RecoveryActions{
		UseMinInterval:        true,
		ConnectBootstrap:      true,
		AggressiveDHTLookup:   true,
		ContinueUntilLowWater: true,
	}
}

