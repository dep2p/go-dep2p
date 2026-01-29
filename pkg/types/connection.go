// Package types 定义 DeP2P 公共类型
//
// 本文件定义连接相关类型。
package types

import (
	"time"
)

// ============================================================================
//                              ConnStat - 连接统计
// ============================================================================

// ConnStat 连接统计信息
type ConnStat struct {
	// Direction 连接方向
	Direction Direction

	// Opened 连接建立时间
	Opened time.Time

	// Transient 是否为临时连接（如中继）
	Transient bool

	// NumStreams 当前流数量
	NumStreams int

	// Limited 是否为受限连接
	Limited bool
}

// ============================================================================
//                              ConnInfo - 连接信息
// ============================================================================

// ConnInfo 连接信息
type ConnInfo struct {
	// ID 连接唯一标识
	ID string

	// LocalPeer 本地节点
	LocalPeer PeerID

	// LocalAddr 本地地址
	LocalAddr Multiaddr

	// RemotePeer 远端节点
	RemotePeer PeerID

	// RemoteAddr 远端地址
	RemoteAddr Multiaddr

	// Stat 统计信息
	Stat ConnStat

	// Streams 流列表
	Streams []StreamInfo
}

// ============================================================================
//                              ConnState - 连接状态
// ============================================================================

// ConnState 连接状态
type ConnState int

const (
	// ConnStateConnecting 连接中
	ConnStateConnecting ConnState = iota
	// ConnStateConnected 已连接
	ConnStateConnected
	// ConnStateDisconnecting 断开中
	ConnStateDisconnecting
	// ConnStateDisconnected 已断开
	ConnStateDisconnected
)

// String 返回状态的字符串表示
func (s ConnState) String() string {
	switch s {
	case ConnStateConnecting:
		return "connecting"
	case ConnStateConnected:
		return "connected"
	case ConnStateDisconnecting:
		return "disconnecting"
	case ConnStateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              ConnScope - 连接资源范围
// ============================================================================

// ConnScope 连接资源范围
type ConnScope struct {
	// ConnID 连接 ID
	ConnID string

	// PeerID 节点 ID
	PeerID PeerID

	// Direction 方向
	Direction Direction

	// Memory 内存使用
	Memory int64

	// NumStreams 流数量
	NumStreams int
}

// ============================================================================
//                              ConnectionStat - 连接统计（兼容）
// ============================================================================

// ConnectionStat 连接统计（简化版本，用于 Host 接口）
type ConnectionStat struct {
	// NumConns 连接总数
	NumConns int

	// NumConnsInbound 入站连接数
	NumConnsInbound int

	// NumConnsOutbound 出站连接数
	NumConnsOutbound int

	// NumStreams 流总数
	NumStreams int

	// NumPeers 已连接节点数
	NumPeers int
}
