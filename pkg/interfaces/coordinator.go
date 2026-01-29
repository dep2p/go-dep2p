// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Coordinator 接口，对应 internal/discovery/coordinator/ 实现。
// Coordinator 是 Discovery Layer 的门面，同时定义了 Discovery 契约接口。
package interfaces

import (
	"context"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// Discovery 契约接口
// ════════════════════════════════════════════════════════════════════════════

// Discovery 定义发现服务契约接口
//
// Discovery 是发现层的核心契约，由 Coordinator 定义，
// 被 mDNS、DNS、Rendezvous 等发现组件实现。
type Discovery interface {
	// FindPeers 发现节点
	//
// 在指定命名空间中发现节点，返回异步 channel。
// 命名空间仅承载 payload，不应直接传入完整 DHT Key。
	FindPeers(ctx context.Context, ns string, opts ...DiscoveryOption) (<-chan types.PeerInfo, error)

	// Advertise 广播自身
	//
// 在指定命名空间中广播本节点，返回广播 TTL。
// 命名空间仅承载 payload，不应直接传入完整 DHT Key。
	Advertise(ctx context.Context, ns string, opts ...DiscoveryOption) (time.Duration, error)

	// Start 启动发现服务
	Start(ctx context.Context) error

	// Stop 停止发现服务
	Stop(ctx context.Context) error
}

// ════════════════════════════════════════════════════════════════════════════
// 发现选项
// ════════════════════════════════════════════════════════════════════════════

// DiscoveryOption 发现选项函数类型
type DiscoveryOption func(*DiscoveryOptions)

// DiscoveryOptions 发现选项集合
type DiscoveryOptions struct {
	// Limit 发现数量限制
	Limit int

	// TTL 广播 TTL
	TTL time.Duration
}

// WithLimit 设置发现数量限制
func WithLimit(limit int) DiscoveryOption {
	return func(o *DiscoveryOptions) {
		o.Limit = limit
	}
}

// WithTTL 设置广播 TTL
func WithTTL(ttl time.Duration) DiscoveryOption {
	return func(o *DiscoveryOptions) {
		o.TTL = ttl
	}
}

// ════════════════════════════════════════════════════════════════════════════
// Coordinator 接口
// ════════════════════════════════════════════════════════════════════════════

// Coordinator 定义发现协调器接口
//
// Coordinator 是发现层的门面，负责统一调度各种发现组件（DHT、mDNS、Bootstrap、DNS、Rendezvous）。
// 它聚合所有发现子模块，提供统一的节点发现和广播能力。
//
// 架构位置：Discovery Layer
// 实现位置：internal/discovery/coordinator/
//
// 使用示例:
//
//	coord := coordinator.NewCoordinator(config)
//	coord.RegisterDiscovery("mdns", mdnsDiscovery)
//	coord.Start(ctx)
//	defer coord.Stop(ctx)
//
//	// 发现节点（并行查询所有发现器）
//	ch, _ := coord.FindPeers(ctx, "myapp", WithLimit(10))
//	for peer := range ch {
//	    fmt.Printf("发现节点: %s\n", peer.ID)
//	}
type Coordinator interface {
	Discovery

	// RegisterDiscovery 注册发现器
	//
	// 可在启动前或启动后注册发现器。
	RegisterDiscovery(name string, discovery Discovery)

	// UnregisterDiscovery 取消注册发现器
	UnregisterDiscovery(name string)

	// Discoveries 返回所有已注册的发现器
	Discoveries() map[string]Discovery

	// Stats 返回协调器统计信息
	Stats() CoordinatorStats
}

// CoordinatorStats 协调器统计信息
type CoordinatorStats struct {
	// RegisteredDiscoveries 已注册的发现器数量
	RegisteredDiscoveries int

	// TotalFindPeers 总发现请求数
	TotalFindPeers int64

	// TotalAdvertises 总广播请求数
	TotalAdvertises int64

	// TotalPeersFound 总发现节点数
	TotalPeersFound int64

	// LastFindPeers 最后一次发现时间
	LastFindPeers time.Time

	// LastAdvertise 最后一次广播时间
	LastAdvertise time.Time
}

// NormalizeNamespace 统一 namespace 为 payload 形式。
//
// 约定：Discovery 的 namespace 只承载 payload，不应直接传入完整 DHT Key。
// 若传入 /dep2p/... provider key，尝试剥离为 payload。
//
// 支持的格式：
//   - /dep2p/v2/realm/{H(RealmID)}/provider/{payload}
//   - /dep2p/v2/sys/provider/{payload}
func NormalizeNamespace(ns string) string {
	if ns == "" {
		return ns
	}
	if !strings.HasPrefix(ns, "/dep2p/") {
		return ns
	}

	parts := strings.Split(ns, "/")

	// Realm Provider Key: /dep2p/v2/realm/{H(RealmID)}/provider/{payload}
	// parts: ["", "dep2p", "v2", "realm", "{H(RealmID)}", "provider", "{payload...}"]
	if len(parts) >= 7 && parts[1] == "dep2p" && parts[2] == "v2" && parts[3] == "realm" && parts[5] == "provider" {
		return strings.Join(parts[6:], "/")
	}

	// System Provider Key: /dep2p/v2/sys/provider/{payload}
	// parts: ["", "dep2p", "v2", "sys", "provider", "{payload...}"]
	if len(parts) >= 6 && parts[1] == "dep2p" && parts[2] == "v2" && parts[3] == "sys" && parts[4] == "provider" {
		return strings.Join(parts[5:], "/")
	}

	return ns
}
