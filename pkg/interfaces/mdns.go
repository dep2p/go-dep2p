// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 mDNS 接口，对应 internal/discovery/mdns/ 实现。
package interfaces

import (
	"time"
)

// ════════════════════════════════════════════════════════════════════════════
// MDNSService 接口
// ════════════════════════════════════════════════════════════════════════════

// MDNSService 定义局域网 mDNS 发现服务接口
//
// mDNS 使用多播 DNS 协议进行局域网内的节点自动发现，无需中心服务器或互联网连接。
// 适用于开发测试环境和私有局域网场景。
//
// 架构位置：Discovery Layer
// 实现位置：internal/discovery/mdns/
//
// 核心功能:
//   - 服务广播（Advertise）：使用 zeroconf 注册 mDNS 服务
//   - 服务发现（FindPeers）：监听局域网内的 mDNS 广播
//   - 地址过滤：只广播适合 LAN 的地址
//
// 使用示例:
//
//	mdns := mdns.New(host, config)
//	mdns.Start(ctx)
//	defer mdns.Stop(ctx)
//
//	// 发现局域网节点
//	ch, _ := mdns.FindPeers(ctx, "my-app")
//	for peer := range ch {
//	    fmt.Printf("发现节点: %s\n", peer.ID)
//	}
//
// 限制:
//   - 仅限局域网（LAN）发现
//   - 不适合大规模网络（推荐使用 DHT）
//   - 依赖网络支持多播
type MDNSService interface {
	Discovery

	// Stats 返回 mDNS 服务统计信息
	Stats() MDNSStats
}

// MDNSStats mDNS 服务统计信息
type MDNSStats struct {
	// Running 服务是否正在运行
	Running bool

	// PeersDiscovered 发现的节点总数
	PeersDiscovered int64

	// LastDiscovery 最后一次发现时间
	LastDiscovery time.Time

	// AdvertiseCount 广播次数
	AdvertiseCount int64

	// LastAdvertise 最后一次广播时间
	LastAdvertise time.Time
}
