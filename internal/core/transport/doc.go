// Package transport 实现传输层抽象
//
// Transport 抽象不同的传输协议（QUIC、TCP、WebSocket）。
//
// # 核心职责
//
//   - 连接管理：监听入站连接，发起出站连接
//   - QUIC 传输：0-RTT 快速握手，内置多路复用，拥塞控制
//   - TCP 传输：兼容传统网络，需配合 yamux 多路复用
//   - WebSocket 传输：浏览器兼容，防火墙穿透
//   - 地址处理：Multiaddr 解析，协议适配
//
// # 支持的传输协议
//
//   - QUIC (默认): /ip4/.../udp/.../quic-v1
//   - TCP: /ip4/.../tcp/...
//   - WebSocket: /ip4/.../tcp/.../ws
//
// # 使用示例
//
//	// 创建 QUIC 传输
//	transport := quic.New(localPeer, tlsConfig)
//
//	// 监听连接
//	listener, err := transport.Listen(types.NewMultiaddr("/ip4/0.0.0.0/udp/4001/quic-v1"))
//
//	// 接受连接
//	conn, err := listener.Accept()
//
//	// 拨号连接
//	conn, err := transport.Dial(ctx, remoteAddr, remotePeer)
//
//	// 创建流
//	stream, err := conn.NewStream(ctx)
//
// # TTL 常量
//
// 无特定 TTL 常量（由 peerstore 管理地址 TTL）
//
// # GC 清理
//
// 无需 GC（连接由 swarm 管理）
//
// # 并发安全
//
// 所有传输实现使用 sync.RWMutex 保护并发访问。
//
// # Fx 模块集成
//
//	import (
//	    "go.uber.org/fx"
//	    "github.com/dep2p/go-dep2p/internal/core/transport"
//	)
//
//	app := fx.New(
//	    transport.Module(),
//	    fx.Invoke(func(tm *transport.TransportManager) {
//	        // 使用传输管理器
//	    }),
//	)
//
// # 架构设计
//
// Transport 层位于五层架构的第 2 层（传输层）：
//
//	L5: Protocol Layer (协议层)
//	L4: Host Layer (主机层)
//	L3: Swarm Layer (连接群层)
//	L2: Transport Layer (传输层) ← 本模块
//	L1: Identity Layer (身份层)
//
// # 相关文档
//
//   - 设计文档：design/03_architecture/L6_domains/core_transport/
//   - 接口定义：pkg/interfaces/transport.go
//   - 架构说明：design/_discussions/20260113-architecture-v1.1.0.md
//
// 架构层：Core Layer
// 公共接口：pkg/interfaces/transport.go
package transport
