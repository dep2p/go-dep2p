package connector

import "errors"

var (
	// ErrNotMember 目标不是 Realm 成员
	ErrNotMember = errors.New("target is not a realm member")

	// ErrNoAddress 无法解析目标地址
	ErrNoAddress = errors.New("no address found for target")

	// ErrDirectConnectFailed 直连失败
	ErrDirectConnectFailed = errors.New("direct connection failed")

	// ErrHolePunchFailed 打洞失败
	ErrHolePunchFailed = errors.New("hole punch failed")

	// ErrRelayConnectFailed Relay 连接失败
	ErrRelayConnectFailed = errors.New("relay connection failed")

	// ErrConnectorClosed 连接器已关闭
	ErrConnectorClosed = errors.New("connector is closed")

	// ErrInvalidTarget 无效的目标 ID
	ErrInvalidTarget = errors.New("invalid target ID")

	// ErrTimeout 连接超时
	ErrTimeout = errors.New("connection timeout")

	// ErrNoRelayAvailable 没有可用的 Relay
	ErrNoRelayAvailable = errors.New("no relay available")
)
