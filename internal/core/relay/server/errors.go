package server

import "errors"

var (
	// ErrServerClosed 服务端已关闭
	ErrServerClosed = errors.New("relay server closed")

	// ErrMalformedMessage 消息格式错误
	ErrMalformedMessage = errors.New("malformed message")

	// ErrUnexpectedMessage 意外消息
	ErrUnexpectedMessage = errors.New("unexpected message type")

	// ErrPermissionDenied 权限被拒绝
	ErrPermissionDenied = errors.New("permission denied")

	// ErrNoReservation 没有预约
	ErrNoReservation = errors.New("no reservation")

	// ErrResourceLimitExceeded 资源限制超出
	ErrResourceLimitExceeded = errors.New("resource limit exceeded")
)
