// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 DNS 发现接口，对应 internal/discovery/dns/ 实现。
package interfaces

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// DNSDiscovery 接口
// ════════════════════════════════════════════════════════════════════════════

// DNSDiscovery 定义基于 DNS 的节点发现接口
//
// DNS 发现通过 DNS TXT 记录发现节点，支持 dnsaddr 格式解析、
// 递归嵌套解析、结果缓存和后台刷新。
//
// 架构位置：Discovery Layer
// 实现位置：internal/discovery/dns/
//
// 使用场景:
//   - 引导节点配置（使用 dnsaddr 而非硬编码 IP）
//   - 动态节点发现
//   - 多地域负载均衡
//
// DNS 记录格式:
//
//	_dnsaddr.bootstrap.dep2p.io. 300 IN TXT "dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/QmYwAPJzv..."
//
// 使用示例:
//
//	config := dns.DefaultConfig()
//	config.Domains = []string{"bootstrap.dep2p.io"}
//
//	dns := dns.NewDiscoverer(config)
//	dns.Start(ctx)
//	defer dns.Stop(ctx)
//
//	// 发现节点
//	ch, _ := dns.FindPeers(ctx, "dns")
//	for peer := range ch {
//	    host.Connect(ctx, peer)
//	}
//
//	// 直接解析域名
//	peers, _ := dns.Resolve(ctx, "bootstrap.dep2p.io")
type DNSDiscovery interface {
	Discovery

	// Resolve 解析指定域名
	//
	// 直接解析域名的 dnsaddr TXT 记录，支持递归解析。
	Resolve(ctx context.Context, domain string) ([]types.PeerInfo, error)

	// AddDomain 添加域名
	//
	// 添加要监控的域名。
	AddDomain(domain string)

	// RemoveDomain 移除域名
	RemoveDomain(domain string)

	// Domains 返回当前监控的域名列表
	Domains() []string

	// Stats 返回 DNS 发现统计信息
	Stats() DNSDiscoveryStats
}

// DNSDiscoveryStats DNS 发现统计信息
type DNSDiscoveryStats struct {
	// DomainsCount 监控的域名数量
	DomainsCount int

	// TotalResolves 总解析次数
	TotalResolves int64

	// SuccessfulResolves 成功解析次数
	SuccessfulResolves int64

	// PeersDiscovered 发现的节点总数
	PeersDiscovered int64

	// LastResolve 最后一次解析时间
	LastResolve time.Time

	// CacheHits 缓存命中次数
	CacheHits int64

	// CacheMisses 缓存未命中次数
	CacheMisses int64
}
