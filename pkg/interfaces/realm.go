// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Realm 接口，提供隔离域功能。
package interfaces

import (
	"context"
)

// Realm 定义隔离域接口
//
// Realm 是 DeP2P 的核心创新，提供独立的 P2P 子网络。
// 该接口包含用户方法和系统内部方法，用户 API 层只暴露部分方法。
type Realm interface {
	// ════════════════════════════════════════════════════════════════════════
	// 用户方法（通过 dep2p.Realm 包装后暴露）
	// ════════════════════════════════════════════════════════════════════════

	// ID 返回 Realm ID
	ID() string

	// Name 返回 Realm 名称
	Name() string

	// Join 加入 Realm
	Join(ctx context.Context) error

	// Leave 离开 Realm
	Leave(ctx context.Context) error

	// Members 返回当前成员列表
	Members() []string

	// IsMember 检查节点是否为成员
	IsMember(peerID string) bool

	// Messaging 返回消息服务
	Messaging() Messaging

	// PubSub 返回发布订阅服务
	PubSub() PubSub

	// Streams 返回流服务
	Streams() Streams

	// Liveness 返回存活检测服务
	Liveness() Liveness

	// Connect 使用纯 NodeID 连接 Realm 成员
	//
	// 用户只需提供目标节点的 NodeID，系统自动完成地址解析和连接建立。
	// 连接优先级：直连 → 打洞 → Relay 保底。
	//
	// 注意：目标必须是 Realm 成员，否则返回错误。
	Connect(ctx context.Context, target string) (Connection, error)

	// ConnectWithHint 使用 NodeID 和地址提示连接 Realm 成员
	//
	// 与 Connect 类似，但允许用户提供地址提示来加速连接。
	// 提示地址会被优先尝试，如果失败则回退到自动发现流程。
	//
	// 参数:
	//   - target: 目标节点 NodeID
	//   - hints: 地址提示列表（如 "/ip4/1.2.3.4/tcp/4001"）
	//
	// 注意：目标必须是 Realm 成员，否则返回错误。
	ConnectWithHint(ctx context.Context, target string, hints []string) (Connection, error)

	// Close 关闭 Realm
	Close() error

	// EventBus 返回事件总线（用于订阅成员事件）
	//
	// 支持的事件类型：
	//   - types.EvtRealmMemberJoined: 成员加入
	//   - types.EvtRealmMemberLeft: 成员离开
	//
	// 示例：
	//   sub, _ := realm.EventBus().Subscribe(new(types.EvtRealmMemberJoined))
	//   for evt := range sub.C() {
	//       if e, ok := evt.(*types.EvtRealmMemberJoined); ok {
	//           fmt.Println("Member joined:", e.PeerID)
	//       }
	//   }
	EventBus() EventBus

	// ════════════════════════════════════════════════════════════════════════
	// 系统内部方法（仅供内部模块使用，不暴露给用户）
	// ════════════════════════════════════════════════════════════════════════

	// PSK 返回预共享密钥（供内部认证使用）
	PSK() []byte

	// Authenticate 验证对方身份（供内部使用）
	Authenticate(ctx context.Context, peerID string, proof []byte) (bool, error)

	// GenerateProof 生成认证证明（供内部使用）
	GenerateProof(ctx context.Context) ([]byte, error)
}

// RealmManager 定义 Realm 管理器接口
type RealmManager interface {
	// CreateWithOpts 使用选项创建新 Realm
	CreateWithOpts(ctx context.Context, opts ...RealmOption) (Realm, error)

	// GetRealm 获取已存在的 Realm
	GetRealm(realmID string) (Realm, bool)

	// ListRealms 列出所有 Realm
	ListRealms() []Realm

	// Current 返回当前活跃的 Realm（如果有）
	Current() Realm

	// NotifyNetworkChange 通知所有活跃的 Realm 网络已变化
	//
	// 遍历所有活跃的 Realm，触发：
	//   - DHT 重新发布
	//   - Relay 地址簿更新
	//   - MemberList 广播
	//
	// 对齐设计文档 Section 7.3 AddressChangeCoordinator
	NotifyNetworkChange(ctx context.Context, event NetworkChangeEvent) error

	// Close 关闭所有 Realm
	Close() error
}

// RealmOption 定义 Realm 选项函数
type RealmOption func(*RealmConfig)

// RealmConfig Realm 配置
type RealmConfig struct {
	// ID Realm 唯一标识
	ID string

	// Name Realm 名称
	Name string

	// PSK 预共享密钥
	PSK []byte

	// MaxMembers 最大成员数
	MaxMembers int

	// AuthMode 认证模式
	AuthMode RealmAuthMode
}

// RealmAuthMode 认证模式
type RealmAuthMode int

const (
	// AuthModePSK 预共享密钥认证
	AuthModePSK RealmAuthMode = iota
	// AuthModeCert 证书认证
	AuthModeCert
	// AuthModeCustom 自定义认证
	AuthModeCustom
)

// WithRealmID 设置 Realm ID
func WithRealmID(id string) RealmOption {
	return func(c *RealmConfig) {
		c.ID = id
	}
}

// WithRealmName 设置 Realm 名称
func WithRealmName(name string) RealmOption {
	return func(c *RealmConfig) {
		c.Name = name
	}
}

// WithPSK 设置预共享密钥
func WithPSK(psk []byte) RealmOption {
	return func(c *RealmConfig) {
		c.PSK = psk
		c.AuthMode = AuthModePSK
	}
}

// WithMaxMembers 设置最大成员数
func WithMaxMembers(max int) RealmOption {
	return func(c *RealmConfig) {
		c.MaxMembers = max
	}
}
