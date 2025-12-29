// Package app 提供 dep2p 应用编排层
//
// app 包负责：
// - fx 模块组装
// - 依赖注入协调
// - 生命周期管理
package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/config"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	connmgrif "github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	livenessif "github.com/dep2p/go-dep2p/pkg/interfaces/liveness"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
)

// Bootstrap 应用引导程序
//
// Bootstrap 负责：
// - 解析配置
// - 组装 fx 模块
// - 管理应用生命周期
type Bootstrap struct {
	config   *config.Config
	fxApp    *fx.App
	endpoint endpointif.Endpoint

	// 这些子系统在 fx 中已构建，但 endpointif.Endpoint 不直接暴露（保持接口稳定）。
	// BuildRuntime 会把它们一并取出，供 pkg/dep2p Facade 使用。
	connManager   connmgrif.ConnectionManager
	messaging     messagingif.MessagingService
	liveness      livenessif.LivenessService
	realm         realmif.RealmManager
	addressParser addressif.AddressParser
	reachability  reachabilityif.Coordinator
}

// NewBootstrap 创建引导程序
func NewBootstrap(cfg *config.Config) *Bootstrap {
	return &Bootstrap{
		config: cfg,
	}
}

// Build 构建 dep2p 节点（不启动）
//
// 返回的 Endpoint 需要调用 Listen() 才能开始接受连接
func (b *Bootstrap) Build() (endpointif.Endpoint, error) {
	// 应用日志配置（必须在所有模块初始化之前）
	if err := b.setupLogging(); err != nil {
		return nil, fmt.Errorf("设置日志失败: %w", err)
	}

	// 组装模块
	modules, err := b.setupModules()
	if err != nil {
		return nil, fmt.Errorf("设置模块失败: %w", err)
	}

	// 创建 fx 应用
	b.fxApp = fx.New(
		fx.Options(modules...),
		fx.NopLogger,
		// 使用 Invoke 来获取命名的 Endpoint
		fx.Invoke(
			fx.Annotate(
				func(ep endpointif.Endpoint) {
					b.endpoint = ep
				},
				fx.ParamTags(`name:"endpoint"`),
			),
		),
		// 取出 ConnectionManager（供 Facade 使用）
		fx.Invoke(
			fx.Annotate(
				func(cm connmgrif.ConnectionManager) {
					b.connManager = cm
				},
				fx.ParamTags(`name:"conn_manager"`),
			),
		),
		// 取出 MessagingService（供 Facade 使用）
		fx.Invoke(
			fx.Annotate(
				func(ms messagingif.MessagingService) {
					b.messaging = ms
				},
				fx.ParamTags(`name:"messaging"`),
			),
		),
		// 取出 LivenessService（供 Facade 使用，可选）
		fx.Invoke(
			fx.Annotate(
				func(ls livenessif.LivenessService) {
					b.liveness = ls
				},
				fx.ParamTags(`name:"liveness" optional:"true"`),
			),
		),
		// 取出 RealmManager（供 Facade 使用，可选）
		fx.Invoke(
			fx.Annotate(
				func(rm realmif.RealmManager) {
					b.realm = rm
				},
			fx.ParamTags(`name:"realm_manager"`),
			),
		),
		// 取出 AddressParser（供 Facade 使用）
		fx.Invoke(
			fx.Annotate(
				func(ap addressif.AddressParser) {
					b.addressParser = ap
				},
				fx.ParamTags(`name:"address_parser_if"`),
			),
		),
		// 取出 ReachabilityCoordinator（供 Facade 使用，可选）
		fx.Invoke(
			fx.Annotate(
				func(rc reachabilityif.Coordinator) {
					b.reachability = rc
				},
				fx.ParamTags(`name:"reachability_coordinator" optional:"true"`),
			),
		),
	)

	// 启动 fx 应用（初始化依赖）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := b.fxApp.Start(ctx); err != nil {
		return nil, fmt.Errorf("启动应用失败: %w", err)
	}

	return b.endpoint, nil
}

// BuildRuntime 构建 dep2p 运行时（Endpoint + 子系统）并返回可 Stop 的句柄。
//
// 说明：
// - endpointif.Endpoint 保持最小稳定接口；对外"一把梭体验"由 pkg/dep2p Facade 提供
// - Runtime.Stop() 会触发 fx OnStop，确保各模块按生命周期关闭
func (b *Bootstrap) BuildRuntime() (*Runtime, error) {
	ep, err := b.Build()
	if err != nil {
		return nil, err
	}

	return &Runtime{
		Endpoint:          ep,
		ConnectionManager: b.connManager,
		Messaging:         b.messaging,
		Liveness:          b.liveness,
		Realm:             b.realm,
		AddressParser:     b.addressParser,
		Reachability:      b.reachability,
		stop:              b.Stop,
	}, nil
}

// Start 构建并启动 dep2p 节点
//
// 等价于 Build() + Endpoint.Listen()
func (b *Bootstrap) Start(ctx context.Context) (endpointif.Endpoint, error) {
	ep, err := b.Build()
	if err != nil {
		return nil, err
	}

	if err := ep.Listen(ctx); err != nil {
		_ = b.Stop(context.Background())
		return nil, fmt.Errorf("启动监听失败: %w", err)
	}

	return ep, nil
}

