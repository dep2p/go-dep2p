// Package upgrader 实现连接升级器
//
// # 概述
//
// upgrader 负责将原始网络连接升级为安全、多路复用的 P2P 连接。
// 它协调安全握手（TLS/Noise）和流复用器（yamux）的协商。
//
// # 升级流程
//
// 连接升级分为 4 个步骤：
//
//  1. 安全协议协商（multistream-select）
//     - 客户端提议：[/tls/1.0.0, /noise]
//     - 服务器选择：/tls/1.0.0
//
//  2. 安全握手
//     - TLS 1.3 握手
//     - 验证 PeerID（INV-001）
//
//  3. 多路复用器协商（multistream-select）
//     - 客户端提议：[/yamux/1.0.0]
//     - 服务器选择：/yamux/1.0.0
//
//  4. 多路复用设置
//     - 创建 yamux session
//     - 启用 keepalive
//
// # 使用示例
//
//	import "github.com/dep2p/go-dep2p/internal/core/upgrader"
//
//	// 创建 upgrader
//	id, _ := identity.Generate()
//	tlsTransport, _ := tls.New(id)
//	muxer := yamux.New()
//
//	upgrader, err := upgrader.New(id, upgrader.Config{
//	    SecurityTransports: []SecureTransport{tlsTransport},
//	    StreamMuxers: []StreamMuxer{muxer},
//	})
//
//	// 升级连接
//	conn, _ := net.Dial("tcp", "example.com:4001")
//	upgradedConn, err := upgrader.Upgrade(
//	    context.Background(),
//	    conn,
//	    DirOutbound,
//	    remotePeerID,
//	)
//
//	// 使用升级后的连接
//	stream, _ := upgradedConn.OpenStream(ctx)
//	stream.Write([]byte("hello"))
//
// # QUIC 特殊处理
//
// QUIC 连接自带加密和多路复用，会跳过升级流程。
//
// # Fx 集成
//
//	fx.Module("upgrader",
//	    fx.Provide(upgrader.ProvideUpgrader),
//	)
//
// # 依赖
//
// 内部模块依赖：
//   - internal/core/security: TLS/Noise 安全传输
//   - internal/core/muxer: yamux 流复用器
//   - internal/core/identity: Ed25519 身份
//
// 外部库：
//   - go-multistream: 协议协商
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/core_upgrader/
//   - multistream-select 规范
package upgrader
