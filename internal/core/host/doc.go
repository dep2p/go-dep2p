// Package host 实现 P2P 主机服务
//
// host 作为 Core Layer 的聚合点，整合 Swarm、Protocol、NAT、Relay 等子系统，
// 为上层提供统一的网络服务接口。
//
// # Host 架构
//
// Host 采用门面（Facade）模式，组合以下组件：
//   - Swarm: 连接群管理
//   - Protocol Router: 协议注册和路由
//   - Peerstore: 节点信息存储
//   - EventBus: 事件通知
//   - ConnManager: 连接生命周期管理
//   - ResourceManager: 资源限制
//   - NAT: NAT 穿透服务（可选）
//   - Relay: 中继服务（可选）
//
// # 使用示例
//
//	// 创建 Host
//	host, err := host.New(
//	    host.WithSwarm(swarm),
//	    host.WithPeerstore(peerstore),
//	    host.WithEventBus(eventbus),
//	)
//
//	// 启动 Host
//	err = host.Start(ctx)
//
//	// 连接节点
//	err = host.Connect(ctx, peerID, addrs)
//
//	// 注册协议
//	host.SetStreamHandler("/my/proto/1.0.0", handler)
//
//	// 创建流
//	stream, err := host.NewStream(ctx, peerID, "/my/proto/1.0.0")
//
//	// 关闭 Host
//	err = host.Close()
//
// # 委托模式
//
// Host 采用委托模式将具体实现委托给子系统：
//
// Connect() 委托链：
//
//	Host.Connect()
//	  ├─> Peerstore.AddAddrs()  // 持久化地址
//	  └─> Swarm.DialPeer()      // 实际拨号
//
// NewStream() 委托链：
//
//	Host.NewStream()
//	  ├─> Swarm.NewStream()     // 创建流
//	  └─> Protocol.Negotiate()  // 协议协商
//
// SetStreamHandler() 委托链：
//
//	Host.SetStreamHandler()
//	  └─> Protocol.Register()   // 注册处理器
//
// # v1.0 实现范围
//
// ✅ 已实现：
//   - Host 聚合框架
//   - 地址管理（监听地址）
//   - 协议路由集成
//   - NAT/Relay 集成
//   - 生命周期管理
//
// ⬜ v1.1+ 计划：
//   - TD-HOST-001: 高级地址观测（ObservedAddrsManager）
//   - TD-HOST-002: AutoNAT v2 集成
//   - TD-HOST-003: 地址过滤策略增强
//
// # 技术债
//
// TD-HOST-001: 高级地址观测（ObservedAddrsManager）
//   - 当前状态：仅支持监听地址
//   - 阻塞原因：需要复杂的地址验证和观测逻辑
//   - 优先级：P2
//   - 预估工作量：2-3天
//   - 解除阻塞：实现地址观测、验证、评分机制
//
// TD-HOST-002: AutoNAT v2 集成
//   - 当前状态：使用 NAT 模块 AutoNAT 客户端
//   - 阻塞原因：需要 AutoNAT v2 服务端实现
//   - 优先级：P2
//   - 预估工作量：1-2天
//
// TD-HOST-003: 地址过滤策略增强
//   - 当前状态：简单的 AddrsFactory
//   - 阻塞原因：需要更多实际使用场景
//   - 优先级：P3
//   - 预估工作量：1天
//
// # 依赖
//
// 内部模块依赖：
//   - internal/core/swarm: 连接群管理
//   - internal/core/protocol: 协议路由
//   - internal/core/peerstore: 节点存储
//   - internal/core/eventbus: 事件总线
//   - internal/core/connmgr: 连接管理
//   - internal/core/nat: NAT 穿透（可选）
//   - internal/core/relay: 中继服务（可选）
//
// # 架构层
//
// Core Layer
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/core_host/README.md
//   - internal/core/host/DESIGN_REVIEW.md
package host
