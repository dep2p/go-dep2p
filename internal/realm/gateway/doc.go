// Package gateway 实现 Realm Gateway（域网关）
//
// # 模块概述
//
// gateway 包提供 Realm 内部的中继转发服务，负责执行实际的中继转发，
// 与 routing 子模块协作，形成完整的路由与中继系统。
//
// 核心职责：
//   - Realm 内部中继服务（仅同 Realm 成员）
//   - PSK 认证保证 Realm 隔离性
//   - 连接池管理（复用连接）
//   - 带宽限流（Token Bucket）
//   - 协议前缀验证（/dep2p/realm/*, /dep2p/app/*）
//   - 与 routing 的深度集成
//
// # 核心组件
//
// ## Gateway（网关核心）
//
// Gateway 是网关系统的核心，协调各个子组件完成中继转发。
//
// 特性：
//   - PSK 认证验证
//   - 协议前缀验证
//   - 连接池管理
//   - 带宽限流
//   - 性能指标收集
//
// ## RelayService（中继服务）
//
// RelayService 处理中继请求，执行双向流转发。
//
// 特性：
//   - 双向异步转发
//   - 零拷贝优化（io.Copy）
//   - 超时保护
//   - 会话管理
//
// ## ConnectionPool（连接池）
//
// ConnectionPool 管理到其他节点的连接，支持复用和空闲清理。
//
// 特性：
//   - LRU 淘汰策略
//   - 空闲超时清理
//   - 并发控制（最多 1000 连接）
//   - 每节点连接限制
//
// ## BandwidthLimiter（带宽限流器）
//
// BandwidthLimiter 使用 Token Bucket 算法实现流量控制。
//
// 特性：
//   - Token Bucket 算法
//   - 突发流量支持
//   - 动态速率调整
//   - 限流统计
//
// ## ProtocolValidator（协议验证器）
//
// ProtocolValidator 验证协议前缀和 RealmID。
//
// 特性：
//   - 白名单验证
//   - RealmID 提取与匹配
//   - 系统协议（/dep2p/sys/*）由节点级 Relay 处理
//
// ## RouterAdapter（Routing 协作适配器）
//
// RouterAdapter 实现与 routing 子模块的双向通信。
//
// 特性：
//   - 注册到 Router
//   - 定期报告容量
//   - 处理路由的中继请求
//
// # 使用示例
//
// ## 创建 Gateway
//
//	// 创建配置
//	config := gateway.DefaultConfig()
//	config.MaxBandwidth = 100 * 1024 * 1024 // 100 MB/s
//	config.MaxConcurrent = 1000
//
//	// 创建 Gateway
//	gw := gateway.NewGateway("realm-id", host, auth, config)
//	defer gw.Close()
//
//	// 启动 Gateway
//	ctx := context.Background()
//	if err := gw.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 启动中继服务
//	go gw.ServeRelay(ctx)
//
// ## 处理中继请求
//
//	// 创建中继请求
//	req := &interfaces.RelayRequest{
//	    SourcePeerID: "peer1",
//	    TargetPeerID: "peer2",
//	    Protocol:     "/dep2p/realm/my-realm/messaging",
//	    RealmID:      "my-realm",
//	    Data:         []byte("hello"),
//	}
//
//	// 执行中继
//	if err := gw.Relay(ctx, req); err != nil {
//	    log.Printf("Relay failed: %v", err)
//	}
//
// ## 与 Routing 集成
//
//	// 创建 RouterAdapter
//	adapter := gateway.NewRouterAdapter(gw)
//
//	// 注册到 Router
//	if err := adapter.RegisterWithRouter(gw); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 定期报告容量
//	ticker := time.NewTicker(10 * time.Second)
//	go func() {
//	    for range ticker.C {
//	        adapter.ReportCapacity(ctx)
//	    }
//	}()
//
// # 性能指标
//
//   - 转发吞吐量：> 10 MB/s
//   - 中继延迟增加：< 5ms
//   - 连接池命中率：> 85%
//   - 带宽限流精度：± 5%
//   - 最大并发会话：1000
//   - 内存占用：< 100MB（1000会话）
//
// # 协议验证规则
//
// Gateway 执行 Realm 协议验证：
//
// ## 允许的协议前缀
//
//   - /dep2p/realm/<realmID>/*  （Realm 内部协议）
//   - /dep2p/app/<realmID>/*    （应用协议）
//
// ## 不处理的协议前缀
//
//   - /dep2p/sys/*              （系统协议由节点级 Relay 处理）
//
// ## 验证流程
//
//  1. 提取协议中的 <realmID>
//  2. 检查 <realmID> == 本 Gateway 服务的 RealmID
//  3. 验证请求方持有该 Realm 的 PSK 证明
//  4. 全部通过 → 转发；否则 → 拒绝
//
// # 与 routing 的协作
//
// Gateway 与 routing 协作完成 Realm 内部路由：
//
//   - routing 负责：路由选择、负载均衡、路径查找
//   - gateway 负责：中继转发、带宽控制、连接管理
//
// 交互流程：
//
//  1. routing.FindRoute() 发现目标需要中继
//  2. routing.gatewayAdapter.QueryReachable() 查询可用网关
//  3. routing.loadBalancer.SelectNode() 选择最优网关
//  4. routing.gatewayAdapter.RequestRelay() 请求中继
//  5. gateway.Relay() 执行实际转发
//  6. gateway.ReportState() 报告状态给 routing
//
// # 线程安全
//
// 所有公共方法都是线程安全的，可以并发调用。
package gateway
