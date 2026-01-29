// Package tcp 实现 TCP 传输层
//
// tcp 使用 TCP 协议提供可靠的传输层，需要配合安全层（TLS/Noise）
// 和多路复用器（Yamux）使用。
//
// # 特性
//
//   - 基于 TCP
//   - 广泛兼容
//   - 需要 Upgrader 升级
//
// # 地址格式
//
//   /ip4/1.2.3.4/tcp/4001
//   /ip6/::1/tcp/4001
//
// # 使用示例
//
//	transport := tcp.NewTransport()
//
//	// 监听
//	listener, err := transport.Listen("/ip4/0.0.0.0/tcp/4001")
//
//	// 拨号
//	conn, err := transport.Dial(ctx, "/ip4/1.2.3.4/tcp/4001")
//
// # 升级流程
//
//  1. 建立 TCP 连接
//  2. 安全层握手（TLS/Noise）
//  3. 多路复用器协商（Yamux）
package tcp
