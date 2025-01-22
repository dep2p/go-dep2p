package yamux

import (
	"context"

	"github.com/dep2p/go-dep2p/core/network"
	logging "github.com/dep2p/log"

	"github.com/dep2p/go-dep2p/p2plib/yamux"
)

var log = logging.Logger("yamux-conn")

// conn 在 yamux.Session 之上实现了 mux.MuxedConn 接口
type conn yamux.Session

var _ network.MuxedConn = &conn{}

// NewMuxedConn 从 yamux.Session 构造一个新的 MuxedConn
// 参数:
//   - m: *yamux.Session - yamux 会话对象
//
// 返回值:
//   - network.MuxedConn: 多路复用连接接口
func NewMuxedConn(m *yamux.Session) network.MuxedConn {
	// 将 yamux.Session 转换为 conn 类型并返回
	return (*conn)(m)
}

// Close 关闭底层的 yamux 连接
// 返回值:
//   - error: 关闭过程中的错误,如果没有错误则返回 nil
func (c *conn) Close() error {
	// 调用底层 yamux 会话的 Close 方法关闭连接
	return c.yamux().Close()
}

// IsClosed 检查 yamux.Session 是否处于关闭状态
// 返回值:
//   - bool: 如果连接已关闭返回 true,否则返回 false
func (c *conn) IsClosed() bool {
	// 调用底层 yamux 会话的 IsClosed 方法检查状态
	return c.yamux().IsClosed()
}

// OpenStream 创建一个新的流
// 参数:
//   - ctx: context.Context - 上下文对象,用于控制超时等
//
// 返回值:
//   - network.MuxedStream: 创建的多路复用流
//   - error: 创建过程中的错误,如果没有错误则返回 nil
func (c *conn) OpenStream(ctx context.Context) (network.MuxedStream, error) {
	// 调用底层 yamux 会话的 OpenStream 方法创建新流
	s, err := c.yamux().OpenStream(ctx)
	if err != nil {
		// 如果发生错误,返回 nil 和错误信息
		log.Debugf("创建多路复用流失败: %v", err)
		return nil, err
	}

	// 将 yamux 流转换为 stream 类型并返回
	return (*stream)(s), nil
}

// AcceptStream 接受对端打开的流
// 返回值:
//   - network.MuxedStream: 接受的多路复用流
//   - error: 接受过程中的错误,如果没有错误则返回 nil
func (c *conn) AcceptStream() (network.MuxedStream, error) {
	// 调用底层 yamux 会话的 AcceptStream 方法接受新流
	s, err := c.yamux().AcceptStream()
	// 将 yamux 流转换为 stream 类型并返回结果
	return (*stream)(s), err
}

// yamux 将 conn 转换回 yamux.Session
// 返回值:
//   - *yamux.Session: yamux 会话对象
func (c *conn) yamux() *yamux.Session {
	// 将当前 conn 对象转换为 yamux.Session 类型并返回
	return (*yamux.Session)(c)
}
