package sampledconn

import (
	"errors"
	"io"
	"net"
	"syscall"
	"time"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// 每次预读取的字节数
const peekSize = 3

// PeekedBytes 定义预读取字节的数组类型
type PeekedBytes = [peekSize]byte

// ErrNotTCPConn 表示传入的连接不是 TCP 连接
var ErrNotTCPConn = errors.New("传入的连接不是 TCP 连接")

// PeekBytes 预读取连接中的字节
// 参数:
//   - conn: manet.Conn 网络连接对象
//
// 返回值:
//   - PeekedBytes 预读取的字节数组
//   - manet.Conn 包装后的连接对象
//   - error 可能的错误
func PeekBytes(conn manet.Conn) (PeekedBytes, manet.Conn, error) {
	// 尝试将连接转换为 ManetTCPConnInterface 类型
	if c, ok := conn.(ManetTCPConnInterface); ok {
		return newWrappedSampledConn(c)
	}

	return PeekedBytes{}, nil, ErrNotTCPConn
}

// wrappedSampledConn 包装了 TCP 连接并提供预读取功能
type wrappedSampledConn struct {
	ManetTCPConnInterface
	peekedBytes PeekedBytes // 存储预读取的字节
	bytesPeeked uint8       // 已读取的字节数
}

// tcpConnInterface 定义了 TCP 连接需要实现的接口
// 注意: `SyscallConn() (syscall.RawConn, error)` 包含在此接口中是为了更容易将其用作 TCP 连接，
// 但如果使用回退机制可能会跳过预读取的字节，这是一个潜在的问题
type tcpConnInterface interface {
	net.Conn
	syscall.Conn

	CloseRead() error
	CloseWrite() error

	SetLinger(sec int) error
	SetKeepAlive(keepalive bool) error
	SetKeepAlivePeriod(d time.Duration) error
	SetNoDelay(noDelay bool) error
	MultipathTCP() (bool, error)

	io.ReaderFrom
	io.WriterTo
}

// ManetTCPConnInterface 组合了 manet.Conn 和 tcpConnInterface
type ManetTCPConnInterface interface {
	manet.Conn
	tcpConnInterface
}

// newWrappedSampledConn 创建新的包装连接对象
// 参数:
//   - conn: ManetTCPConnInterface TCP 连接对象
//
// 返回值:
//   - PeekedBytes 预读取的字节数组
//   - *wrappedSampledConn 包装后的连接对象
//   - error 可能的错误
func newWrappedSampledConn(conn ManetTCPConnInterface) (PeekedBytes, *wrappedSampledConn, error) {
	s := &wrappedSampledConn{ManetTCPConnInterface: conn}
	// 预读取指定数量的字节
	n, err := io.ReadFull(conn, s.peekedBytes[:])
	if err != nil {
		if n == 0 && err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return s.peekedBytes, nil, err
	}
	return s.peekedBytes, s, nil
}

// Read 实现了 io.Reader 接口
// 参数:
//   - b: []byte 用于存储读取数据的缓冲区
//
// 返回值:
//   - int 实际读取的字节数
//   - error 可能的错误
func (sc *wrappedSampledConn) Read(b []byte) (int, error) {
	// 如果还有预读取的字节未返回，先返回这些字节
	if int(sc.bytesPeeked) != len(sc.peekedBytes) {
		red := copy(b, sc.peekedBytes[sc.bytesPeeked:])
		sc.bytesPeeked += uint8(red)
		return red, nil
	}

	// 预读取的字节已全部返回，直接从底层连接读取
	return sc.ManetTCPConnInterface.Read(b)
}
