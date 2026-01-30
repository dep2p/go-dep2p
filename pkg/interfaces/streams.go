// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Streams 接口，提供双向流服务。
package interfaces

import (
	"context"
	"io"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════
//                         流优先级类型 (v1.2 新增)
// ════════════════════════════════════════════════════════════════════════════

// StreamPriority 流优先级
//
// QUIC (RFC 9000) 原生支持流优先级，用于在网络拥塞时优先调度重要流。
// 优先级数值越小，优先级越高。
type StreamPriority int

const (
	// StreamPriorityCritical 关键优先级 (共识消息: proposal, vote, commit)
	//
	// 用于区块链共识协议，必须保证最低延迟。
	StreamPriorityCritical StreamPriority = 0

	// StreamPriorityHigh 高优先级 (交易广播)
	//
	// 用于交易传播，需要较低延迟。
	StreamPriorityHigh StreamPriority = 1

	// StreamPriorityNormal 普通优先级 (默认)
	//
	// 用于一般数据传输，如区块头同步。
	StreamPriorityNormal StreamPriority = 2

	// StreamPriorityLow 低优先级 (历史数据、区块体同步)
	//
	// 用于非实时数据传输，可以被高优先级流抢占带宽。
	StreamPriorityLow StreamPriority = 3
)

// String 返回优先级名称
func (p StreamPriority) String() string {
	switch p {
	case StreamPriorityCritical:
		return "critical"
	case StreamPriorityHigh:
		return "high"
	case StreamPriorityNormal:
		return "normal"
	case StreamPriorityLow:
		return "low"
	default:
		return "unknown"
	}
}

// StreamOptions 流选项 (v1.2 新增)
//
// 用于创建流时指定选项，如优先级。
type StreamOptions struct {
	// Priority 流优先级
	//
	// 默认为 StreamPriorityNormal。
	// 仅在 QUIC 连接上生效，TCP 连接会忽略此选项。
	Priority StreamPriority
}

// DefaultStreamOptions 返回默认流选项
func DefaultStreamOptions() StreamOptions {
	return StreamOptions{
		Priority: StreamPriorityNormal,
	}
}

// ════════════════════════════════════════════════════════════════════════════

// Streams 定义流服务接口
//
// Streams 提供原始的双向流通信能力。
type Streams interface {
	// Open 打开到指定节点的流（默认优先级）
	Open(ctx context.Context, peerID string, protocol string) (BiStream, error)

	// OpenWithOptions 打开到指定节点的流（指定选项）(v1.2 新增)
	//
	// 允许指定流选项，如优先级。
	// 在 QUIC 连接上，优先级会传递给底层传输层，实现流级别的 QoS。
	// 在 TCP 连接上，优先级选项会被忽略（优雅降级）。
	//
	// 示例：
	//
	//	// 创建高优先级流用于共识消息
	//	stream, err := streams.OpenWithOptions(ctx, peerID, "consensus",
	//	    interfaces.StreamOptions{Priority: interfaces.StreamPriorityCritical})
	//
	//	// 创建低优先级流用于历史数据同步
	//	stream, err := streams.OpenWithOptions(ctx, peerID, "sync",
	//	    interfaces.StreamOptions{Priority: interfaces.StreamPriorityLow})
	OpenWithOptions(ctx context.Context, peerID string, protocol string, opts StreamOptions) (BiStream, error)

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
