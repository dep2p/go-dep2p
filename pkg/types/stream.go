// Package types 定义 DeP2P 公共类型
//
// 本文件定义流相关类型。
package types

import (
	"time"
)

// ============================================================================
//                              StreamStat - 流统计
// ============================================================================

// StreamStat 流统计信息
type StreamStat struct {
	// Direction 流方向
	Direction Direction

	// Opened 流打开时间
	Opened time.Time

	// Protocol 使用的协议
	Protocol ProtocolID

	// BytesRead 已读取字节数
	BytesRead int64

	// BytesWritten 已写入字节数
	BytesWritten int64
}

// ============================================================================
//                              StreamInfo - 流信息
// ============================================================================

// StreamInfo 流信息
type StreamInfo struct {
	// ID 流唯一标识
	ID string

	// Protocol 协议 ID
	Protocol ProtocolID

	// Direction 方向
	Direction Direction

	// LocalPeer 本地节点
	LocalPeer PeerID

	// RemotePeer 远端节点
	RemotePeer PeerID

	// Opened 打开时间
	Opened time.Time

	// Stat 统计信息
	Stat StreamStat
}

// ============================================================================
//                              StreamState - 流状态
// ============================================================================

// StreamState 流状态
type StreamState int

const (
	// StreamStateOpen 打开状态
	StreamStateOpen StreamState = iota
	// StreamStateReadClosed 读端已关闭
	StreamStateReadClosed
	// StreamStateWriteClosed 写端已关闭
	StreamStateWriteClosed
	// StreamStateClosed 完全关闭
	StreamStateClosed
	// StreamStateReset 已重置
	StreamStateReset
)

// String 返回状态的字符串表示
func (s StreamState) String() string {
	switch s {
	case StreamStateOpen:
		return "open"
	case StreamStateReadClosed:
		return "read-closed"
	case StreamStateWriteClosed:
		return "write-closed"
	case StreamStateClosed:
		return "closed"
	case StreamStateReset:
		return "reset"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              StreamScope - 流资源范围
// ============================================================================

// StreamScope 流资源范围
type StreamScope struct {
	// StreamID 流 ID
	StreamID string

	// PeerID 节点 ID
	PeerID PeerID

	// Protocol 协议 ID
	Protocol ProtocolID

	// Direction 方向
	Direction Direction

	// Memory 内存使用
	Memory int64
}
