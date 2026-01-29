package connmgr

import (
	"time"
)

// Config 连接管理器配置
type Config struct {
	// LowWater 低水位（目标连接数）
	LowWater int

	// HighWater 高水位（触发回收）
	HighWater int

	// GracePeriod 新连接保护期
	GracePeriod time.Duration

	// DecayInterval 衰减间隔（未实现）
	DecayInterval time.Duration

	// EmergencyWater 紧急水位线
	//
	// 当连接数超过此值时拒绝所有新连接
	// 默认值: 150（如果未设置，使用 HighWater * 1.5）
	EmergencyWater int

	// IdleTimeout 空闲超时
	//
	// 超过此时间没有活动的连接优先被裁剪
	// 默认值: 5 分钟
	IdleTimeout time.Duration

	// TrimInterval 裁剪检查间隔
	//
	// 后台定期检查是否需要裁剪的间隔
	// 默认值: 1 分钟
	TrimInterval time.Duration

	// DialRatio 入站/出站比例控制
	//
	// 控制出站连接与入站连接的比例。
	// 例如：DialRatio = 3 表示允许 1/3 的连接为出站，2/3 为入站。
	// 设置为 0 表示使用默认值 3。
	// 默认值: 3
	DialRatio int
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		LowWater:       100,
		HighWater:      400,
		EmergencyWater: 600, // HighWater * 1.5
		GracePeriod:    20 * time.Second,
		DecayInterval:  1 * time.Minute,
		IdleTimeout:    5 * time.Minute,
		TrimInterval:   1 * time.Minute,
		DialRatio:      3, // 默认 1/3 出站，2/3 入站
	}
}

// Validate 验证配置
func (c Config) Validate() error {
	if c.LowWater <= 0 {
		return ErrInvalidConfig
	}
	if c.HighWater <= c.LowWater {
		return ErrInvalidConfig
	}
	if c.EmergencyWater > 0 && c.EmergencyWater <= c.HighWater {
		return ErrInvalidConfig
	}
	if c.GracePeriod < 0 {
		return ErrInvalidConfig
	}
	if c.IdleTimeout < 0 {
		return ErrInvalidConfig
	}
	if c.TrimInterval < 0 {
		return ErrInvalidConfig
	}
	if c.DialRatio < 0 {
		return ErrInvalidConfig
	}
	return nil
}

// WithLowWater 设置低水位
func (c Config) WithLowWater(low int) Config {
	c.LowWater = low
	return c
}

// WithHighWater 设置高水位
func (c Config) WithHighWater(high int) Config {
	c.HighWater = high
	return c
}

// WithGracePeriod 设置保护期
func (c Config) WithGracePeriod(period time.Duration) Config {
	c.GracePeriod = period
	return c
}
