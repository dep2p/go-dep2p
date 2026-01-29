// Package upgrader 实现连接升级器
package upgrader

import "errors"

var (
	// ErrNilIdentity 身份为空
	ErrNilIdentity = errors.New("upgrader: identity is nil")

	// ErrNoPeerID 缺少 PeerID
	ErrNoPeerID = errors.New("upgrader: outbound connection requires remote peer ID")

	// ErrNoSecurityTransport 没有安全传输
	ErrNoSecurityTransport = errors.New("upgrader: no security transport configured")

	// ErrNoStreamMuxer 没有流复用器
	ErrNoStreamMuxer = errors.New("upgrader: no stream muxer configured")

	// ErrNegotiationFailed 协商失败
	ErrNegotiationFailed = errors.New("upgrader: protocol negotiation failed")

	// ErrHandshakeFailed 握手失败
	ErrHandshakeFailed = errors.New("upgrader: handshake failed")

	// ErrMuxerSetupFailed 多路复用器设置失败
	ErrMuxerSetupFailed = errors.New("upgrader: muxer setup failed")
)
