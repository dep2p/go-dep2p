package addressbook

import "errors"

// 地址簿错误定义
var (
	// ErrMemberNotFound 成员未找到
	ErrMemberNotFound = errors.New("addressbook: member not found")

	// ErrMemberExists 成员已存在
	ErrMemberExists = errors.New("addressbook: member already exists")

	// ErrStoreClosed 存储已关闭
	ErrStoreClosed = errors.New("addressbook: store is closed")

	// ErrInvalidEntry 无效的条目
	ErrInvalidEntry = errors.New("addressbook: invalid entry")

	// ErrInvalidNodeID 无效的节点 ID
	ErrInvalidNodeID = errors.New("addressbook: invalid node ID")

	// ErrBookClosed 地址簿已关闭
	ErrBookClosed = errors.New("addressbook: book is closed")

	// ErrEngineRequired BadgerDB 存储需要提供引擎
	ErrEngineRequired = errors.New("addressbook: storage engine is required for badger store")

	// ErrProtocolNotSupported 远端不支持地址簿协议
	ErrProtocolNotSupported = errors.New("addressbook: protocol not supported")
)
