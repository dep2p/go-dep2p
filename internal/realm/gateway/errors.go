package gateway

import "errors"

var (
	// ErrRealmMismatch RealmID 不匹配
	ErrRealmMismatch = errors.New("gateway: realm ID mismatch")

	// ErrAuthFailed 认证失败
	ErrAuthFailed = errors.New("gateway: authentication failed")

	// ErrInvalidProtocol 无效的协议
	ErrInvalidProtocol = errors.New("gateway: invalid protocol")

	// ErrInvalidRequest 无效的请求
	ErrInvalidRequest = errors.New("gateway: invalid request")

	// ErrInvalidConfig 无效的配置
	ErrInvalidConfig = errors.New("gateway: invalid config")

	// ErrPoolExhausted 连接池耗尽
	ErrPoolExhausted = errors.New("gateway: connection pool exhausted")

	// ErrBandwidthLimit 带宽限制
	ErrBandwidthLimit = errors.New("gateway: bandwidth limit exceeded")

	// ErrRelayTimeout 中继超时
	ErrRelayTimeout = errors.New("gateway: relay timeout")

	// ErrNoConnection 无连接
	ErrNoConnection = errors.New("gateway: no connection")

	// ErrSessionClosed 会话已关闭
	ErrSessionClosed = errors.New("gateway: session closed")

	// ErrGatewayClosed Gateway 已关闭
	ErrGatewayClosed = errors.New("gateway: gateway is closed")

	// ErrNotStarted 未启动
	ErrNotStarted = errors.New("gateway: not started")

	// ErrAlreadyStarted 已启动
	ErrAlreadyStarted = errors.New("gateway: already started")

	// ErrNoHost Host 不可用
	ErrNoHost = errors.New("gateway: host is not available")

	// ErrNoAuth 认证器不可用
	ErrNoAuth = errors.New("gateway: authenticator is not available")
)
