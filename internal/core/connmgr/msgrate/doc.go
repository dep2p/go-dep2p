// Package msgrate 提供消息速率跟踪器实现
//
// 该模块用于动态测量和估计节点的消息处理能力，
// 从而优化请求大小和超时设置。
//
// # 核心概念
//
// Tracker: 单个节点的速率追踪器
//   - 记录每种消息类型的吞吐量
//   - 估计往返时间 (RTT)
//   - 计算在目标 RTT 内可处理的消息数量
//
// Trackers: 多个节点的追踪器集合
//   - 管理所有节点的追踪器
//   - 计算全局目标 RTT 和超时
//   - 提供平均容量统计
//
// # 使用示例
//
//	config := msgrate.DefaultConfig()
//	trackers := msgrate.NewTrackers(config)
//
//	// 为节点创建追踪器
//	tracker := msgrate.NewTracker(config, nil, 500*time.Millisecond)
//	trackers.Track("peer1", tracker)
//
//	// 更新测量结果
//	trackers.Update("peer1", MsgTypeHeaders, 100*time.Millisecond, 1000)
//
//	// 获取容量
//	capacity := trackers.Capacity("peer1", MsgTypeHeaders, trackers.TargetRoundTrip())
//
// # 架构归属
//
// 本模块属于 Core Layer，提供 QoS 相关的消息速率跟踪能力。
// 可被 Protocol 层（Messaging、PubSub）使用。
//
// # 参考
//
// 设计参考 go-ethereum eth/fetcher 的速率限制机制
package msgrate
