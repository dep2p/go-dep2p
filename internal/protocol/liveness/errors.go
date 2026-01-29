// Package liveness 实现存活检测服务
package liveness

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

	// ErrPingTimeout Ping超时
	ErrPingTimeout = errors.New("ping timeout")

	// ErrPingFailed Ping失败
	ErrPingFailed = errors.New("ping failed")

	// ErrNoRealm 未找到Realm
	ErrNoRealm = errors.New("no realm found")

	// ErrNotMember 非Realm成员
	ErrNotMember = errors.New("not a realm member")

	// ErrInvalidPeerID 无效节点ID
	ErrInvalidPeerID = errors.New("invalid peer id")

	// ErrWatchNotFound Watch未找到
	ErrWatchNotFound = errors.New("watch not found")

	// ErrInvalidMessage 无效消息
	ErrInvalidMessage = errors.New("invalid message")
)
