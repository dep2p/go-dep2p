package network

import (
	"context"
	"io"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// Conn 是与远程节点的连接，它可以复用流。
// 通常不需要直接使用 Conn，但它可能对获取对端节点的信息有用:
//
//	stream.Conn().RemotePeer()
type Conn interface {
	io.Closer

	ConnSecurity
	ConnMultiaddrs
	ConnStat
	ConnScoper

	// ID 返回一个标识符，在当前运行期间唯一标识此主机中的这个 Conn。连接 ID 在重启后可能会重复。
	ID() string

	// NewStream 在此连接上构造一个新的流。
	NewStream(context.Context) (Stream, error)

	// GetStreams 返回此连接上的所有打开的流。
	GetStreams() []Stream

	// IsClosed 返回连接是否完全关闭，以便可以进行垃圾回收。
	IsClosed() bool
}

// ConnectionState 保存连接的信息。
type ConnectionState struct {
	// 此连接使用的流多路复用器(如果有)。例如: /yamux/1.0.0
	StreamMultiplexer protocol.ID
	// 此连接使用的安全协议(如果有)。例如: /tls/1.0.0
	Security protocol.ID
	// 此连接使用的传输协议。例如: tcp
	Transport string
	// 指示 StreamMultiplexer 是否使用内联多路复用器协商选择
	UsedEarlyMuxerNegotiation bool
}

// ConnSecurity 是一个接口，可以混入到连接接口中以提供安全方法。
type ConnSecurity interface {
	// LocalPeer 返回我们的节点 ID
	LocalPeer() peer.ID

	// RemotePeer 返回远程节点的节点 ID
	RemotePeer() peer.ID

	// RemotePublicKey 返回远程节点的公钥
	RemotePublicKey() ic.PubKey

	// ConnState 返回连接状态的信息
	ConnState() ConnectionState
}

// ConnMultiaddrs 是一个接口混入，用于提供端点多地址的连接类型。
type ConnMultiaddrs interface {
	// LocalMultiaddr 返回与此连接关联的本地多地址
	LocalMultiaddr() ma.Multiaddr

	// RemoteMultiaddr 返回与此连接关联的远程多地址
	RemoteMultiaddr() ma.Multiaddr
}

// ConnStat 是一个接口混入，用于提供连接统计信息的连接类型。
type ConnStat interface {
	// Stat 存储与此连接相关的元数据
	Stat() ConnStats
}

// ConnScoper 是一个接口，可以混入到连接接口中以提供资源管理作用域
type ConnScoper interface {
	// Scope 返回此连接的资源作用域的用户视图
	Scope() ConnScope
}
