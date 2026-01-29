package resourcemgr

import (
	"context"
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// Fx 模块测试
// ============================================================================

// TestModule_Load 测试模块加载
func TestModule_Load(t *testing.T) {
	app := fxtest.New(t,
		Module,
		fx.Invoke(func(rm pkgif.ResourceManager) {
			if rm == nil {
				t.Error("ResourceManager is nil")
			}
		}),
	)
	defer app.RequireStart().RequireStop()
}

// TestModule_Provides 测试模块提供的类型
func TestModule_Provides(t *testing.T) {
	var rm pkgif.ResourceManager

	app := fxtest.New(t,
		Module,
		fx.Populate(&rm),
	)
	defer app.RequireStart().RequireStop()

	if rm == nil {
		t.Fatal("ResourceManager not populated")
	}

	// 测试基本功能
	err := rm.ViewSystem(func(s pkgif.ResourceScope) error {
		stat := s.Stat()
		if stat.Memory < 0 {
			t.Error("Invalid memory stat")
		}
		return nil
	})
	if err != nil {
		t.Errorf("ViewSystem() failed: %v", err)
	}
}

// TestModule_Lifecycle 测试生命周期钩子
func TestModule_Lifecycle(t *testing.T) {
	var rm pkgif.ResourceManager
	var startCalled, stopCalled bool

	app := fx.New(
		Module,
		fx.Populate(&rm),
		fx.Invoke(func(lc fx.Lifecycle) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					startCalled = true
					return nil
				},
				OnStop: func(ctx context.Context) error {
					stopCalled = true
					return nil
				},
			})
		}),
		fx.NopLogger,
	)

	// 启动应用
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start app: %v", err)
	}

	if !startCalled {
		t.Error("OnStart hook not called")
	}

	// 测试 ResourceManager 可用
	if rm == nil {
		t.Fatal("ResourceManager not available after start")
	}

	err := rm.ViewSystem(func(s pkgif.ResourceScope) error {
		return nil
	})
	if err != nil {
		t.Errorf("ViewSystem() failed after start: %v", err)
	}

	// 停止应用
	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("Failed to stop app: %v", err)
	}

	if !stopCalled {
		t.Error("OnStop hook not called")
	}
}
