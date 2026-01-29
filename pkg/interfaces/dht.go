// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 DHT 接口，对应 internal/discovery/dht/ 实现。
package interfaces

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// DHTDiscovery 接口
// ════════════════════════════════════════════════════════════════════════════

// DHTDiscovery 定义 DHT 发现服务接口
//
// DHTDiscovery 扩展了基础 Discovery 接口，提供特定于 DHT 的节点查找方法。
type DHTDiscovery interface {
	Discovery

	// FindPeer 查找指定节点
	//
	// 通过 DHT 查询节点的地址信息。
	FindPeer(ctx context.Context, peerID types.PeerID) (types.PeerInfo, error)
}

// ════════════════════════════════════════════════════════════════════════════
// DHT 接口
// ════════════════════════════════════════════════════════════════════════════

// DHT 定义分布式哈希表接口
//
// DHT 提供 Kademlia 分布式哈希表的完整功能，包括：
// - 节点发现（Discovery 接口）
// - 键值存储（GetValue/PutValue）
// - 内容提供者发现（Provide/FindProviders）
// - 节点查找（FindPeer）
// - PeerRecord 管理（v2.0 新增）
// - 权威查询（v2.0 新增）
//
// 架构位置：Discovery Layer
// 实现位置：internal/discovery/dht/
//
// 使用示例:
//
//	dht := dht.New(host, config)
//	dht.Start(ctx)
//	defer dht.Stop(ctx)
//
//	// 存储值
//	dht.PutValue(ctx, "/app/key", []byte("value"))
//
//	// 获取值
//	value, _ := dht.GetValue(ctx, "/app/key")
//
//	// 提供内容
//	dht.Provide(ctx, "content-hash", true)
//
//	// 查找提供者
//	providers, _ := dht.FindProviders(ctx, "content-hash")
//
//	// v2.0: 查询权威 PeerRecord
//	result, _ := dht.GetAuthoritativePeerRecord(ctx, realmID, nodeID)
type DHT interface {
	Discovery

	// GetValue 获取值
	//
	// 从 DHT 网络中获取指定键的值。
	GetValue(ctx context.Context, key string) ([]byte, error)

	// PutValue 存储值
	//
	// 将键值对存储到 DHT 网络中。
	PutValue(ctx context.Context, key string, value []byte) error

	// FindPeer 查找特定节点
	//
	// 通过 DHT 查询节点的地址信息。
	FindPeer(ctx context.Context, peerID string) (types.PeerInfo, error)

	// Provide 提供内容
	//
	// 声明本节点提供指定内容。
	// 参数:
	//   - key: 内容标识符
	//   - broadcast: 是否广播到网络
	Provide(ctx context.Context, key string, broadcast bool) error

	// FindProviders 查找内容提供者
	//
	// 查找提供指定内容的节点。
	FindProviders(ctx context.Context, key string) (<-chan types.PeerInfo, error)

	// Bootstrap 执行引导过程
	//
	// 连接引导节点并填充路由表。
	Bootstrap(ctx context.Context) error

	// RoutingTable 返回路由表
	RoutingTable() RoutingTable

	// ════════════════════════════════════════════════════════════════════════
	// v2.0 新增：PeerRecord 管理
	// ════════════════════════════════════════════════════════════════════════

	// GetPeerRecord 获取指定节点的 PeerRecord
	//
	// 从 DHT 网络中获取指定 Realm 和节点的签名 PeerRecord。
	// PeerRecord 包含节点的完整地址信息、NAT 类型、能力等。
	//
	// 参数:
	//   - ctx: 上下文
	//   - realmID: Realm 标识符
	//   - nodeID: 目标节点 ID
	//
	// 返回:
	//   - record: 签名的 PeerRecord（序列化字节）
	//   - err: 错误（未找到时返回 ErrNotFound）
	GetPeerRecord(ctx context.Context, realmID types.RealmID, nodeID types.NodeID) ([]byte, error)

	// PublishPeerRecord 发布 PeerRecord
	//
	// 将签名的 PeerRecord 发布到 DHT 网络。
	// 记录会被存储到距离 realmID+nodeID 最近的 K 个节点上。
	//
	// 参数:
	//   - ctx: 上下文
	//   - record: 签名的 PeerRecord（序列化字节）
	//
	// 返回:
	//   - err: 发布失败时返回错误
	PublishPeerRecord(ctx context.Context, record []byte) error

	// UnpublishPeerRecord 取消发布 PeerRecord
	//
	// Phase D Step D3: 优雅关闭时调用，通知 DHT 网络本节点即将离线。
	// 实现逻辑：
	//   1. 停止续期循环（republishLoop）
	//   2. 从本地 peerRecordStore 删除记录
	//   3. 可选：向最近的 K 个节点发送删除通知（TTL=0 记录）
	//
	// 参数:
	//   - ctx: 上下文
	//
	// 返回:
	//   - err: 取消发布失败时返回错误
	UnpublishPeerRecord(ctx context.Context) error

	// ════════════════════════════════════════════════════════════════════════
	// v2.0 重构：Realm 成员发现（Provider Record）
	// ════════════════════════════════════════════════════════════════════════

	// ProvideRealmMembership 声明自己是 Realm 成员
	//
	// 发布 Provider Record 到 DHT，声明本节点是指定 Realm 的成员。
	// Key: /dep2p/v2/realm/<H(RealmID)>/members
	//
	// 这是"先发布后发现"模式的核心，无需入口节点即可加入 Realm。
	//
	// 参数:
	//   - ctx: 上下文
	//   - realmID: Realm 标识符
	//
	// 返回:
	//   - err: 发布失败时返回错误
	ProvideRealmMembership(ctx context.Context, realmID types.RealmID) error

	// FindRealmMembers 查找 Realm 成员列表
	//
	// 通过 DHT Provider 机制查找指定 Realm 的所有成员。
	// Key: /dep2p/v2/realm/<H(RealmID)>/members
	//
	// 参数:
	//   - ctx: 上下文
	//   - realmID: Realm 标识符
	//
	// 返回:
	//   - ch: 成员 PeerID 通道（异步返回）
	//   - err: 查询失败时返回错误
	FindRealmMembers(ctx context.Context, realmID types.RealmID) (<-chan types.PeerID, error)

	// ════════════════════════════════════════════════════════════════════════
	// v2.0 重构：Realm 成员地址查询
	// ════════════════════════════════════════════════════════════════════════

	// PublishRealmPeerRecord 发布 Realm 成员地址
	//
	// 将签名的 PeerRecord 发布到 DHT，供其他 Realm 成员查询。
	// Key: /dep2p/v2/realm/<H(RealmID)>/peer/<NodeID>
	//
	// 参数:
	//   - ctx: 上下文
	//   - realmID: Realm 标识符
	//   - record: 签名的 PeerRecord（序列化字节）
	//
	// 返回:
	//   - err: 发布失败时返回错误
	PublishRealmPeerRecord(ctx context.Context, realmID types.RealmID, record []byte) error

	// FindRealmPeerRecord 查询 Realm 成员地址
	//
	// 从 DHT 查询指定 Realm 成员的 PeerRecord。
	// Key: /dep2p/v2/realm/<H(RealmID)>/peer/<NodeID>
	//
	// 参数:
	//   - ctx: 上下文
	//   - realmID: Realm 标识符
	//   - nodeID: 目标节点 ID
	//
	// 返回:
	//   - record: 签名的 PeerRecord（序列化字节）
	//   - err: 未找到时返回 ErrNotFound
	FindRealmPeerRecord(ctx context.Context, realmID types.RealmID, nodeID types.NodeID) ([]byte, error)

	// ════════════════════════════════════════════════════════════════════════
	// v2.0 新增：权威查询
	// ════════════════════════════════════════════════════════════════════════

	// GetAuthoritativePeerRecord 获取权威 PeerRecord
	//
	// 按优先级从多个来源查询节点的 PeerRecord：
	//   1. DHT 网络（权威来源）
	//   2. Relay 地址簿（缓存来源）
	//   3. 本地 Peerstore（本地缓存）
	//
	// 参数:
	//   - ctx: 上下文
	//   - realmID: Realm 标识符
	//   - nodeID: 目标节点 ID
	//
	// 返回:
	//   - result: 权威查询结果（包含来源、地址列表、TTL 等）
	//   - err: 所有来源都失败时返回错误
	GetAuthoritativePeerRecord(ctx context.Context, realmID types.RealmID, nodeID types.NodeID) (*AuthoritativeQueryResult, error)
}

