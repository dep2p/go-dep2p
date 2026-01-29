// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Relay 相关接口，属于 Core Layer。
//
// v2.0 统一 Relay 架构：
// - 单一 Relay 服务，不再区分 System/Realm Relay
// - 三大职责：缓存加速 + 打洞协调 + 数据保底
//
// 详见设计文档：
// - design/_discussions/20260123-nat-relay-concept-clarification.md §9.0
package interfaces

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// RelayDialer 接口 - 统一中继拨号接口
// ════════════════════════════════════════════════════════════════════════════

// RelayDialer 定义统一中继拨号接口
//
// 此接口用于 Swarm 的惰性中继回退。
// 中继是可选的兜底方案：
//   - 没有配置 Relay → 直连失败就返回失败
//   - 配置了 Relay → 直连失败后尝试中继回退
type RelayDialer interface {
	// DialViaRelay 通过 Relay 连接目标节点
	//
	// 此方法由 Swarm 在直连失败后调用。
	//
	// 参数：
	//   - ctx: 上下文，支持超时和取消
	//   - target: 目标节点 ID
	//
	// 返回：
	//   - Connection: 中继连接（对用户透明）
	//   - error: 中继失败时返回错误
	DialViaRelay(ctx context.Context, target string) (Connection, error)

	// HasRelay 检查是否配置了 Relay
	//
	// Swarm 在直连失败后调用此方法，决定是否尝试中继回退。
	HasRelay() bool
}

// ════════════════════════════════════════════════════════════════════════════
// RelayManager 接口 - 统一中继管理接口
// ════════════════════════════════════════════════════════════════════════════

// RelayManager 定义 Relay 管理器接口
//
// v2.0 统一 Relay 架构：单一 Relay 服务
type RelayManager interface {
	// EnableRelay 启用 Relay 能力
	//
	// 将当前节点设置为中继服务器，为 NAT 后的节点提供中继服务。
	// 前置条件：节点必须有公网可达地址。
	EnableRelay(ctx context.Context) error

	// DisableRelay 禁用 Relay 能力
	//
	// 停止作为中继服务。已建立的中继电路会被优雅关闭。
	DisableRelay(ctx context.Context) error

	// IsRelayEnabled 检查 Relay 能力是否已启用
	IsRelayEnabled() bool

	// RelayStats 返回 Relay 统计信息
	RelayStats() RelayStats

	// SetRelayAddr 设置要使用的 Relay 地址（客户端使用）
	//
	// 普通节点调用此方法设置要连接的 Relay 地址。
	SetRelayAddr(addr types.Multiaddr) error

	// RemoveRelayAddr 移除 Relay 地址配置
	RemoveRelayAddr() error

	// RelayAddr 获取当前配置的 Relay 地址
	RelayAddr() (types.Multiaddr, bool)

	// Close 关闭管理器
	Close() error
}

// ════════════════════════════════════════════════════════════════════════════
// Relay 统计信息
// ════════════════════════════════════════════════════════════════════════════

// RelayStats 统一 Relay 统计信息
type RelayStats struct {
	// Enabled 是否已启用 Relay 能力
	Enabled bool

	// RealmID 关联的 Realm ID（可选）
	RealmID string

	// ActiveCircuits 当前活跃的中继电路数
	ActiveCircuits int

	// TotalRelayed 累计中继数据量（字节）
	TotalRelayed uint64

	// ReservationCount 当前预约数
	ReservationCount int

	// PeakCircuits 峰值电路数
	PeakCircuits int

	// AddressBookSize 地址簿大小（如果启用）
	AddressBookSize int

	// ConnectedMembers 已连接成员数（如果启用）
	ConnectedMembers int
}

// ════════════════════════════════════════════════════════════════════════════
// AutoRelay 接口
// ════════════════════════════════════════════════════════════════════════════

// AutoRelay 定义自动中继管理接口
//
// AutoRelay 负责：
//   - 自动发现可用的中继服务器
//   - 自动预留和续期中继资源
//   - 中继健康检查和故障恢复
//   - 黑名单管理（避免频繁重试失败的中继）
type AutoRelay interface {
	// Start 启动 AutoRelay
	Start(ctx context.Context) error

	// Stop 停止 AutoRelay
	Stop() error

	// Enable 启用 AutoRelay
	Enable()

	// Disable 禁用 AutoRelay
	Disable()

	// IsEnabled 返回是否已启用
	IsEnabled() bool

	// Relays 返回当前活跃的中继节点列表
	Relays() []string

	// RelayAddrs 返回当前的中继地址列表
	RelayAddrs() []string

	// Status 返回 AutoRelay 状态
	Status() AutoRelayStatus

	// AddCandidate 添加候选中继
	AddCandidate(relayID string, addrs []string, priority int)

	// RemoveCandidate 移除候选中继
	RemoveCandidate(relayID string)

	// SetPreferredRelays 设置首选中继列表
	SetPreferredRelays(relayIDs []string)

	// SetOnAddrsChanged 设置地址变更回调
	SetOnAddrsChanged(callback func(addrs []string))
}

// AutoRelayStatus AutoRelay 状态
type AutoRelayStatus struct {
	Enabled        bool
	NumRelays      int
	RelayAddrs     []string
	NumCandidates  int
	NumBlacklisted int
}

// ════════════════════════════════════════════════════════════════════════════
// RelayClient 接口
// ════════════════════════════════════════════════════════════════════════════

// RelayClient 定义中继客户端接口
type RelayClient interface {
	// Reserve 在中继节点上预留资源
	Reserve(ctx context.Context, relayID string) (Reservation, error)

	// FindRelays 发现可用的中继服务器
	FindRelays(ctx context.Context) ([]string, error)
}

// Reservation 中继预留
type Reservation interface {
	// Expiry 返回预留过期时间
	Expiry() int64

	// Addrs 返回中继地址
	Addrs() []string

	// Refresh 刷新预留
	Refresh(ctx context.Context) error

	// Cancel 取消预留
	Cancel()
}
