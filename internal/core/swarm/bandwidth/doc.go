// Package bandwidth 提供带宽统计模块的实现
//
// # 核心功能
//
// 1. 流量统计 - 多维度流量统计
//   - 总流量统计（入站/出站）
//   - 按 Peer 分类统计
//   - 按 Protocol 分类统计
//   - 实时速率计算（EWMA）
//
// 2. 实时速率 - 指数加权移动平均
//   - 使用 EWMA 算法平滑速率计算
//   - 避免瞬时波动影响
//   - 提供准确的实时速率
//
// 3. 内存管理 - 自动清理空闲条目
//   - 定期清理长时间无活动的 Peer/Protocol 记录
//   - 防止内存泄漏
//   - 可配置清理间隔和超时
//
// # 快速开始
//
// 创建带宽计数器：
//
//	cfg := interfaces.DefaultBandwidthConfig()
//	counter := bandwidth.NewCounter(cfg)
//
// 记录流量：
//
//	// 记录发送的消息
//	counter.LogSentMessage(1024)
//
//	// 记录流上的流量（关联 Peer 和 Protocol）
//	counter.LogSentStream(2048, "/chat/1.0", "peer-id-123")
//	counter.LogRecvStream(1024, "/chat/1.0", "peer-id-123")
//
// 获取统计：
//
//	// 获取总统计
//	totals := counter.GetTotals()
//	fmt.Printf("总流量: 入 %d, 出 %d\n", totals.TotalIn, totals.TotalOut)
//	fmt.Printf("总速率: 入 %.2f B/s, 出 %.2f B/s\n", totals.RateIn, totals.RateOut)
//
//	// 获取指定 Peer 的统计
//	peerStats := counter.GetForPeer("peer-id-123")
//
//	// 获取指定协议的统计
//	protoStats := counter.GetForProtocol("/chat/1.0")
//
//	// 获取所有 Peer 的统计
//	allPeers := counter.GetByPeer()
//
//	// 获取所有协议的统计
//	allProtocols := counter.GetByProtocol()
//
// 管理：
//
//	// 重置所有统计
//	counter.Reset()
//
//	// 清理空闲条目（1小时前无活动）
//	counter.TrimIdle(time.Now().Add(-time.Hour))
//
// # 配置
//
// 默认配置：
//
//	Enabled:         true
//	TrackByPeer:     true
//	TrackByProtocol: true
//	IdleTimeout:     1 hour
//	TrimInterval:    10 minutes
//
// 自定义配置：
//
//	cfg := interfaces.BandwidthConfig{
//	    Enabled:         true,
//	    TrackByPeer:     true,
//	    TrackByProtocol: false,  // 禁用按协议统计
//	    IdleTimeout:     2 * time.Hour,
//	    TrimInterval:    5 * time.Minute,
//	}
//	counter := bandwidth.NewCounter(cfg)
//
// # 架构
//
// bandwidth 依赖：
//   - pkg/interfaces: 接口定义
//
// bandwidth 被依赖：
//   - internal/core/transport: Stream 实现集成
//   - internal/core/swarm: Stream 包装集成
//
// # EWMA 算法
//
// 使用指数加权移动平均 (EWMA) 计算实时速率：
//
//   rate_new = α * instant_rate + (1 - α) * rate_old
//
// 其中：
//   - α = 0.25（平滑因子）
//   - instant_rate = bytes / elapsed_time
//
// 优点：
//   - 平滑瞬时波动
//   - 快速响应变化
//   - 计算开销低
//
// # 性能考虑
//
// 1. 线程安全: 所有操作都是并发安全的
// 2. 内存占用: 每个 Peer/Protocol 一个 Meter（约 100 bytes）
// 3. 计算开销: EWMA 计算 O(1)，统计查询 O(n)
// 4. 清理策略: 定期清理空闲条目，防止内存泄漏
//
// # 注意事项
//
// 1. 启用统计: 如果 Enabled=false，所有记录操作会被跳过
// 2. 内存管理: 长时间运行需要定期调用 TrimIdle
// 3. 精度: EWMA 速率是近似值，不是精确值
// 4. 线程安全: 所有方法都是并发安全的
package bandwidth
