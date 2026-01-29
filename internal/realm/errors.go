package realm

import "errors"

// ============================================================================
//                              Manager 错误
// ============================================================================

var (
	// ErrNotStarted Manager 未启动
	ErrNotStarted = errors.New("manager not started")

	// ErrAlreadyStarted Manager 已启动
	ErrAlreadyStarted = errors.New("manager already started")

	// ErrClosed Manager 已关闭
	ErrClosed = errors.New("manager closed")

	// ErrAlreadyInRealm 已在 Realm 中
	ErrAlreadyInRealm = errors.New("already in a realm")

	// ErrNotInRealm 不在 Realm 中
	ErrNotInRealm = errors.New("not in any realm")

	// ErrRealmNotFound Realm 未找到
	ErrRealmNotFound = errors.New("realm not found")

	// ErrRealmExists Realm 已存在
	ErrRealmExists = errors.New("realm already exists")
)

// ============================================================================
//                              Realm 错误
// ============================================================================

var (
	// ErrInvalidRealmID 无效的 RealmID
	ErrInvalidRealmID = errors.New("invalid realm id")

	// ErrInvalidPSK 无效的 PSK
	ErrInvalidPSK = errors.New("invalid psk")

	// ErrInvalidConfig 无效的配置
	ErrInvalidConfig = errors.New("invalid config")

	// ErrRealmInactive Realm 未激活
	ErrRealmInactive = errors.New("realm inactive")

	// ErrRealmClosed Realm 已关闭
	ErrRealmClosed = errors.New("realm closed")
)

// ============================================================================
//                              子模块错误
// ============================================================================

var (
	// ErrAuthFailed 认证失败
	ErrAuthFailed = errors.New("authentication failed")

	// ErrMemberSyncFailed 成员同步失败
	ErrMemberSyncFailed = errors.New("member sync failed")

	// ErrRoutingFailed 路由失败
	ErrRoutingFailed = errors.New("routing failed")

	// ErrGatewayFailed 网关失败
	ErrGatewayFailed = errors.New("gateway failed")

	// ErrNoRoute 无路由
	ErrNoRoute = errors.New("no route to peer")

	// ErrRelayFailed 中继失败
	ErrRelayFailed = errors.New("relay failed")
)

// ============================================================================
//                              服务错误
// ============================================================================

var (
	// ErrServiceNotAvailable 服务不可用
	ErrServiceNotAvailable = errors.New("service not available")

	// ErrInvalidTarget 无效的目标
	ErrInvalidTarget = errors.New("invalid target")

	// ErrNotMember 不是成员
	ErrNotMember = errors.New("not a member")

	// ErrSendFailed 发送失败
	ErrSendFailed = errors.New("send failed")

	// ErrSubscribeFailed 订阅失败
	ErrSubscribeFailed = errors.New("subscribe failed")
)

// ============================================================================
//                              工厂错误
// ============================================================================

var (
	// ErrNoFactory 无工厂
	ErrNoFactory = errors.New("factory not provided")

	// ErrFactoryFailed 工厂失败
	ErrFactoryFailed = errors.New("factory failed to create instance")
)

// ============================================================================
//                              超时错误
// ============================================================================

var (
	// ErrJoinTimeout Join 超时
	ErrJoinTimeout = errors.New("join timeout")

	// ErrLeaveTimeout Leave 超时
	ErrLeaveTimeout = errors.New("leave timeout")

	// ErrAuthTimeout Auth 超时
	ErrAuthTimeout = errors.New("auth timeout")

	// ErrSyncTimeout 同步超时
	ErrSyncTimeout = errors.New("sync timeout")
)
