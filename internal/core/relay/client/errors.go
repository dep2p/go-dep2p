package client

import "errors"

var (
	// ErrClientClosed 客户端已关闭
	ErrClientClosed = errors.New("relay client closed")

	// ErrNotConnected 未连接
	ErrNotConnected = errors.New("not connected to relay")

	// ErrNoReservation 没有预约
	ErrNoReservation = errors.New("no reservation")

	// ErrReservationExpired 预约已过期
	ErrReservationExpired = errors.New("reservation expired")

	// ErrUnexpectedMessage 意外消息
	ErrUnexpectedMessage = errors.New("unexpected message type")

	// ErrPermissionDenied 权限被拒绝
	ErrPermissionDenied = errors.New("permission denied")

	// ErrResourceLimitExceeded 资源限制超出
	ErrResourceLimitExceeded = errors.New("resource limit exceeded")

	// ErrMalformedMessage 消息格式错误
	ErrMalformedMessage = errors.New("malformed message")

	// ErrUnknownStatus 未知状态
	ErrUnknownStatus = errors.New("unknown status")

	// ErrConnectionClosed 连接已关闭
	ErrConnectionClosed = errors.New("connection closed")

	// ErrNotSupported 不支持
	ErrNotSupported = errors.New("not supported")

	// ErrTargetUnreachable 目标节点不可达
	ErrTargetUnreachable = errors.New("target peer unreachable")

	// ErrProtocolError 协议错误
	ErrProtocolError = errors.New("relay protocol error")

	// ErrInternalError 内部错误
	ErrInternalError = errors.New("relay internal error")
)
