// Package routing 实现 Realm 域内路由
//
// # 模块概述
//
// routing 包提供 Realm 层的智能路由功能，基于 DHT 路由表实现延迟感知的多跳路由。
//
// 核心职责：
//   - 域内路由表管理（基于 DHT）
//   - 智能路由选择（延迟感知）
//   - 多跳路径发现（Dijkstra 算法）
//   - 负载均衡（加权轮询）
//   - 路由缓存（LRU + TTL）
//   - Gateway 协作（中继路由）
//
// # 核心组件
//
// ## Router（路由器）
//
// Router 是路由系统的核心，协调各个子组件完成路由决策。
//
// 特性：
//   - 多策略路由选择
//   - 故障自动转移
//   - 路由结果缓存
//   - 性能指标收集
//
// ## RouteTable（路由表）
//
// RouteTable 基于 DHT 路由表实现域内节点管理。
//
// 特性：
//   - XOR 距离计算
//   - K 近邻查找
//   - 节点健康检查
//   - 定期路由表刷新
//
// ## PathFinder（路径查找器）
//
// PathFinder 实现多跳路径发现与评分。
//
// 特性：
//   - Dijkstra 最短路径算法
//   - 多路径查找（K 条备用路径）
//   - 路径质量评分
//   - 路径缓存管理
//
// ## LoadBalancer（负载均衡器）
//
// LoadBalancer 提供智能负载分配。
//
// 特性：
//   - 节点负载跟踪
//   - 加权轮询算法
//   - 过载保护
//   - 负载报告
//
// ## LatencyProber（延迟探测器）
//
// LatencyProber 负责网络延迟测量与预测。
//
// 特性：
//   - 主动 Ping 测试
//   - 被动延迟收集
//   - 延迟统计（平均、P95、P99）
//   - 延迟预测
//
// ## GatewayAdapter（Gateway 协作适配器）
//
// GatewayAdapter 与 gateway 子模块协作，处理跨域路由。
//
// 特性：
//   - 中继请求转发
//   - 可达性查询
//   - 状态同步
//
// # 使用示例
//
// ## 创建路由器
//
//	// 创建配置
//	config := routing.DefaultConfig()
//	config.CacheSize = 1000
//	config.DefaultPolicy = interfaces.PolicyMixed
//
//	// 创建路由器
//	router := routing.NewRouter("realm-id", dht, config)
//	defer router.Close()
//
//	// 启动路由器
//	ctx := context.Background()
//	if err := router.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// ## 查找路由
//
//	// 查找到目标节点的路由
//	route, err := router.FindRoute(ctx, "target-peer")
//	if err != nil {
//	    log.Printf("Route not found: %v", err)
//	}
//
//	log.Printf("Next hop: %s, Latency: %v", route.NextHop, route.Latency)
//
// ## 负载均衡路由
//
//	// 查找多条路由
//	routes, err := router.FindRoutes(ctx, "target-peer", 3)
//	if err != nil {
//	    log.Printf("Routes not found: %v", err)
//	}
//
//	// 选择最佳路由（负载均衡）
//	best, err := router.SelectBestRoute(ctx, routes, interfaces.PolicyLoadBalance)
//	if err != nil {
//	    log.Printf("No valid route: %v", err)
//	}
//
// ## 延迟测量
//
//	// 创建延迟探测器
//	prober := routing.NewLatencyProber(host)
//
//	// 启动探测器
//	if err := prober.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 测量延迟
//	latency, err := prober.MeasureLatency(ctx, "peer-id")
//	log.Printf("Latency: %v", latency)
//
// # 性能指标
//
//   - 路由查询延迟：< 10ms
//   - 缓存命中率：> 90%
//   - 路径发现时间：< 100ms
//   - 负载均衡偏差：< 15%
//   - 内存占用：< 50MB（1000节点）
//
// # 路由算法
//
// ## Dijkstra 最短路径
//
// 使用 Dijkstra 算法查找最短路径，权重为节点延迟。
//
// 时间复杂度：O(E log V)
//
// ## 路径评分公式
//
//	score = latency * 0.5 + hops * 0.3 + load * 0.2
//
// # 线程安全
//
// 所有公共方法都是线程安全的，可以并发调用。
package routing
