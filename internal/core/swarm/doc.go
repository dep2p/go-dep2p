// Package swarm 实现连接群管理
//
// swarm 是 DeP2P 网络层的核心组件，负责管理节点间的所有连接和流。
// 它作为 Host 的底层引擎，处理多路复用连接池、拨号调度、连接生命周期管理等关键功能。
//
// # 核心功能
//
// 连接池管理：
//   - 维护到所有节点的多路复用连接池
//   - 连接复用（同一节点的多个流共享连接）
//   - 连接生命周期（创建、保持、关闭）
//   - 连接状态跟踪
//
// 拨号调度：
//   - 智能地址排序（优先本地、优先 QUIC）
//   - 并发拨号（Dial Many）
//   - 拨号超时与重试
//
// 监听管理：
//   - 多地址监听
//   - 入站连接接受
//   - 传输层适配
//
// 流管理：
//   - 流创建与复用
//   - 协议协商
//   - 流超时控制
//
// 事件通知：
//   - 连接建立/断开事件
//   - 流打开/关闭事件
//   - 通知器（Notifiee）机制
//
// # 快速开始
//
// 创建 Swarm：
//
//	swarm, err := NewSwarm("local-peer-id")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer swarm.Close()
//
// 添加传输层：
//
//	tcpTransport := tcp.NewTransport()
//	swarm.AddTransport("tcp", tcpTransport)
//
// 设置升级器：
//
//	upgrader := upgrader.NewUpgrader(...)
//	swarm.SetUpgrader(upgrader)
//
// 监听地址：
//
//	err = swarm.Listen("/ip4/0.0.0.0/tcp/4001")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// 拨号连接：
//
//	ctx := context.Background()
//	conn, err := swarm.DialPeer(ctx, "remote-peer-id")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// 创建流：
//
//	stream, err := swarm.NewStream(ctx, "remote-peer-id")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer stream.Close()
//
// 注册事件通知：
//
//	type MyNotifier struct{}
//
//	func (n *MyNotifier) Connected(conn Connection) {
//	    fmt.Printf("连接建立: %s\n", conn.RemotePeer())
//	}
//
//	func (n *MyNotifier) Disconnected(conn Connection) {
//	    fmt.Printf("连接断开: %s\n", conn.RemotePeer())
//	}
//
//	swarm.Notify(&MyNotifier{})
//
// # 架构设计
//
// Swarm 分层架构：
//
//	┌─────────────────────────────────────┐
//	│             Swarm                   │
//	├─────────────────────────────────────┤
//	│  ┌──────────┐  ┌────────────────┐  │
//	│  │ Dial Mgr │  │ Listen Manager │  │
//	│  └──────────┘  └────────────────┘  │
//	│  ┌──────────────────────────────┐  │
//	│  │     Connection Pool          │  │
//	│  └──────────────────────────────┘  │
//	├─────────────────────────────────────┤
//	│          Upgrader                   │
//	│  Security (TLS) + Muxer (Yamux)     │
//	├─────────────────────────────────────┤
//	│         Transports                  │
//	│      QUIC  |  TCP                   │
//	└─────────────────────────────────────┘
//
// # 并发安全
//
// Swarm 的所有公共方法都是并发安全的，可以从多个 goroutine 同时调用。
//
// 内部使用 sync.RWMutex 保护共享状态：
//   - 读操作（Peers, Conns, ConnsToPeer）使用读锁
//   - 写操作（addConn, removeConn）使用写锁
//   - 事件通知异步执行，不持有锁
//
// # 配置
//
// Swarm 支持以下配置项：
//   - DialTimeout: 远程网络拨号超时（默认 15s）
//   - DialTimeoutLocal: 本地网络拨号超时（默认 5s）
//   - NewStreamTimeout: 创建流超时（默认 15s）
//   - MaxConcurrentDials: 最大并发拨号数（默认 100）
//   - ConnHealthInterval: 连接健康检测间隔（默认 30s，设为 0 禁用）
//   - ConnHealthTimeout: 单次健康检测超时（默认 10s）
//
// 示例：
//
//	config := &Config{
//	    DialTimeout:        30 * time.Second,
//	    DialTimeoutLocal:   10 * time.Second,
//	    ConnHealthInterval: 30 * time.Second,  // 连接健康检测
//	    ConnHealthTimeout:  10 * time.Second,
//	}
//	swarm, err := NewSwarm("peer-id", WithConfig(config))
//
// # 依赖
//
// 内部模块依赖：
//   - internal/core/transport: 传输层（QUIC/TCP）
//   - internal/core/upgrader: 连接升级器（Security + Muxer）
//   - internal/core/peerstore: 节点存储（可选，用于获取节点地址）
//   - internal/core/connmgr: 连接管理器（可选，用于连接生命周期管理）
//   - internal/core/eventbus: 事件总线（可选，用于发布事件）
//
// # 错误处理
//
// Swarm 定义以下错误：
//   - ErrSwarmClosed: Swarm 已关闭
//   - ErrNoAddresses: 没有可用地址
//   - ErrDialTimeout: 拨号超时
//   - ErrNoTransport: 没有可用传输层
//   - ErrAllDialsFailed: 所有拨号都失败
//   - ErrNoConnection: 没有到节点的连接
//   - ErrDialToSelf: 尝试拨号自己
//
// # 性能
//
// 并发拨号：
//   - 同时尝试多个地址
//   - 第一个成功的连接胜出
//   - 其他拨号自动取消
//
// 地址优先级：
//   - 本地网络 > QUIC > TCP
//   - 减少延迟，提高成功率
//
// 连接复用：
//   - 每个节点可以有多个连接
//   - 同一连接上可以创建多个流
//   - 减少连接开销
//
// # 限制
//
// v1.0 限制：
//   - 暂不支持黑洞检测
//   - 暂不支持资源管理器集成
//   - 暂不支持 dialSync（多协程拨号去重）
//   - 暂不支持拨号退避（Backoff）
//
// 这些功能将在 v1.1+ 版本中实现。
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/core_swarm/README.md
//   - design/03_architecture/L6_domains/core_swarm/DESIGN_REVIEW.md
package swarm
