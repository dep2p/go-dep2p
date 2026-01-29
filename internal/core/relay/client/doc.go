// Package client 实现中继客户端
//
// client 提供通过中继节点连接其他节点的能力，用于无法直连的场景。
//
// # 功能
//
//   - 预约中继资源
//   - 通过中继建立连接
//   - 自动中继（AutoRelay）
//
// # 使用示例
//
//	client := client.NewClient(host)
//
//	// 预约中继
//	reservation, err := client.Reserve(ctx, relayPeer)
//
//	// 通过中继连接
//	conn, err := client.DialThroughRelay(ctx, relayAddr, targetPeer)
//
// # AutoRelay
//
// AutoRelay 自动发现和使用中继节点：
//
//	autoRelay := client.NewAutoRelay(host, options...)
//	autoRelay.Start(ctx)
package client
