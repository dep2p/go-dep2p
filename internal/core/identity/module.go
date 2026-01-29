// Package identity 实现身份管理
package identity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"go.uber.org/fx"
)

// ============================================================================
// 配置
// ============================================================================

// Config 身份模块配置
type Config struct {
	// KeyType 密钥类型（默认 Ed25519）
	KeyType pkgif.KeyType

	// PrivKeyPath 私钥文件路径（可选）
	// 如果指定，则从文件加载；否则生成新密钥
	PrivKeyPath string

	// AutoCreate 是否自动创建身份（默认 true）
	// 如果为 false 且无法加载身份，则返回错误
	AutoCreate bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		KeyType:    pkgif.KeyTypeEd25519,
		AutoCreate: true, // 默认自动创建
	}
}

// ConfigFromUnified 从统一配置创建身份配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}

	// 将字符串密钥类型转换为 pkgif.KeyType
	var keyType pkgif.KeyType
	switch cfg.Identity.KeyType {
	case "RSA":
		keyType = pkgif.KeyTypeRSA
	case "ECDSA":
		keyType = pkgif.KeyTypeECDSA
	case "Secp256k1":
		keyType = pkgif.KeyTypeSecp256k1
	default:
		keyType = pkgif.KeyTypeEd25519
	}

	return &Config{
		KeyType:     keyType,
		PrivKeyPath: cfg.Identity.KeyFile,
		AutoCreate:  cfg.Identity.AutoGenerate,
	}
}

// ============================================================================
// Fx 模块
// ============================================================================

// Params Fx 模块输入参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
	Config     *Config        `optional:"true"` // 直接配置（用于测试或手动配置）
}

// Result Fx 模块输出结果
type Result struct {
	fx.Out

	Identity  pkgif.Identity
	LocalPeer string // 提供给 Swarm 的本地节点 ID
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("identity",
		fx.Provide(
			ProvideIdentity,
			ProvideDeviceIdentity,
		),
		fx.Invoke(registerLifecycle),
	)
}

// DeviceIdentityParams DeviceIdentity 依赖参数
type DeviceIdentityParams struct {
	fx.In

	Identity pkgif.Identity
}

// ProvideDeviceIdentity 提供 DeviceIdentity 实例
func ProvideDeviceIdentity(p DeviceIdentityParams) (*DeviceIdentity, error) {
	di, err := NewDeviceIdentity(DefaultDeviceIdentityConfig())
	if err != nil {
		return nil, err
	}

	// 自动绑定到节点身份
	if err := di.BindToPeer(p.Identity); err != nil {
		return nil, err
	}

	return di, nil
}

// ProvideIdentity 提供 Identity 实例
func ProvideIdentity(p Params) (Result, error) {
	// 优先使用直接配置，否则从统一配置转换
	cfg := p.Config
	if cfg == nil {
		cfg = ConfigFromUnified(p.UnifiedCfg)
	}

	var id *Identity
	var err error

	// 如果指定了私钥路径，尝试加载
	if cfg.PrivKeyPath != "" {
		id, err = loadIdentityFromFile(cfg.PrivKeyPath)
		if err == nil {
			return Result{Identity: id, LocalPeer: string(id.PeerID())}, nil
		}

		// 如果加载失败且文件存在，返回错误
		if !os.IsNotExist(err) {
			return Result{}, fmt.Errorf("failed to load identity from %s: %w", cfg.PrivKeyPath, err)
		}

		// 文件不存在
		if !cfg.AutoCreate {
			// 不允许自动创建，返回错误
			return Result{}, fmt.Errorf("identity file not found and AutoCreate is false: %s", cfg.PrivKeyPath)
		}

		// 生成新身份并保存
		id, err = Generate()
		if err != nil {
			return Result{}, fmt.Errorf("failed to generate identity: %w", err)
		}

		if err := saveIdentityToFile(id, cfg.PrivKeyPath); err != nil {
			return Result{}, fmt.Errorf("failed to save identity to %s: %w", cfg.PrivKeyPath, err)
		}

		return Result{Identity: id, LocalPeer: string(id.PeerID())}, nil
	}

	// 没有指定路径，生成临时身份
	id, err = Generate()
	if err != nil {
		return Result{}, fmt.Errorf("failed to generate identity: %w", err)
	}

	return Result{Identity: id, LocalPeer: string(id.PeerID())}, nil
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(lc fx.Lifecycle, id pkgif.Identity) {
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// 启动时打印 PeerID
			fmt.Printf("Identity started with PeerID: %s\n", id.PeerID())
			return nil
		},
		OnStop: func(_ context.Context) error {
			// 停止时清理（如有需要）
			return nil
		},
	})
}

// ============================================================================
// 辅助函数
// ============================================================================

// loadIdentityFromFile 从文件加载身份
func loadIdentityFromFile(path string) (*Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	priv, err := UnmarshalPrivateKeyPEM(data)
	if err != nil {
		return nil, err
	}

	return New(priv)
}

// saveIdentityToFile 保存身份到文件
func saveIdentityToFile(id *Identity, path string) error {
	pemBytes, err := MarshalPrivateKeyPEM(id.PrivateKey())
	if err != nil {
		return err
	}

	// 确保父目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	return os.WriteFile(path, pemBytes, 0600) // 只有所有者可读写
}
