// Package delivery 提供可靠消息投递功能
//
// # IMPL-NETWORK-RESILIENCE Phase 4: 可靠投递
//
// 本包实现了消息队列和 ACK 确认机制，用于保证消息的可靠投递。
//
// # 核心组件
//
// ReliablePublisher - 可靠消息发布器:
//   - 消息队列：网络不可用时缓存消息
//   - 自动重发：网络恢复后自动发送队列中的消息
//   - 发送状态回调：通知调用方消息的实际发送状态
//   - ACK 确认：支持关键节点确认机制
//
// MessageQueue - 消息队列:
//   - FIFO + LRU 淘汰策略
//   - 消息过期自动清理
//   - 最大重试次数限制
//
// # ACK 协议
//
// ACK 协议用于确保关键消息被指定节点接收：
//
//	消息格式: [ack_request_len(2bytes)][ack_request][payload]
//
// ACK 请求嵌入到消息中，接收方收到后发送 ACK 响应。
//
// # 使用示例
//
// 基本使用（不启用 ACK）:
//
//	publisher := delivery.NewReliablePublisher(underlying, nil)
//	publisher.Start(ctx)
//
//	// 发布消息
//	err := publisher.Publish(ctx, "topic", data)
//
//	// 注册状态回调
//	publisher.OnStatusChange(func(msgID string, status delivery.DeliveryStatus, err error) {
//	    log.Info("消息状态变更", "msgID", msgID, "status", status)
//	})
//
// 启用 ACK 确认:
//
//	config := delivery.DefaultPublisherConfig()
//	config.EnableAck = true
//	config.CriticalPeers = []string{"peer1", "peer2"}
//
//	publisher := delivery.NewReliablePublisher(underlying, config)
//	publisher.Start(ctx)
//
//	// 发布并等待 ACK
//	result, err := publisher.PublishWithAck(ctx, "topic", data)
//	if result.Success {
//	    log.Info("所有关键节点已确认")
//	}
package delivery
