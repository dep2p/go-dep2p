package realm

import "errors"

// v1.1 变更:
//   - 移除 ErrMultiRealmDisabled 和 ErrMaxRealmsReached（采用严格单 Realm 模型）
//   - 新增 ErrAlreadyJoined、ErrRealmMismatch、ErrRealmAuthFailed

var (
	// ErrNotMember 不是任何 Realm 成员
	// v1.1: 更新描述，用于强制隔离检查
	ErrNotMember = errors.New("not a member of any realm")

	// ErrAlreadyJoined 已加入 Realm (v1.1 新增)
	// 严格单 Realm 模型下，已加入 Realm 时再次 JoinRealm 返回此错误
	ErrAlreadyJoined = errors.New("already joined a realm")

	// ErrRealmKeyRequired 必须提供 RealmKey (IMPL-1227 新增)
	// PSK 成员认证模型要求必须提供 realmKey
	ErrRealmKeyRequired = errors.New("realm key is required for PSK authentication")

	// ErrRealmNotFound Realm 不存在
	ErrRealmNotFound = errors.New("realm not found")

	// ErrRealmMismatch Realm 不匹配 (v1.1 新增)
	// 当连接的 Realm 与本地 Realm 不一致时返回
	ErrRealmMismatch = errors.New("realm mismatch")

	// ErrRealmAuthFailed RealmAuth 验证失败 (v1.1 新增)
	ErrRealmAuthFailed = errors.New("realm authentication failed")

	// ErrInvalidJoinKey 无效的加入密钥
	ErrInvalidJoinKey = errors.New("invalid join key")

	// ErrAccessDenied 访问被拒绝
	ErrAccessDenied = errors.New("access denied")

	// ErrNoConnection 没有连接
	ErrNoConnection = errors.New("no connection available")

	// ErrRealmAuthTimeout RealmAuth 超时 (v1.1 新增)
	ErrRealmAuthTimeout = errors.New("realm auth timeout")

	// ErrInvalidSignature 无效签名 (v1.1 新增)
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrMessageTooLarge 消息过大
	ErrMessageTooLarge = errors.New("message too large")

	// ErrInvalidFullAddress 无效的 Full Address 格式（REQ-BOOT-005）
	// Full Address 必须包含 /p2p/<NodeID> 后缀
	ErrInvalidFullAddress = errors.New("invalid full address: must contain /p2p/<NodeID>")

	// ErrDiscoveryNotAvailable 发现服务不可用（P1 修复）
	// 当需要发现服务但未注入时返回
	ErrDiscoveryNotAvailable = errors.New("discovery service not available")

	// ErrEndpointNotAvailable Endpoint 不可用（P1 修复）
	// 当需要 Endpoint 但未注入时返回
	ErrEndpointNotAvailable = errors.New("endpoint not available")

	// ErrInvalidProtocol 无效的协议（P1 修复）
	// 当协议验证失败时返回
	ErrInvalidProtocol = errors.New("invalid protocol")
)


