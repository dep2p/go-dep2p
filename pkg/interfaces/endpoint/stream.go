package endpoint

import (
	"io"
	"time"
)

// ============================================================================
//                              Stream 接口
// ============================================================================

// Stream 表示一个双向数据流
//
// Stream 是 Connection 上的逻辑通信通道，支持全双工通信。
// 多个 Stream 可以在单个 Connection 上并发运行，互不影响。
//
// Stream 实现了 io.Reader, io.Writer, io.Closer 接口，
// 可以直接用于标准库的 io 操作。
//
// 使用示例:
//
//	stream, _ := conn.OpenStream(ctx, "/echo/1.0")
//	defer stream.Close()
//
//	// 写入数据
//	stream.Write([]byte("Hello, World!"))
//
//	// 读取响应
//	buf := make([]byte, 1024)
//	n, _ := stream.Read(buf)
//	fmt.Println(string(buf[:n]))
type Stream interface {
	// 嵌入标准接口
	io.Reader
	io.Writer
	io.Closer

	// ==================== 元信息 ====================

	// ID 返回流 ID
	// 流 ID 在单个连接中是唯一的
	ID() StreamID

	// ProtocolID 返回协议 ID
	// 返回此流使用的协议标识符
	ProtocolID() ProtocolID

	// Connection 返回所属连接
	// 返回创建此流的连接
	Connection() Connection

	// ==================== 超时控制 ====================

	// SetDeadline 设置读写超时
	//
	// 设置读和写操作的截止时间。
	// 超时后，Read 和 Write 会返回错误。
	// 传入零值 time.Time{} 表示不超时。
	SetDeadline(t time.Time) error

	// SetReadDeadline 设置读超时
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline 设置写超时
	SetWriteDeadline(t time.Time) error

	// ==================== 半关闭 ====================

	// CloseRead 关闭读端
	//
	// 关闭流的读取端，不再接收数据。
	// 对端的写入会收到错误。
	// 此操作不影响写入端。
	CloseRead() error

	// CloseWrite 关闭写端
	//
	// 关闭流的写入端，发送 FIN 信号。
	// 对端会收到 EOF。
	// 此操作不影响读取端，仍可以读取对端发送的数据。
	CloseWrite() error

	// ==================== 流控制 ====================

	// SetPriority 设置流优先级
	//
	// 设置流的调度优先级。
	// 高优先级的流在资源竞争时会优先处理。
	SetPriority(priority Priority)

	// Priority 返回流优先级
	Priority() Priority

	// ==================== 统计信息 ====================

	// Stats 返回流统计
	Stats() StreamStats

	// ==================== 状态检查 ====================

	// IsClosed 检查流是否已关闭
	IsClosed() bool
}

// ============================================================================
//                              流状态
// ============================================================================

// StreamState 流状态
type StreamState int

const (
	// StreamStateOpen 流已打开
	StreamStateOpen StreamState = iota
	// StreamStateReadClosed 读端已关闭
	StreamStateReadClosed
	// StreamStateWriteClosed 写端已关闭
	StreamStateWriteClosed
	// StreamStateClosed 流已完全关闭
	StreamStateClosed
)

// String 返回状态的字符串表示
func (s StreamState) String() string {
	switch s {
	case StreamStateOpen:
		return "open"
	case StreamStateReadClosed:
		return "read_closed"
	case StreamStateWriteClosed:
		return "write_closed"
	case StreamStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              流事件
// ============================================================================

// 流事件类型
const (
	// EventStreamOpened 流已打开
	EventStreamOpened = "stream.opened"
	// EventStreamClosed 流已关闭
	EventStreamClosed = "stream.closed"
)

// StreamOpenedEvent 流打开事件
type StreamOpenedEvent struct {
	Stream     Stream
	ProtocolID ProtocolID
	Direction  Direction
}

// Type 返回事件类型
func (e StreamOpenedEvent) Type() string {
	return EventStreamOpened
}

// StreamClosedEvent 流关闭事件
type StreamClosedEvent struct {
	StreamID   StreamID
	ProtocolID ProtocolID
	Stats      StreamStats
}

// Type 返回事件类型
func (e StreamClosedEvent) Type() string {
	return EventStreamClosed
}

// 注意：StreamFactory 接口已删除（v1.1 清理）。
// 流创建通过 `internal/core/endpoint/stream.go` 中的 `NewStream` 函数直接实现，无需工厂模式。

