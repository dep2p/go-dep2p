package network

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
)

// ErrReset 在读取或写入已重置的流时返回。
var ErrReset = errors.New("stream reset")

// MuxedStream 是连接中的双向IO管道。
type MuxedStream interface {
	io.Reader
	io.Writer

	// Close 关闭流。
	//
	// * 所有用于写入的缓冲数据都将被刷新。
	// * 未来的读取将失败。
	// * 任何正在进行的读/写操作都将被中断。
	//
	// Close 可能是异步的，并且不保证数据的接收。
	//
	// Close 同时关闭流的读取和写入。
	// Close 等同于调用 `CloseRead` 和 `CloseWrite`。重要的是，Close 不会等待任何形式的确认。
	// 如果需要确认，调用者必须调用 `CloseWrite`，然后等待流的响应(或EOF)，
	// 然后调用 Close() 来释放流对象。
	//
	// 当完成流的使用时，用户必须调用 Close() 或 `Reset()` 来丢弃流，即使在调用 `CloseRead` 和/或 `CloseWrite` 之后也是如此。
	io.Closer

	// CloseWrite 关闭流的写入但保持读取打开。
	//
	// CloseWrite 不会释放流，用户仍然必须调用 Close 或 Reset。
	CloseWrite() error

	// CloseRead 关闭流的读取但保持写入打开。
	//
	// 当调用 CloseRead 时，所有正在进行的 Read 调用都会被非 EOF 错误中断，并且后续的 Read 调用都将失败。
	//
	// 调用此函数后对流上新传入数据的处理由具体实现定义。
	//
	// CloseRead 不会释放流，用户仍然必须调用 Close 或 Reset。
	CloseRead() error

	// Reset 关闭流的两端。使用此方法告诉远程端挂断并离开。
	Reset() error

	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

// MuxedConn 表示到远程对等点的连接，该连接已扩展以支持流多路复用。
//
// MuxedConn 允许单个 net.Conn 连接承载多个逻辑独立的双向二进制数据流。
//
// 与 network.ConnSecurity 一起，MuxedConn 是 transport.CapableConn 接口的组件，
// 该接口表示已被"升级"以支持 dep2p 安全通信和流多路复用功能的"原始"网络连接。
type MuxedConn interface {
	// Close 关闭流多路复用器和底层的 net.Conn。
	io.Closer

	// IsClosed 返回连接是否完全关闭，以便可以进行垃圾回收。
	IsClosed() bool

	// OpenStream 创建一个新流。
	OpenStream(context.Context) (MuxedStream, error)

	// AcceptStream 接受由另一端打开的流。
	AcceptStream() (MuxedStream, error)
}

// Multiplexer 使用流多路复用实现包装 net.Conn，并返回支持在底层 net.Conn 上打开多个流的 MuxedConn
type Multiplexer interface {
	// NewConn 构造一个新的连接
	NewConn(c net.Conn, isServer bool, scope PeerScope) (MuxedConn, error)
}
