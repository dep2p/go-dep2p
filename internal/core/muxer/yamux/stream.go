// Package yamux 提供基于 yamux 的多路复用实现
package yamux

import (
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

// Stream 封装 yamux.Stream，实现 muxer.Stream 接口
type Stream struct {
	stream *yamux.Stream
	id     uint32
	closed atomic.Bool // 使用 atomic.Bool 简化并发控制
}

// 确保实现 muxer.Stream 接口
var _ muxerif.Stream = (*Stream)(nil)

// NewStream 创建 Stream 封装
func NewStream(s *yamux.Stream) *Stream {
	return &Stream{
		stream: s,
		id:     s.StreamID(),
	}
}

// Read 从流中读取数据
func (s *Stream) Read(p []byte) (int, error) {
	return s.stream.Read(p)
}

// Write 向流写入数据
func (s *Stream) Write(p []byte) (int, error) {
	return s.stream.Write(p)
}

// Close 关闭流
func (s *Stream) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil // 已经关闭
	}
	return s.stream.Close()
}

// ID 返回流 ID
func (s *Stream) ID() uint32 {
	return s.id
}

// SetDeadline 设置读写超时
func (s *Stream) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

// CloseRead 关闭读端
//
// 注意: yamux 不直接支持真正的半关闭。此方法通过设置过去的读截止时间来
// 模拟关闭读端，后续的读取操作将立即返回超时错误。
// 这不是真正的 TCP 半关闭，只是阻止进一步读取。
// 如果需要重新启用读取，可以调用 SetReadDeadline(time.Time{})。
func (s *Stream) CloseRead() error {
	return s.stream.SetReadDeadline(time.Now())
}

// CloseWrite 关闭写端
//
// 注意: yamux 的 Close() 会同时关闭读写两端（发送 FIN）。
// 调用此方法后，流将完全关闭，不仅仅是写端。
// 这是 yamux 的限制，如需真正的半关闭语义，请考虑使用其他多路复用实现。
func (s *Stream) CloseWrite() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil // 已经关闭
	}
	return s.stream.Close()
}

// Reset 重置流
// 立即关闭流并发送 RST
func (s *Stream) Reset() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil // 已经关闭
	}

	// yamux 使用 Close 来重置流
	// 虽然不是真正的 RST，但效果类似
	return s.stream.Close()
}

// IsClosed 检查流是否已关闭
func (s *Stream) IsClosed() bool {
	return s.closed.Load()
}

