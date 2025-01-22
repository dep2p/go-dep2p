package dep2ptls

import (
	"crypto/tls"

	ci "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/sec"
)

// conn 实现了安全连接接口
// 注意:
//   - 该结构体实现了 sec.SecureConn 接口
//   - 用于在 dep2p 网络中提供安全的点对点通信
type conn struct {
	// TLS连接对象,提供底层的加密通信功能
	*tls.Conn

	// 本地节点的唯一标识符
	localPeer peer.ID
	// 远程节点的唯一标识符
	remotePeer peer.ID
	// 远程节点的公钥,用于验证身份
	remotePubKey ci.PubKey
	// 当前连接的状态信息
	connectionState network.ConnectionState
}

// 确保 conn 实现了 SecureConn 接口
var _ sec.SecureConn = &conn{}

// LocalPeer 返回本地节点ID
// 返回值:
//   - peer.ID 本地节点的ID
func (c *conn) LocalPeer() peer.ID {
	return c.localPeer
}

// RemotePeer 返回远程节点ID
// 返回值:
//   - peer.ID 远程节点的ID
func (c *conn) RemotePeer() peer.ID {
	return c.remotePeer
}

// RemotePublicKey 返回远程节点的公钥
// 返回值:
//   - ci.PubKey 远程节点的公钥
func (c *conn) RemotePublicKey() ci.PubKey {
	return c.remotePubKey
}

// ConnState 返回当前连接状态
// 返回值:
//   - network.ConnectionState 当前连接的状态信息
func (c *conn) ConnState() network.ConnectionState {
	return c.connectionState
}
