// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Transport 接口，抽象底层传输协议。
package interfaces

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// Transport 定义传输层接口
//
// Transport 抽象不同的传输协议（QUIC、TCP、WebSocket）。
type Transport interface {
	// Dial 拨号连接到指定地址
	Dial(ctx context.Context, raddr types.Multiaddr, peerID types.PeerID) (Connection, error)

	// CanDial 检查是否支持拨号到指定地址
	CanDial(addr types.Multiaddr) bool

	// Listen 在指定地址监听
	Listen(laddr types.Multiaddr) (Listener, error)

	// Protocols 返回支持的协议标识（协议编号）
	Protocols() []int

	// Close 关闭传输
	Close() error
}

// Listener 定义监听器接口
type Listener interface {
	// Accept 接受新连接
	Accept() (Connection, error)

	// Close 关闭监听器
	Close() error

	// Addr 返回监听地址
	Addr() types.Multiaddr

	// Multiaddr 返回多地址格式
	Multiaddr() types.Multiaddr
}

// Connection 定义连接接口
type Connection interface {
	// LocalPeer 返回本地节点 ID
	LocalPeer() types.PeerID

	// LocalMultiaddr 返回本地多地址
	LocalMultiaddr() types.Multiaddr

	// RemotePeer 返回远端节点 ID
	RemotePeer() types.PeerID

	// RemoteMultiaddr 返回远端多地址
	RemoteMultiaddr() types.Multiaddr

	// NewStream 在此连接上创建新流
	NewStream(ctx context.Context) (Stream, error)

	// AcceptStream 接受对方创建的流
	AcceptStream() (Stream, error)

	// GetStreams 获取此连接上的所有流
	GetStreams() []Stream

	// Stat 返回连接统计
	Stat() ConnectionStat

	// ConnType 返回连接类型（v2.0 新增）
	//
	// 返回连接的类型（直连或中继）。
	// 仅供调试和监控使用，不影响正常使用。
	ConnType() ConnectionType

	// Close 关闭连接
	Close() error

	// IsClosed 检查连接是否已关闭
	IsClosed() bool
}

// ConnectionType 连接类型（v2.0 新增）
type ConnectionType int

const (
	// ConnectionTypeDirect 直连
	ConnectionTypeDirect ConnectionType = iota
	// ConnectionTypeRelay 中继连接
	ConnectionTypeRelay
)

// String 返回连接类型字符串
func (t ConnectionType) String() string {
	switch t {
	case ConnectionTypeDirect:
		return "direct"
	case ConnectionTypeRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// IsDirect 是否为直连
func (t ConnectionType) IsDirect() bool {
	return t == ConnectionTypeDirect
}

// IsRelay 是否为中继连接
func (t ConnectionType) IsRelay() bool {
	return t == ConnectionTypeRelay
}

// ConnectionStat 连接统计信息
type ConnectionStat struct {
	// Direction 连接方向
	Direction Direction

	// Opened 连接建立时间戳
	Opened int64

	// Transient 是否为临时连接（如中继）
	Transient bool

	// NumStreams 流数量
	NumStreams int
}

// TransportUpgrader 定义传输升级器接口
type TransportUpgrader interface {
	// Upgrade 升级原始连接
	Upgrade(ctx context.Context, t Transport, conn Connection, dir Direction, peerID types.PeerID) (Connection, error)
}
