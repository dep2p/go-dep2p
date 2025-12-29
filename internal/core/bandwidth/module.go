package bandwidth

import (
	"context"
	"time"

	"go.uber.org/fx"

	bandwidthif "github.com/dep2p/go-dep2p/pkg/interfaces/bandwidth"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置（可选）
	Config *bandwidthif.Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// Counter 带宽计数器
	Counter bandwidthif.Counter `name:"bandwidth_counter"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	// 使用默认配置或输入配置
	config := bandwidthif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 创建计数器
	counter := NewCounter(config)

	return ModuleOutput{
		Counter: counter,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("bandwidth",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Counter bandwidthif.Counter `name:"bandwidth_counter"`
	Config  *bandwidthif.Config `optional:"true"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	config := bandwidthif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	var stopTrim chan struct{}

	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("带宽统计模块启动")

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
			log.Info("带宽统计模块停止")

			// 停止清理任务
			if stopTrim != nil {
				close(stopTrim)
			}

			return nil
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	Version     = "1.0.0"
	Name        = "bandwidth"
	Description = "带宽统计模块，提供按总量/Peer/Protocol 的流量统计能力"
)
