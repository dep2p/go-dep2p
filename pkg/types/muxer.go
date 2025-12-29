package types

// ============================================================================
//                              MuxerStats - 多路复用器统计
// ============================================================================

// MuxerStats 多路复用器统计
type MuxerStats struct {
	// NumStreams 当前流数量
	NumStreams int

	// NumInboundStreams 入站流数量
	NumInboundStreams int

	// NumOutboundStreams 出站流数量
	NumOutboundStreams int

	// BytesSent 发送字节数
	BytesSent uint64

	// BytesRecv 接收字节数
	BytesRecv uint64

	// StreamsOpened 打开的总流数
	StreamsOpened uint64

	// StreamsClosed 关闭的总流数
	StreamsClosed uint64
}

// TotalStreams 返回流总数（历史）
func (s MuxerStats) TotalStreams() uint64 {
	return s.StreamsOpened
}

// TotalBytes 返回总传输字节数
func (s MuxerStats) TotalBytes() uint64 {
	return s.BytesSent + s.BytesRecv
}

