// Package transport 实现传输层
package transport

import "errors"

var (
	// ErrNoTransport 没有可用的传输
	ErrNoTransport = errors.New("no suitable transport for address")

	// ErrInvalidAddress 无效地址
	ErrInvalidAddress = errors.New("invalid multiaddr")

	// ErrPeerIDMismatch PeerID 不匹配
	ErrPeerIDMismatch = errors.New("peer ID mismatch")
)
