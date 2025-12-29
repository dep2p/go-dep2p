package types

import "time"

// ============================================================================
//                              RelayStats - 中继统计
// ============================================================================

// RelayStats 中继统计
type RelayStats struct {
	// ActiveReservations 当前活跃预留数
	ActiveReservations int

	// ActiveConnections 当前活跃连接数
	ActiveConnections int

	// TotalConnections 总连接数
	TotalConnections uint64

	// TotalBytesRelayed 总转发字节数
	TotalBytesRelayed uint64

	// TotalReservations 总预留数
	TotalReservations uint64

	// Uptime 运行时间
	Uptime time.Duration
}

// ============================================================================
//                              ReservationInfo - 预留信息
// ============================================================================

// ReservationInfo 预留信息
type ReservationInfo struct {
	// PeerID 预留者节点 ID
	PeerID NodeID

	// Addrs 分配的地址（字符串格式）
	Addrs []string

	// CreatedAt 创建时间
	CreatedAt time.Time

	// Expiry 过期时间
	Expiry time.Time

	// ActiveConnections 当前连接数
	ActiveConnections int

	// UsedSlots 已使用槽位
	UsedSlots int

	// MaxSlots 最大槽位
	MaxSlots int
}

// IsExpired 检查预留是否过期
func (ri ReservationInfo) IsExpired() bool {
	return time.Now().After(ri.Expiry)
}

// TTL 返回剩余有效时间
func (ri ReservationInfo) TTL() time.Duration {
	remaining := time.Until(ri.Expiry)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ============================================================================
//                              AutoRelayStatus - 自动中继状态
// ============================================================================

// AutoRelayStatus 自动中继状态
type AutoRelayStatus struct {
	// Enabled 是否启用
	Enabled bool

	// NumRelays 当前中继数
	NumRelays int

	// ReservationCount 当前预留数
	ReservationCount int

	// RelayAddrs 中继地址列表
	RelayAddrs []string

	// LastRefresh 最后刷新时间
	LastRefresh time.Time
}

// HasRelays 检查是否有可用中继
func (s AutoRelayStatus) HasRelays() bool {
	return s.NumRelays > 0
}

