// Package rendezvous 实现基于命名空间的节点发现
//
// # 模块概述
//
// rendezvous 通过命名空间实现轻量级节点发现，与 DHT 不同，
// 它通过中心化的 Rendezvous Point 来协调节点发现，适用于 Realm 内成员发现和服务发现场景。
//
// # 核心功能
//
// 1. 命名空间注册
//   - 在命名空间注册本节点
//   - TTL 管理
//   - 自动续期
//
// 2. 命名空间发现
//   - 发现命名空间内的节点
//   - 分页查询
//   - 异步发现
//
// 3. Rendezvous Point 服务端
//   - 存储注册信息
//   - 处理发现请求
//   - 过期清理
//
// # 使用场景
//
//   - Realm 内节点发现（命名空间 = RealmID）
//   - 应用级节点分组
//   - 服务发现
//
// # 使用示例
//
// ## 作为客户端
//
//	config := rendezvous.DefaultDiscovererConfig()
//	config.Points = []types.PeerID{"12D3KooW..."}
//
//	discoverer := rendezvous.NewDiscoverer(host, config)
//	if err := discoverer.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer discoverer.Stop(ctx)
//
//	// 注册到命名空间
//	if err := discoverer.Register(ctx, "my-app/chat", 2*time.Hour); err != nil {
//	    log.Warn("register failed:", err)
//	}
//
//	// 发现节点
//	peers, err := discoverer.Discover(ctx, "my-app/chat", 10)
//	if err != nil {
//	    log.Warn("discover failed:", err)
//	}
//
//	// 异步发现
//	peerCh, err := discoverer.FindPeers(ctx, "my-app/chat")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for peer := range peerCh {
//	    log.Info("found peer:", peer.ID)
//	}
//
// ## 作为服务点
//
//	config := rendezvous.DefaultPointConfig()
//	config.MaxRegistrations = 10000
//
//	point := rendezvous.NewPoint(host, config)
//	if err := point.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer point.Stop()
//
//	// 获取统计信息
//	stats := point.Stats()
//	log.Infof("注册数: %d, 命名空间数: %d", stats.TotalRegistrations, stats.TotalNamespaces)
//
// # 架构设计
//
// Rendezvous 采用客户端-服务端架构：
//
//	┌─────────────────────────────────────────┐
//	│         Discovery 接口层                 │
//	│  FindPeers, Advertise, Start, Stop      │
//	├─────────────────────────────────────────┤
//	│        Discoverer 客户端                 │
//	│  Register, Discover, 自动续约           │
//	├─────────────────────────────────────────┤
//	│        Protocol 协议层                   │
//	│  REGISTER, DISCOVER, UNREGISTER         │
//	├─────────────────────────────────────────┤
//	│        Point 服务端                      │
//	│  Store, Handler, CleanupLoop            │
//	└─────────────────────────────────────────┘
//	              ↓
//	┌─────────────────────────────────────────┐
//	│       interfaces.Host 门面               │
//	│  Connect, NewStream, SetStreamHandler   │
//	└─────────────────────────────────────────┘
//
// # 协议
//
// 协议 ID: /dep2p/sys/rendezvous/1.0.0
//
// 消息类型（Protobuf）：
//   - REGISTER: 注册请求
//   - REGISTER_RESPONSE: 注册响应
//   - UNREGISTER: 取消注册请求
//   - DISCOVER: 发现请求
//   - DISCOVER_RESPONSE: 发现响应
//
// # 配置参数
//
// ## Discoverer
//
//   - DefaultTTL: 2h - 默认注册 TTL
//   - RenewalInterval: 1h - 续约间隔
//   - DiscoverTimeout: 30s - 发现超时
//   - RegisterTimeout: 30s - 注册超时
//   - MaxRetries: 3 - 最大重试次数
//
// ## Point
//
//   - MaxRegistrations: 10000 - 最大注册总数
//   - MaxNamespaces: 1000 - 最大命名空间数
//   - MaxTTL: 72h - 最大 TTL
//   - CleanupInterval: 5min - 清理间隔
//
// # 生命周期
//
// ## Discoverer
//
//  1. renewalLoop(): 自动续约循环（RenewalInterval）
//
// ## Point
//
//  1. cleanupLoop(): 过期清理循环（CleanupInterval）
//
// # 并发安全
//
// 所有公共方法都是并发安全的：
//   - Store 使用 RWMutex 保护
//   - 注册缓存使用 RWMutex 保护
//   - 状态标志使用 atomic 操作
//
// # 依赖
//
//   - interfaces.Host: 网络主机门面
//   - interfaces.Peerstore: 节点信息存储（可选）
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/discovery_rendezvous/
//   - internal/discovery/rendezvous/DESIGN_REVIEW.md
package rendezvous
