// Package dns 实现基于 DNS 的节点发现
//
// # 模块概述
//
// dns 通过 DNS TXT 记录发现节点，支持 dnsaddr 格式解析、
// 递归嵌套解析、结果缓存和后台刷新。
//
// # 核心功能
//
// 1. dnsaddr 解析
//   - 解析 /dnsaddr/example.com 格式地址
//   - 递归解析嵌套 dnsaddr 引用
//   - 提取节点 ID 和地址
//
// 2. DNS TXT 查询
//   - 查询 _dnsaddr.<domain> TXT 记录
//   - 解析 dnsaddr= 格式记录
//   - 支持自定义 DNS 服务器
//
// 3. 结果缓存
//   - 缓存 DNS 查询结果
//   - 自动过期清理
//   - 减少重复查询
//
// 4. 后台刷新
//   - 定期刷新配置的域名
//   - 更新节点列表
//
// # DNS 记录格式
//
// ## dnsaddr TXT 记录
//
//	_dnsaddr.bootstrap.dep2p.io.  300  IN  TXT  "dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/QmYwAPJzv..."
//	_dnsaddr.bootstrap.dep2p.io.  300  IN  TXT  "dnsaddr=/ip6/::1/tcp/4001/p2p/QmYwAPJzv..."
//	_dnsaddr.bootstrap.dep2p.io.  300  IN  TXT  "dnsaddr=/dnsaddr/us-east.dep2p.io"
//
// ## 递归解析
//
// 1. 查询 _dnsaddr.bootstrap.dep2p.io
// 2. 解析每条记录：
//   - 直接地址 -> 提取节点信息
//   - 嵌套 dnsaddr -> 递归查询
//
// 3. 去重并返回所有节点
//
// # 使用场景
//
//   - 引导节点配置（使用 dnsaddr 而非硬编码 IP）
//   - 动态节点发现
//   - 多地域负载均衡
//
// # 使用示例
//
// ## 基本用法
//
//	config := dns.DefaultConfig()
//	config.Domains = []string{"bootstrap.dep2p.io"}
//
//	discoverer := dns.NewDiscoverer(config)
//	if err := discoverer.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer discoverer.Stop(ctx)
//
//	// 发现节点
//	peerCh, err := discoverer.FindPeers(ctx, "dns")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for peer := range peerCh {
//	    log.Info("found peer:", peer.ID)
//	}
//
// ## 直接解析域名
//
//	peers, err := discoverer.Resolve(ctx, "bootstrap.dep2p.io")
//	if err != nil {
//	    log.Warn("resolve failed:", err)
//	}
//	for _, peer := range peers {
//	    log.Info("resolved peer:", peer.ID)
//	}
//
// # 架构设计
//
// DNS 发现采用分层设计：
//
//	┌─────────────────────────────────────────┐
//	│         Discovery 接口层                 │
//	│  FindPeers, Advertise, Start, Stop      │
//	├─────────────────────────────────────────┤
//	│        Discoverer 发现器                 │
//	│  域名管理, 后台刷新                      │
//	├─────────────────────────────────────────┤
//	│        Resolver 解析器                   │
//	│  DNS 查询, 缓存, 递归解析               │
//	└─────────────────────────────────────────┘
//	              ↓
//	┌─────────────────────────────────────────┐
//	│       net.Resolver (标准库)              │
//	│  LookupTXT, 系统 DNS                     │
//	└─────────────────────────────────────────┘
//
// # dnsaddr 格式规范
//
// ## 直接地址
//
//	dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/QmYwAPJzv...
//	dnsaddr=/ip6/2001:db8::/tcp/4001/p2p/QmYwAPJzv...
//
// ## 嵌套引用
//
//	dnsaddr=/dnsaddr/us-east.bootstrap.dep2p.io
//
// # 配置参数
//
//   - Domains: [] - 要查询的域名列表
//   - Timeout: 10s - DNS 查询超时
//   - MaxDepth: 3 - 最大递归深度
//   - CacheTTL: 5min - 缓存 TTL
//   - RefreshInterval: 5min - 刷新间隔
//
// # 生命周期
//
// ## Discoverer
//
//  1. refreshLoop(): 后台刷新循环（RefreshInterval）
//
// # 并发安全
//
// 所有公共方法都是并发安全的：
//   - Resolver 缓存使用 RWMutex
//   - 域名列表使用 RWMutex
//   - 节点缓存使用 RWMutex
//   - 状态标志使用 atomic 操作
//
// # 依赖
//
//   - net.Resolver: Go 标准库 DNS 解析器
//   - interfaces.Discovery: 发现接口
//
// # 协议标准
//
//   - dnsaddr 地址规范: https://multiformats.io/multiaddr/
//   - DNS-SD: https://tools.ietf.org/html/rfc6763
package dns
