package eventbus

import (
	"context"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"go.uber.org/fx"
)

// ============================================================================
// Fx 模块测试
// ============================================================================

// TestModule_Load 测试 Fx 模块加载
func TestModule_Load(t *testing.T) {
	var loadedBus pkgif.EventBus

	app := fx.New(
		Module(),
		fx.Invoke(func(bus pkgif.EventBus) {
			loadedBus = bus
		}),
	)

	ctx := context.Background()

	// 启动应用
	if err := app.Start(ctx); err != nil {
		t.Fatalf("app.Start() failed: %v", err)
	}

	// 验证 EventBus 注入成功
	if loadedBus == nil {
		t.Error("EventBus not injected by Fx")
	}

	// 停止应用
	if err := app.Stop(ctx); err != nil {
		t.Errorf("app.Stop() failed: %v", err)
	}
}

// TestModule_Provides 测试模块提供的类型
func TestModule_Provides(t *testing.T) {
	result := ProvideEventBus()

	if result.EventBus == nil {
		t.Error("ProvideEventBus() did not provide EventBus")
	}
}

// TestModule_Lifecycle 测试生命周期钩子
func TestModule_Lifecycle(t *testing.T) {
	app := fx.New(
		Module(),
		fx.NopLogger,
	)

	ctx := context.Background()

	// 启动
	if err := app.Start(ctx); err != nil {
		t.Fatalf("app.Start() failed: %v", err)
	}

	// 停止
	if err := app.Stop(ctx); err != nil {
		t.Errorf("app.Stop() failed: %v", err)
	}
}
