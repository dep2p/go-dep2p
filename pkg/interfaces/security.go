// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Security 接口，抽象安全传输协议。
package interfaces

import (
	"context"
	"net"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// SecureTransport 定义安全传输接口
//
// SecureTransport 提供连接加密和身份验证功能。
type SecureTransport interface {
	// SecureInbound 保护入站连接
	SecureInbound(ctx context.Context, conn net.Conn, peerID types.PeerID) (SecureConn, error)

	// SecureOutbound 保护出站连接
	SecureOutbound(ctx context.Context, conn net.Conn, peerID types.PeerID) (SecureConn, error)

	// ID 返回安全协议标识
	ID() types.ProtocolID
}

// SecureConn 定义安全连接接口
type SecureConn interface {
	net.Conn

	// LocalPeer 返回本地节点 ID
	LocalPeer() types.PeerID

	// LocalPublicKey 返回本地公钥
	LocalPublicKey() []byte

	// RemotePeer 返回远端节点 ID
	RemotePeer() types.PeerID

	// RemotePublicKey 返回远端公钥
	RemotePublicKey() []byte

	// ConnState 返回连接状态
	ConnState() SecureConnState
}

// SecureConnState 安全连接状态
type SecureConnState struct {
	// Protocol 使用的安全协议（TLS、Noise等）
	Protocol types.ProtocolID

	// LocalPeer 本地节点 ID
	LocalPeer types.PeerID

	// RemotePeer 远端节点 ID
	RemotePeer types.PeerID

	// LocalPublicKey 本地公钥
	LocalPublicKey []byte

	// RemotePublicKey 远端公钥
	RemotePublicKey []byte

	// Opened 是否已完成握手
	Opened bool
}

// SecurityMultiplexer 安全协议多路复用器
type SecurityMultiplexer interface {
	// SecureInbound 使用多路复用安全协议保护入站连接
	SecureInbound(ctx context.Context, conn net.Conn, peerID types.PeerID) (SecureConn, error)

	// SecureOutbound 使用多路复用安全协议保护出站连接
	SecureOutbound(ctx context.Context, conn net.Conn, peerID types.PeerID) (SecureConn, error)
}
