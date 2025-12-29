package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// App dep2p 应用接口
//
// App 提供应用级别的生命周期管理
type App interface {
	// Endpoint 返回底层的 Endpoint
	Endpoint() endpoint.Endpoint

	// Wait 等待应用收到退出信号
	Wait()

	// Stop 停止应用
	Stop() error
}

// internalApp App 的内部实现
type internalApp struct {
	bootstrap *Bootstrap
	endpoint  endpoint.Endpoint
	stopOnce  sync.Once
	stopped   chan struct{}
}

// RunApp 运行 dep2p 应用
//
// 这是一个便捷函数，用于运行一个完整的 dep2p 应用：
// - 构建并启动节点
// - 等待退出信号
// - 优雅关闭
//
// 示例:
//
//	app, err := app.RunApp(ctx, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	app.Wait()
func RunApp(ctx context.Context, bootstrap *Bootstrap) (App, error) {
	// 构建并启动
	ep, err := bootstrap.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("启动应用失败: %w", err)
	}

	app := &internalApp{
		bootstrap: bootstrap,
		endpoint:  ep,
		stopped:   make(chan struct{}),
	}

	return app, nil
}

// Endpoint 返回底层的 Endpoint
func (a *internalApp) Endpoint() endpoint.Endpoint {
	return a.endpoint
}

// Wait 等待应用收到退出信号
func (a *internalApp) Wait() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signals:
		fmt.Printf("收到信号 %v，正在退出...\n", sig)
	case <-a.stopped:
		return
	}

	// 停止应用
	_ = a.Stop()
}

// Stop 停止应用
func (a *internalApp) Stop() error {
	var err error
	a.stopOnce.Do(func() {
		close(a.stopped)

		// 停止 endpoint
		if a.endpoint != nil {
			if closeErr := a.endpoint.Close(); closeErr != nil {
				err = fmt.Errorf("关闭 endpoint 失败: %w", closeErr)
			}
		}

		// 停止 bootstrap
		if a.bootstrap != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if stopErr := a.bootstrap.Stop(ctx); stopErr != nil {
				if err != nil {
					err = fmt.Errorf("%w; 停止 bootstrap 失败: %v", err, stopErr)
				} else {
					err = fmt.Errorf("停止 bootstrap 失败: %w", stopErr)
				}
			}
		}
	})
	return err
}

// ============================================================================
//                              生命周期钩子
// ============================================================================

// LifecycleHook 生命周期钩子
type LifecycleHook struct {
	// OnStart 启动时调用
	OnStart func(context.Context) error

	// OnStop 停止时调用
	OnStop func(context.Context) error
}

// LifecycleManager 生命周期管理器
type LifecycleManager struct {
	hooks []LifecycleHook
	mu    sync.Mutex
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager() *LifecycleManager {
	return &LifecycleManager{
		hooks: make([]LifecycleHook, 0),
	}
}

// AddHook 添加生命周期钩子
func (m *LifecycleManager) AddHook(hook LifecycleHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// Start 执行所有启动钩子
func (m *LifecycleManager) Start(ctx context.Context) error {
	m.mu.Lock()
	hooks := make([]LifecycleHook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	for i, hook := range hooks {
		if hook.OnStart != nil {
			if err := hook.OnStart(ctx); err != nil {
				// 回滚已启动的钩子
				for j := i - 1; j >= 0; j-- {
					if hooks[j].OnStop != nil {
						_ = hooks[j].OnStop(ctx)
					}
				}
				return fmt.Errorf("启动钩子 %d 失败: %w", i, err)
			}
		}
	}
	return nil
}

// Stop 执行所有停止钩子（逆序）
func (m *LifecycleManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	hooks := make([]LifecycleHook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	var errs []error
	for i := len(hooks) - 1; i >= 0; i-- {
		if hooks[i].OnStop != nil {
			if err := hooks[i].OnStop(ctx); err != nil {
				errs = append(errs, fmt.Errorf("停止钩子 %d 失败: %w", i, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("停止钩子失败: %v", errs)
	}
	return nil
}

