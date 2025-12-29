package types

import "time"

// ============================================================================
//                              ConnectionInfo - 连接信息
// ============================================================================

// ConnectionInfo 连接信息
// 包含连接的完整状态信息，用于连接管理器的决策和监控
type ConnectionInfo struct {
	// NodeID 节点 ID
	NodeID NodeID

	// Direction 连接方向
	Direction Direction

	// CreatedAt 连接建立时间
	CreatedAt time.Time

	// LastActivity 最后活动时间
	LastActivity time.Time

	// BytesSent 发送字节数
	BytesSent uint64

	// BytesRecv 接收字节数
	BytesRecv uint64

	// StreamCount 当前流数量
	StreamCount int

	// Protected 是否受保护
	Protected bool

	// Tags 保护标签列表
	Tags []string

	// Score 连接分数（用于裁剪优先级）
	Score float64
}

// IsIdle 检查连接是否空闲
func (ci ConnectionInfo) IsIdle(timeout time.Duration) bool {
	return time.Since(ci.LastActivity) > timeout
}

// Age 返回连接存活时间
func (ci ConnectionInfo) Age() time.Duration {
	return time.Since(ci.CreatedAt)
}

