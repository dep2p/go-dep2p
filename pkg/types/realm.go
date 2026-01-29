// Package types 定义 DeP2P 公共类型
//
// 本文件定义 Realm 相关类型。
// Realm 是 DeP2P 的核心创新，提供隔离的 P2P 子网络。
package types

import (
	"time"
)

// ============================================================================
//                              RealmInfo - Realm 信息
// ============================================================================

// RealmInfo Realm 信息
type RealmInfo struct {
	// ID Realm 唯一标识
	ID RealmID

	// Name Realm 名称（可选，用于显示）
	Name string

	// Created 创建时间
	Created time.Time

	// MemberCount 成员数量
	MemberCount int

	// OnlineCount 在线成员数量
	OnlineCount int

	// AuthMode 认证模式
	AuthMode RealmAuthMode

	// RelayEnabled 是否启用 Realm 中继
	RelayEnabled bool
}

// ============================================================================
//                              RealmMember - Realm 成员
// ============================================================================

// RealmMember Realm 成员信息
type RealmMember struct {
	// PeerID 节点 ID
	PeerID PeerID

	// Role 角色
	Role RealmRole

	// JoinedAt 加入时间
	JoinedAt time.Time

	// LastSeen 最后活跃时间
	LastSeen time.Time

	// Online 是否在线
	Online bool

	// Metadata 成员元数据
	Metadata map[string]string
}

// IsOnline 检查成员是否在线
func (m RealmMember) IsOnline() bool {
	return m.Online
}

// IsAdmin 检查成员是否为管理员
func (m RealmMember) IsAdmin() bool {
	return m.Role == RoleAdmin
}

// IsRelay 检查成员是否为中继节点
func (m RealmMember) IsRelay() bool {
	return m.Role == RoleRelay
}

// ============================================================================
//                              RealmConfig - Realm 配置
// ============================================================================

// RealmConfig Realm 配置
type RealmConfig struct {
	// Name 名称
	Name string

	// PSK 预共享密钥
	PSK PSK

	// MaxMembers 最大成员数（0 表示无限制）
	MaxMembers int

	// AuthMode 认证模式
	AuthMode RealmAuthMode

	// RelayEnabled 是否启用中继
	RelayEnabled bool

	// RelayPeer 指定中继节点（可选）
	RelayPeer PeerID

	// Metadata Realm 元数据
	Metadata map[string]string
}

// Validate 验证配置有效性
func (c RealmConfig) Validate() error {
	if c.PSK.IsEmpty() {
		return ErrEmptyPSK
	}
	if len(c.PSK) != PSKLength {
		return ErrInvalidPSKLength
	}
	return nil
}

// DeriveRealmID 从配置派生 RealmID
func (c RealmConfig) DeriveRealmID() RealmID {
	return RealmIDFromPSK(c.PSK)
}

// DefaultRealmConfig 返回默认 Realm 配置
func DefaultRealmConfig() RealmConfig {
	return RealmConfig{
		AuthMode:     AuthModePSK,
		MaxMembers:   0, // 无限制
		RelayEnabled: false,
	}
}

// ============================================================================
//                              RealmStats - Realm 统计
// ============================================================================

// RealmStats Realm 统计信息
type RealmStats struct {
	// RealmID Realm ID
	RealmID RealmID

	// MemberCount 成员数量
	MemberCount int

	// OnlineCount 在线数量
	OnlineCount int

	// MessageCount 消息数量
	MessageCount int64

	// BytesSent 发送字节数
	BytesSent int64

	// BytesReceived 接收字节数
	BytesReceived int64

	// Uptime Realm 运行时间
	Uptime time.Duration
}

// ============================================================================
//                              RelayConfig - 中继配置（统一 Relay v2.0）
// ============================================================================

// RelayConfig 中继配置（统一 Relay v2.0）
type RelayConfig struct {
	// Enabled 是否启用
	Enabled bool

	// RelayPeer 中继节点
	RelayPeer PeerID

	// MaxBandwidth 最大带宽（字节/秒，0 表示无限制）
	MaxBandwidth int64

	// MaxDuration 最大连接时长
	MaxDuration time.Duration
}

// RealmRelayConfig 是 RelayConfig 的别名（保持向后兼容）
// Deprecated: 请使用 RelayConfig
type RealmRelayConfig = RelayConfig

// ============================================================================
//                              RealmJoinOptions - 加入选项
// ============================================================================

// RealmJoinOptions 加入 Realm 的选项
type RealmJoinOptions struct {
	// Timeout 加入超时
	Timeout time.Duration

	// Role 期望角色
	Role RealmRole

	// Metadata 成员元数据
	Metadata map[string]string
}

// DefaultRealmJoinOptions 返回默认加入选项
func DefaultRealmJoinOptions() RealmJoinOptions {
	return RealmJoinOptions{
		Timeout: 30 * time.Second,
		Role:    RoleMember,
	}
}

// ============================================================================
//                              RealmFindOptions - 查找选项
// ============================================================================

// RealmFindOptions 查找 Realm 成员的选项
type RealmFindOptions struct {
	// Limit 返回数量限制
	Limit int

	// OnlineOnly 仅返回在线成员
	OnlineOnly bool

	// Role 过滤角色
	Role *RealmRole
}

// DefaultRealmFindOptions 返回默认查找选项
func DefaultRealmFindOptions() RealmFindOptions {
	return RealmFindOptions{
		Limit:      100,
		OnlineOnly: false,
	}
}
