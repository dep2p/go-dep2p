package protocol

import "errors"

// 协议模块错误定义
var (
	// ErrProtocolNotRegistered 协议未注册
	ErrProtocolNotRegistered = errors.New("protocol: protocol not registered")

	// ErrDuplicateProtocol 协议已注册
	ErrDuplicateProtocol = errors.New("protocol: protocol already registered")

	// ErrInvalidProtocolID 无效的协议 ID
	ErrInvalidProtocolID = errors.New("protocol: invalid protocol ID")

	// ErrNegotiationFailed 协议协商失败
	ErrNegotiationFailed = errors.New("protocol: negotiation failed")

	// ErrNoHandler 没有处理器
	ErrNoHandler = errors.New("protocol: no handler for protocol")
)
