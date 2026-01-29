// Package streams 实现流协议
package streams

import (
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// streamWrapper 实现 BiStream 接口
//
// 将 Host 层的 interfaces.Stream 适配为应用层的 interfaces.BiStream
//
// 并发安全设计：
//   - closed 标志使用 atomic 操作，避免在 IO 操作期间持有锁
//   - Read/Write 在 IO 之前检查 closed，但不在 IO 期间持锁
//   - 这允许 Close() 可以中断阻塞的 Read/Write（通过底层流的关闭机制）
type streamWrapper struct {
	stream   interfaces.Stream // Host 层流
	protocol atomic.Value      // 协议ID (string)
	opened   int64             // 打开时间戳
	closed   atomic.Bool       // 是否已关闭（使用 atomic 避免锁）
}

// 确保 streamWrapper 实现了 interfaces.BiStream 接口
var _ interfaces.BiStream = (*streamWrapper)(nil)

// newStreamWrapper 创建流包装器
func newStreamWrapper(stream interfaces.Stream, protocol string) *streamWrapper {
	w := &streamWrapper{
		stream: stream,
		opened: time.Now().Unix(),
	}
	w.protocol.Store(protocol)
	return w
}

// Read 从流读取数据
//
// 注意：不在 IO 期间持锁，以允许 Close() 中断阻塞的读取。
// 关闭流时，底层传输会中断阻塞的 Read 并返回错误。
func (w *streamWrapper) Read(p []byte) (int, error) {
	// 快速检查关闭状态（无锁）
	if w.closed.Load() {
		return 0, ErrStreamClosed
	}

	// 执行 IO（不持锁）
	// 如果在 Read 期间调用 Close()，底层流的 Close 会中断 Read
	n, err := w.stream.Read(p)

	// 如果流在读取期间被关闭，将错误包装为更友好的形式
	if err != nil && w.closed.Load() {
		return n, ErrStreamClosed
	}

	return n, err
}

// Write 向流写入数据
//
// 注意：不在 IO 期间持锁，以允许 Close() 中断阻塞的写入。
func (w *streamWrapper) Write(p []byte) (int, error) {
	// 快速检查关闭状态（无锁）
	if w.closed.Load() {
		return 0, ErrStreamClosed
	}

	// 执行 IO（不持锁）
	n, err := w.stream.Write(p)

	// 如果流在写入期间被关闭，将错误包装为更友好的形式
	if err != nil && w.closed.Load() {
		return n, ErrStreamClosed
	}

	return n, err
}

// Close 关闭流
//
// 使用 atomic CAS 确保只关闭一次。
// 关闭底层流会中断任何阻塞的 Read/Write 操作。
func (w *streamWrapper) Close() error {
	// CAS 确保只关闭一次
	if !w.closed.CompareAndSwap(false, true) {
		return nil // 已关闭
	}

	return w.stream.Close()
}

// Reset 重置流（异常关闭）
//
// 强制关闭流，不等待优雅关闭。
func (w *streamWrapper) Reset() error {
	// CAS 确保只关闭一次
	if !w.closed.CompareAndSwap(false, true) {
		return nil // 已关闭
	}

	return w.stream.Reset()
}

// Protocol 返回协议ID
func (w *streamWrapper) Protocol() string {
	if v := w.protocol.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// RemotePeer 返回远端节点ID
//
// 注意：如果连接已断开，可能返回空字符串。
// 调用者应检查返回值。
func (w *streamWrapper) RemotePeer() string {
	conn := w.stream.Conn()
	if conn == nil {
		return ""
	}
	return string(conn.RemotePeer())
}

// SetProtocol 设置协议ID（仅在协议协商阶段有效）
func (w *streamWrapper) SetProtocol(protocol string) error {
	if w.closed.Load() {
		return ErrStreamClosed
	}
	w.protocol.Store(protocol)
	return nil
}

// CloseRead 关闭读端
//
// 关闭后无法继续读取，但仍可写入。
func (w *streamWrapper) CloseRead() error {
	if w.closed.Load() {
		return nil
	}
	return w.stream.CloseRead()
}

// CloseWrite 关闭写端
//
// 关闭后无法继续写入，但仍可读取。
// 这会发送 FIN 信号告知对方"我已发送完毕"。
func (w *streamWrapper) CloseWrite() error {
	if w.closed.Load() {
		return nil
	}
	return w.stream.CloseWrite()
}

// SetDeadline 设置读写超时
//
// 设置读和写操作的截止时间。
// 超时后，Read 和 Write 会返回错误。
// 传入零值 time.Time{} 表示不超时。
func (w *streamWrapper) SetDeadline(t time.Time) error {
	if w.closed.Load() {
		return ErrStreamClosed
	}
	return w.stream.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (w *streamWrapper) SetReadDeadline(t time.Time) error {
	if w.closed.Load() {
		return ErrStreamClosed
	}
	return w.stream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (w *streamWrapper) SetWriteDeadline(t time.Time) error {
	if w.closed.Load() {
		return ErrStreamClosed
	}
	return w.stream.SetWriteDeadline(t)
}

// Stat 返回流统计信息
func (w *streamWrapper) Stat() interfaces.StreamStat {
	// 委托到底层 Stream 的 Stat() 方法
	streamStat := w.stream.Stat()

	// 转换为 BiStream 接口要求的 StreamStat 类型
	direction := interfaces.DirUnknown
	switch streamStat.Direction {
	case types.DirInbound:
		direction = interfaces.DirInbound
	case types.DirOutbound:
		direction = interfaces.DirOutbound
	}
	return interfaces.StreamStat{
		Direction: direction,
		Opened:    streamStat.Opened.Unix(),
		Protocol:  string(streamStat.Protocol),
	}
}

// IsClosed 检查流是否已关闭
//
// 同时检查本地 closed 标志和底层流状态。
func (w *streamWrapper) IsClosed() bool {
	return w.closed.Load() || w.stream.IsClosed()
}

// State 返回流当前状态
func (w *streamWrapper) State() types.StreamState {
	if w.closed.Load() {
		return types.StreamStateClosed
	}
	return w.stream.State()
}
