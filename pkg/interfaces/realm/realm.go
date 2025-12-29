// Package realm 定义 Realm（领域）管理接口
//
// Realm 是 dep2p 实现业务隔离的核心机制：
// - 共享底层基础设施（DHT、中继、NAT 穿透）
// - 业务层完全隔离（不同 Realm 互不可见）
//
// v1.2 变更（IMPL-1227）:
//   - 新增 Realm 接口作为 Layer 2 产物
//   - JoinRealm 返回 Realm 对象而非 error
//   - RealmID 由 realmKey 派生，不可枚举
//   - 新增 PSK 成员认证机制
//   - Layer 3 服务通过 Realm 接口获取
//
// v1.1 变更:
//   - 采用严格单 Realm 模型（一个节点同时只能加入一个业务 Realm）
//   - 已移除跨 Realm 通信支持
//   - LeaveRealm 改为无参数
//   - IsMember 改为无参数便捷方法
package realm

import (
	"context"
	"errors"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrAlreadyInRealm 已在 Realm 中
	ErrAlreadyInRealm = errors.New("already in a realm, leave first")

	// ErrNotInRealm 不在任何 Realm 中
	ErrNotInRealm = errors.New("not in any realm")

	// ErrRealmKeyRequired 必须提供 RealmKey
	ErrRealmKeyRequired = errors.New("realm key is required")

	// ErrInvalidRealmKey 无效的 RealmKey
	ErrInvalidRealmKey = errors.New("invalid realm key")
)

// ============================================================================
//                              Realm 接口（IMPL-1227 新增）
// ============================================================================

// Realm 代表一个业务网络（Layer 2 产物）
//
// Realm 是访问所有 Layer 3 服务的入口。用户通过 RealmManager.JoinRealm()
// 加入一个 Realm 后获得此对象，然后通过它访问各种服务。
//
// 示例:
//
//	realm, err := node.JoinRealm(ctx, "my-business", dep2p.WithRealmKey(key))
//	if err != nil { ... }
//
//	// 通过 Realm 获取服务
//	messaging := realm.Messaging()
//	pubsub := realm.PubSub()
//
//	// 使用服务
//	messaging.Send(ctx, peerID, data)
//	topic, _ := pubsub.Join(ctx, "blocks")
type Realm interface {
	// ============================
	// 基本信息
	// ============================

	// Name 返回 Realm 的显示名称
	//
	// 这是用户提供的可读名称，仅用于 UI 展示，不参与安全边界。
	Name() string

	// ID 返回 Realm 的唯一标识
	//
	// RealmID 由 realmKey 派生，不可枚举。
	// 公式: RealmID = SHA256("dep2p-realm-id-v1" || H(realmKey))
	ID() types.RealmID

	// Key 返回 Realm 密钥
	//
	// 用于 PSK 成员证明生成。
	// 注意：此方法返回敏感信息，仅限内部使用。
	Key() types.RealmKey

	// ============================
	// 成员管理
	// ============================

	// Members 返回 Realm 内的所有成员节点
	Members() []types.NodeID

	// MemberCount 返回 Realm 内的成员数量
	MemberCount() int

	// IsMember 检查指定节点是否是 Realm 成员
	IsMember(peer types.NodeID) bool

	// ============================
	// Layer 3 服务入口
	// ============================

	// Messaging 获取消息服务
	//
	// 用于点对点消息发送和请求/响应模式。
	Messaging() Messaging

	// PubSub 获取发布订阅服务
	//
	// 用于主题订阅和消息广播。
	PubSub() PubSub

	// Discovery 获取 Realm 内发现服务
	//
	// 用于发现 Realm 内的其他成员。
	Discovery() RealmDiscoveryService

	// Streams 获取流管理服务
	//
	// 用于自定义协议的流式通信。
	Streams() StreamManager

	// Relay 获取 Realm 中继服务
	//
	// 用于管理 Realm 内的中继连接。
	Relay() RealmRelayService

	// ============================
	// 生命周期
	// ============================

	// Leave 离开此 Realm
	//
	// 会向 Realm 内邻居发送 Goodbye 消息，清理相关资源。
	Leave() error

	// Context 返回 Realm 的上下文
	//
	// 当 Realm 被 Leave 后，此上下文会被取消。
	Context() context.Context
}

// ============================================================================
//                              RealmOption 加入选项（IMPL-1227 新增）
// ============================================================================

// RealmOption Realm 加入选项函数
type RealmOption func(*RealmOptions)

// RealmOptions Realm 加入选项结构
type RealmOptions struct {
	// RealmKey Realm 密钥（必须）
	//
	// 用于 PSK 成员认证和 RealmID 派生。
	// 必须通过 WithRealmKey 设置。
	RealmKey types.RealmKey

	// Role 角色（可选）
	Role string

	// PrivateBootstrapPeers 私有引导节点（可选）
	//
	// 用于 Private Realm 自举，绕过公共 DHT 发现。
	PrivateBootstrapPeers []string

	// SkipDHTRegistration 跳过 DHT 注册（可选）
	//
	// Private Realm 不应在公共 DHT 中注册。
	SkipDHTRegistration bool
}

