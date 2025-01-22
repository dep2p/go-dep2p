package websocket

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/transport"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"

	ws "github.com/gorilla/websocket"
)

// GracefulCloseTimeout 是等待优雅关闭连接的超时时间,超时后将直接切断连接
var GracefulCloseTimeout = 100 * time.Millisecond

// Conn 为 gorilla/websocket 实现 net.Conn 接口
type Conn struct {
	*ws.Conn                // WebSocket 连接对象
	secure             bool // 是否使用安全连接
	DefaultMessageType int  // 默认消息类型
	reader             io.Reader
	closeOnceVal       func() error
	laddr              ma.Multiaddr // 本地多地址
	raddr              ma.Multiaddr // 远程多地址

	readLock, writeLock sync.Mutex // 读写锁
}

var _ net.Conn = (*Conn)(nil)
var _ manet.Conn = (*Conn)(nil)

// NewConn 使用普通的 gorilla/websocket Conn 创建一个新的 Conn
//
// 已弃用: 没有理由在外部使用此方法。将在未来版本中设为非导出。
// 参数:
//   - raw: *ws.Conn WebSocket 连接对象
//   - secure: bool 是否使用安全连接
//
// 返回值:
//   - *Conn: 新创建的 WebSocket 连接对象
func NewConn(raw *ws.Conn, secure bool) *Conn {
	// 创建本地地址
	lna := NewAddrWithScheme(raw.LocalAddr().String(), secure)
	laddr, err := manet.FromNetAddr(lna)
	if err != nil {
		log.Debugf("BUG: websocket 连接的本地地址无效", raw.LocalAddr())
		return nil
	}

	// 创建远程地址
	rna := NewAddrWithScheme(raw.RemoteAddr().String(), secure)
	raddr, err := manet.FromNetAddr(rna)
	if err != nil {
		log.Debugf("BUG: websocket 连接的远程地址无效", raw.RemoteAddr())
		return nil
	}

	// 创建连接对象
	c := &Conn{
		Conn:               raw,
		secure:             secure,
		DefaultMessageType: ws.BinaryMessage,
		laddr:              laddr,
		raddr:              raddr,
	}
	c.closeOnceVal = sync.OnceValue(c.closeOnceFn)
	return c
}

// LocalMultiaddr 实现 manet.Conn 接口
// 返回值:
//   - ma.Multiaddr: 本地多地址
func (c *Conn) LocalMultiaddr() ma.Multiaddr {
	return c.laddr
}

// RemoteMultiaddr 实现 manet.Conn 接口
// 返回值:
//   - ma.Multiaddr: 远程多地址
func (c *Conn) RemoteMultiaddr() ma.Multiaddr {
	return c.raddr
}

// Read 实现 io.Reader 接口
// 参数:
//   - b: []byte 读取数据的缓冲区
//
// 返回值:
//   - int: 读取的字节数
//   - error: 读取过程中的错误
func (c *Conn) Read(b []byte) (int, error) {
	c.readLock.Lock()
	defer c.readLock.Unlock()

	if c.reader == nil {
		if err := c.prepNextReader(); err != nil {
			log.Debugf("准备下一个读取器失败: %s", err)
			return 0, err
		}
	}

	for {
		n, err := c.reader.Read(b)
		switch err {
		case io.EOF:
			c.reader = nil

			if n > 0 {
				return n, nil
			}

			if err := c.prepNextReader(); err != nil {
				log.Debugf("准备下一个读取器失败: %s", err)
				return 0, err
			}

			// 显式循环
		default:
			return n, err
		}
	}
}

// prepNextReader 准备下一个读取器
// 返回值:
//   - error: 准备过程中的错误
func (c *Conn) prepNextReader() error {
	t, r, err := c.Conn.NextReader()
	if err != nil {
		if wserr, ok := err.(*ws.CloseError); ok {
			if wserr.Code == 1000 || wserr.Code == 1005 {
				log.Debugf("连接已关闭: %s", err)
				return io.EOF
			}
		}
		log.Debugf("准备下一个读取器失败: %s", err)
		return err
	}

	if t == ws.CloseMessage {
		log.Debugf("连接已关闭: %s", err)
		return io.EOF
	}

	c.reader = r
	return nil
}

// Write 实现 io.Writer 接口
// 参数:
//   - b: []byte 要写入的数据
//
// 返回值:
//   - int: 写入的字节数
//   - error: 写入过程中的错误
func (c *Conn) Write(b []byte) (n int, err error) {
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	if err := c.Conn.WriteMessage(c.DefaultMessageType, b); err != nil {
		log.Debugf("写入消息失败: %s", err)
		return 0, err
	}

	return len(b), nil
}

// Scope 返回连接管理范围
// 返回值:
//   - network.ConnManagementScope: 连接管理范围
func (c *Conn) Scope() network.ConnManagementScope {
	nc := c.NetConn()
	if sc, ok := nc.(interface {
		Scope() network.ConnManagementScope
	}); ok {
		return sc.Scope()
	}
	return nil
}

// Close 关闭连接
// 后续和并发调用将返回相同的错误值
// 此方法是线程安全的
// 返回值:
//   - error: 关闭过程中的错误
func (c *Conn) Close() error {
	return c.closeOnceVal()
}

// closeOnceFn 执行一次性关闭操作
// 返回值:
//   - error: 关闭过程中的错误
func (c *Conn) closeOnceFn() error {
	err1 := c.Conn.WriteControl(
		ws.CloseMessage,
		ws.FormatCloseMessage(ws.CloseNormalClosure, "closed"),
		time.Now().Add(GracefulCloseTimeout),
	)
	err2 := c.Conn.Close()
	return errors.Join(err1, err2)
}

// LocalAddr 返回本地网络地址
// 返回值:
//   - net.Addr: 本地网络地址
func (c *Conn) LocalAddr() net.Addr {
	return NewAddrWithScheme(c.Conn.LocalAddr().String(), c.secure)
}

// RemoteAddr 返回远程网络地址
// 返回值:
//   - net.Addr: 远程网络地址
func (c *Conn) RemoteAddr() net.Addr {
	return NewAddrWithScheme(c.Conn.RemoteAddr().String(), c.secure)
}

// SetDeadline 设置读写操作的截止时间
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error: 设置过程中的错误
func (c *Conn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		log.Debugf("设置读取截止时间失败: %s", err)
		return err
	}

	return c.SetWriteDeadline(t)
}

// SetReadDeadline 设置读操作的截止时间
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error: 设置过程中的错误
func (c *Conn) SetReadDeadline(t time.Time) error {
	// 设置读取截止时间时不加锁,以避免阻止正在进行的读取操作
	return c.Conn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写操作的截止时间
// 参数:
//   - t: time.Time 截止时间
//
// 返回值:
//   - error: 设置过程中的错误
func (c *Conn) SetWriteDeadline(t time.Time) error {
	// 与读取截止时间不同,设置写入截止时间时需要加锁
	c.writeLock.Lock()
	defer c.writeLock.Unlock()

	return c.Conn.SetWriteDeadline(t)
}

// capableConn 包装了 transport.CapableConn
type capableConn struct {
	transport.CapableConn
}

// ConnState 返回连接状态
// 返回值:
//   - network.ConnectionState: 连接状态
func (c *capableConn) ConnState() network.ConnectionState {
	cs := c.CapableConn.ConnState()
	cs.Transport = "websocket"
	return cs
}
