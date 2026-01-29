package rendezvous

import (
	"errors"
)

// 预定义错误
var (
	// ErrInvalidNamespace 无效的命名空间
	ErrInvalidNamespace = errors.New("rendezvous: invalid namespace")

	// ErrInvalidTTL 无效的 TTL
	ErrInvalidTTL = errors.New("rendezvous: invalid TTL")

	// ErrInvalidCookie 无效的分页 cookie
	ErrInvalidCookie = errors.New("rendezvous: invalid cookie")

	// ErrNotAuthorized 未授权
	ErrNotAuthorized = errors.New("rendezvous: not authorized")

	// ErrInternalError 内部错误
	ErrInternalError = errors.New("rendezvous: internal error")

	// ErrUnavailable 服务不可用
	ErrUnavailable = errors.New("rendezvous: service unavailable")

	// ErrMessageTooLarge 消息过大
	ErrMessageTooLarge = errors.New("rendezvous: message too large")

	// ErrInvalidMessage 无效消息
	ErrInvalidMessage = errors.New("rendezvous: invalid message")

	// ErrAlreadyStarted 已启动
	ErrAlreadyStarted = errors.New("rendezvous: already started")

	// ErrNotStarted 未启动
	ErrNotStarted = errors.New("rendezvous: not started")

	// ErrNoPoints 没有可用的 Rendezvous 点
	ErrNoPoints = errors.New("rendezvous: no points available")

	// ErrAllPointsFailed 所有 Rendezvous 点都失败
	ErrAllPointsFailed = errors.New("rendezvous: all points failed")

	// ErrMaxRegistrationsExceeded 超过最大注册数
	ErrMaxRegistrationsExceeded = errors.New("rendezvous: max registrations exceeded")

	// ErrMaxNamespacesExceeded 超过最大命名空间数
	ErrMaxNamespacesExceeded = errors.New("rendezvous: max namespaces exceeded")

	// ErrMaxRegistrationsPerNamespaceExceeded 超过每命名空间最大注册数
	ErrMaxRegistrationsPerNamespaceExceeded = errors.New("rendezvous: max registrations per namespace exceeded")

	// ErrMaxRegistrationsPerPeerExceeded 超过每节点最大注册数
	ErrMaxRegistrationsPerPeerExceeded = errors.New("rendezvous: max registrations per peer exceeded")

	// ErrNilHost Host 为空
	ErrNilHost = errors.New("rendezvous: host is nil")

	// ErrStoreClosed 存储已关闭
	ErrStoreClosed = errors.New("rendezvous: store is closed")
)
