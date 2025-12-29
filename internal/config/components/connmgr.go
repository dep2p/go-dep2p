package components

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
)

// ConnMgrOptions 连接管理选项
type ConnMgrOptions struct {
	// LowWater 低水位线
	LowWater int

	// HighWater 高水位线
	HighWater int

	// EmergencyWater 紧急水位线
	EmergencyWater int

	// GracePeriod 新连接保护期
	GracePeriod time.Duration

	// IdleTimeout 空闲超时
	IdleTimeout time.Duration
}

// NewConnMgrOptions 从配置创建连接管理选项
func NewConnMgrOptions(cfg *config.ConnectionManagerConfig) *ConnMgrOptions {
	return &ConnMgrOptions{
		LowWater:       cfg.LowWater,
		HighWater:      cfg.HighWater,
		EmergencyWater: cfg.EmergencyWater,
		GracePeriod:    cfg.GracePeriod,
		IdleTimeout:    cfg.IdleTimeout,
	}
}

// DefaultConnMgrOptions 默认连接管理选项
func DefaultConnMgrOptions() *ConnMgrOptions {
	return &ConnMgrOptions{
		LowWater:       50,
		HighWater:      100,
		EmergencyWater: 150,
		GracePeriod:    30 * time.Second,
		IdleTimeout:    5 * time.Minute,
	}
}

// NeedsTrim 是否需要裁剪连接
func (o *ConnMgrOptions) NeedsTrim(connCount int) bool {
	return connCount > o.HighWater
}

// NeedsEmergencyTrim 是否需要紧急裁剪
func (o *ConnMgrOptions) NeedsEmergencyTrim(connCount int) bool {
	return connCount > o.EmergencyWater
}

// IsAboveLowWater 是否超过低水位
func (o *ConnMgrOptions) IsAboveLowWater(connCount int) bool {
	return connCount > o.LowWater
}

