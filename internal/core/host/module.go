package host

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/protocol"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ModuleInput 模块输入依赖
type ModuleInput struct {
	fx.In

	// 配置
	UnifiedCfg *config.Config `optional:"true"`

	// 必需依赖（基于接口，不使用 name 标签）
	Swarm     pkgif.Swarm
	Peerstore pkgif.Peerstore
	EventBus  pkgif.EventBus

	// 可选依赖
	ConnMgr  pkgif.ConnManager     `optional:"true"`
	ResMgr   pkgif.ResourceManager `optional:"true"`
	Protocol *protocol.Router      `optional:"true"`
	// 注意：NAT 和 Relay 不在这里直接依赖，避免循环依赖
	// 它们在各自的 lifecycle 中启动
}

// ModuleOutput 模块输出
type ModuleOutput struct {
	fx.Out

	Host pkgif.Host // 移除 name 标签，统一使用接口类型
}

// ConfigFromUnified 从统一配置创建 Host 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}
	return &Config{
		UserAgent:          "dep2p/1.0.0",
		ProtocolVersion:    "dep2p/1.0.0",
		AddrsFactory:       DefaultAddrsFactory,
		NegotiationTimeout: cfg.Security.NegotiateTimeout.Duration(),
		EnableMetrics:      cfg.Resource.EnableResourceManager,
	}
}

// ProvideHost 提供 Host 服务
func ProvideHost(input ModuleInput) (ModuleOutput, error) {
	// 从统一配置获取 Host 配置
	hostCfg := ConfigFromUnified(input.UnifiedCfg)

	// 构建 Host
	host, err := New(
		WithSwarm(input.Swarm),
		WithPeerstore(input.Peerstore),
		WithEventBus(input.EventBus),
		WithConfig(hostCfg),
	)
	if err != nil {
		return ModuleOutput{}, err
	}

	// 设置可选依赖
	if input.ConnMgr != nil {
		host.connmgr = input.ConnMgr
	}

	if input.ResMgr != nil {
		host.resourcemgr = input.ResMgr
	}

	if input.Protocol != nil {
		host.protocol = input.Protocol
	}

	// NAT 和 Relay 在各自的生命周期中管理

	return ModuleOutput{Host: host}, nil
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("host",
		fx.Provide(ProvideHost),
		fx.Invoke(
			registerLifecycle,
			registerIdentityKey, //
		),
	)
}

// lifecycleInput Lifecycle 注册输入
type lifecycleInput struct {
	fx.In
	LC   fx.Lifecycle
	Host pkgif.Host // 移除 name 标签
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 启动 Host
			if h, ok := input.Host.(*Host); ok {
				return h.Start(ctx)
			}
			return nil
		},
		OnStop: func(_ context.Context) error {
			// 停止 Host
			return input.Host.Close()
		},
	})
}

// identityKeyInput Identity 私钥注册输入
//
// 但 Identity 模块创建的私钥从未被添加到 Peerstore。
type identityKeyInput struct {
	fx.In

	LC        fx.Lifecycle
	Identity  pkgif.Identity
	Peerstore pkgif.Peerstore
}

// registerIdentityKey 将 Identity 私钥注册到 Peerstore
//
//   - Identity 模块生成/加载私钥，但不负责存储到 Peerstore
//   - Peerstore 模块创建 KeyBook，但不知道 Identity 的私钥
//   - 这导致 MemberLeave 签名时找不到私钥（key not found）
//
// 此函数在 Host 模块初始化时执行，将 Identity 私钥注册到 Peerstore，
// 使得后续的签名操作可以正常获取私钥。
func registerIdentityKey(input identityKeyInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			peerID := types.PeerID(input.Identity.PeerID())
			privKey := input.Identity.PrivateKey()

			if privKey == nil {
				return fmt.Errorf("Identity 私钥为空")
			}

			if err := input.Peerstore.AddPrivKey(peerID, privKey); err != nil {
				logger.Error("注册 Identity 私钥到 Peerstore 失败", "error", err)
				return fmt.Errorf("注册 Identity 私钥失败: %w", err)
			}

			logger.Debug("Identity 私钥已注册到 Peerstore", "peerID", string(peerID)[:8])
			return nil
		},
	})
}
