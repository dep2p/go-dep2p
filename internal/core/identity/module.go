// Package identity 提供身份管理模块的实现
//
// 身份模块负责：
// - 密钥对生成和管理
// - 签名和验证
// - 身份持久化
package identity

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// 配置（可选，使用默认配置）
	Config *identityif.Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// 公共接口导出（命名依赖）
	Identity        identityif.Identity        `name:"identity"`
	IdentityManager identityif.IdentityManager `name:"identity_manager"`

	// KeyFactory 密钥工厂（供其他组件通过 Fx 注入使用）
	KeyFactory identityif.KeyFactory `name:"key_factory"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	// 使用默认配置或输入配置
	config := identityif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 创建身份管理器
	manager := NewManager(config)

	// 创建或加载身份
	// 优先级：PrivateKey > IdentityPath > AutoCreate
	var id identityif.Identity
	var err error

	if config.PrivateKey != nil {
		// 优先使用直接注入的私钥（WithIdentity 场景）
		id = NewIdentity(config.PrivateKey)
	} else if config.IdentityPath != "" {
		// 尝试从文件加载
		id, err = manager.Load(config.IdentityPath)
		if err != nil && config.AutoCreate {
			// 加载失败，自动创建新身份
			id, err = manager.Create()
			if err != nil {
				return ModuleOutput{}, fmt.Errorf("创建身份失败: %w", err)
			}
			// 保存到文件
			if saveErr := manager.Save(id, config.IdentityPath); saveErr != nil {
				// 保存失败不影响功能，仅记录
				_ = saveErr
			}
		} else if err != nil {
			return ModuleOutput{}, fmt.Errorf("加载身份失败: %w", err)
		}
	} else if config.AutoCreate {
		// 没有指定路径，直接创建新身份
		id, err = manager.Create()
		if err != nil {
			return ModuleOutput{}, fmt.Errorf("创建身份失败: %w", err)
		}
	}

	if id == nil {
		return ModuleOutput{}, fmt.Errorf("未能创建或加载身份")
	}

	// 创建密钥工厂（供其他组件使用）
	keyFactory := NewKeyFactory()

	return ModuleOutput{
		Identity:        id,
		IdentityManager: manager,
		KeyFactory:      keyFactory,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("identity",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC       fx.Lifecycle
	Identity identityif.Identity `name:"identity"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// 身份模块启动（当前无需特殊启动逻辑）
			return nil
		},
		OnStop: func(_ context.Context) error {
			// 身份模块停止（当前无需特殊停止逻辑）
			return nil
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	// Version 模块版本
	Version = "1.0.0"
	// Name 模块名称
	Name = "identity"
	// Description 模块描述
	Description = "身份管理模块，提供密钥对生成、签名验证和身份管理能力"
)
