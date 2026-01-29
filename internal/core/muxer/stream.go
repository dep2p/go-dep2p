package muxer

import (
	"time"

	"github.com/libp2p/go-yamux/v5"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// muxedStream 包装 yamux.Stream，实现 MuxedStream 接口
type muxedStream struct {
	stream *yamux.Stream
}

// 确保实现接口
var _ pkgif.MuxedStream = (*muxedStream)(nil)

// Read 从流中读取数据
func (s *muxedStream) Read(p []byte) (n int, err error) {
	n, err = s.stream.Read(p)
	return n, parseError(err)
}

// Write 向流中写入数据
func (s *muxedStream) Write(p []byte) (n int, err error) {
	n, err = s.stream.Write(p)
	return n, parseError(err)
}

// Close 关闭流（正常关闭）
func (s *muxedStream) Close() error {
	return s.stream.Close()
}

// CloseWrite 关闭写端
func (s *muxedStream) CloseWrite() error {
	return s.stream.CloseWrite()
}

// CloseRead 关闭读端
func (s *muxedStream) CloseRead() error {
	return s.stream.CloseRead()
}

// Reset 重置流（异常关闭）
func (s *muxedStream) Reset() error {
	return s.stream.Reset()
}

// SetDeadline 设置读写截止时间
func (s *muxedStream) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

// SetReadDeadline 设置读截止时间
func (s *muxedStream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写截止时间
func (s *muxedStream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}
