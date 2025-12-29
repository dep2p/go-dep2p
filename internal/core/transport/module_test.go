package transport

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// identityProvider 提供命名的身份依赖
type identityProvider struct {
	fx.Out

	Identity identityif.Identity `name:"identity"`
}

func provideIdentity(t *testing.T) identityProvider {
	mgr := identity.NewManager(identityif.DefaultConfig())
	id, err := mgr.Create()
	require.NoError(t, err)
	return identityProvider{Identity: id}
}

// transportConsumer 消费命名的传输层依赖
type transportConsumer struct {
	fx.In

	Transport transportif.Transport `name:"transport"`
}

// TestModule 测试模块加载
func TestModule(t *testing.T) {
	var consumer transportConsumer

	app := fxtest.New(t,
		fx.Provide(func() identityProvider { return provideIdentity(t) }),
		Module(),
		fx.Populate(&consumer),
	)
	defer app.RequireStart().RequireStop()

	assert.NotNil(t, consumer.Transport, "传输层不应为 nil")
}

// TestModuleWithConfig 测试带配置的模块加载
func TestModuleWithConfig(t *testing.T) {
	config := transportif.Config{
		MaxConnections:    500,
		MaxStreamsPerConn: 50,
		IdleTimeout:       60 * time.Second,
		HandshakeTimeout:  15 * time.Second,
	}

	var consumer transportConsumer

	app := fxtest.New(t,
		fx.Provide(func() identityProvider { return provideIdentity(t) }),
		fx.Provide(func() *transportif.Config {
			return &config
		}),
		Module(),
		fx.Populate(&consumer),
	)
	defer app.RequireStart().RequireStop()

	assert.NotNil(t, consumer.Transport, "传输层不应为 nil")
}

// TestModuleLifecycle 测试模块生命周期
func TestModuleLifecycle(t *testing.T) {
	var consumer transportConsumer

	app := fxtest.New(t,
		fx.Provide(func() identityProvider { return provideIdentity(t) }),
		Module(),
		fx.Populate(&consumer),
	)

	// 启动
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := app.Start(ctx)
	require.NoError(t, err, "启动失败")

	// 验证传输层可用
	assert.NotNil(t, consumer.Transport, "传输层不应为 nil")

	// 停止
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	err = app.Stop(stopCtx)
	require.NoError(t, err, "停止失败")
}

// TestModuleMetadata 测试模块元信息
func TestModuleMetadata(t *testing.T) {
	assert.NotEmpty(t, Version, "版本号不应为空")
	assert.NotEmpty(t, Name, "模块名不应为空")
	assert.NotEmpty(t, Description, "描述不应为空")

	t.Logf("模块: %s v%s - %s", Name, Version, Description)
}

// ============================================================================
//                              错误场景测试
// ============================================================================

// TestProvideServices_NilIdentity 测试 nil Identity 返回错误
func TestProvideServices_NilIdentity(t *testing.T) {
	input := ModuleInput{
		Identity: nil,
	}

	_, err := ProvideServices(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity is required")
}

// TestProvideServices_Directly 测试直接调用 ProvideServices
func TestProvideServices_Directly(t *testing.T) {
	provider := provideIdentity(t)

	input := ModuleInput{
		Identity: provider.Identity,
	}

	output, err := ProvideServices(input)
	require.NoError(t, err)
	assert.NotNil(t, output.Transport)
	assert.NotNil(t, output.QUICTransport)

	// 清理
	err = output.Transport.Close()
	assert.NoError(t, err)
}

// TestProvideServices_WithConfig 测试带配置直接调用
func TestProvideServices_WithConfig(t *testing.T) {
	provider := provideIdentity(t)
	config := transportif.Config{
		MaxConnections:    100,
		MaxStreamsPerConn: 10,
		IdleTimeout:       30 * time.Second,
		HandshakeTimeout:  5 * time.Second,
	}

	input := ModuleInput{
		Identity: provider.Identity,
		Config:   &config,
	}

	output, err := ProvideServices(input)
	require.NoError(t, err)
	assert.NotNil(t, output.Transport)

	// 清理
	err = output.Transport.Close()
	assert.NoError(t, err)
}