// WithRealmKey 设置 Realm 密钥（必须）
//
// 这是加入 Realm 的必要参数，用于 PSK 成员认证。
//
// 示例:
//
//	realm, err := node.JoinRealm(ctx, "my-realm", realm.WithRealmKey(key))
func WithRealmKey(key types.RealmKey) RealmOption {
	return func(o *RealmOptions) {
		o.RealmKey = key
	}
}

// WithRealmRole 设置角色
func WithRealmRole(role string) RealmOption {
	return func(o *RealmOptions) {
		o.Role = role
	}
}

// WithRealmBootstrapPeers 设置私有引导节点
func WithRealmBootstrapPeers(peers ...string) RealmOption {
	return func(o *RealmOptions) {
		o.PrivateBootstrapPeers = peers
	}
}

// WithRealmSkipDHT 跳过 DHT 注册
func WithRealmSkipDHT(skip bool) RealmOption {
	return func(o *RealmOptions) {
		o.SkipDHTRegistration = skip
	}
}

// ApplyRealmOptions 应用 Realm 加入选项
func ApplyRealmOptions(opts ...RealmOption) *RealmOptions {
	options := &RealmOptions{}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// ============================================================================
//                              RealmManager 接口（IMPL-1227 重构）
// ============================================================================

// RealmManager Realm 管理接口
//
// 负责管理节点在 Realm 中的成员身份。
//
// v1.2 变更（IMPL-1227）:
//   - JoinRealm 返回 Realm 对象
//   - CurrentRealm 返回 Realm 对象
//   - 新增 JoinRealmWithKey 便捷方法
//   - 部分方法移至 Realm 接口
//
// v1.1 变更:
//   - 采用严格单 Realm 模型，节点同时只能加入一个业务 Realm
//   - 移除 Realms() 方法
//   - LeaveRealm 改为无参数（离开当前 Realm）
//   - IsMember 改为无参数（检查是否已加入任何 Realm）
type RealmManager interface {
	// ============================
	// Realm 成员管理（v1.2 新 API）
	// ============================

	// JoinRealm 加入指定 Realm，返回 Realm 对象
	//
	// 必须通过 WithRealmKey 提供 realmKey，用于 PSK 成员认证。
	// RealmID 由 realmKey 自动派生。
	//
	// v1.2 变更: 返回 Realm 对象，接收 name 而非 realmID
	//
	// 示例:
	//
	//	realm, err := manager.JoinRealm(ctx, "my-business", realm.WithRealmKey(key))
	//	if err != nil { ... }
	//	messaging := realm.Messaging()
	JoinRealm(ctx context.Context, name string, opts ...RealmOption) (Realm, error)

	// JoinRealmWithKey 使用密钥加入 Realm（便捷方法）
	//
	// 等价于 JoinRealm(ctx, name, WithRealmKey(key), opts...)
	JoinRealmWithKey(ctx context.Context, name string, key types.RealmKey, opts ...RealmOption) (Realm, error)

	// CurrentRealm 返回当前 Realm 对象
	//
	// v1.2 变更: 返回 Realm 对象而非 RealmID
	// 如果未加入任何 Realm，返回 nil。
	CurrentRealm() Realm

	// LeaveRealm 离开当前 Realm
	//
	// 会向 Realm 内邻居发送 Goodbye 消息。
	// 如果未加入任何 Realm，返回 ErrNotInRealm。
	LeaveRealm() error

	// IsMember 检查是否已加入 Realm
	//
	// 返回 true 表示已加入某个 Realm。
	IsMember() bool

	// ============================
	// Realm 感知的 DHT
	// ============================

	// RealmDHTKey 计算 Realm 感知的 DHT Key
	//
	// 公式:
	//   - 无 Realm: Key = SHA256("dep2p/v1/sys/peer/{nodeID}")
	//   - 有 Realm: Key = SHA256("dep2p/v1/realm/{realmID}/peer/{nodeID}")
	RealmDHTKey(nodeID types.NodeID, realmID types.RealmID) []byte

	// ============================
	// 生命周期
	// ============================

	// Start 启动 Realm 管理服务
	Start(ctx context.Context) error

	// Stop 停止 Realm 管理服务
	Stop() error
}

// ============================================================================
//                              JoinOption 加入选项
// ============================================================================

// JoinOption 加入 Realm 的选项
type JoinOption func(*JoinOptions)

// JoinOptions 加入选项结构
type JoinOptions struct {
	// JoinKey 加入密钥（用于 Protected/Private Realm）
	JoinKey []byte

	// Role 角色
	Role string

	// SetAsPrimary 是否设为主 Realm
	SetAsPrimary bool

	// ============================================================================
	//                              Private Realm 自举（REQ-BOOT-005）
	// ============================================================================

	// PrivateBootstrapPeers 私有引导节点（用于 Private Realm 自举）
	// 格式：Full Address（含 /p2p/<NodeID>）
	// Private Realm 不在公共 DHT 中注册，需要通过已知节点地址直接连接
	PrivateBootstrapPeers []string

	// InviteData 邀请数据（用于 Private Realm 验证）
	// 由 Realm 管理员生成，包含签名和过期时间
	InviteData []byte

	// SkipDHTRegistration 跳过 DHT 注册（用于 Private Realm）
	// Private Realm 不应在公共 DHT 中注册
	SkipDHTRegistration bool
}

// WithJoinKey 设置加入密钥
func WithJoinKey(key []byte) JoinOption {
	return func(o *JoinOptions) {
		o.JoinKey = key
	}
}

// WithRole 设置角色
func WithRole(role string) JoinOption {
	return func(o *JoinOptions) {
		o.Role = role
	}
}

// WithSetAsPrimary 设置为主 Realm
func WithSetAsPrimary(primary bool) JoinOption {
	return func(o *JoinOptions) {
		o.SetAsPrimary = primary
	}
}

// WithPrivateBootstrapPeers 设置私有引导节点（REQ-BOOT-005）
//
// 用于 Private Realm 自举，绕过公共 DHT 发现。
// peers 格式：Full Address（含 /p2p/<NodeID>）
//
// 示例:
//
//	node.JoinRealm(ctx, "private-realm",
//	    realm.WithPrivateBootstrapPeers(
//	        "/ip4/192.168.1.100/tcp/4001/p2p/QmXXX...",
//	        "/ip4/10.0.0.5/udp/4001/quic-v1/p2p/QmYYY...",
//	    ),
//	    realm.WithInviteData(inviteBytes),
//	)
func WithPrivateBootstrapPeers(peers ...string) JoinOption {
	return func(o *JoinOptions) {
		o.PrivateBootstrapPeers = peers
	}
}

// WithInviteData 设置邀请数据（REQ-BOOT-005）
//
// 用于 Private Realm 验证，包含管理员签名和过期时间。
func WithInviteData(data []byte) JoinOption {
	return func(o *JoinOptions) {
		o.InviteData = data
	}
}

// WithSkipDHTRegistration 跳过 DHT 注册（REQ-BOOT-005）
//
// Private Realm 不应在公共 DHT 中注册。
func WithSkipDHTRegistration(skip bool) JoinOption {
	return func(o *JoinOptions) {
		o.SkipDHTRegistration = skip
	}
}

// ApplyJoinOptions 应用加入选项
func ApplyJoinOptions(opts ...JoinOption) *JoinOptions {
	options := &JoinOptions{}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// ============================================================================
//                              RealmDiscovery 接口
// ============================================================================

// RealmDiscovery Realm 感知的节点发现接口
//
// 在 Realm 层面对发现服务进行封装，确保只发现同 Realm 的节点。
type RealmDiscovery interface {
	// FindRealmPeers 在指定 Realm 内发现节点
	FindRealmPeers(ctx context.Context, realmID types.RealmID, limit int) (<-chan types.NodeID, error)

	// AnnounceToRealm 在指定 Realm 内通告自己
	AnnounceToRealm(ctx context.Context, realmID types.RealmID) error
}

// ============================================================================
//                              RealmMessaging 接口
// ============================================================================

// RealmMessaging Realm 感知的消息接口
//
// 确保 Pub-Sub 消息在 Realm 内隔离。
type RealmMessaging interface {
	// PublishToRealm 在 Realm 内发布消息
	PublishToRealm(ctx context.Context, realmID types.RealmID, topic string, data []byte) error

	// SubscribeInRealm 在 Realm 内订阅主题
	SubscribeInRealm(ctx context.Context, realmID types.RealmID, topic string) (RealmSubscription, error)
}

// RealmSubscription Realm 内的订阅
type RealmSubscription interface {
	// Messages 返回消息通道
	Messages() <-chan *RealmMessage

	// Cancel 取消订阅
	Cancel()
}

// RealmMessage Realm 内的消息
type RealmMessage struct {
	// RealmID 消息所属 Realm
	RealmID types.RealmID

	// Topic 主题
	Topic string

	// From 发送者
	From types.NodeID

	// Data 消息数据
	Data []byte
}

// ============================================================================
//                              RealmAccess 访问控制接口
// ============================================================================

// RealmAccessController 访问控制接口
type RealmAccessController interface {
	// SetAccess 设置 Realm 访问级别
	SetAccess(realmID types.RealmID, access types.AccessLevel) error

	// GetAccess 获取 Realm 访问级别
	GetAccess(realmID types.RealmID) types.AccessLevel

	// SetJoinKey 设置 Realm 加入密钥
	SetJoinKey(realmID types.RealmID, key []byte) error

	// ValidateJoinKey 验证加入密钥
	ValidateJoinKey(realmID types.RealmID, key []byte) bool

	// GenerateInvite 生成邀请（用于 Private Realm）
	GenerateInvite(realmID types.RealmID, targetNode types.NodeID) ([]byte, error)

	// ValidateInvite 验证邀请
	ValidateInvite(realmID types.RealmID, invite []byte, nodeID types.NodeID) bool

	// KickMember 踢出成员
	KickMember(realmID types.RealmID, nodeID types.NodeID) error
}


