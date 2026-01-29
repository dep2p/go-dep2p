package metrics

import (
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// ============================================================================
// Fx 模块测试
// ============================================================================

// TestModule_Load 测试模块加载
func TestModule_Load(t *testing.T) {
	app := fxtest.New(t,
		Module,
		fx.Invoke(func(reporter Reporter) {
			if reporter == nil {
				t.Error("Reporter is nil")
			}
		}),
	)
	defer app.RequireStart().RequireStop()
}

// TestModule_Provides 测试模块提供的类型
func TestModule_Provides(t *testing.T) {
	var reporter Reporter

	app := fxtest.New(t,
		Module,
		fx.Populate(&reporter),
	)
	defer app.RequireStart().RequireStop()

	if reporter == nil {
		t.Fatal("Reporter not populated")
	}

	// 测试基本功能
	reporter.LogSentMessage(100)
	reporter.LogRecvMessage(200)

	stats := reporter.GetBandwidthTotals()
	if stats.TotalOut != 100 {
		t.Errorf("TotalOut = %d, want 100", stats.TotalOut)
	}
	if stats.TotalIn != 200 {
		t.Errorf("TotalIn = %d, want 200", stats.TotalIn)
	}
}

// TestModule_Lifecycle 测试生命周期
func TestModule_Lifecycle(t *testing.T) {
	var reporter Reporter

	app := fxtest.New(t,
		Module,
		fx.Populate(&reporter),
	)

	app.RequireStart()

	// 测试 Reporter 可用
	if reporter == nil {
		t.Fatal("Reporter not available after start")
	}

	reporter.LogSentMessage(512)
	stats := reporter.GetBandwidthTotals()
	if stats.TotalOut != 512 {
		t.Errorf("TotalOut = %d, want 512", stats.TotalOut)
	}

	app.RequireStop()
}