// StartRuntime 构建并启动 dep2p 运行时（包含子系统句柄）。
func (b *Bootstrap) StartRuntime(ctx context.Context) (*Runtime, error) {
	rt, err := b.BuildRuntime()
	if err != nil {
		return nil, err
	}
	if err := rt.Endpoint.Listen(ctx); err != nil {
		_ = b.Stop(context.Background())
		return nil, fmt.Errorf("启动监听失败: %w", err)
	}
	return rt, nil
}

// Stop 停止应用
func (b *Bootstrap) Stop(ctx context.Context) error {
	if b.fxApp == nil {
		return nil
	}

	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return b.fxApp.Stop(stopCtx)
}

// setupModules 组装所有 fx 模块
func (b *Bootstrap) setupModules() ([]fx.Option, error) {
	return []fx.Option{
		// 配置模块（Tier 0）
		b.setupConfigModule(),

		// 基础层（Tier 1: Foundation）
		b.setupFoundationLayer(),

		// 传输层（Tier 2: Transport）
		b.setupTransportLayer(),

		// 网络服务层（Tier 3: Service）
		b.setupNetworkLayer(),

		// 领域层（Tier 4: Realm）
		b.setupRealmLayer(),

		// 消息层（Tier 5: Messaging + Application）
		b.setupApplicationLayer(),

		// 监控层（Tier 5.5: Monitoring）
		b.setupMonitoringLayer(),

		// API 层（Tier 6: Endpoint）
		b.setupEndpointLayer(),
	}, nil
}

// setupConfigModule 配置模块
func (b *Bootstrap) setupConfigModule() fx.Option {
	return fx.Options(
		// 提供配置
		fx.Supply(b.config),

		// 配置模块（来自 internal/config）
		config.Module(),
	)
}

// setupFoundationLayer 基础层模块
//
// Tier 1: identity, address
// 这些模块是其他所有模块的基础
func (b *Bootstrap) setupFoundationLayer() fx.Option {
	return FoundationModules()
}

// setupTransportLayer 传输层模块
//
// Tier 2: transport, security, muxer
// 这些模块提供底层网络传输能力
func (b *Bootstrap) setupTransportLayer() fx.Option {
	return TransportModules()
}

// setupNetworkLayer 网络服务层模块
//
// Tier 3: nat, discovery, relay, liveness
// 这些模块提供网络层服务。
func (b *Bootstrap) setupNetworkLayer() fx.Option {
	modules := []fx.Option{}

	// NAT 穿透
	if b.config.NAT.Enable {
		modules = append(modules, NATModule())
	}

	// 发现服务
	// v1.1+ 强制内建：Discovery（DHT + mDNS + Bootstrap）始终启用，不再由用户配置开关控制。
	modules = append(modules, DiscoveryModule())

	// 中继服务
	if b.config.Relay.Enable {
		modules = append(modules, RelayModule())
	}

	// 可达性协调（可达性优先：统一管理通告地址来源）
	// 注意：应当在 Relay 之后装配，以便接入 auto_relay。
	modules = append(modules, ReachabilityModule())

	// 存活检测服务
	if b.config.Liveness.Enable {
		modules = append(modules, LivenessModule())
	}

	return fx.Options(modules...)
}

// setupRealmLayer 领域层模块
//
// Tier 4: realm
// 提供业务网络隔离能力。
func (b *Bootstrap) setupRealmLayer() fx.Option {
	// v1.1+ 强制内建：Realm 始终启用，不再由用户配置开关控制。
	return RealmModule()
}

// setupApplicationLayer 应用层模块
//
// Tier 5: protocol, connmgr, messaging
// 这些模块提供应用层功能
func (b *Bootstrap) setupApplicationLayer() fx.Option {
	return ApplicationModules()
}

// setupMonitoringLayer 监控层模块
//
// Tier 5.5: bandwidth, netreport
// 提供带宽监控和网络诊断能力
func (b *Bootstrap) setupMonitoringLayer() fx.Option {
	return MonitoringModules()
}

// setupEndpointLayer 聚合层模块
//
// Tier 6: endpoint
// 聚合所有模块，提供统一的 Endpoint 接口
func (b *Bootstrap) setupEndpointLayer() fx.Option {
	return EndpointModules()
}

// setupLogging 配置日志输出
//
// 如果指定了 LogFile，将所有日志重定向到文件
func (b *Bootstrap) setupLogging() error {
	if b.config.LogFile == "" {
		return nil
	}

	// 打开日志文件（追加模式）
	file, err := os.OpenFile(b.config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}

	// 注意：这里不关闭文件，因为它会在整个程序生命周期内使用
	// 实际应用中可能需要在 Bootstrap.Close() 中关闭

	// 重定向日志输出
	logger.SetOutput(file)
	
	// 写入一条标记日志，确认文件创建成功
	log := logger.Logger("bootstrap")
	log.Info("日志文件初始化成功", "path", b.config.LogFile)
	
	return nil
}
