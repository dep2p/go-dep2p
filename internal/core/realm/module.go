// Package realm 实现 Realm（领域）管理模块
//
// Realm 是 dep2p 实现业务隔离的核心机制：
// - 共享底层基础设施（DHT、中继、NAT 穿透）
// - 业务层完全隔离（不同 Realm 互不可见）
//
// v1.1 变更:
//   - 采用严格单 Realm 模型
//   - 已移除跨 Realm 通信支持
package realm

import (
	"context"
	"crypto/ed25519"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/config"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置
	Config *config.Config

	// Endpoint 网络端点（可选）
	Endpoint endpoint.Endpoint `name:"endpoint" optional:"true"`

	// Identity 身份（可选，用于 RealmAuth 出站签名）
	Identity identityif.Identity `name:"identity" optional:"true"`

	// Messaging 消息服务（可选）
	Messaging messagingif.MessagingService `name:"messaging" optional:"true"`

	// IMPL-1227 Phase 4: Relay 服务（可选，用于 Realm Relay 适配）
	RelayServer relayif.RelayServer `name:"relay_server" optional:"true"`
	RelayClient relayif.RelayClient `name:"relay" optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// RealmManager Realm 管理器
	RealmManager realmif.RealmManager `name:"realm_manager"`

	// AccessController 访问控制器
	// 使用 realm_access_controller 避免与 security 模块的 access_controller 冲突
	AccessController realmif.RealmAccessController `name:"realm_access_controller"`

	// RealmDiscovery Realm 发现服务
	RealmDiscovery realmif.RealmDiscovery `name:"realm_discovery"`

	// RealmMessaging Realm 消息服务
	RealmMessaging realmif.RealmMessaging `name:"realm_messaging"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	cfg := input.Config.Realm

	// 创建 RealmManager
	realmManager := NewManager(cfg, input.Endpoint)

	// IMPL-1227 Phase 4: 注入消息服务
	if input.Messaging != nil {
		realmManager.SetMessaging(input.Messaging)
	}

	// IMPL-1227 Phase 4: 注入 Relay 服务（用于 Realm Relay 适配）
	if input.RelayServer != nil || input.RelayClient != nil {
		realmManager.SetRelayServices(input.RelayServer, input.RelayClient)
		log.Debug("已注入 Relay 服务到 RealmManager",
			"hasServer", input.RelayServer != nil,
			"hasClient", input.RelayClient != nil)
	}

	// v1.1: RealmAuth（连接级验证）
	// - 入站：注册 RealmAuth 协议处理器（在 lifecycle OnStart 中完成）
	// - 出站：可选使用本地私钥签名请求；当前实现未强制校验签名，因此即使没有私钥也可工作
	if cfg.RealmAuthEnabled {
		var pk ed25519.PrivateKey
		if input.Identity != nil {
			if raw, ok := input.Identity.PrivateKey().Raw().(ed25519.PrivateKey); ok {
				pk = raw
			} else {
				log.Warn("RealmAuth 出站签名不可用（非 ed25519 私钥），将以无签名模式运行",
					"keyType", input.Identity.PrivateKey().Type())
			}
		} else {
			log.Warn("RealmAuth 出站签名不可用（identity 未注入），将以无签名模式运行")
		}

		auth := NewAuthenticator(realmManager, pk)
		if cfg.RealmAuthTimeout > 0 {
			auth.SetTimeout(cfg.RealmAuthTimeout)
		}
		realmManager.SetAuthenticator(auth)
	}

	// 创建 AccessController
	accessController := NewAccessController(realmManager)

	// 创建 RealmDiscovery
	realmDiscovery := NewRealmDiscoveryWrapper(realmManager)

	// 创建 RealmMessaging
	realmMessaging := NewRealmMessagingWrapper(realmManager, input.Messaging)

	return ModuleOutput{
		RealmManager:     realmManager,
		AccessController: accessController,
		RealmDiscovery:   realmDiscovery,
		RealmMessaging:   realmMessaging,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("realm",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In

	LC           fx.Lifecycle
	RealmManager realmif.RealmManager `name:"realm_manager"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("Realm 模块启动")

			// 启动 RealmManager
			if manager, ok := input.RealmManager.(*Manager); ok {
				if err := manager.Start(ctx); err != nil {
					return err
				}

				// 注册 RealmAuth 协议处理器（系统协议，无需 RealmContext）
				manager.RegisterRealmAuthHandler()
			}

			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("Realm 模块停止")

			// 停止 RealmManager
			if manager, ok := input.RealmManager.(*Manager); ok {
				manager.Stop()
			}

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
	// v1.1: 严格单 Realm 模型
	Version = "1.1.0"
	// Name 模块名称
	Name = "realm"
	// Description 模块描述
	// v1.1: 移除跨 Realm 通信描述
	Description = "Realm 管理模块，提供业务隔离和强制访问控制能力"
)
