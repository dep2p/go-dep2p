package dep2pwebtransport

import (
	"errors"
	"net"

	"github.com/dep2p/go-dep2p/core/network"

	"github.com/quic-go/webtransport-go"
)

// 重置流的错误码
const (
	reset webtransport.StreamErrorCode = 0
)

// webtransportStream 实现了 WebTransport 流的封装
type webtransportStream struct {
	webtransport.Stream                       // WebTransport 流对象
	wsess               *webtransport.Session // WebTransport 会话对象
}

// 确保 webtransportStream 实现了 net.Conn 接口
var _ net.Conn = &webtransportStream{}

// LocalAddr 返回本地地址
// 返回值:
//   - net.Addr: 本地网络地址
func (s *webtransportStream) LocalAddr() net.Addr {
	return s.wsess.LocalAddr()
}

// RemoteAddr 返回远程地址
// 返回值:
//   - net.Addr: 远程网络地址
func (s *webtransportStream) RemoteAddr() net.Addr {
	return s.wsess.RemoteAddr()
}

// stream 实现了多路复用流接口
type stream struct {
	webtransport.Stream // WebTransport 流对象
}

// 确保 stream 实现了 network.MuxedStream 接口
var _ network.MuxedStream = &stream{}

// Read 从流中读取数据
// 参数:
//   - b: []byte 用于存储读取数据的缓冲区
//
// 返回值:
//   - n: int 实际读取的字节数
//   - err: error 读取过程中的错误
func (s *stream) Read(b []byte) (n int, err error) {
	n, err = s.Stream.Read(b)
	if err != nil && errors.Is(err, &webtransport.StreamError{}) {
		log.Debugf("读取流失败: %s", err)
		err = network.ErrReset // 将 WebTransport 流错误转换为重置错误
	}
	return n, err
}

// Write 向流中写入数据
// 参数:
//   - b: []byte 要写入的数据
//
// 返回值:
//   - n: int 实际写入的字节数
//   - err: error 写入过程中的错误
func (s *stream) Write(b []byte) (n int, err error) {
	n, err = s.Stream.Write(b)
	if err != nil && errors.Is(err, &webtransport.StreamError{}) {
		log.Debugf("写入流失败: %s", err)
		err = network.ErrReset // 将 WebTransport 流错误转换为重置错误
	}
	return n, err
}

// Reset 重置流的读写状态
// 返回值:
//   - error 重置过程中的错误
func (s *stream) Reset() error {
	s.Stream.CancelRead(reset)  // 取消读操作
	s.Stream.CancelWrite(reset) // 取消写操作
	return nil
}

// Close 关闭流
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) Close() error {
	s.Stream.CancelRead(reset) // 取消读操作
	return s.Stream.Close()    // 关闭流
}

// CloseRead 关闭流的读取端
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) CloseRead() error {
	s.Stream.CancelRead(reset) // 取消读操作
	return nil
}

// CloseWrite 关闭流的写入端
// 返回值:
//   - error 关闭过程中的错误
func (s *stream) CloseWrite() error {
	return s.Stream.Close() // 关闭流的写入端
}