// ════════════════════════════════════════════════════════════════════════════
// RoutingTable 接口
// ════════════════════════════════════════════════════════════════════════════

// RoutingTable 定义 DHT 路由表接口
//
// 路由表管理 DHT 中已知节点的信息，用于高效路由查询。
type RoutingTable interface {
	// Size 返回路由表大小
	Size() int

	// NearestPeers 返回距离指定键最近的节点
	//
	// 参数:
	//   - key: 目标键（通常是节点 ID 或内容哈希）
	//   - count: 返回数量
	NearestPeers(key string, count int) []string

	// Update 更新节点
	//
	// 将节点添加到路由表或更新其位置。
	Update(peerID string) error

	// Remove 移除节点
	Remove(peerID string)
}

// ════════════════════════════════════════════════════════════════════════════
// v2.0 新增：权威查询相关类型
// ════════════════════════════════════════════════════════════════════════════

// AuthoritativeSource 权威来源枚举
//
// 表示 PeerRecord 的来源，按权威性从低到高排列。
// 
type AuthoritativeSource int

const (
	// SourceUnknown 未知来源
	SourceUnknown AuthoritativeSource = iota

	// SourceLocal 本地 Peerstore 来源（最低权威性）
	//
	// 本地缓存的地址信息，可能已过期。
	SourceLocal

	// SourceRelay Relay 地址簿来源（中等权威性）
	//
	// Relay 地址簿是 DHT 的缓存，优先级次于 DHT。
	SourceRelay

	// SourceDHT DHT 来源（最高权威性）
	//
	// DHT 是 PeerRecord 的权威来源，记录经过签名验证。
	SourceDHT
)

