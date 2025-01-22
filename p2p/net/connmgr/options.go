package connmgr

import (
	"errors"
	"time"

	"github.com/benbjohnson/clock"
)

// config 是基础连接管理器的配置结构体
type config struct {
	// 连接数量的高水位线
	highWater int
	// 连接数量的低水位线
	lowWater int
	// 新连接的宽限期
	gracePeriod time.Duration
	// 清理检查的静默期
	silencePeriod time.Duration
	// 衰减器配置
	decayer *DecayerCfg
	// 是否启用紧急修剪
	emergencyTrim bool
	// 内部时钟实现
	clock clock.Clock
}

// Option 表示基础连接管理器的配置选项函数
// 参数:
//   - *config: 配置对象指针
//
// 返回值:
//   - error: 配置错误信息
type Option func(*config) error

// DecayerConfig 应用衰减器的配置
// 参数:
//   - opts: 衰减器配置选项
//
// 返回值:
//   - Option: 配置选项函数
func DecayerConfig(opts *DecayerCfg) Option {
	return func(cfg *config) error {
		// 设置衰减器配置
		cfg.decayer = opts
		return nil
	}
}

// WithClock 设置内部时钟实现
// 参数:
//   - c: 时钟实现对象
//
// 返回值:
//   - Option: 配置选项函数
func WithClock(c clock.Clock) Option {
	return func(cfg *config) error {
		// 设置时钟实现
		cfg.clock = c
		return nil
	}
}

// WithGracePeriod 设置宽限期
// 宽限期是新打开的连接在被修剪之前给予的时间
// 参数:
//   - p: 宽限期时长
//
// 返回值:
//   - Option: 配置选项函数
func WithGracePeriod(p time.Duration) Option {
	return func(cfg *config) error {
		// 检查宽限期是否为负值
		if p < 0 {
			log.Debugf("宽限期必须是非负值")
			return errors.New("宽限期必须是非负值")
		}
		// 设置宽限期
		cfg.gracePeriod = p
		return nil
	}
}

// WithSilencePeriod 设置静默期
// 如果连接数量超过高水位线，连接管理器将在每个静默期执行一次清理
// 参数:
//   - p: 静默期时长
//
// 返回值:
//   - Option: 配置选项函数
func WithSilencePeriod(p time.Duration) Option {
	return func(cfg *config) error {
		// 检查静默期是否小于等于0
		if p <= 0 {
			log.Debugf("静默期必须是非零值")
			return errors.New("静默期必须是非零值")
		}
		// 设置静默期
		cfg.silencePeriod = p
		return nil
	}
}
