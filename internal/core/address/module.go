// Package address 提供地址管理模块的实现
//
// 地址模块负责：
// - 地址解析与格式化
// - 地址簿管理
// - 地址签名与验证
// - 地址优先级排序
package address

import (
	"context"

	"go.uber.org/fx"

	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Identity 身份服务
	Identity identityif.Identity `name:"identity"`

	// NAT 服务（可选）
	NATService natif.NATService `name:"nat" optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// AddressBook 地址簿
	AddressBook *AddressBook `name:"address_book"`

	// AddressManager 地址管理器
	AddressManager *AddressManager `name:"address_manager"`

	// Parser 地址解析器（实现类型）
	Parser *Parser `name:"address_parser"`

	// AddressParser 地址解析器接口（供其他组件通过 Fx 注入使用）
	AddressParser addressif.AddressParser `name:"address_parser_if"`

	// Validator 地址验证器
	Validator *Validator `name:"address_validator"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	// 创建地址簿
	book := NewAddressBook()

	// 创建地址管理器
	manager := NewAddressManager(DefaultAddressManagerConfig())
	if input.NATService != nil {
		manager.SetNATService(input.NATService)
	}

	// 创建解析器
	parser := NewParser()

	// 创建验证器
	validator := NewValidator(DefaultValidatorConfig())

	return ModuleOutput{
		AddressBook:    book,
		AddressManager: manager,
		Parser:         parser,
		AddressParser:  parser, // 同时提供接口类型
		Validator:      validator,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("address",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC             fx.Lifecycle
	AddressBook    *AddressBook    `name:"address_book"`
	AddressManager *AddressManager `name:"address_manager"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 启动地址管理器
			return input.AddressManager.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			// 停止地址管理器
			return input.AddressManager.Stop()
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	Version     = "1.0.0"
	Name        = "address"
	Description = "地址管理模块，提供地址解析、地址簿和地址签名能力"
)
