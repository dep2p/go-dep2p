// Package metrics 提供监控指标收集
//
// metrics 模块实现带宽统计功能，基于 go-flow-metrics 库提供：
//   - 带宽统计（全局/按协议/按节点）
//   - 流量速率计算（自动速率窗口）
//   - 并发安全（flow.Meter 内置并发安全）
//   - 内存管理（TrimIdle 清理空闲统计）
//
// # 快速开始
//
//	counter := metrics.NewBandwidthCounter()
//
//	// 记录全局消息
//	counter.LogSentMessage(1024)
//	counter.LogRecvMessage(2048)
//
//	// 记录流消息（关联协议和节点）
//	counter.LogSentMessageStream(512, proto, peer)
//	counter.LogRecvMessageStream(256, proto, peer)
//
//	// 获取统计
//	stats := counter.GetBandwidthTotals()
//	fmt.Printf("In: %d, Out: %d\n", stats.TotalIn, stats.TotalOut)
//	fmt.Printf("RateIn: %.2f B/s, RateOut: %.2f B/s\n", stats.RateIn, stats.RateOut)
//
// # 分层统计
//
// metrics 支持三层带宽统计：
//
//	// 1. 全局统计（所有流量）
//	totalStats := counter.GetBandwidthTotals()
//
//	// 2. 按协议统计
//	protoStats := counter.GetBandwidthForProtocol(types.ProtocolID("/test/1.0.0"))
//
//	// 3. 按节点统计
//	peerStats := counter.GetBandwidthForPeer(types.PeerID("peer1"))
//
// # 速率计算
//
// 速率由 flow.Meter 自动计算：
//   - 基于指数加权移动平均（EWMA）
//   - 默认时间窗口（由 go-flow-metrics 控制）
//   - 实时更新，并发安全
//
// # Fx 模块
//
//	import "go.uber.org/fx"
//
//	app := fx.New(
//	    metrics.Module,
//	    fx.Invoke(func(reporter metrics.Reporter) {
//	        reporter.LogSentMessage(100)
//	        stats := reporter.GetBandwidthTotals()
//	        log.Printf("Stats: %+v", stats)
//	    }),
//	)
//
// # 内存管理
//
// 定期清理空闲统计，防止内存泄漏：
//
//	// 清理 1 小时前的空闲计量器
//	counter.TrimIdle(time.Now().Add(-1 * time.Hour))
//
// # 架构定位
//
// Tier: Core Layer Level 1（无依赖）
//
// 依赖关系：
//   - 依赖：pkg/interfaces, pkg/types, go-flow-metrics
//   - 被依赖：swarm, host
//
// # 并发安全
//
// 所有方法都是并发安全的：
//   - flow.Meter 使用原子操作
//   - flow.MeterRegistry 内置锁保护
//   - 无需额外同步
//
// # 实现说明
//
// Phase 1 实现：
//   - ✅ BandwidthCounter（核心带宽统计）
//   - ✅ Reporter 接口
//   - ✅ Stats 结构（快照）
//   - ✅ Fx 模块
//
// Phase 2 扩展（未实现）：
//   - ⏸️ Prometheus 集成
//   - ⏸️ OpenTelemetry 集成
//   - ⏸️ Counter/Gauge/Histogram/Summary 指标类型
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/core_metrics/
//   - pkg/interfaces/metrics.go
package metrics
