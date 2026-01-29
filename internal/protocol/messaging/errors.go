// Package messaging 实现点对点消息传递协议
package messaging

import "errors"

// 错误定义
var (
	// ErrNotStarted 服务未启动
	ErrNotStarted = errors.New("messaging: service not started")

	// ErrAlreadyStarted 服务已启动
	ErrAlreadyStarted = errors.New("messaging: service already started")

	// ErrInvalidProtocol 无效的协议格式
	ErrInvalidProtocol = errors.New("messaging: invalid protocol format")

	// ErrNotRealmMember 节点不是 Realm 成员
	ErrNotRealmMember = errors.New("messaging: peer is not realm member")

	// ErrHandlerNotFound 处理器未找到
	ErrHandlerNotFound = errors.New("messaging: handler not found")

	// ErrTimeout 请求超时
	ErrTimeout = errors.New("messaging: request timeout")

	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("messaging: stream closed")

	// ErrInvalidMessage 无效的消息格式
	ErrInvalidMessage = errors.New("messaging: invalid message format")

	// ErrNilHost Host 接口为 nil
	ErrNilHost = errors.New("messaging: host is nil")

	// ErrNilRealmManager RealmManager 接口为 nil
	ErrNilRealmManager = errors.New("messaging: realm manager is nil")

	// ErrHandlerAlreadyRegistered 处理器已注册
	ErrHandlerAlreadyRegistered = errors.New("messaging: handler already registered")
)
