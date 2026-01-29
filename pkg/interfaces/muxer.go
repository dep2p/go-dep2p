// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Muxer 接口，抽象流多路复用协议。
package interfaces

import (
	"context"
	"net"
	"time"
)

// StreamMuxer 定义流多路复用器接口
//
// StreamMuxer 允许在单个连接上创建多个独立的流。
type StreamMuxer interface {
	// NewConn 在网络连接上创建多路复用连接
	NewConn(conn net.Conn, isServer bool, scope PeerScope) (MuxedConn, error)

	// ID 返回多路复用协议标识
	ID() string
}

// MuxedConn 定义多路复用连接接口
type MuxedConn interface {
	// OpenStream 打开新流
	OpenStream(ctx context.Context) (MuxedStream, error)

	// AcceptStream 接受新流
	AcceptStream() (MuxedStream, error)

	// Close 关闭连接
	Close() error

	// IsClosed 检查连接是否已关闭
	IsClosed() bool
}

// MuxedStream 定义多路复用流接口
type MuxedStream interface {
	// Read 从流中读取数据
	Read(p []byte) (n int, err error)

	// Write 向流中写入数据
	Write(p []byte) (n int, err error)

	// Close 关闭流（正常关闭）
	Close() error

	// CloseWrite 关闭写端
	CloseWrite() error

	// CloseRead 关闭读端
	CloseRead() error

	// Reset 重置流（异常关闭）
	Reset() error

	// SetDeadline 设置读写截止时间
	SetDeadline(t time.Time) error

	// SetReadDeadline 设置读截止时间
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline 设置写截止时间
	SetWriteDeadline(t time.Time) error
}

// StreamMuxerMultiplexer 流多路复用协议选择器
type StreamMuxerMultiplexer interface {
	// NewConn 创建多路复用连接
	NewConn(conn net.Conn, isServer bool, scope PeerScope) (MuxedConn, string, error)
}
