package relay

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/lifecycle"
	"github.com/dep2p/go-dep2p/internal/core/relay/client"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"go.uber.org/fx"
)

// RelayServiceInput RelayService 依赖
type RelayServiceInput struct {
	fx.In
	Swarm pkgif.Swarm
	Host  pkgif.Host
}

// RelayServiceOutput RelayService 输出
type RelayServiceOutput struct {
	fx.Out
	RelayService *RelayService `name:"relay_service"`
}

// ProvideRelayService 提供 RelayService
func ProvideRelayService(input RelayServiceInput) (RelayServiceOutput, error) {
	service, err := NewRelayService(input.Swarm, input.Host)
	if err != nil {
		return RelayServiceOutput{}, err
	}
	return RelayServiceOutput{RelayService: service}, nil
}

// AutoRelayInput AutoRelay 依赖
type AutoRelayInput struct {
	fx.In

	Config    *config.Config `optional:"true"`
	Swarm     pkgif.Swarm
	Host      pkgif.Host
	Peerstore pkgif.Peerstore
	Discovery pkgif.Discovery `optional:"true"`
}

// ProvideAutoRelay 提供 AutoRelay
func ProvideAutoRelay(input AutoRelayInput) pkgif.AutoRelay {
	// 创建 RelayClient
	relayClient := client.NewRelayClientService(input.Swarm, input.Peerstore, input.Discovery)

	// 创建 AutoRelay 配置
	cfg := client.DefaultAutoRelayConfig()
	if input.Config != nil {
		// 使用已配置的 StaticRelays
		if len(input.Config.Relay.StaticRelays) > 0 {
			cfg.StaticRelays = input.Config.Relay.StaticRelays
		}
		// 如果配置了 RelayAddr，提取 PeerID 并添加到 StaticRelays
		if input.Config.Relay.RelayAddr != "" {
			ai, err := types.AddrInfoFromString(input.Config.Relay.RelayAddr)
			if err == nil {
				cfg.StaticRelays = append(cfg.StaticRelays, string(ai.ID))
				// 将 Relay 地址写入 Peerstore
				if len(ai.Addrs) > 0 && input.Peerstore != nil {
					input.Peerstore.AddAddrs(ai.ID, ai.Addrs, 24*time.Hour)
				}
			}
		}
	}

	return client.NewAutoRelay(cfg, relayClient, input.Host, input.Peerstore)
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("relay",
		fx.Provide(
			ConfigFromUnified,
			NewManager,
			NewSelector,
			ProvideRelayService,
			ProvideAutoRelay,
			ProvideRelayTransport,
			ProvideRelayDiscovery,
		),

		fx.Invoke(registerLifecycle),
		fx.Invoke(bindManagerDependencies),
		fx.Invoke(registerAutoRelayLifecycle),
		fx.Invoke(registerRelayTransportLifecycle),
		fx.Invoke(registerRelayDiscoveryLifecycle),
	)
}

// RelayTransportInput RelayTransport 依赖
type RelayTransportInput struct {
	fx.In
	Manager   *Manager `optional:"true"`
	Host      pkgif.Host
	Peerstore pkgif.Peerstore
}

// ProvideRelayTransport 提供 RelayTransport
func ProvideRelayTransport(input RelayTransportInput) *RelayTransport {
	if input.Manager == nil {
		return nil
	}
	return NewRelayTransport(input.Manager, input.Host, input.Peerstore, DefaultRelayTransportConfig())
}

// RelayDiscoveryInput RelayDiscovery 依赖
type RelayDiscoveryInput struct {
	fx.In
	Discovery pkgif.Discovery `optional:"true"`
	Host      pkgif.Host
	Peerstore pkgif.Peerstore
}

// ProvideRelayDiscovery 提供 RelayDiscovery
func ProvideRelayDiscovery(input RelayDiscoveryInput) *RelayDiscovery {
	return NewRelayDiscovery(input.Discovery, input.Host, input.Peerstore, DefaultRelayDiscoveryConfig())
}

