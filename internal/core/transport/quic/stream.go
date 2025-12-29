// Package quic 提供基于 QUIC 的传输层实现
package quic

import (
	"time"

	"github.com/quic-go/quic-go"

	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// Stream QUIC 流封装
// 注意: quic.Stream 是 quic-go 的结构体类型
type Stream struct {
	quicStream *quic.Stream
	conn       *Conn
}

// 确保实现 transport.Stream 接口
var _ transportif.Stream = (*Stream)(nil)

// NewStream 创建流封装
func NewStream(qs *quic.Stream, conn *Conn) *Stream {
	return &Stream{
		quicStream: qs,
		conn:       conn,
	}
}

// Read 从流中读取数据
func (s *Stream) Read(p []byte) (int, error) {
	return s.quicStream.Read(p)
}

// Write 向流写入数据
func (s *Stream) Write(p []byte) (int, error) {
	return s.quicStream.Write(p)
}

// Close 关闭流
func (s *Stream) Close() error {
	return s.quicStream.Close()
}

// ID 返回流 ID
func (s *Stream) ID() uint64 {
	return uint64(s.quicStream.StreamID())
}

// SetDeadline 设置读写超时
func (s *Stream) SetDeadline(t time.Time) error {
	return s.quicStream.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.quicStream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.quicStream.SetWriteDeadline(t)
}

// CloseRead 关闭读端
func (s *Stream) CloseRead() error {
	s.quicStream.CancelRead(0)
	return nil
}

// CloseWrite 关闭写端
func (s *Stream) CloseWrite() error {
	return s.quicStream.Close()
}

// Conn 返回所属连接
func (s *Stream) Conn() *Conn {
	return s.conn
}

// QuicStream 返回底层 QUIC 流
func (s *Stream) QuicStream() *quic.Stream {
	return s.quicStream
}
