// Package netmon 提供网络状态监控功能
//
// IMPL-NETWORK-RESILIENCE: 系统事件监听
package netmon

import (
	"context"
	"time"
)

// ============================================================================
//                              SystemWatcher 接口
// ============================================================================

// SystemWatcher 系统网络变化监听器接口
//
// IMPL-NETWORK-RESILIENCE: 监听操作系统级网络接口变化
// Phase 5.2: 支持 NetworkMonitor 检测系统网络变化
type SystemWatcher interface {
	// Start 启动监听
	Start(ctx context.Context) error

	// Stop 停止监听
	Stop() error

	// Events 返回事件通道
	// 当检测到网络变化时，会向此通道发送事件
	Events() <-chan NetworkEvent

	// IsRunning 检查是否正在运行
	IsRunning() bool
}

// ============================================================================
//                              网络事件
// ============================================================================

// NetworkEvent 网络变化事件
// IMPL-NETWORK-RESILIENCE: 描述网络接口变化
type NetworkEvent struct {
	// Type 事件类型
	Type NetworkEventType

	// Interface 接口名称（如 "en0", "eth0"）
	Interface string

	// Address 相关地址（可选）
	Address string

	// Timestamp 事件时间
	Timestamp time.Time

	// Details 额外详情
	Details map[string]string
}

// NetworkEventType 网络事件类型
type NetworkEventType int

const (
	// EventNetworkChanged 通用网络变化事件
	// 当无法确定具体类型时使用
	EventNetworkChanged NetworkEventType = iota

	// EventInterfaceUp 接口启用
	EventInterfaceUp

	// EventInterfaceDown 接口禁用
	EventInterfaceDown

	// EventAddressAdded 地址添加
	EventAddressAdded

	// EventAddressRemoved 地址移除
	EventAddressRemoved

	// EventRouteChanged 路由变化
	EventRouteChanged

	// EventGatewayChanged 网关变化
	EventGatewayChanged
)

// String 返回事件类型字符串
func (t NetworkEventType) String() string {
	switch t {
	case EventNetworkChanged:
		return "network_changed"
	case EventInterfaceUp:
		return "interface_up"
	case EventInterfaceDown:
		return "interface_down"
	case EventAddressAdded:
		return "address_added"
	case EventAddressRemoved:
		return "address_removed"
	case EventRouteChanged:
		return "route_changed"
	case EventGatewayChanged:
		return "gateway_changed"
	default:
		return "unknown"
	}
}

// IsMajorChange 检查是否是重大变化
// IMPL-NETWORK-RESILIENCE: 重大变化应触发 rebind
func (t NetworkEventType) IsMajorChange() bool {
	switch t {
	case EventInterfaceDown, EventGatewayChanged, EventNetworkChanged:
		return true
	default:
		return false
	}
}

// ============================================================================
//                              NoOpWatcher
// ============================================================================

// NoOpWatcher 空操作监听器
// IMPL-NETWORK-RESILIENCE: 当系统不支持或禁用监听时使用
type NoOpWatcher struct {
	events chan NetworkEvent
}

// NewNoOpWatcher 创建空操作监听器
func NewNoOpWatcher() *NoOpWatcher {
	return &NoOpWatcher{
		events: make(chan NetworkEvent),
	}
}

// Start 启动（空操作）
func (w *NoOpWatcher) Start(_ context.Context) error {
	return nil
}

// Stop 停止（空操作）
func (w *NoOpWatcher) Stop() error {
	return nil
}

// Events 返回事件通道（永远不会有事件）
func (w *NoOpWatcher) Events() <-chan NetworkEvent {
	return w.events
}

// IsRunning 检查是否运行
func (w *NoOpWatcher) IsRunning() bool {
	return false
}

// ============================================================================
//                              WatcherConfig
// ============================================================================

// WatcherConfig 监听器配置
type WatcherConfig struct {
	// Enabled 是否启用系统监听
	// 默认: true
	Enabled bool

	// PollInterval 轮询间隔
	// 默认: 5s
	PollInterval time.Duration

	// EventBufferSize 事件缓冲区大小
	// 默认: 16
	EventBufferSize int
}

// DefaultWatcherConfig 返回默认配置
func DefaultWatcherConfig() *WatcherConfig {
	return &WatcherConfig{
		Enabled:         true,
		PollInterval:    5 * time.Second,
		EventBufferSize: 16,
	}
}

// Validate 验证配置
func (c *WatcherConfig) Validate() error {
	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Second
	}
	if c.EventBufferSize <= 0 {
		c.EventBufferSize = 16
	}
	return nil
}
