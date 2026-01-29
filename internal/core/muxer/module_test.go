package muxer

import (
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
		fx.Invoke(func(muxer pkgif.StreamMuxer) {
			if muxer == nil {
				t.Error("StreamMuxer is nil")
			}
		}),
	)
	defer app.RequireStart().RequireStop()
}

// TestModule_Provides 测试模块提供的类型
func TestModule_Provides(t *testing.T) {
	var muxer pkgif.StreamMuxer

	app := fxtest.New(t,
		Module,
		fx.Populate(&muxer),
	)
	defer app.RequireStart().RequireStop()

	if muxer == nil {
		t.Fatal("StreamMuxer not populated")
	}

	// 测试基本功能
	id := muxer.ID()
	if id != "/yamux/1.0.0" {
		t.Errorf("ID() = %s, want /yamux/1.0.0", id)
	}
}

// TestModule_Lifecycle 测试生命周期钩子
func TestModule_Lifecycle(t *testing.T) {
	var muxer pkgif.StreamMuxer

	app := fxtest.New(t,
		Module,
		fx.Populate(&muxer),
	)

	app.RequireStart()

	// 测试 Muxer 可用
	if muxer == nil {
		t.Fatal("StreamMuxer not available after start")
	}

	id := muxer.ID()
	if id == "" {
		t.Error("ID() returned empty string")
	}

	app.RequireStop()
}
