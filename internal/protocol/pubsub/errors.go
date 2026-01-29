// Package pubsub 实现发布订阅协议
package pubsub

import "errors"

// 错误定义
var (
	// ErrNotStarted 服务未启动
	ErrNotStarted = errors.New("pubsub: service not started")

	// ErrAlreadyStarted 服务已启动
	ErrAlreadyStarted = errors.New("pubsub: service already started")

	// ErrTopicNotFound 主题未找到
	ErrTopicNotFound = errors.New("pubsub: topic not found")

	// ErrTopicAlreadyJoined 主题已加入
	ErrTopicAlreadyJoined = errors.New("pubsub: topic already joined")

	// ErrTopicClosed 主题已关闭
	ErrTopicClosed = errors.New("pubsub: topic closed")

	// ErrSubscriptionCancelled 订阅已取消
	ErrSubscriptionCancelled = errors.New("pubsub: subscription cancelled")

	// ErrInvalidMessage 无效的消息
	ErrInvalidMessage = errors.New("pubsub: invalid message")

	// ErrMessageTooLarge 消息过大
	ErrMessageTooLarge = errors.New("pubsub: message too large")

	// ErrNotRealmMember 节点不是 Realm 成员
	ErrNotRealmMember = errors.New("pubsub: peer is not realm member")

	// ErrDuplicateMessage 重复消息
	ErrDuplicateMessage = errors.New("pubsub: duplicate message")

	// ErrInvalidSignature 无效签名
	ErrInvalidSignature = errors.New("pubsub: invalid signature")

	// ErrNilHost Host 为 nil
	ErrNilHost = errors.New("pubsub: host is nil")

	// ErrNilRealmManager RealmManager 为 nil
	ErrNilRealmManager = errors.New("pubsub: realm manager is nil")

	// ErrMeshFull Mesh 已满
	ErrMeshFull = errors.New("pubsub: mesh is full")

	// ErrNotInMesh 节点不在 Mesh 中
	ErrNotInMesh = errors.New("pubsub: peer not in mesh")

	// ErrNoConnectedPeers 没有可连接的节点
	// Phase 5.1: Publish 语义修复 - 无连接时返回错误，不静默成功
	ErrNoConnectedPeers = errors.New("pubsub: no connected peers in mesh")

	// ErrAllSendsFailed 所有发送都失败
	ErrAllSendsFailed = errors.New("pubsub: all sends failed")
)
