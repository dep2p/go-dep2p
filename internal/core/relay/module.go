// Package relay 提供中继服务模块的实现
//
// 中继模块负责：
// - 中继客户端
// - 中继服务器
// - 资源预留
// - 自动中继管理和连接升级
package relay

import (
	"context"
	"io"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/relay/client"
	"github.com/dep2p/go-dep2p/internal/core/relay/server"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var log = logger.Logger("relay")

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Transport 传输服务
	Transport transportif.Transport `name:"transport"`

	// Endpoint 端点（用于连接中继）
	Endpoint coreif.Endpoint `name:"endpoint" optional:"true"`

	// HolePuncher 打洞服务（用于中继→直连升级）
	HolePuncher natif.HolePuncher `name:"hole_puncher" optional:"true"`

	// DiscoveryService 发现服务（用于 Relay Server 注册到 DHT）
	DiscoveryService discoveryif.DiscoveryService `name:"discovery" optional:"true"`

	// Config 配置（可选）
	Config *relayif.Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// RelayClient 中继客户端
	RelayClient relayif.RelayClient `name:"relay"`

	// RelayServer 中继服务器（可选，仅在 EnableServer=true 时创建）
	// Layer1 修复：确保启用 relay server 时实际注册协议处理器
	RelayServer relayif.RelayServer `name:"relay_server" optional:"true"`

	// AutoRelay 自动中继管理器
	AutoRelay relayif.AutoRelay `name:"auto_relay" optional:"true"`

	// RelayDiscovery 中继发现服务（用于 Relay Server 注册到 DHT）
	RelayDiscovery relayif.RelayDiscovery `name:"relay_discovery" optional:"true"`

	// RelayTransport 中继传输层（实现 Transport 接口）
	// Relay Transport Integration: 使 Relay 成为一等公民传输
	RelayTransport transportif.Transport `name:"relay_transport" optional:"true"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	config := relayif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 创建基础中继客户端
	// 使用 endpointDialerAdapter 将 Endpoint 适配为 relayif.Dialer
	var dialer relayif.Dialer
	if input.Endpoint != nil {
		dialer = &endpointDialerAdapter{ep: input.Endpoint}
	}
	relayClient := NewRelayClient(input.Transport, dialer, config)

	output := ModuleOutput{
		RelayClient: relayClient,
	}

	// 如果有 Endpoint，创建 AutoRelay、ConnectionUpgrader 和 RelayTransport
	if input.Endpoint != nil {
		// 创建连接升级器配置
		upgraderConfig := client.DefaultUpgraderConfig()

		// 创建本地地址获取函数
		localAddrs := func() []coreif.Address {
			if input.Endpoint == nil {
				return nil
			}
			return input.Endpoint.ListenAddrs()
		}

		// 创建连接升级器
		var upgrader *client.ConnectionUpgrader
		if input.HolePuncher != nil {
			upgrader = client.NewConnectionUpgrader(
				upgraderConfig,
				input.HolePuncher,
				input.Endpoint,
				localAddrs,
			)
			log.Info("连接升级器已创建（支持中继→直连升级）")
		}

		// 创建自动中继管理器
		autoRelayConfig := client.DefaultAutoRelayConfig()
		autoRelay := client.NewAutoRelay(autoRelayConfig, relayClient, input.Endpoint)

		// 关联升级器
		if upgrader != nil {
			autoRelay.SetUpgrader(upgrader)
		}

		output.AutoRelay = autoRelay

		// Relay Transport Integration: 创建 RelayTransport
		// RelayTransport 实现 Transport 接口，使 Relay 成为透明的传输层
		relayTransport := NewRelayTransport(relayClient, input.Endpoint)
		output.RelayTransport = relayTransport
		log.Info("RelayTransport 已创建（Relay 作为一等公民传输）")

		// Layer1 修复：如果启用了 Relay Server，创建并装配 Server
		if config.EnableServer {
			localID := input.Endpoint.ID()

			// 从 Config 构建 ServerConfig
			serverConfig := relayif.DefaultServerConfig()
			serverConfig.MaxReservations = config.MaxReservations
			serverConfig.MaxCircuits = config.MaxCircuits
			serverConfig.ReservationTTL = config.ReservationTTL

			// 创建 Relay Server
			relayServer := server.NewServer(serverConfig, localID, input.Endpoint)
			output.RelayServer = relayServer
			log.Info("RelayServer 已创建",
				"max_reservations", serverConfig.MaxReservations,
				"max_circuits", serverConfig.MaxCircuits)

			// Layer1 修复：创建 RelayDiscovery 用于注册到 DHT
			if input.DiscoveryService != nil {
				relayDiscovery := NewRelayDiscovery(
					input.DiscoveryService,
					input.Endpoint,
					localID,
					true, // isServer=true，启动时会通告到 DHT
				)
				output.RelayDiscovery = relayDiscovery
				log.Info("RelayDiscovery 已创建（Relay Server 将注册到 DHT）")
			}
		}
	}

	return output, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("relay",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC             fx.Lifecycle
	Endpoint       coreif.Endpoint         `name:"endpoint" optional:"true"`
	RelayClient    relayif.RelayClient     `name:"relay"`
	RelayServer    relayif.RelayServer     `name:"relay_server" optional:"true"`
	AutoRelay      relayif.AutoRelay       `name:"auto_relay" optional:"true"`
	RelayDiscovery relayif.RelayDiscovery  `name:"relay_discovery" optional:"true"`
	RelayTransport transportif.Transport   `name:"relay_transport" optional:"true"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("中继模块启动")

			// 注意：不再需要 SetRelayClient 延迟注入
			// RelayClient 现在通过 Dialer 接口依赖 Endpoint，无循环依赖

			// Relay Transport Integration: 将 RelayTransport 注册到 Endpoint
			// 这使得 Endpoint.Connect() 可以透明地使用中继地址
			if input.RelayTransport != nil && input.Endpoint != nil {
				if adder, ok := input.Endpoint.(interface {
					AddTransport(transportif.Transport) error
				}); ok {
					if err := adder.AddTransport(input.RelayTransport); err != nil {
						log.Warn("注册 RelayTransport 失败", "err", err)
					} else {
						log.Info("RelayTransport 已注册到 Endpoint",
							"protocols", input.RelayTransport.Protocols())
					}
				}

				// 注册 STOP 协议处理器，处理来自 Relay Server 的入站连接
				// 这是 RelayTransport.Listen() 工作的关键：
				// 当 Relay Server 转发连接到本节点时，会打开 ProtocolRelayHop 流并发送 STOP 消息
				if rt, ok := input.RelayTransport.(*RelayTransport); ok {
					input.Endpoint.SetProtocolHandler(
						protocolids.SysRelayHop,
						rt.HandleStopConnect,
					)
					log.Debug("注册 STOP 协议处理器", "protocol", protocolids.SysRelayHop)
				}
			}

			// Layer1 修复：启动 RelayServer 并注册协议处理器
			// 这是闭环的关键：确保宣告到 DHT 的服务实际可用
			if input.RelayServer != nil && input.Endpoint != nil {
				// 获取底层 Server 实现以注册协议处理器
				if srv, ok := input.RelayServer.(*server.Server); ok {
					// 启动服务器
					if err := srv.Start(ctx); err != nil {
						log.Error("RelayServer 启动失败", "err", err)
						return err
					}

					// 注册 Reserve 协议处理器
					input.Endpoint.SetProtocolHandler(
						protocolids.SysRelay,
						srv.HandleReserve,
					)
					log.Debug("注册 Reserve 协议处理器", "protocol", protocolids.SysRelay)

					// 注册 Connect (Hop) 协议处理器
					input.Endpoint.SetProtocolHandler(
						protocolids.SysRelayHop,
						srv.HandleConnect,
					)
					log.Debug("注册 Connect 协议处理器", "protocol", protocolids.SysRelayHop)

					log.Info("RelayServer 已启动并注册协议处理器",
						"reserve_protocol", protocolids.SysRelay,
						"hop_protocol", protocolids.SysRelayHop)
				}
			}

			// 启动 AutoRelay（如果存在）
			if input.AutoRelay != nil {
				if starter, ok := input.AutoRelay.(interface{ Start(context.Context) error }); ok {
					if err := starter.Start(ctx); err != nil {
						log.Error("AutoRelay 启动失败", "err", err)
						return err
					}
					// 默认启用 AutoRelay
					if enabler, ok := input.AutoRelay.(interface{ Enable() }); ok {
						enabler.Enable()
					}
					log.Info("AutoRelay 已启动（支持中继→直连自动升级）")
				}
			}

			// Layer1 修复：启动 RelayDiscovery（如果存在）
			// 这会将 Relay Server 注册到 DHT，使其可被其他节点发现
			// 注意：必须在 RelayServer 启动并注册协议后再宣告到 DHT
			if input.RelayDiscovery != nil {
				if starter, ok := input.RelayDiscovery.(interface{ Start(context.Context) error }); ok {
					if err := starter.Start(ctx); err != nil {
						log.Error("RelayDiscovery 启动失败", "err", err)
						// 不阻止启动，仅记录错误
					} else {
						log.Info("RelayDiscovery 已启动（Relay Server 已注册到 DHT）")
					}
				}
			}

			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("中继模块停止")

			// 停止 RelayDiscovery（先停止宣告，避免其他节点继续连接）
			if input.RelayDiscovery != nil {
				if stopper, ok := input.RelayDiscovery.(interface{ Stop() error }); ok {
					if err := stopper.Stop(); err != nil {
						log.Error("RelayDiscovery 停止失败", "err", err)
					}
				}
			}

			// 停止 RelayServer
			if input.RelayServer != nil {
				if stopper, ok := input.RelayServer.(interface{ Stop() error }); ok {
					if err := stopper.Stop(); err != nil {
						log.Error("RelayServer 停止失败", "err", err)
					}
				}
				// 移除协议处理器
				if input.Endpoint != nil {
					input.Endpoint.RemoveProtocolHandler(protocolids.SysRelay)
					input.Endpoint.RemoveProtocolHandler(protocolids.SysRelayHop)
				}
			}

			// 停止 AutoRelay
			if input.AutoRelay != nil {
				if stopper, ok := input.AutoRelay.(interface{ Stop() error }); ok {
					if err := stopper.Stop(); err != nil {
						log.Error("AutoRelay 停止失败", "err", err)
					}
				}
			}

			// 关闭 RelayClient
			if closer, ok := input.RelayClient.(io.Closer); ok {
				return closer.Close()
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
	Version     = "1.0.0"
	Name        = "relay"
	Description = "中继服务模块，提供中继客户端和服务器能力"
)

// ============================================================================
//                              Dialer 适配器
// ============================================================================

// endpointDialerAdapter 将 Endpoint 适配为 relayif.Dialer
//
// 目的：提供 RelayClient 需要的最小接口，避免循环依赖
type endpointDialerAdapter struct {
	ep coreif.Endpoint
}

// Connect 实现 relayif.Dialer 接口
func (a *endpointDialerAdapter) Connect(ctx context.Context, nodeID types.NodeID) (relayif.Connection, error) {
	conn, err := a.ep.Connect(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return &connectionAdapter{conn: conn}, nil
}

// ID 实现 relayif.Dialer 接口
func (a *endpointDialerAdapter) ID() types.NodeID {
	return a.ep.ID()
}

// Discovery 实现 relayif.Dialer 接口
func (a *endpointDialerAdapter) Discovery() relayif.DiscoveryService {
	disc := a.ep.Discovery()
	if disc == nil {
		return nil
	}
	return &discoveryAdapter{disc: disc}
}

// connectionAdapter 将 endpoint.Connection 适配为 relayif.Connection
type connectionAdapter struct {
	conn coreif.Connection
}

func (c *connectionAdapter) RemoteID() types.NodeID {
	return c.conn.RemoteID()
}

func (c *connectionAdapter) OpenStream(ctx context.Context, protocolID string) (relayif.Stream, error) {
	stream, err := c.conn.OpenStream(ctx, coreif.ProtocolID(protocolID))
	if err != nil {
		return nil, err
	}
	return stream, nil // endpoint.Stream 已经满足 relayif.Stream 接口
}

func (c *connectionAdapter) Close() error {
	return c.conn.Close()
}

// discoveryAdapter 将 endpoint.DiscoveryService 适配为 relayif.DiscoveryService
type discoveryAdapter struct {
	disc coreif.DiscoveryService
}

func (d *discoveryAdapter) DiscoverPeers(ctx context.Context, namespace string) (<-chan relayif.PeerInfo, error) {
	ch, err := d.disc.DiscoverPeers(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// 转换通道类型
	out := make(chan relayif.PeerInfo, 100)
	go func() {
		defer close(out)
		for peer := range ch {
			out <- relayif.PeerInfo{
				ID:    peer.ID,
				Addrs: peer.Addrs,
			}
		}
	}()

	return out, nil
}
