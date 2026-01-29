// Package tcp 实现 TCP 传输
package tcp

import "errors"

var (
	// ErrTransportClosed 传输已关闭
	ErrTransportClosed = errors.New("transport closed")

	// ErrConnectionClosed 连接已关闭
	ErrConnectionClosed = errors.New("connection closed")

	// ErrNoMuxer TCP 连接需要 Muxer
	ErrNoMuxer = errors.New("TCP connection requires muxer for streams")
)
