package yamux

import (
	"time"

	"github.com/dep2p/go-dep2p/core/network"

	"github.com/dep2p/go-dep2p/p2plib/yamux"
)

// stream 在 yamux.Stream 之上实现了 mux.MuxedStream 接口
type stream yamux.Stream

var _ network.MuxedStream = &stream{}

// Read 从流中读取数据
// 参数:
//   - b: 用于存储读取数据的字节切片
//
// 返回:
//   - n: 实际读取的字节数
//   - err: 读取过程中的错误,如果流被重置则返回 network.ErrReset
func (s *stream) Read(b []byte) (n int, err error) {
	// 调用底层yamux流的Read方法读取数据
	n, err = s.yamux().Read(b)
	// 如果发生流重置错误,转换为network包定义的错误类型
	if err == yamux.ErrStreamReset {
		err = network.ErrReset
	}

	return n, err
}

// Write 向流中写入数据
// 参数:
//   - b: 要写入的数据字节切片
//
// 返回:
//   - n: 实际写入的字节数
//   - err: 写入过程中的错误,如果流被重置则返回 network.ErrReset
func (s *stream) Write(b []byte) (n int, err error) {
	// 调用底层yamux流的Write方法写入数据
	n, err = s.yamux().Write(b)
	// 如果发生流重置错误,转换为network包定义的错误类型
	if err == yamux.ErrStreamReset {
		err = network.ErrReset
	}

	return n, err
}

// Close 关闭流
// 返回:
//   - error: 关闭过程中的错误
func (s *stream) Close() error {
	// 调用底层yamux流的Close方法关闭流
	return s.yamux().Close()
}

// Reset 重置流
// 返回:
//   - error: 重置过程中的错误
func (s *stream) Reset() error {
	// 调用底层yamux流的Reset方法重置流
	return s.yamux().Reset()
}

// CloseRead 关闭流的读取端
// 返回:
//   - error: 关闭读取端过程中的错误
func (s *stream) CloseRead() error {
	// 调用底层yamux流的CloseRead方法关闭读取端
	return s.yamux().CloseRead()
}

// CloseWrite 关闭流的写入端
// 返回:
//   - error: 关闭写入端过程中的错误
func (s *stream) CloseWrite() error {
	// 调用底层yamux流的CloseWrite方法关闭写入端
	return s.yamux().CloseWrite()
}

// SetDeadline 设置流的读写超时时间
// 参数:
//   - t: 超时时间点
//
// 返回:
//   - error: 设置超时时间过程中的错误
func (s *stream) SetDeadline(t time.Time) error {
	// 调用底层yamux流的SetDeadline方法设置超时时间
	return s.yamux().SetDeadline(t)
}

// SetReadDeadline 设置流的读取超时时间
// 参数:
//   - t: 超时时间点
//
// 返回:
//   - error: 设置读取超时时间过程中的错误
func (s *stream) SetReadDeadline(t time.Time) error {
	// 调用底层yamux流的SetReadDeadline方法设置读取超时时间
	return s.yamux().SetReadDeadline(t)
}

// SetWriteDeadline 设置流的写入超时时间
// 参数:
//   - t: 超时时间点
//
// 返回:
//   - error: 设置写入超时时间过程中的错误
func (s *stream) SetWriteDeadline(t time.Time) error {
	// 调用底层yamux流的SetWriteDeadline方法设置写入超时时间
	return s.yamux().SetWriteDeadline(t)
}

// yamux 将stream类型转换为yamux.Stream类型
// 返回:
//   - *yamux.Stream: 转换后的yamux流指针
func (s *stream) yamux() *yamux.Stream {
	// 将当前stream指针转换为yamux.Stream指针并返回
	return (*yamux.Stream)(s)
}
