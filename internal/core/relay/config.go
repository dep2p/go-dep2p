package relay

import (
	"errors"
	"time"
)

// Config 中继服务配置（v2.0 统一配置）
type Config struct {
	// 客户端/服务端开关
	EnableClient bool // 启用中继客户端（默认 true）
	EnableServer bool // 启用中继服务端（默认 false）

	// 服务端配置
	MaxReservations int           // 服务端最大预约数（默认 1024）
	MaxCircuits     int           // 服务端最大活跃电路（0 = 不限制，默认 128）
	ReservationTTL  time.Duration // 预约有效期（默认 1h）
	BufferSize      int           // 中继缓冲区大小（默认 4096）

	// 统一限制
	MaxBandwidth  int64         // 单电路最大带宽（0 = 不限制）
	MaxDuration   time.Duration // 单电路最大持续时间（0 = 不限制）
	MaxCircuitsPerPeer int      // 单节点最大并发电路数（0 = 不限制，默认 16）

	// v2.0 新增：打洞相关配置
	KeepRelayAfterHolePunch bool // 打洞成功后保留 Relay 连接作为备份（默认 true）
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		EnableClient: true,
		EnableServer: false,

		MaxReservations: DefaultMaxReservations,
		MaxCircuits:     DefaultMaxCircuitsTotal,
		ReservationTTL:  DefaultReservationTTL,
		BufferSize:      DefaultBufferSize,

		MaxBandwidth:        DefaultMaxBandwidth,
		MaxDuration:         DefaultMaxDuration,
		MaxCircuitsPerPeer:  DefaultMaxCircuitsPerPeer,

		KeepRelayAfterHolePunch: true, // v2.0 默认保留备份连接
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.MaxReservations < 1 {
		return errors.New("MaxReservations must be >= 1")
	}

	if c.MaxCircuits < 0 {
		return errors.New("MaxCircuits must be >= 0")
	}

	if c.ReservationTTL < time.Minute {
		return errors.New("ReservationTTL must be >= 1 minute")
	}

	if c.BufferSize < 1024 {
		return errors.New("BufferSize must be >= 1024")
	}

	// MaxCircuits = 0 表示不限制，是合法值
	// MaxBandwidth = 0 表示不限制，是合法值
	// MaxDuration = 0 表示不限制，是合法值
	// MaxCircuitsPerPeer = 0 表示不限制，是合法值

	return nil
}