// String 返回来源的字符串表示
func (s AuthoritativeSource) String() string {
	switch s {
	case SourceDHT:
		return "DHT"
	case SourceRelay:
		return "Relay"
	case SourceLocal:
		return "Local"
	default:
		return "Unknown"
	}
}

// IsAuthoritative 检查来源是否具有权威性
func (s AuthoritativeSource) IsAuthoritative() bool {
	return s == SourceDHT
}

// AuthoritativeQueryResult 权威查询结果
//
// 封装从多个来源查询 PeerRecord 的结果。
// 
type AuthoritativeQueryResult struct {
	// Source 最终使用的来源
	Source AuthoritativeSource

	// Addresses 聚合后的地址列表
	Addresses []string

	// DirectAddrs 直连地址列表
	DirectAddrs []string

	// RelayAddrs 中继地址列表
	RelayAddrs []string

	// QueryDuration 查询耗时
	QueryDuration time.Duration

	// FallbackUsed 是否使用了回退来源
	//
	// 如果 DHT 查询失败，回退到 Relay 或 Local 时为 true
	FallbackUsed bool

	// FallbackReason 回退原因（如果使用了回退）
	FallbackReason string

	// ExpiresAt 记录过期时间（如果可用）
	ExpiresAt time.Time
}

// HasAddresses 是否有可用地址
func (r *AuthoritativeQueryResult) HasAddresses() bool {
	return len(r.Addresses) > 0
}
