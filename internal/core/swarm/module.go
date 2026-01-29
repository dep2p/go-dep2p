package swarm

import (
	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// SwarmOutput Swarm 模块输出
type SwarmOutput struct {
	fx.Out

	Swarm pkgif.Swarm // 移除 name 标签，统一使用接口
}

// Module Swarm Fx 模块
var Module = fx.Module("swarm",
	fx.Provide(
		provideSwarm,
	),
)

// provideSwarm 提供带名称标签的 Swarm
func provideSwarm(params SwarmParams) (SwarmOutput, error) {
	swarm, err := NewSwarmFromParams(params)
	if err != nil {
		return SwarmOutput{}, err
	}
	return SwarmOutput{Swarm: swarm}, nil
}

// SwarmParams Swarm 依赖参数
type SwarmParams struct {
	fx.In

	LocalPeer  string
	UnifiedCfg *config.Config `optional:"true"`

	// 可选依赖（移除 name 标签，使用接口类型）
	Transports        []pkgif.Transport       `group:"transports"` // value groups 不能设置 optional
	Upgrader          pkgif.Upgrader          `optional:"true"`
	Peerstore         pkgif.Peerstore         `optional:"true"`
	ConnMgr           pkgif.ConnManager       `optional:"true"`
	EventBus          pkgif.EventBus          `optional:"true"`
	BandwidthCounter  pkgif.BandwidthCounter  `optional:"true"`
	PathHealthManager pkgif.PathHealthManager `optional:"true"` // Phase 0 修复：路径健康管理
}

// ConfigFromUnified 从统一配置创建 Swarm 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}

	// 获取默认配置以获取健康检测的默认值
	defaultCfg := DefaultConfig()

	return &Config{
		DialTimeout:        cfg.Transport.DialTimeout.Duration(),
		DialTimeoutLocal:   cfg.Transport.DialTimeout.Duration() / 3, // 本地拨号更快
		NewStreamTimeout:   cfg.Transport.DialTimeout.Duration(),
		MaxConcurrentDials: 100, // 默认值

		// 连接健康检测配置
		// 使用默认配置的值，确保健康检测正常工作
		ConnHealthInterval: defaultCfg.ConnHealthInterval,
		ConnHealthTimeout:  defaultCfg.ConnHealthTimeout,
	}
}

// NewSwarmFromParams 从参数创建 Swarm
func NewSwarmFromParams(params SwarmParams) (pkgif.Swarm, error) {
	// 从统一配置获取 Swarm 配置
	cfg := ConfigFromUnified(params.UnifiedCfg)

	// 创建 Swarm
	s, err := NewSwarm(params.LocalPeer, WithConfig(cfg))
	if err != nil {
		return nil, err
	}

	// 设置传输层
	for _, transport := range params.Transports {
		// 从 Transport.Protocols() 推断协议类型
		protocols := transport.Protocols()
		protocol := "tcp" // 默认
		for _, p := range protocols {
			if p == types.ProtocolQUIC_V1 || p == types.ProtocolQUIC {
				protocol = "quic"
				break
			}
		}
		s.AddTransport(protocol, transport)
	}

	// 设置升级器
	if params.Upgrader != nil {
		s.SetUpgrader(params.Upgrader)
	}

	// 设置 Peerstore
	if params.Peerstore != nil {
		s.SetPeerstore(params.Peerstore)
	}

	// 设置 ConnMgr
	if params.ConnMgr != nil {
		s.SetConnMgr(params.ConnMgr)
	}

	// 设置 EventBus
	if params.EventBus != nil {
		s.SetEventBus(params.EventBus)
	}

	// 设置 BandwidthCounter
	if params.BandwidthCounter != nil {
		s.SetBandwidthCounter(params.BandwidthCounter)
	}

	// Phase 0 修复：设置 PathHealthManager
	if params.PathHealthManager != nil {
		s.SetPathHealthManager(params.PathHealthManager)
	}

	return s, nil
}