// registerRelayTransportLifecycle 注册 RelayTransport 生命周期
func registerRelayTransportLifecycle(lc fx.Lifecycle, rt *RelayTransport) {
	if rt == nil {
		return
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return rt.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return rt.Close()
		},
	})
}

// registerRelayDiscoveryLifecycle 注册 RelayDiscovery 生命周期
func registerRelayDiscoveryLifecycle(lc fx.Lifecycle, rd *RelayDiscovery) {
	if rd == nil {
		return
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return rd.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return rd.Close()
		},
	})
}

// bindManagerDependencies 绑定 RelayManager 的外部依赖
func bindManagerDependencies(input struct {
	fx.In
	Manager              *Manager
	AutoRelay            pkgif.AutoRelay               `optional:"true"`
	Coordinator          pkgif.ReachabilityCoordinator `name:"reachability_coordinator" optional:"true"`
	LifecycleCoordinator *lifecycle.Coordinator        `optional:"true"`
}) {
	if input.AutoRelay != nil {
		input.Manager.SetAutoRelay(input.AutoRelay)
	}
	if input.Coordinator != nil {
		input.Manager.SetCoordinator(input.Coordinator)
	}
	if input.LifecycleCoordinator != nil {
		input.Manager.SetLifecycleCoordinator(input.LifecycleCoordinator)
	}
}

type autoRelayLifecycleInput struct {
	fx.In
	LC        fx.Lifecycle
	AutoRelay pkgif.AutoRelay `optional:"true"`
}

// registerAutoRelayLifecycle 注册 AutoRelay 生命周期
func registerAutoRelayLifecycle(input autoRelayLifecycleInput) {
	if input.AutoRelay == nil {
		return
	}
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := input.AutoRelay.Start(ctx); err != nil {
				return err
			}
			input.AutoRelay.Enable()
			return nil
		},
		OnStop: func(_ context.Context) error {
			return input.AutoRelay.Stop()
		},
	})
}

// ConfigFromUnified 从统一配置创建中继配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}
	return &Config{
		EnableClient: cfg.Relay.EnableClient,
		EnableServer: cfg.Relay.EnableServer,

		MaxReservations: cfg.Relay.Server.MaxReservations,
		MaxCircuits:     cfg.Relay.Server.MaxCircuits,
		ReservationTTL:  ReservationTTLDuration(cfg.Relay.Server.ReservationTTL),
		BufferSize:      cfg.Relay.Server.BufferSize,

		MaxBandwidth:        cfg.Relay.Limits.Bandwidth,
		MaxDuration:         cfg.Relay.Limits.Duration,
		MaxCircuitsPerPeer:  cfg.Relay.Limits.MaxCircuitsPerPeer,
	}
}

// registerLifecycleInput 生命周期注册的输入
type registerLifecycleInput struct {
	fx.In
	Lifecycle fx.Lifecycle
	Manager   *Manager
	Config    *config.Config `optional:"true"`
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input registerLifecycleInput) {
	input.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := input.Manager.Start(ctx); err != nil {
				return err
			}

			// 如果配置了 RelayAddr，必须调用 SetRelayAddr
			if input.Config != nil && input.Config.Relay.RelayAddr != "" {
				addr, err := types.ParseMultiaddr(input.Config.Relay.RelayAddr)
				if err == nil {
					if err := input.Manager.SetRelayAddr(addr); err != nil {
						logger.Warn("设置 RelayAddr 失败", "error", err)
					} else {
						logger.Info("已设置 RelayAddr（用于直连失败回退）",
							"addr", input.Config.Relay.RelayAddr)
					}
				}
			}

			return nil
		},
		OnStop: func(_ context.Context) error {
			return input.Manager.Stop()
		},
	})
}

// ReservationTTLDuration 辅助函数：将配置中的值转为 time.Duration
func ReservationTTLDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 1 * time.Hour
	}
	return d
}
