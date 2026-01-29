// Package bootstrap 实现引导节点发现
//
// bootstrap 负责通过预配置的引导节点进行初始节点发现，
// 是网络启动的第一步。
//
// # 引导流程
//
// Bootstrap 采用并发连接策略，快速建立与引导节点的连接：
//
//  1. 并发连接所有引导节点（goroutine + WaitGroup）
//  2. 为每个连接设置独立超时（默认30s）
//  3. 将引导节点地址添加到 Peerstore（永久TTL）
//  4. 检查最小成功连接数（默认4个）
//  5. 返回引导结果
//
// # 使用示例
//
//	// 创建 Bootstrap 服务
//	config := &bootstrap.Config{
//	    Peers: []types.PeerInfo{
//	        {
//	            ID:    types.PeerID("QmBootstrapNode1..."),
//	            Addrs: []types.Multiaddr{
//	                types.Multiaddr("/ip4/104.131.131.82/tcp/4001"),
//	            },
//	        },
//	    },
//	    Timeout:  30 * time.Second,
//	    MinPeers: 4,
//	}
//
//	bootstrap, err := bootstrap.New(host, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 执行引导
//	ctx := context.Background()
//	if err := bootstrap.Bootstrap(ctx); err != nil {
//	    log.Printf("bootstrap failed: %v", err)
//	}
//
//	// 使用 Discovery 接口查找节点
//	peerCh, err := bootstrap.FindPeers(ctx, "namespace")
//	for peer := range peerCh {
//	    log.Printf("found peer: %s", peer.ID)
//	}
//
// # 并发连接策略
//
// Bootstrap 使用 goroutine 并发连接所有引导节点，避免单个慢节点阻塞整个流程：
//
//	for _, peer := range peers {
//	    wg.Add(1)
//	    go func(p types.PeerInfo) {
//	        defer wg.Done()
//
//	        // 独立超时控制
//	        connCtx, cancel := context.WithTimeout(ctx, timeout)
//	        defer cancel()
//
//	        // 添加地址到 Peerstore
//	        peerstore.AddAddrs(p.ID, p.Addrs, PermanentAddrTTL)
//
//	        // 连接节点
//	        host.Connect(connCtx, p.ID, p.Addrs)
//	    }(peer)
//	}
//	wg.Wait()
//
// # 失败处理
//
// Bootstrap 采用部分成功容忍策略：
//
//   - 只要达到 MinPeers 个成功连接，即认为引导成功
//   - 全部连接都失败才返回 ErrAllConnectionsFailed
//   - 成功数不足 MinPeers 返回 ErrMinPeersNotMet
//
// # 配置参数
//
// | 参数 | 默认值 | 说明 |
// |------|--------|------|
// | Peers | [] | 引导节点列表 |
// | Timeout | 30s | 单个节点连接超时 |
// | MinPeers | 4 | 最少成功连接数 |
// | MaxRetries | 3 | 最大重试次数（v1.1） |
//
// # v1.0 实现范围
//
// ✅ 已实现：
//   - 并发连接引导节点
//   - 最小成功连接数检查
//   - 独立超时控制
//   - Peerstore 地址持久化
//   - Discovery 接口实现
//   - Fx 生命周期集成
//
// ⬜ v1.1+ 计划：
//   - 动态引导节点发现（DNS）
//   - 引导节点健康监控
//   - 高级重试策略
//
// # 技术债
//
// 无技术债项目。Bootstrap 是完整实现，不依赖未完成模块。
//
// # 依赖
//
// 内部模块依赖：
//   - internal/core/host: 节点连接（Host.Connect）
//   - internal/core/peerstore: 地址存储（Peerstore.AddAddrs）
//
// # 架构层
//
// Discovery Layer
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/discovery_bootstrap/README.md
//   - internal/discovery/bootstrap/DESIGN_REVIEW.md
package bootstrap
