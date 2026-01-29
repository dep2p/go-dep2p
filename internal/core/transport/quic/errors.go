// Package quic 实现 QUIC 传输
package quic

import "errors"

var (
	// ErrTransportClosed 传输已关闭
	ErrTransportClosed = errors.New("transport closed")

	// ErrConnectionClosed 连接已关闭
	ErrConnectionClosed = errors.New("connection closed")

	// ErrInvalidAddress 无效地址
	ErrInvalidAddress = errors.New("invalid address")

	// ErrNoCertificate 没有证书
	ErrNoCertificate = errors.New("no TLS certificate available")
)
