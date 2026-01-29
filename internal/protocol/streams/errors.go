// Package streams 实现流协议
package streams

import "errors"

// 定义错误
var (
	// ErrNilHost Host为nil
	ErrNilHost = errors.New("host is nil")

	// ErrNilRealmManager RealmManager为nil
	ErrNilRealmManager = errors.New("realm manager is nil")

	// ErrAlreadyStarted 服务已启动
	ErrAlreadyStarted = errors.New("service already started")

	// ErrNotStarted 服务未启动
	ErrNotStarted = errors.New("service not started")

	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("stream closed")

	// ErrEmptyProtocol 协议为空
	ErrEmptyProtocol = errors.New("protocol is empty")

	// ErrInvalidProtocol 无效协议
	ErrInvalidProtocol = errors.New("invalid protocol")

	// ErrHandlerNotFound 处理器未找到
	ErrHandlerNotFound = errors.New("handler not found")

	// ErrHandlerExists 处理器已存在
	ErrHandlerExists = errors.New("handler already exists")

	// ErrNoRealm 未找到Realm
	ErrNoRealm = errors.New("no realm found")

	// ErrNotMember 非Realm成员
	ErrNotMember = errors.New("not a realm member")

	// ErrInvalidPeerID 无效节点ID
	ErrInvalidPeerID = errors.New("invalid peer id")

	// ErrContextCanceled 上下文已取消
	ErrContextCanceled = errors.New("context canceled")

	// ErrTimeout 超时
	ErrTimeout = errors.New("timeout")
)
