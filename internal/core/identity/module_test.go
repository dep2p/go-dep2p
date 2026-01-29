package identity

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
	var loadedID pkgif.Identity

	app := fx.New(
		Module(),
		fx.Invoke(func(id pkgif.Identity) {
			loadedID = id
		}),
	)

	ctx := context.Background()

	// 启动应用
	if err := app.Start(ctx); err != nil {
		t.Fatalf("app.Start() failed: %v", err)
	}

	// 验证 Identity 注入成功
	if loadedID == nil {
		t.Error("Identity not injected by Fx")
	}

	if loadedID.PeerID() == "" {
		t.Error("Injected Identity has empty PeerID")
	}

	// 停止应用
	if err := app.Stop(ctx); err != nil {
		t.Errorf("app.Stop() failed: %v", err)
	}
}

// TestModule_Provides 测试模块提供的类型
func TestModule_Provides(t *testing.T) {
	result, err := ProvideIdentity(Params{})
	if err != nil {
		t.Fatalf("ProvideIdentity() failed: %v", err)
	}

	if result.Identity == nil {
		t.Error("ProvideIdentity() did not provide Identity")
	}
}
