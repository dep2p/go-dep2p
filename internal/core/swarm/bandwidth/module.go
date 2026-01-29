// Package bandwidth 提供带宽统计模块的实现
package bandwidth

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("bandwidth",
		fx.Provide(ProvideCounter),
		fx.Invoke(registerLifecycle),
	)
}

// ProvideCounter 提供带宽计数器
func ProvideCounter(cfg *interfaces.BandwidthConfig) interfaces.BandwidthCounter {
	if cfg == nil {
		cfgVal := interfaces.DefaultBandwidthConfig()
		cfg = &cfgVal
	}
	return NewCounter(*cfg)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Counter interfaces.BandwidthCounter
	Config  *interfaces.BandwidthConfig `optional:"true"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	config := interfaces.DefaultBandwidthConfig()
	if input.Config != nil {
		config = *input.Config
	}

	var stopTrim chan struct{}

	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// 启动定期清理任务
			if config.TrimInterval > 0 {
				stopTrim = make(chan struct{})
				go func() {
					ticker := time.NewTicker(config.TrimInterval)
					defer ticker.Stop()

					for {
						select {
						case <-ticker.C:
							input.Counter.TrimIdle(time.Now().Add(-config.IdleTimeout))
						case <-stopTrim:
							return
						}
					}
				}()
			}

			return nil
		},
		OnStop: func(_ context.Context) error {
			// 停止清理任务
			if stopTrim != nil {
				close(stopTrim)
			}

			return nil
		},
	})
}
