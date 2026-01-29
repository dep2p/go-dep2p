// Package quic 实现 QUIC 传输层
//
// quic 使用 QUIC 协议提供可靠、安全、多路复用的传输层。
// QUIC 内置 TLS 1.3，无需额外的安全层。
//
// # 特性
//
//   - 基于 UDP
//   - 内置 TLS 1.3
//   - 原生多路复用
//   - 0-RTT 连接恢复
//   - 连接迁移
//
// # 地址格式
//
//   /ip4/1.2.3.4/udp/4001/quic-v1
//   /ip6/::1/udp/4001/quic-v1
//
// # 使用示例
//
//	transport, err := quic.NewTransport(identity)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 监听
//	listener, err := transport.Listen("/ip4/0.0.0.0/udp/4001/quic-v1")
//
//	// 拨号
//	conn, err := transport.Dial(ctx, "/ip4/1.2.3.4/udp/4001/quic-v1", peerID)
package quic
