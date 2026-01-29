package peerstore

import "errors"

var (
	// ErrNotFound 节点或数据未找到
	ErrNotFound = errors.New("peer not found")

	// ErrInvalidPublicKey 公钥与 PeerID 不匹配
	ErrInvalidPublicKey = errors.New("invalid public key for peer")

	// ErrInvalidAddr 无效地址
	ErrInvalidAddr = errors.New("invalid address")

	// ErrClosed 存储已关闭
	ErrClosed = errors.New("peerstore closed")
)
