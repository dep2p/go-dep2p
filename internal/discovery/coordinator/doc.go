// Package coordinator 实现 Discovery 层协调器
//
// # 模块概述
//
// coordinator 是发现服务的**门面模式**实现，负责统一调度各种发现组件。
//
// 核心职责：
//   - 聚合所有发现子模块（bootstrap, mdns, dht, rendezvous, dns）
//   - 提供统一的 interfaces.Discovery 接口
//   - 并行执行发现策略
//   - 结果去重与合并
//   - 发现优先级管理
//   - 生命周期管理
//
// # 架构设计
//
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                     Coordinator（门面）                              │
//	├─────────────────────────────────────────────────────────────────────┤
//	│                                                                     │
//	│  • FindPeers()  - 并行查询所有发现器，合并去重                        │
//	│  • Advertise()  - 向支持广播的发现器广播                             │
//	│  • Start()      - 启动所有子发现器                                   │
//	│  • Stop()       - 停止所有子发现器                                   │
//	│                                                                     │
//	└──────────────┬──────────┬──────────┬──────────┬──────────┬─────────┘
//	               │          │          │          │          │
//	               ↓          ↓          ↓          ↓          ↓
//	         ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
//	         │bootstrap │ │  mdns    │ │   dht    │ │rendezvous│ │   dns    │
//	         │(引导节点)│ │(局域网)  │ │(Kademlia)│ │(命名空间)│ │(DNS-SD)  │
//	         └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘
//
// # 使用示例
//
//	// 创建协调器
//	config := coordinator.DefaultConfig()
//	coord := coordinator.NewCoordinator(config)
//
//	// 注册发现器
//	coord.RegisterDiscovery("bootstrap", bootstrapDiscovery)
//	coord.RegisterDiscovery("mdns", mdnsDiscovery)
//	coord.RegisterDiscovery("dht", dhtDiscovery)
//
//	// 启动
//	ctx := context.Background()
//	if err := coord.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer coord.Stop(ctx)
//
//	// 发现节点（并行查询所有发现器）
//	ch, err := coord.FindPeers(ctx, "myapp", interfaces.WithLimit(10))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for peer := range ch {
//	    fmt.Printf("发现节点: %s\n", peer.ID)
//	}
//
//	// 广播自身
//	ttl, err := coord.Advertise(ctx, "myapp", interfaces.WithTTL(time.Hour))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("广播成功，TTL: %v\n", ttl)
//
// # 发现策略
//
// 协调器采用以下发现策略：
//
//  1. 并行查询：所有发现器同时执行查询，提高发现速度
//  2. 结果去重：基于 PeerID 进行去重，避免重复节点
//  3. 超时控制：支持配置发现超时，防止长时间阻塞
//  4. 优雅降级：单个发现器失败不影响其他发现器
//
// # 配置参数
//
//   - FindTimeout：发现超时时间（默认 30 秒）
//   - AdvertiseTimeout：广播超时时间（默认 10 秒）
//   - EnableCache：是否启用节点缓存（默认 true）
//   - CacheTTL：缓存过期时间（默认 5 分钟）
//
// # 生命周期
//
//  1. 创建：NewCoordinator()
//  2. 注册：RegisterDiscovery() - 可在启动前或启动后注册
//  3. 启动：Start() - 启动所有已注册的发现器
//  4. 运行：FindPeers() / Advertise() - 正常使用
//  5. 停止：Stop() - 停止所有发现器
//
// # 线程安全
//
// 所有公开方法都是线程安全的，可以并发调用。
package coordinator
