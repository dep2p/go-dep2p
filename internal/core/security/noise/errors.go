// Package noise 实现 Noise 协议安全传输
package noise

import "errors"

var (
	// ErrNotImplemented Noise 协议未完全实现
	ErrNotImplemented = errors.New("noise: not implemented")

	// ErrInvalidHandshake 握手失败
	ErrInvalidHandshake = errors.New("noise: invalid handshake")

	// ErrPeerIDMismatch PeerID 不匹配
	ErrPeerIDMismatch = errors.New("noise: peer ID mismatch")
)
