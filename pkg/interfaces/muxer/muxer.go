// Package muxer 定义多路复用接口
//
// 多路复用模块负责在单个连接上创建多个独立的流，包括：
// - 流的创建和管理
// - 流量控制
// - 优先级调度
package muxer

import (
	"context"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Muxer 接口
// ============================================================================

// Muxer 多路复用器接口
//
// Muxer 在单个底层连接上提供多个独立的逻辑流。
// 每个流都是双向的，支持独立的流量控制。
type Muxer interface {
	// NewStream 创建新流
	NewStream(ctx context.Context) (Stream, error)

	// AcceptStream 接受新流
	// 阻塞直到有新流到达或连接关闭
	AcceptStream() (Stream, error)

	// Close 关闭多路复用器
	// 所有流都会被关闭
	Close() error

	// IsClosed 检查是否已关闭
	IsClosed() bool

	// NumStreams 返回当前流数量
	NumStreams() int
}

// ============================================================================
//                              Stream 接口
// ============================================================================

// Stream 多路复用流接口
//
// Stream 是 Muxer 上的逻辑流，支持全双工通信。
type Stream interface {
	io.ReadWriteCloser

	// ID 返回流 ID
	// 在单个 Muxer 中唯一
	ID() uint32

	// SetDeadline 设置读写超时
	SetDeadline(t time.Time) error

	// SetReadDeadline 设置读超时
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline 设置写超时
	SetWriteDeadline(t time.Time) error

	// CloseRead 关闭读端
	// 对端的写入会收到错误
	CloseRead() error

	// CloseWrite 关闭写端
	// 发送 FIN，对端会收到 EOF
	CloseWrite() error

	// Reset 重置流
	// 立即关闭流，发送 RST
	Reset() error
}

// ============================================================================
//                              MuxerFactory 接口
// ============================================================================

// MuxerFactory 多路复用器工厂接口
//
// 用于从底层连接创建多路复用器。
type MuxerFactory interface {
	// NewMuxer 从连接创建多路复用器
	// isServer 表示是否是服务端
	NewMuxer(conn io.ReadWriteCloser, isServer bool) (Muxer, error)

	// Protocol 返回协议名称
	// 如 "yamux", "mplex"
	Protocol() string
}

// ============================================================================
//                              v1.1 接口清理说明
// ============================================================================

// 注意：以下接口已删除（v1.1 清理）：
// - FlowController：yamux/QUIC 内置流控，无需外部接口
// - PriorityScheduler：当前无优先级调度需求

// ============================================================================
//                              统计信息
// ============================================================================

// Stats 多路复用器统计（类型别名，实际定义在 types 包）
type Stats = types.MuxerStats

// ============================================================================
//                              配置
// ============================================================================

// Config 多路复用配置
type Config struct {
	// MaxStreams 最大流数
	MaxStreams int

	// MaxStreamWindowSize 最大流窗口大小
	MaxStreamWindowSize uint32

	// MaxSessionWindowSize 最大会话窗口大小
	MaxSessionWindowSize uint32

	// StreamReceiveWindow 流接收窗口
	StreamReceiveWindow uint32

	// KeepAliveInterval 保活间隔
	KeepAliveInterval time.Duration

	// KeepAliveTimeout 保活超时
	KeepAliveTimeout time.Duration

	// EnableKeepAlive 是否启用保活
	EnableKeepAlive bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MaxStreams:           256,
		MaxStreamWindowSize:  256 * 1024,       // 256 KB
		MaxSessionWindowSize: 16 * 1024 * 1024, // 16 MB
		StreamReceiveWindow:  256 * 1024,       // 256 KB
		KeepAliveInterval:    30 * time.Second,
		KeepAliveTimeout:     10 * time.Second,
		EnableKeepAlive:      true,
	}
}
