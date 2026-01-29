package bootstrap

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 使用 bootstrap.go 中定义的 logger

// ModuleInput 模块输入依赖
type ModuleInput struct {
	fx.In

	Host       pkgif.Host     // 移除 name 标签，使用接口
	DHT        pkgif.DHT      `optional:"true"` // 用于 Random Walk 发现
	Liveness   pkgif.Liveness `optional:"true"` // 移除 name 标签
	UnifiedCfg *config.Config `optional:"true"`
}

// ModuleOutput 模块输出
type ModuleOutput struct {
	fx.Out

	Bootstrap        pkgif.Discovery   `name:"bootstrap"`
	BootstrapService *BootstrapService `name:"bootstrap_service"`
}

// ConfigFromUnified 从统一配置创建 Bootstrap 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil || !cfg.Discovery.EnableBootstrap {
		return &Config{Enabled: false}
	}
	
	// 转换统一配置中的字符串为 PeerInfo
	// 注意：cfg.Discovery.Bootstrap.Peers 已包含默认值（来自 config/discovery.go）
	// 或用户通过 JSON/代码覆盖的值
	var peers []types.PeerInfo
	if len(cfg.Discovery.Bootstrap.Peers) > 0 {
		peers = parseBootstrapPeers(cfg.Discovery.Bootstrap.Peers)
	}
	
	return &Config{
		Peers:      peers,
		MinPeers:   cfg.Discovery.Bootstrap.MinPeers,
		Timeout:    cfg.Discovery.Bootstrap.Timeout.Duration(),
		MaxRetries: 3,
		Interval:   cfg.Discovery.Bootstrap.Interval.Duration(),
		Enabled:    cfg.Discovery.EnableBootstrap,
	}
}

// parseBootstrapPeers 解析字符串格式的引导节点为 PeerInfo
//
// 将 multiaddr 字符串（如 "/ip4/1.2.3.4/tcp/4001/p2p/QmXXX"）
// 解析为 PeerInfo 结构。
func parseBootstrapPeers(addrs []string) []types.PeerInfo {
	var peers []types.PeerInfo
	
	for _, addr := range addrs {
		// 解析 multiaddr 字符串为 AddrInfo
		addrInfo, err := types.AddrInfoFromString(addr)
		if err != nil {
			// 记录错误但继续处理其他节点
			logger.Debug("解析 Bootstrap 节点地址失败", "addr", addr, "error", err)
			continue
		}
		
		// 转换为 PeerInfo
		peerInfo := addrInfo.ToPeerInfo()
		peerInfo.Source = types.SourceBootstrap
		
		peers = append(peers, peerInfo)
	}
	
	return peers
}

// ProvideBootstrap 提供 Bootstrap 服务
func ProvideBootstrap(input ModuleInput) (ModuleOutput, error) {
	cfg := ConfigFromUnified(input.UnifiedCfg)

	// 创建原有的 Bootstrap 实例（用于连接引导节点）
	bootstrap, err := New(input.Host, cfg)
	if err != nil {
		return ModuleOutput{}, err
	}

	// 创建 BootstrapService（用于 EnableBootstrap 能力）
	serviceOpts := []ServiceOption{
		WithDataDir("."),
	}
	if input.DHT != nil {
		serviceOpts = append(serviceOpts, WithDHT(input.DHT))
	}
	if input.Liveness != nil {
		serviceOpts = append(serviceOpts, WithLiveness(input.Liveness))
	}
	service := NewBootstrapService(input.Host, serviceOpts...)

	return ModuleOutput{
		Bootstrap:        bootstrap,
		BootstrapService: service,
	}, nil
}

// Module 返回 Fx 模块
var Module = fx.Module("discovery/bootstrap",
	fx.Provide(ProvideBootstrap),
	fx.Invoke(registerLifecycle),
)

// lifecycleInput Lifecycle 注册输入
type lifecycleInput struct {
	fx.In
	LC               fx.Lifecycle
	Bootstrap        pkgif.Discovery   `name:"bootstrap"`
	BootstrapService *BootstrapService `name:"bootstrap_service"`
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 启动 Bootstrap 服务（连接引导节点）
			return input.Bootstrap.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			// 停止 Bootstrap 服务
			if err := input.Bootstrap.Stop(ctx); err != nil {
				// 记录错误但继续关闭其他组件
				logger.Warn("停止 Bootstrap 服务失败", "error", err)
			}
			// 关闭 BootstrapService（能力服务）
			if input.BootstrapService != nil {
				return input.BootstrapService.Close()
			}
			return nil
		},
	})
}
