// Package relay 提供中继服务
//
// 本文件定义 Relay 能力的内置默认值。
// 这些值是经过调优的，用户无法通过配置修改。
//
// v2.0 统一 Relay 架构
package relay

import "time"

// ════════════════════════════════════════════════════════════════════════════
// 统一 Relay 内置默认值（v2.0）
// ════════════════════════════════════════════════════════════════════════════

const (
	// DefaultMaxBandwidth 单电路最大带宽 (0 = 无限制)
	DefaultMaxBandwidth = 0

	// DefaultMaxDuration 单电路最大持续时间 (0 = 无限制)
	DefaultMaxDuration = 0

	// DefaultMaxDataPerConn 单电路最大数据量 (0 = 无限制)
	DefaultMaxDataPerConn = 0

	// DefaultMaxReservations 最大预约数
	DefaultMaxReservations = 1024

	// DefaultMaxCircuitsPerPeer 每节点最大电路数
	DefaultMaxCircuitsPerPeer = 16

	// DefaultMaxCircuitsTotal 总最大电路数
	DefaultMaxCircuitsTotal = 128

	// DefaultReservationTTL 预约有效期
	// v2.0 统一：与 config/relay.go 保持一致，设计建议 1h
	DefaultReservationTTL = 1 * time.Hour
)

// ════════════════════════════════════════════════════════════════════════════
// 地址簿内置默认值
// ════════════════════════════════════════════════════════════════════════════

const (
	// DefaultAddressBookSize 地址簿最大成员数
	DefaultAddressBookSize = 10000

	// DefaultAddressEntryTTL 地址条目过期时间
	DefaultAddressEntryTTL = 24 * time.Hour

	// DefaultAddressCleanupInterval 地址清理间隔
	DefaultAddressCleanupInterval = 1 * time.Hour
)

// ════════════════════════════════════════════════════════════════════════════
// 共用默认值
// ════════════════════════════════════════════════════════════════════════════

const (
	// DefaultBufferSize 中继缓冲区大小
	DefaultBufferSize = 4096

	// DefaultConnectTimeout 连接超时
	DefaultConnectTimeout = 30 * time.Second

	// DefaultIdleTimeout 空闲超时
	DefaultIdleTimeout = 5 * time.Minute
)

// ════════════════════════════════════════════════════════════════════════════
// 默认配置构造
// ════════════════════════════════════════════════════════════════════════════

// RelayDefaults 统一 Relay 内置默认值
type RelayDefaults struct {
	MaxBandwidth       int64
	MaxDuration        time.Duration
	MaxDataPerConn     int64
	MaxReservations    int
	MaxCircuitsPerPeer int
	MaxCircuitsTotal   int
	ReservationTTL     time.Duration
	AddressBookSize    int
	AddressEntryTTL    time.Duration
	CleanupInterval    time.Duration
	BufferSize         int
	ConnectTimeout     time.Duration
	IdleTimeout        time.Duration
}

// GetRelayDefaults 返回统一 Relay 内置默认配置
func GetRelayDefaults() RelayDefaults {
	return RelayDefaults{
		MaxBandwidth:       DefaultMaxBandwidth,
		MaxDuration:        DefaultMaxDuration,
		MaxDataPerConn:     DefaultMaxDataPerConn,
		MaxReservations:    DefaultMaxReservations,
		MaxCircuitsPerPeer: DefaultMaxCircuitsPerPeer,
		MaxCircuitsTotal:   DefaultMaxCircuitsTotal,
		ReservationTTL:     DefaultReservationTTL,
		AddressBookSize:    DefaultAddressBookSize,
		AddressEntryTTL:    DefaultAddressEntryTTL,
		CleanupInterval:    DefaultAddressCleanupInterval,
		BufferSize:         DefaultBufferSize,
		ConnectTimeout:     DefaultConnectTimeout,
		IdleTimeout:        DefaultIdleTimeout,
	}
}
