// Package interfaces 定义 realm 模块内部接口
//
// 本文件定义成员地址簿接口，用于 Realm 内成员地址管理。
// 地址簿是"仅 ID 连接"能力的核心组件，存储 Realm 成员的地址信息。
package interfaces

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              MemberEntry - 成员地址条目
// ============================================================================

// MemberEntry 成员地址条目
//
// 存储 Realm 成员的地址信息，用于"仅 ID 连接"时的地址发现。
// 每个条目对应一个 Realm 成员节点。
type MemberEntry struct {
	// NodeID 成员节点 ID
	NodeID types.NodeID

	// DirectAddrs 直连地址列表
	//
	// 节点的可达地址，按优先级排序（本地网络地址优先）。
	DirectAddrs []types.Multiaddr

	// NATType NAT 类型
	//
	// 用于判断直连可能性和选择打洞策略。
	NATType types.NATType

	// Capabilities 能力标签
	//
	// 节点支持的能力，如 "relay"、"dht"、"nat-traversal" 等。
	Capabilities []string

	// Online 是否在线
	//
	// 基于心跳检测的在线状态。
	Online bool

	// LastSeen 最后活跃时间
	//
	// 最后一次收到该节点消息的时间。
	LastSeen time.Time

	// LastUpdate 最后更新时间
	//
	// 地址信息的最后更新时间。
	LastUpdate time.Time
}

// IsEmpty 检查条目是否为空
func (e MemberEntry) IsEmpty() bool {
	return e.NodeID.IsEmpty()
}

// HasAddrs 检查是否有可用地址
func (e MemberEntry) HasAddrs() bool {
	return len(e.DirectAddrs) > 0
}

// ============================================================================
//                              AddressBookStore - 存储接口
// ============================================================================

// AddressBookStore 成员地址簿存储接口
//
// 定义地址簿的持久化操作，支持内存和持久化存储实现。
// 这是一个底层存储接口，不包含业务逻辑。
type AddressBookStore interface {
	// Put 存储成员条目
	//
	// 如果条目已存在，则覆盖。
	Put(ctx context.Context, entry MemberEntry) error

	// Get 获取成员条目
	//
	// 返回值：条目、是否存在、错误
	Get(ctx context.Context, nodeID types.NodeID) (MemberEntry, bool, error)

	// Delete 删除成员条目
	Delete(ctx context.Context, nodeID types.NodeID) error

	// List 列出所有成员条目
	List(ctx context.Context) ([]MemberEntry, error)

	// SetTTL 设置条目过期时间
	//
	// 用于自动清理离线节点。
	SetTTL(ctx context.Context, nodeID types.NodeID, ttl time.Duration) error

	// CleanExpired 清理过期条目
	//
	// 删除超过 TTL 的条目。
	CleanExpired(ctx context.Context) error

	// Close 关闭存储
	Close() error
}

// ============================================================================
//                              AddressBook - 聚合根接口
// ============================================================================

// AddressBook 成员地址簿接口
//
// 这是地址簿的聚合根接口，封装业务逻辑。
// 地址簿用于管理 Realm 成员地址信息。
type AddressBook interface {
	// Register 注册成员地址
	//
	// 成员加入 Realm 时调用，注册其地址信息。
	// 如果成员已存在，则更新其地址。
	Register(ctx context.Context, entry MemberEntry) error

	// Query 查询成员地址
	//
	// 通过 NodeID 查询成员地址信息。
	// 如果成员不存在，返回 ErrMemberNotFound。
	Query(ctx context.Context, nodeID types.NodeID) (MemberEntry, error)

	// Update 更新成员地址
	//
	// 更新已存在成员的地址信息。
	// 如果成员不存在，返回 ErrMemberNotFound。
	Update(ctx context.Context, entry MemberEntry) error

	// Remove 移除成员
	//
	// 成员离开 Realm 时调用。
	Remove(ctx context.Context, nodeID types.NodeID) error

	// Members 获取所有成员
	//
	// 返回所有已注册的成员列表。
	Members(ctx context.Context) ([]MemberEntry, error)

	// OnlineMembers 获取在线成员
	//
	// 返回当前在线的成员列表。
	OnlineMembers(ctx context.Context) ([]MemberEntry, error)

	// SetOnline 设置成员在线状态
	//
	// 心跳检测时调用，更新成员在线状态。
	SetOnline(ctx context.Context, nodeID types.NodeID, online bool) error

	// Close 关闭地址簿
	Close() error
}

// ============================================================================
//                              AddressBookClient - 客户端接口
// ============================================================================

// AddressBookClient 地址簿客户端接口
//
// 用于非 Relay 节点查询地址簿。
// 通过协议与 Relay 节点的 AddressBook 交互。
type AddressBookClient interface {
	// QueryAddress 查询成员地址
	//
	// 向 Relay 发送查询请求，获取目标成员的地址信息。
	QueryAddress(ctx context.Context, targetNodeID types.NodeID) (MemberEntry, error)

	// RegisterAddress 注册本节点地址
	//
	// 向 Relay 注册本节点的地址信息。
	RegisterAddress(ctx context.Context, entry MemberEntry) error

	// Close 关闭客户端
	Close() error
}
