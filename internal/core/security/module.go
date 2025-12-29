// Package security 提供安全层模块的实现
//
// 安全模块负责：
// - 连接加密
// - 身份验证
// - TLS/Noise 配置
// - 访问控制
package security

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"

	noiseimpl "github.com/dep2p/go-dep2p/internal/core/security/noise"
	tlsimpl "github.com/dep2p/go-dep2p/internal/core/security/tls"
)

var log = logger.Logger("security")

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Identity 身份服务
	Identity identityif.Identity `name:"identity"`

	// KeyFactory 密钥工厂（用于验证远程 identity，仅 Noise 协议需要）
	KeyFactory identityif.KeyFactory `name:"key_factory" optional:"true"`

	// Config 配置（可选）
	Config *securityif.Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// SecureTransport 安全传输
	SecureTransport securityif.SecureTransport `name:"secure_transport"`

	// CertificateManager 证书管理器
	CertificateManager securityif.CertificateManager `name:"certificate_manager"`

	// AccessController 访问控制器
	AccessController securityif.AccessController `name:"access_controller"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	// 验证必需依赖
	if input.Identity == nil {
		return ModuleOutput{}, fmt.Errorf("identity is required")
	}

	// 使用默认配置或输入配置
	config := securityif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 创建证书管理器（TLS 使用）
	certManager := tlsimpl.NewCertificateManager(input.Identity)

	// 创建访问控制器
	accessController := tlsimpl.NewAccessController()

	// 根据配置选择安全协议
	var transport securityif.SecureTransport
	var err error

	switch config.Protocol {
	case "noise":
		// Noise 协议需要 KeyFactory 来验证远程 identity
		if input.KeyFactory == nil {
			return ModuleOutput{}, fmt.Errorf("Noise 协议需要 KeyFactory 依赖")
		}

		// 确保 NoiseConfig 不为 nil
		noiseConfig := config.NoiseConfig
		if noiseConfig == nil {
			noiseConfig = securityif.DefaultNoiseConfig()
		}

		// 创建 Noise 传输（传入 KeyFactory 用于验证远程 identity）
		transport, err = noiseimpl.NewTransport(input.Identity, noiseConfig, input.KeyFactory)
		if err != nil {
			return ModuleOutput{}, fmt.Errorf("创建 Noise 传输失败: %w", err)
		}
		log.Info("使用 Noise 安全协议",
			"pattern", noiseConfig.HandshakePattern,
			"cipher", noiseConfig.CipherSuite)

	case "tls", "":
		// 创建 TLS 传输（默认）
		tlsTransport, err := tlsimpl.NewTransport(input.Identity, config)
		if err != nil {
			return ModuleOutput{}, fmt.Errorf("创建 TLS 传输失败: %w", err)
		}

		// 设置访问控制器
		tlsTransport.SetAccessController(accessController)
		transport = tlsTransport
		log.Info("使用 TLS 安全协议")

	default:
		return ModuleOutput{}, fmt.Errorf("不支持的安全协议: %s", config.Protocol)
	}

	return ModuleOutput{
		SecureTransport:    transport,
		CertificateManager: certManager,
		AccessController:   accessController,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("security",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC              fx.Lifecycle
	SecureTransport securityif.SecureTransport `name:"secure_transport"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("安全模块启动",
				"protocol", input.SecureTransport.Protocol())
			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("安全模块停止")
			return nil
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	Version     = "1.1.0"
	Name        = "security"
	Description = "安全层模块，提供 TLS/Noise 加密和身份验证"
)
