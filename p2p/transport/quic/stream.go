package dep2pquic

import (
	"errors"

	"github.com/dep2p/go-dep2p/core/network"

	"github.com/quic-go/quic-go"
)

// 重置流错误码常量
const (
	reset quic.StreamErrorCode = 0
)

// stream 封装了 QUIC 流的结构体
type stream struct {
	quic.Stream // QUIC 流对象
}

// 确保 stream 实现了 network.MuxedStream 接口
var _ network.MuxedStream = &stream{}

// Read 从流中读取数据
// 参数:
//   - b: []byte 用于存储读取数据的字节切片
//
// 返回值:
//   - n: int 实际读取的字节数
//   - err: error 读取过程中的错误
func (s *stream) Read(b []byte) (n int, err error) {
	// 从底层 QUIC 流中读取数据
	n, err = s.Stream.Read(b)
	// 如果发生 StreamError 错误，转换为 network.ErrReset
	if err != nil && errors.Is(err, &quic.StreamError{}) {
		log.Debugf("读取流时发生错误: %s", err)
		err = network.ErrReset
	}
	return n, err
}

// Write 向流中写入数据
// 参数:
//   - b: []byte 要写入的数据字节切片
//
// 返回值:
//   - n: int 实际写入的字节数
//   - err: error 写入过程中的错误
func (s *stream) Write(b []byte) (n int, err error) {
	// 向底层 QUIC 流写入数据
	n, err = s.Stream.Write(b)
	// 如果发生 StreamError 错误，转换为 network.ErrReset
	if err != nil && errors.Is(err, &quic.StreamError{}) {
		log.Debugf("写入流时发生错误: %s", err)
		err = network.ErrReset
	}
	return n, err
}

// Reset 重置流的读写状态
// 返回值:
//   - error 重置过程中的错误
func (s *stream) Reset() error {
	// 取消流的读取
	s.Stream.CancelRead(reset)
	// 取消流的写入
	s.Stream.CancelWrite(reset)
	return nil
}

// Close 关闭流
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) Close() error {
	// 取消流的读取
	s.Stream.CancelRead(reset)
	// 关闭流
	return s.Stream.Close()
}

// CloseRead 关闭流的读取端
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) CloseRead() error {
	// 取消流的读取
	s.Stream.CancelRead(reset)
	return nil
}

// CloseWrite 关闭流的写入端
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) CloseWrite() error {
	// 关闭流
	return s.Stream.Close()
}
