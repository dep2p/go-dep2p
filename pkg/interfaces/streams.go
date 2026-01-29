// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Streams 接口，提供双向流服务。
package interfaces

import (
	"context"
	"io"
	"time"
)

// Streams 定义流服务接口
//
// Streams 提供原始的双向流通信能力。
type Streams interface {
	// Open 打开到指定节点的流
	Open(ctx context.Context, peerID string, protocol string) (BiStream, error)

	// RegisterHandler 注册流处理器
	RegisterHandler(protocol string, handler BiStreamHandler) error

	// UnregisterHandler 注销流处理器
	UnregisterHandler(protocol string) error

	// Close 关闭服务
	Close() error
}

// BiStreamHandler 双向流处理函数类型
type BiStreamHandler func(stream BiStream)

// BiStream 定义双向流接口
type BiStream interface {
	io.Reader
	io.Writer
	io.Closer

	// Protocol 返回流使用的协议
	Protocol() string

	// RemotePeer 返回远端节点 ID
	//
	// 注意：如果连接已断开，可能返回空字符串。
	// 调用者应检查返回值。
	RemotePeer() string

	// SetProtocol 设置协议（仅在协议协商阶段有效）
	SetProtocol(protocol string) error

	// Reset 重置流（异常关闭）
	Reset() error

	// CloseRead 关闭读端
	CloseRead() error

	// CloseWrite 关闭写端
	CloseWrite() error

	// SetDeadline 设置读写超时
	//
	// 设置读和写操作的截止时间。
	// 超时后，Read 和 Write 会返回错误。
	// 传入零值 time.Time{} 表示不超时。
	//
	// 重要：对于长时间运行的流，建议设置超时以避免 goroutine 泄漏。
	SetDeadline(t time.Time) error

	// SetReadDeadline 设置读超时
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline 设置写超时
	SetWriteDeadline(t time.Time) error

	// Stat 返回流统计信息
	Stat() StreamStat
}

// StreamStat 流统计信息
type StreamStat struct {
	// Direction 流方向
	Direction Direction

	// Opened 流打开时间
	Opened int64

	// Protocol 使用的协议
	Protocol string
}

// StreamReadWriter 流读写包装器
type StreamReadWriter struct {
	stream BiStream
}

// NewStreamReadWriter 创建流读写包装器
func NewStreamReadWriter(s BiStream) *StreamReadWriter {
	return &StreamReadWriter{stream: s}
}

// Read 从流读取
func (rw *StreamReadWriter) Read(p []byte) (int, error) {
	return rw.stream.Read(p)
}

// Write 向流写入
func (rw *StreamReadWriter) Write(p []byte) (int, error) {
	return rw.stream.Write(p)
}

// Close 关闭流
func (rw *StreamReadWriter) Close() error {
	return rw.stream.Close()
}
