package types

import "time"

// ============================================================================
//                              ConnectionStats - 连接统计
// ============================================================================

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	// OpenedAt 连接建立时间
	OpenedAt time.Time

	// BytesSent 发送字节数
	BytesSent uint64

	// BytesRecv 接收字节数
	BytesRecv uint64

	// StreamsOpen 当前打开的流数
	StreamsOpen int

	// StreamsTotal 总流数（包括已关闭）
	StreamsTotal int

	// RTT 往返时间
	RTT time.Duration
}

// Duration 返回连接持续时间
func (s ConnectionStats) Duration() time.Duration {
	return time.Since(s.OpenedAt)
}

// TotalBytes 返回总传输字节数
func (s ConnectionStats) TotalBytes() uint64 {
	return s.BytesSent + s.BytesRecv
}

// ============================================================================
//                              StreamStats - 流统计
// ============================================================================

// StreamStats 流统计信息
type StreamStats struct {
	// OpenedAt 流打开时间
	OpenedAt time.Time

	// BytesSent 发送字节数
	BytesSent uint64

	// BytesRecv 接收字节数
	BytesRecv uint64
}

// Duration 返回流持续时间
func (s StreamStats) Duration() time.Duration {
	return time.Since(s.OpenedAt)
}

// TotalBytes 返回总传输字节数
func (s StreamStats) TotalBytes() uint64 {
	return s.BytesSent + s.BytesRecv
}

// ============================================================================
//                              NetworkStats - 网络统计
// ============================================================================

// NetworkStats 整体网络统计信息
type NetworkStats struct {
	// Connections 活跃连接数
	Connections int

	// Streams 活跃流数
	Streams int

	// BytesSentTotal 总发送字节数
	BytesSentTotal uint64

	// BytesRecvTotal 总接收字节数
	BytesRecvTotal uint64

	// PeersConnected 已连接节点数
	PeersConnected int

	// PeersKnown 已知节点数（地址簿中）
	PeersKnown int
}

