// Package muxer 实现流多路复用
//
// 提供基于 yamux 协议的流多路复用能力，支持：
//   - 单连接多流（1000+ 并发流）
//   - 流量控制（16MB 窗口）
//   - 心跳保活（30s 间隔）
//   - 资源管理集成
//   - 并发安全
//
// # 快速开始
//
//	transport := muxer.NewTransport()
//
//	// 服务端
//	muxedConn, _ := transport.NewConn(conn, true, peerScope)
//	stream, _ := muxedConn.AcceptStream()
//	defer stream.Close()
//
//	// 客户端
//	muxedConn, _ := transport.NewConn(conn, false, peerScope)
//	stream, _ := muxedConn.OpenStream(ctx)
//	defer stream.Close()
//
//	// 读写数据
//	stream.Write([]byte("hello"))
//	buf := make([]byte, 1024)
//	n, _ := stream.Read(buf)
//
// # yamux 配置
//
// yamux 是一个可靠的流多路复用协议，提供：
//   - MaxStreamWindowSize: 16MB（高吞吐量优化）
//   - KeepAliveInterval: 30s（心跳检测）
//   - MaxIncomingStreams: 无限制（由 ResourceManager 控制）
//   - ReadBufSize: 0（安全传输层已有缓冲）
//
// 配置说明：
//   - 16MB 窗口在 100ms 延迟下可达 160MB/s 吞吐量
//   - 心跳保活及时检测连接断开
//   - 流数量由资源管理器动态控制，防止资源耗尽
//
// # Fx 模块
//
//	import "go.uber.org/fx"
//
//	app := fx.New(
//	    muxer.Module,
//	    fx.Invoke(func(transport pkgif.StreamMuxer) {
//	        id := transport.ID()
//	        fmt.Printf("Muxer: %s\n", id) // "/yamux/1.0.0"
//	    }),
//	)
//
// # 架构定位
//
// Tier: Core Layer Level 1（无依赖）
//
// 依赖关系：
//   - 依赖：pkg/interfaces, go-yamux
//   - 被依赖：transport, host
//
// # 并发安全
//
// yamux 本身是并发安全的：
//   - OpenStream/AcceptStream 可以并发调用
//   - 多个流可以并发读写
//   - Close 操作是线程安全的
//
// # 资源管理集成
//
// 通过 PeerScope.BeginSpan() 集成资源管理：
//
//	newSpan := func() (yamux.MemoryManager, error) {
//	    return scope.BeginSpan()
//	}
//	sess, _ := yamux.Server(conn, config, newSpan)
//
// ResourceScopeSpan 实现 yamux.MemoryManager 接口：
//   - ReserveMemory(size int, prio uint8) error
//   - ReleaseMemory(size int)
//
// # 流使用规范
//
// 正确关闭流：
//
//	stream, _ := conn.OpenStream(ctx)
//	defer stream.Close()  // 优雅关闭
//
//	// 或者在错误时
//	if err != nil {
//	    stream.Reset()  // 强制关闭
//	}
//
// 超时设置：
//
//	stream.SetReadDeadline(time.Now().Add(5 * time.Second))
//	defer stream.SetReadDeadline(time.Time{})  // 清除超时
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/core_muxer/
//   - pkg/interfaces/muxer.go
package muxer
