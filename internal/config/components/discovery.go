package components

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
)

// DiscoveryOptions 发现服务选项
type DiscoveryOptions struct {
	// BootstrapPeers 引导节点
	BootstrapPeers []string

	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration

	// DHT DHT 配置
	DHT DHTOptions

	// MDNS mDNS 配置
	MDNS MDNSOptions
}

// DHTOptions DHT 选项
type DHTOptions struct {
	// Mode DHT 模式
	// 可选: auto, server, client
	Mode string

	// BucketSize K-桶大小
	BucketSize int

	// Concurrency 并发度
	Concurrency int
}

// MDNSOptions mDNS 选项
type MDNSOptions struct {
	// ServiceTag 服务标签
	ServiceTag string

	// Interval 发现间隔
	Interval time.Duration
}

// NewDiscoveryOptions 从配置创建发现选项
func NewDiscoveryOptions(cfg *config.DiscoveryConfig) *DiscoveryOptions {
	return &DiscoveryOptions{
		BootstrapPeers:  cfg.BootstrapPeers,
		RefreshInterval: cfg.RefreshInterval,
		DHT: DHTOptions{
			Mode:        cfg.DHT.Mode,
			BucketSize:  cfg.DHT.BucketSize,
			Concurrency: cfg.DHT.Concurrency,
		},
		MDNS: MDNSOptions{
			ServiceTag: cfg.MDNS.ServiceTag,
			Interval:   cfg.MDNS.Interval,
		},
	}
}

// DefaultDiscoveryOptions 默认发现选项
func DefaultDiscoveryOptions() *DiscoveryOptions {
	return &DiscoveryOptions{
		BootstrapPeers:  []string{},
		RefreshInterval: 3 * time.Minute,
		DHT: DHTOptions{
			Mode:        "auto",
			BucketSize:  20,
			Concurrency: 3,
		},
		MDNS: MDNSOptions{
			ServiceTag: "_dep2p._udp",
			Interval:   10 * time.Second,
		},
	}
}

