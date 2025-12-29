package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
)

// provideTestIdentity 提供测试用的 Identity
func provideTestIdentity(t *testing.T) identityif.Identity {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)
	return ident
}

func TestModule(t *testing.T) {
	ident := provideTestIdentity(t)

	var secureTransport securityif.SecureTransport
	var certManager securityif.CertificateManager
	var accessController securityif.AccessController

	app := fxtest.New(t,
		fx.Provide(func() identityif.Identity { return ident }),
		fx.Provide(func() *identityif.Identity {
			return nil // 可选参数
		}),
		fx.Decorate(func(ident identityif.Identity) identityif.Identity {
			return ident
		}),
		// 手动提供命名依赖
		fx.Provide(
			fx.Annotate(
				func() identityif.Identity { return ident },
				fx.ResultTags(`name:"identity"`),
			),
		),
		Module(),
		fx.Populate(
			fx.Annotate(&secureTransport, fx.ParamTags(`name:"secure_transport"`)),
			fx.Annotate(&certManager, fx.ParamTags(`name:"certificate_manager"`)),
			fx.Annotate(&accessController, fx.ParamTags(`name:"access_controller"`)),
		),
	)
	defer app.RequireStart().RequireStop()

	assert.NotNil(t, secureTransport)
	assert.NotNil(t, certManager)
	assert.NotNil(t, accessController)
	assert.Equal(t, "tls", secureTransport.Protocol())
}

func TestModuleWithConfig(t *testing.T) {
	ident := provideTestIdentity(t)

	customConfig := securityif.Config{
		Protocol:          "tls",
		RequireClientAuth: false,
	}

	var secureTransport securityif.SecureTransport

	app := fxtest.New(t,
		fx.Provide(
			fx.Annotate(
				func() identityif.Identity { return ident },
				fx.ResultTags(`name:"identity"`),
			),
		),
		fx.Provide(func() *securityif.Config { return &customConfig }),
		Module(),
		fx.Populate(
			fx.Annotate(&secureTransport, fx.ParamTags(`name:"secure_transport"`)),
		),
	)
	defer app.RequireStart().RequireStop()

	assert.NotNil(t, secureTransport)
}

func TestModuleLifecycle(t *testing.T) {
	ident := provideTestIdentity(t)

	var secureTransport securityif.SecureTransport

	app := fxtest.New(t,
		fx.Provide(
			fx.Annotate(
				func() identityif.Identity { return ident },
				fx.ResultTags(`name:"identity"`),
			),
		),
		Module(),
		fx.Populate(
			fx.Annotate(&secureTransport, fx.ParamTags(`name:"secure_transport"`)),
		),
	)

	// 启动应用
	err := app.Start(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, secureTransport)

	// 停止应用
	err = app.Stop(context.Background())
	require.NoError(t, err)
}

func TestProvideServicesDirectly(t *testing.T) {
	ident := provideTestIdentity(t)

	input := ModuleInput{
		Identity: ident,
	}

	output, err := ProvideServices(input)
	require.NoError(t, err)
	assert.NotNil(t, output.SecureTransport)
	assert.NotNil(t, output.CertificateManager)
	assert.NotNil(t, output.AccessController)
}

func TestProvideServicesWithConfig(t *testing.T) {
	ident := provideTestIdentity(t)
	config := securityif.DefaultConfig()

	input := ModuleInput{
		Identity: ident,
		Config:   &config,
	}

	output, err := ProvideServices(input)
	require.NoError(t, err)
	assert.NotNil(t, output.SecureTransport)
}

func TestModuleConstants(t *testing.T) {
	assert.Equal(t, "1.1.0", Version)
	assert.Equal(t, "security", Name)
	assert.NotEmpty(t, Description)
}

// ============================================================================
//                              错误场景测试
// ============================================================================

func TestProvideServices_NilIdentity(t *testing.T) {
	input := ModuleInput{
		Identity: nil, // nil Identity
	}

	_, err := ProvideServices(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity is required")
}

func TestProvideServices_InvalidProtocol(t *testing.T) {
	ident := provideTestIdentity(t)

	config := securityif.Config{
		Protocol: "invalid_protocol",
	}

	input := ModuleInput{
		Identity: ident,
		Config:   &config,
	}

	_, err := ProvideServices(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的安全协议")
}

func TestProvideServices_NoiseProtocol(t *testing.T) {
	ident := provideTestIdentity(t)
	keyFactory := identity.NewKeyFactory()

	config := securityif.Config{
		Protocol:    "noise",
		NoiseConfig: securityif.DefaultNoiseConfig(),
	}

	input := ModuleInput{
		Identity:   ident,
		KeyFactory: keyFactory,
		Config:     &config,
	}

	output, err := ProvideServices(input)
	require.NoError(t, err)
	assert.NotNil(t, output.SecureTransport)
	assert.Equal(t, "noise", output.SecureTransport.Protocol())
}

func TestProvideServices_NoiseProtocol_NilNoiseConfig(t *testing.T) {
	ident := provideTestIdentity(t)
	keyFactory := identity.NewKeyFactory()

	// NoiseConfig 为 nil，应使用默认值
	config := securityif.Config{
		Protocol:    "noise",
		NoiseConfig: nil,
	}

	input := ModuleInput{
		Identity:   ident,
		KeyFactory: keyFactory,
		Config:     &config,
	}

	output, err := ProvideServices(input)
	require.NoError(t, err)
	assert.NotNil(t, output.SecureTransport)
	assert.Equal(t, "noise", output.SecureTransport.Protocol())
}

