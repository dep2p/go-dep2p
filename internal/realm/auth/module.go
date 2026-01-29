package auth

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//
//	Fx 模块定义
//
// ============================================================================

// Module Auth Fx 模块
var Module = fx.Module("realm_auth",
	fx.Provide(
		NewAuthManagerFromParams,
		NewAuthenticatorFactoryFromParams,
	),
	fx.Invoke(registerLifecycle),
)

// ============================================================================
//
//	配置转换
//
// ============================================================================

// AuthManagerConfig AuthManager 配置（从统一配置提取）
type AuthManagerConfig struct {
	EnablePSK       bool
	EnableChallenge bool
	MaxRetries      int
}

// ConfigFromUnified 从统一配置创建 Auth 配置
func ConfigFromUnified(cfg *config.Config) AuthManagerConfig {
	if cfg == nil || !cfg.Realm.EnableAuth {
		return AuthManagerConfig{
			EnablePSK:       true,
			EnableChallenge: false,
			MaxRetries:      3,
		}
	}
	return AuthManagerConfig{
		EnablePSK:       cfg.Realm.Auth.EnablePSK,
		EnableChallenge: cfg.Realm.Auth.EnableChallenge,
		MaxRetries:      cfg.Realm.Auth.MaxRetries,
	}
}

// ============================================================================
//
//	Fx 参数和结果
//
// ============================================================================

// ManagerParams AuthManager 依赖参数
type ManagerParams struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// ManagerResult AuthManager 导出结果
type ManagerResult struct {
	fx.Out

	Manager interfaces.AuthManager
}

// FactoryParams AuthenticatorFactory 依赖参数
type FactoryParams struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// FactoryResult AuthenticatorFactory 导出结果
type FactoryResult struct {
	fx.Out

	Factory *AuthenticatorFactory
}

// ============================================================================
//
//	构造函数
//
// ============================================================================

// NewAuthManagerFromParams 从 Fx 参数创建 AuthManager
func NewAuthManagerFromParams(p ManagerParams) (ManagerResult, error) {
	// 从统一配置读取（当前 AuthManager 不需要配置）
	_ = ConfigFromUnified(p.UnifiedCfg)
	manager := NewAuthManager()

	return ManagerResult{
		Manager: manager,
	}, nil
}

// NewAuthenticatorFactoryFromParams 从 Fx 参数创建 AuthenticatorFactory
func NewAuthenticatorFactoryFromParams(_ FactoryParams) (FactoryResult, error) {
	factory := NewAuthenticatorFactory()

	return FactoryResult{
		Factory: factory,
	}, nil
}

// ============================================================================
//
//	生命周期管理
//
// ============================================================================

// lifecycleInput Lifecycle 注册输入
type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Manager interfaces.AuthManager
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// AuthManager 不需要特殊的启动逻辑
			return nil
		},
		OnStop: func(_ context.Context) error {
			// 关闭 AuthManager
			return input.Manager.Close()
		},
	})
}
