// Package tls 实现 TLS 1.3 安全传输
package tls

import (
	"crypto/tls"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 确保实现了接口
var _ pkgif.SecureConn = (*secureConn)(nil)

// secureConn 安全连接实现
type secureConn struct {
	*tls.Conn // 嵌入 TLS 连接

	localPeer    types.PeerID
	remotePeer   types.PeerID
	localPubKey  []byte
	remotePubKey []byte
}

// newSecureConn 创建安全连接
func newSecureConn(
	tlsConn *tls.Conn,
	localPeer, remotePeer types.PeerID,
	localPubKey, remotePubKey []byte,
) *secureConn {
	return &secureConn{
		Conn:         tlsConn,
		localPeer:    localPeer,
		remotePeer:   remotePeer,
		localPubKey:  localPubKey,
		remotePubKey: remotePubKey,
	}
}

// LocalPeer 返回本地节点 ID
func (c *secureConn) LocalPeer() types.PeerID {
	return c.localPeer
}

// RemotePeer 返回远端节点 ID
func (c *secureConn) RemotePeer() types.PeerID {
	return c.remotePeer
}

// LocalPublicKey 返回本地公钥
func (c *secureConn) LocalPublicKey() []byte {
	return c.localPubKey
}

// RemotePublicKey 返回远端公钥
func (c *secureConn) RemotePublicKey() []byte {
	return c.remotePubKey
}

// ConnState 返回连接状态
func (c *secureConn) ConnState() pkgif.SecureConnState {
	return pkgif.SecureConnState{
		Protocol:        types.ProtocolID("/tls/1.0.0"),
		LocalPeer:       c.localPeer,
		RemotePeer:      c.remotePeer,
		LocalPublicKey:  c.localPubKey,  // []byte
		RemotePublicKey: c.remotePubKey, // []byte
		Opened:          true,            // 握手已完成
	}
}
