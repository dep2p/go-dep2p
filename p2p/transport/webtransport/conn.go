package dep2pwebtransport

import (
	"context"

	"github.com/dep2p/go-dep2p/core/network"
	tpt "github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
)

// connSecurityMultiaddrs 连接安全和多地址组合结构体
type connSecurityMultiaddrs struct {
	network.ConnSecurity   // 连接安全接口
	network.ConnMultiaddrs // 连接多地址接口
}

// connMultiaddrs 连接多地址结构体
type connMultiaddrs struct {
	local, remote ma.Multiaddr // 本地和远程多地址
}

var _ network.ConnMultiaddrs = &connMultiaddrs{} // 确保实现了 ConnMultiaddrs 接口

// LocalMultiaddr 获取本地多地址
// 返回值:
//   - ma.Multiaddr 本地多地址
func (c *connMultiaddrs) LocalMultiaddr() ma.Multiaddr { return c.local }

// RemoteMultiaddr 获取远程多地址
// 返回值:
//   - ma.Multiaddr 远程多地址
func (c *connMultiaddrs) RemoteMultiaddr() ma.Multiaddr { return c.remote }

// conn WebTransport 连接结构体
type conn struct {
	*connSecurityMultiaddrs // 连接安全和多地址组合

	transport *transport            // WebTransport 传输层
	session   *webtransport.Session // WebTransport 会话

	scope network.ConnManagementScope // 连接管理作用域
	qconn quic.Connection             // QUIC 连接
}

var _ tpt.CapableConn = &conn{} // 确保实现了 CapableConn 接口

// newConn 创建新的 WebTransport 连接
// 参数:
//   - tr: *transport WebTransport 传输层
//   - sess: *webtransport.Session WebTransport 会话
//   - sconn: *connSecurityMultiaddrs 连接安全和多地址组合
//   - scope: network.ConnManagementScope 连接管理作用域
//   - qconn: quic.Connection QUIC 连接
//
// 返回值:
//   - *conn 创建的连接对象
func newConn(tr *transport, sess *webtransport.Session, sconn *connSecurityMultiaddrs, scope network.ConnManagementScope, qconn quic.Connection) *conn {
	return &conn{
		connSecurityMultiaddrs: sconn,
		transport:              tr,
		session:                sess,
		scope:                  scope,
		qconn:                  qconn,
	}
}

// OpenStream 打开新的流
// 参数:
//   - ctx: context.Context 上下文
//
// 返回值:
//   - network.MuxedStream 创建的多路复用流
//   - error 可能的错误
func (c *conn) OpenStream(ctx context.Context) (network.MuxedStream, error) {
	str, err := c.session.OpenStreamSync(ctx)
	if err != nil {
		log.Debugf("打开流失败: %s", err)
		return nil, err
	}
	return &stream{str}, nil
}

// AcceptStream 接受新的流
// 返回值:
//   - network.MuxedStream 接受的多路复用流
//   - error 可能的错误
func (c *conn) AcceptStream() (network.MuxedStream, error) {
	str, err := c.session.AcceptStream(context.Background())
	if err != nil {
		log.Debugf("接受流失败: %s", err)
		return nil, err
	}
	return &stream{str}, nil
}

// allowWindowIncrease 判断是否允许增加窗口大小
// 参数:
//   - size: uint64 要增加的大小
//
// 返回值:
//   - bool 是否允许增加
func (c *conn) allowWindowIncrease(size uint64) bool {
	return c.scope.ReserveMemory(int(size), network.ReservationPriorityMedium) == nil
}

// Close 关闭连接
// 即使对端关闭了连接,也必须调用此方法以确保包中的垃圾回收正常工作
// 返回值:
//   - error 可能的错误
func (c *conn) Close() error {
	defer c.scope.Done()
	c.transport.removeConn(c.session)
	err := c.session.CloseWithError(0, "")
	_ = c.qconn.CloseWithError(1, "")
	return err
}

// IsClosed 判断连接是否已关闭
// 返回值:
//   - bool 连接是否已关闭
func (c *conn) IsClosed() bool { return c.session.Context().Err() != nil }

// Scope 获取连接作用域
// 返回值:
//   - network.ConnScope 连接作用域
func (c *conn) Scope() network.ConnScope { return c.scope }

// Transport 获取传输层
// 返回值:
//   - tpt.Transport 传输层接口
func (c *conn) Transport() tpt.Transport { return c.transport }

// ConnState 获取连接状态
// 返回值:
//   - network.ConnectionState 连接状态
func (c *conn) ConnState() network.ConnectionState {
	return network.ConnectionState{Transport: "webtransport"}
}
