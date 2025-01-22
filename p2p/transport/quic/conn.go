package dep2pquic

import (
	"context"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	tpt "github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/quic-go/quic-go"
)

// conn 结构体表示一个 QUIC 连接
type conn struct {
	// quic-go 的底层连接对象
	quicConn quic.Connection
	// 所属的传输层对象
	transport *transport
	// 连接管理作用域
	scope network.ConnManagementScope

	// 本地节点 ID
	localPeer peer.ID
	// 本地多地址
	localMultiaddr ma.Multiaddr

	// 远程节点 ID
	remotePeerID peer.ID
	// 远程节点公钥
	remotePubKey ic.PubKey
	// 远程多地址
	remoteMultiaddr ma.Multiaddr
}

// 确保 conn 实现了 tpt.CapableConn 接口
var _ tpt.CapableConn = &conn{}

// Close 关闭连接
// 即使对端关闭了连接,也必须调用此方法以确保本包中的垃圾回收正常工作
// 返回值:
//   - error 关闭过程中的错误,如果没有错误则返回 nil
func (c *conn) Close() error {
	return c.closeWithError(0, "")
}

// closeWithError 使用指定的错误码和错误信息关闭连接
// 参数:
//   - errCode: 应用层错误码
//   - errString: 错误描述字符串
//
// 返回值:
//   - error 关闭过程中的错误,如果没有错误则返回 nil
func (c *conn) closeWithError(errCode quic.ApplicationErrorCode, errString string) error {
	c.transport.removeConn(c.quicConn)
	err := c.quicConn.CloseWithError(errCode, errString)
	c.scope.Done()
	return err
}

// IsClosed 返回连接是否已完全关闭
// 返回值:
//   - bool 如果连接已关闭返回 true,否则返回 false
func (c *conn) IsClosed() bool {
	return c.quicConn.Context().Err() != nil
}

// allowWindowIncrease 检查是否允许增加窗口大小
// 参数:
//   - size: 要增加的窗口大小
//
// 返回值:
//   - bool 如果允许增加返回 true,否则返回 false
func (c *conn) allowWindowIncrease(size uint64) bool {
	return c.scope.ReserveMemory(int(size), network.ReservationPriorityMedium) == nil
}

// OpenStream 创建一个新的流
// 参数:
//   - ctx: 上下文对象,用于控制超时和取消
//
// 返回值:
//   - network.MuxedStream 创建的新流
//   - error 创建过程中的错误,如果没有错误则返回 nil
func (c *conn) OpenStream(ctx context.Context) (network.MuxedStream, error) {
	qstr, err := c.quicConn.OpenStreamSync(ctx)
	return &stream{Stream: qstr}, err
}

// AcceptStream 接受对端打开的流
// 返回值:
//   - network.MuxedStream 接受的流
//   - error 接受过程中的错误,如果没有错误则返回 nil
func (c *conn) AcceptStream() (network.MuxedStream, error) {
	qstr, err := c.quicConn.AcceptStream(context.Background())
	return &stream{Stream: qstr}, err
}

// LocalPeer 返回本地节点 ID
// 返回值:
//   - peer.ID 本地节点的 ID
func (c *conn) LocalPeer() peer.ID { return c.localPeer }

// RemotePeer 返回远程节点的 ID
// 返回值:
//   - peer.ID 远程节点的 ID
func (c *conn) RemotePeer() peer.ID { return c.remotePeerID }

// RemotePublicKey 返回远程节点的公钥
// 返回值:
//   - ic.PubKey 远程节点的公钥
func (c *conn) RemotePublicKey() ic.PubKey { return c.remotePubKey }

// LocalMultiaddr 返回与连接关联的本地多地址
// 返回值:
//   - ma.Multiaddr 本地多地址
func (c *conn) LocalMultiaddr() ma.Multiaddr { return c.localMultiaddr }

// RemoteMultiaddr 返回与连接关联的远程多地址
// 返回值:
//   - ma.Multiaddr 远程多地址
func (c *conn) RemoteMultiaddr() ma.Multiaddr { return c.remoteMultiaddr }

// Transport 返回传输层对象
// 返回值:
//   - tpt.Transport 传输层对象
func (c *conn) Transport() tpt.Transport { return c.transport }

// Scope 返回连接管理作用域
// 返回值:
//   - network.ConnScope 连接管理作用域
func (c *conn) Scope() network.ConnScope { return c.scope }

// ConnState 返回安全连接的状态
// 返回值:
//   - network.ConnectionState 连接状态对象
func (c *conn) ConnState() network.ConnectionState {
	t := "quic-v1"
	if _, err := c.LocalMultiaddr().ValueForProtocol(ma.P_QUIC); err == nil {
		t = "quic"
	}
	return network.ConnectionState{Transport: t}
}
