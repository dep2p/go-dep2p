// Package quic 提供基于 QUIC 的传输层实现
package quic

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

// MuxerAdapter 将 QUIC 连接适配为 muxerif.Muxer 接口
// QUIC 协议内置多路复用，此适配器将其暴露给 endpoint 层
type MuxerAdapter struct {
	qc          *quic.Conn
	closed      atomic.Bool
	streamCount atomic.Int32
}

// 确保实现 muxerif.Muxer 接口
var _ muxerif.Muxer = (*MuxerAdapter)(nil)

// NewMuxerAdapter 从 QUIC 连接创建 muxer 适配器
func NewMuxerAdapter(qc *quic.Conn) *MuxerAdapter {
	return &MuxerAdapter{
		qc: qc,
	}
}

// NewStream 创建新流
func (m *MuxerAdapter) NewStream(ctx context.Context) (muxerif.Stream, error) {
	if m.closed.Load() {
		return nil, errors.New("muxer already closed")
	}

	stream, err := m.qc.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	m.streamCount.Add(1)
	return newStreamAdapter(stream, m), nil
}

// AcceptStream 接受新流
func (m *MuxerAdapter) AcceptStream() (muxerif.Stream, error) {
	if m.closed.Load() {
		return nil, errors.New("muxer already closed")
	}

	stream, err := m.qc.AcceptStream(m.qc.Context())
	if err != nil {
		return nil, err
	}

	m.streamCount.Add(1)
	return newStreamAdapter(stream, m), nil
}

// Close 关闭多路复用器
func (m *MuxerAdapter) Close() error {
	if m.closed.Swap(true) {
		return nil
	}
	return m.qc.CloseWithError(0, "muxer closed")
}

// IsClosed 检查是否已关闭
func (m *MuxerAdapter) IsClosed() bool {
	return m.closed.Load()
}

// NumStreams 返回当前流数量
func (m *MuxerAdapter) NumStreams() int {
	return int(m.streamCount.Load())
}

// decrementStreamCount 减少流计数
func (m *MuxerAdapter) decrementStreamCount() {
	m.streamCount.Add(-1)
}

// StreamAdapter 将 quic.Stream 适配为 muxerif.Stream 接口
type StreamAdapter struct {
	stream *quic.Stream
	muxer  *MuxerAdapter
	closed atomic.Bool
}

// 确保实现 muxerif.Stream 接口
var _ muxerif.Stream = (*StreamAdapter)(nil)

// newStreamAdapter 创建流适配器
func newStreamAdapter(stream *quic.Stream, muxer *MuxerAdapter) *StreamAdapter {
	return &StreamAdapter{
		stream: stream,
		muxer:  muxer,
	}
}

// Read 从流读取数据
func (s *StreamAdapter) Read(p []byte) (int, error) {
	return (*s.stream).Read(p)
}

// Write 向流写入数据
func (s *StreamAdapter) Write(p []byte) (int, error) {
	return (*s.stream).Write(p)
}

// Close 关闭流
func (s *StreamAdapter) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	if s.muxer != nil {
		s.muxer.decrementStreamCount()
	}
	return (*s.stream).Close()
}

// ID 返回流 ID
func (s *StreamAdapter) ID() uint32 {
	return uint32((*s.stream).StreamID())
}

// SetDeadline 设置读写超时
func (s *StreamAdapter) SetDeadline(t time.Time) error {
	return (*s.stream).SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (s *StreamAdapter) SetReadDeadline(t time.Time) error {
	return (*s.stream).SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (s *StreamAdapter) SetWriteDeadline(t time.Time) error {
	return (*s.stream).SetWriteDeadline(t)
}

// CloseRead 关闭读端
func (s *StreamAdapter) CloseRead() error {
	(*s.stream).CancelRead(0)
	return nil
}

// CloseWrite 关闭写端
func (s *StreamAdapter) CloseWrite() error {
	return (*s.stream).Close()
}

// Reset 重置流
func (s *StreamAdapter) Reset() error {
	(*s.stream).CancelRead(0)
	(*s.stream).CancelWrite(0)
	if s.closed.Swap(true) {
		return nil
	}
	if s.muxer != nil {
		s.muxer.decrementStreamCount()
	}
	return nil
}

// MuxerAdapterFactory 创建 QUIC muxer 适配器的工厂
type MuxerAdapterFactory struct{}

// 确保实现 muxerif.MuxerFactory 接口
var _ muxerif.MuxerFactory = (*MuxerAdapterFactory)(nil)

// NewMuxer 从连接创建多路复用器
// 注意：对于 QUIC，我们期望接收到的 conn 是 *quic.Conn 或包装了它的类型
func (f *MuxerAdapterFactory) NewMuxer(conn io.ReadWriteCloser, _ bool) (muxerif.Muxer, error) {
	// 尝试从连接中获取 QUIC 连接
	switch c := conn.(type) {
	case *Conn:
		// 我们的 QUIC Conn 包装器
		return NewMuxerAdapter(c.QuicConn()), nil
	case interface{ QuicConn() *quic.Conn }:
		// 任何暴露 QuicConn() 方法的类型
		return NewMuxerAdapter(c.QuicConn()), nil
	default:
		return nil, errors.New("连接不是 QUIC 连接")
	}
}

// Protocol 返回协议名称
func (f *MuxerAdapterFactory) Protocol() string {
	return "quic"
}

