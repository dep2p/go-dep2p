// Package dial 提供拨号调度器实现
//
// Scheduler 负责管理和协调对等节点的连接建立：
//   - 静态节点：始终尝试保持连接，断开后自动重连
//   - 动态节点：按需连接，受连接数限制
//   - 拨号历史：防止频繁重连同一节点
//   - 并发控制：限制同时进行的拨号数量
//
// # 使用示例
//
//	config := dial.DefaultConfig()
//	scheduler := dial.NewScheduler(config, setupFunc)
//	scheduler.Start(ctx)
//
//	// 添加静态节点（始终保持连接）
//	scheduler.AddStatic(dial.PeerInfo{ID: "peer1", Addrs: addrs})
//
//	// 添加动态节点（按需连接）
//	scheduler.AddDynamic(dial.PeerInfo{ID: "peer2", Addrs: addrs})
//
//	// 通知连接状态变化
//	scheduler.PeerAdded("peer1")
//	scheduler.PeerRemoved("peer1")
//
// # 静态节点与动态节点
//
// 静态节点：
//   - 配置后始终尝试保持连接
//   - 断开后按配置的延迟自动重连
//   - 适用于 bootstrap 节点、relay 节点等
//
// 动态节点：
//   - 由发现服务或应用层按需添加
//   - 受连接数限制和拨号历史约束
//   - 适用于通过 DHT 发现的节点
//
// # 拨号历史
//
// 为防止频繁重连同一节点，调度器维护拨号历史：
//   - 成功或失败的拨号都会记录
//   - 历史记录有过期时间（默认 30 秒）
//   - 在过期前不会重新拨号同一节点
package dial
