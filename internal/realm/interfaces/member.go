// Package interfaces 定义 realm 模块内部接口
package interfaces

import (
	"context"
	"time"
)

// ============================================================================
//                              成员管理器接口
// ============================================================================

// MemberManager 成员管理器接口
//
// MemberManager 负责管理 Realm 成员，包括注册、注销、查询、同步等。
type MemberManager interface {
	// 基础操作
	Add(ctx context.Context, member *MemberInfo) error
	Remove(ctx context.Context, peerID string) error
	Get(ctx context.Context, peerID string) (*MemberInfo, error)
	List(ctx context.Context, opts *ListOptions) ([]*MemberInfo, error)

	// 状态管理
	UpdateStatus(ctx context.Context, peerID string, online bool) error
	UpdateLastSeen(ctx context.Context, peerID string) error

	// 批量操作
	BatchAdd(ctx context.Context, members []*MemberInfo) error
	BatchRemove(ctx context.Context, peerIDs []string) error

	// 查询操作
	IsMember(ctx context.Context, peerID string) bool
	GetOnlineCount() int
	GetTotalCount() int
	GetStats() *Stats

	// 同步操作
	SyncMembers(ctx context.Context) error

	// 生命周期
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Close() error
}

// ============================================================================
//                              成员接口
// ============================================================================

// Member 成员接口
//
// Member 表示 Realm 中的一个成员节点。
type Member interface {
	PeerID() string
	RealmID() string
	Role() Role
	IsOnline() bool
	IsAdmin() bool
	IsRelay() bool
	HasRole(role Role) bool
	JoinedAt() time.Time
	LastSeen() time.Time
}

// ============================================================================
//                              成员信息
// ============================================================================

// MemberInfo 成员信息
type MemberInfo struct {
	// 基础信息
	PeerID   string
	RealmID  string
	Role     Role

	// 状态信息
	Online   bool
	JoinedAt time.Time
	LastSeen time.Time

	// 网络信息
	Addrs []string

	// 元数据
	Metadata map[string]string

	// 统计信息
	BytesSent     int64
	BytesReceived int64
	MessagesSent  int64
}

// ============================================================================
//                              角色定义
// ============================================================================

// Role 成员角色
type Role int

const (
	// RoleMember 普通成员
	RoleMember Role = iota
	// RoleAdmin 管理员
	RoleAdmin
	// RoleRelay 中继节点
	RoleRelay
)

// String 返回角色名称
func (r Role) String() string {
	switch r {
	case RoleMember:
		return "Member"
	case RoleAdmin:
		return "Admin"
	case RoleRelay:
		return "Relay"
	default:
		return "Unknown"
	}
}

// ============================================================================
//                              查询选项
// ============================================================================

// ListOptions 列表查询选项
type ListOptions struct {
	// Limit 返回数量限制
	Limit int

	// OnlineOnly 仅返回在线成员
	OnlineOnly bool

	// Role 过滤角色
	Role *Role

	// SortBy 排序字段（joined_at, last_seen）
	SortBy string

	// Descending 降序排序
	Descending bool
}

// DefaultListOptions 返回默认列表选项
func DefaultListOptions() *ListOptions {
	return &ListOptions{
		Limit:      100,
		OnlineOnly: false,
		SortBy:     "joined_at",
		Descending: false,
	}
}

// ============================================================================
//                              统计信息
// ============================================================================

// Stats 成员统计信息
type Stats struct {
	// TotalCount 总成员数
	TotalCount int

	// OnlineCount 在线成员数
	OnlineCount int

	// AdminCount 管理员数
	AdminCount int

	// RelayCount 中继节点数
	RelayCount int

	// CacheHitRate 缓存命中率
	CacheHitRate float64

	// LastSyncTime 最后同步时间
	LastSyncTime time.Time
}

// ============================================================================
//                              缓存接口
// ============================================================================

// MemberCache 成员缓存接口
type MemberCache interface {
	Get(peerID string) (*MemberInfo, bool)
	Set(member *MemberInfo)
	Delete(peerID string)
	Clear()
	Size() int
}

// ============================================================================
//                              存储接口
// ============================================================================

// MemberStore 成员存储接口
type MemberStore interface {
	Save(member *MemberInfo) error
	Load(peerID string) (*MemberInfo, error)
	Delete(peerID string) error
	LoadAll() ([]*MemberInfo, error)
	Compact() error
	Close() error
}

// ============================================================================
//                              同步器接口
// ============================================================================

// MemberSynchronizer 成员同步器接口
type MemberSynchronizer interface {
	SyncFull(ctx context.Context, members []*MemberInfo) error
	SyncDelta(ctx context.Context, added, removed []*MemberInfo) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// ============================================================================
//                              心跳监控接口
// ============================================================================

// HeartbeatMonitor 心跳监控接口
type HeartbeatMonitor interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	SendHeartbeat(ctx context.Context, peerID string) error
	ReceiveHeartbeat(ctx context.Context, peerID string) error
	GetStatus(peerID string) (bool, error)
}
