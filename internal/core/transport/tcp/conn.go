// Package tcp 提供基于 TCP 的传输层实现
package tcp

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// ============================================================================
//                              Conn 实现
// ============================================================================

// Conn TCP 连接
type Conn struct {
	conn       net.Conn
	localAddr  *Address
	remoteAddr *Address
	closed     atomic.Bool
}

// 确保实现接口
var _ transportif.Conn = (*Conn)(nil)

// NewConn 创建 TCP 连接包装
func NewConn(conn net.Conn) (*Conn, error) {
	localAddr, err := NewAddressFromNetAddr(conn.LocalAddr())
	if err != nil {
		return nil, err
	}

	remoteAddr, err := NewAddressFromNetAddr(conn.RemoteAddr())
	if err != nil {
		return nil, err
	}

	return &Conn{
		conn:       conn,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}, nil
}

// NewConnWithAddrs 使用指定地址创建连接
func NewConnWithAddrs(conn net.Conn, localAddr, remoteAddr *Address) *Conn {
	return &Conn{
		conn:       conn,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
}

// ============================================================================
//                              io.ReadWriteCloser 实现
// ============================================================================

// Read 读取数据
func (c *Conn) Read(p []byte) (int, error) {
	return c.conn.Read(p)
}

// Write 写入数据
func (c *Conn) Write(p []byte) (int, error) {
	return c.conn.Write(p)
}

// Close 关闭连接
func (c *Conn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		return c.conn.Close()
	}
	return nil
}

// ============================================================================
//                              transport.Conn 接口实现
// ============================================================================

// LocalAddr 返回本地地址
func (c *Conn) LocalAddr() endpoint.Address {
	return c.localAddr
}

// RemoteAddr 返回远程地址
func (c *Conn) RemoteAddr() endpoint.Address {
	return c.remoteAddr
}

// LocalNetAddr 返回底层 net.Addr
func (c *Conn) LocalNetAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteNetAddr 返回底层远程 net.Addr
func (c *Conn) RemoteNetAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline 设置读写超时
func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// IsClosed 检查连接是否已关闭
func (c *Conn) IsClosed() bool {
	return c.closed.Load()
}

// Transport 返回传输协议名称
func (c *Conn) Transport() string {
	return "tcp"
}

// ============================================================================
//                              辅助方法
// ============================================================================

// NetConn 返回底层 net.Conn
func (c *Conn) NetConn() net.Conn {
	return c.conn
}

// SetNoDelay 设置 TCP_NODELAY 选项
func (c *Conn) SetNoDelay(noDelay bool) error {
	if tcpConn, ok := c.conn.(*net.TCPConn); ok {
		return tcpConn.SetNoDelay(noDelay)
	}
	return nil
}

// SetKeepAlive 设置 TCP 保活
func (c *Conn) SetKeepAlive(keepAlive bool) error {
	if tcpConn, ok := c.conn.(*net.TCPConn); ok {
		return tcpConn.SetKeepAlive(keepAlive)
	}
	return nil
}

// SetKeepAlivePeriod 设置 TCP 保活间隔
func (c *Conn) SetKeepAlivePeriod(d time.Duration) error {
	if tcpConn, ok := c.conn.(*net.TCPConn); ok {
		return tcpConn.SetKeepAlivePeriod(d)
	}
	return nil
}

