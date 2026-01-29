// Package realm 实现 Realm Manager（域管理器）
//
// # 概述
//
// realm 是 Realm 层的聚合模块，负责整合和协调 4 个子模块：
//   - auth：成员认证（PSK/Cert）
//   - member：成员管理（注册、同步、心跳）
//   - routing：域内路由（查找路径、负载均衡）
//   - gateway：域网关（中继转发、带宽限流）
//
// realm 提供统一的 Realm 管理接口和生命周期管理，采用门面模式简化上层调用。
//
// # 核心职责
//
// 1. **聚合子模块**：整合 auth、member、routing、gateway 四个子模块
// 2. **生命周期管理**：Join/Leave Realm，切换 Realm
// 3. **统一接口**：Manager 和 Realm 接口门面
// 4. **协调交互**：子模块间的协作（Auth → Member → Routing → Gateway）
// 5. **Protocol 工厂**：在 JoinRealm 时动态创建 Messaging、PubSub、Streams、Liveness 服务
// 6. **服务门面**：提供绑定到 Realm 的协议服务入口
//
// # 聚合架构
//
//	┌─────────────────────────────────────────────────────────────────┐
//	│                       Realm Manager                             │
//	├─────────────────────────────────────────────────────────────────┤
//	│                                                                 │
//	│  Manager API        Realm API           Service Facade         │
//	│  ────────────       ────────────         ──────────────         │
//	│  • Join()           • Members()          • Messaging()          │
//	│  • Leave()          • Router()           • PubSub()             │
//	│  • Current()        • Gateway()          • Streams()            │
//	│  • Get()            • Authenticate()     • Discovery()          │
//	│  • List()           • FindRoute()        │                      │
//	│                     • Relay()            │                      │
//	│                                                                 │
//	├─────────────────────────────────────────────────────────────────┤
//	│                      子模块协调层                                │
//	├─────────────────────────────────────────────────────────────────┤
//	│                                                                 │
//	│  Auth ──验证通过──> Member ──上线/下线──> Routing ──选路──> Gateway│
//	│                                                                 │
//	│  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐               │
//	│  │ Auth   │  │ Member │  │ Routing│  │Gateway │               │
//	│  │  PSK   │→ │ 缓存   │→ │ 路由表 │→ │  中继  │               │
//	│  │  Cert  │  │ 同步   │  │ 负载均衡│  │  限流  │               │
//	│  └────────┘  └────────┘  └────────┘  └────────┘               │
//	│                                                                 │
//	└─────────────────────────────────────────────────────────────────┘
//
// # 使用示例
//
// ## 基本使用
//
//	// 方式 1: 通过 Node API（推荐）
//	node, err := dep2p.New(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer node.Close()
//
//	// 加入 Realm
//	realm, err := node.JoinRealm(ctx, []byte("my-psk"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 方式 2: 直接使用 Manager（内部使用）
//	// Manager 由 Fx 自动注入，无需手动创建
//	// Protocol 服务（Messaging、PubSub、Streams、Liveness）
//	// 在 JoinRealm 时动态创建并绑定到 Realm
//
//	// 获取成员列表
//	members := realm.Members()
//	fmt.Printf("Members: %v\n", members)
//
//	// 发送消息（Protocol 服务绑定到 Realm）
//	messaging := realm.Messaging()
//	resp, err := messaging.Send(ctx, targetPeerID, "protocol", data)
//
//	// 离开 Realm
//	err = node.LeaveRealm(ctx)
//
// ## 服务门面使用
//
//	realm, _ := manager.Join(ctx, "my-realm", psk)
//
//	// Messaging 服务（点对点消息）
//	messaging := realm.Messaging()
//	messaging.Send(ctx, peerID, data)
//
//	// PubSub 服务（发布订阅）
//	pubsub := realm.PubSub()
//	pubsub.Subscribe(ctx, "my-topic")
//	pubsub.Publish(ctx, "my-topic", data)
//
//	// Streams 服务（流管理）
//	streams := realm.Streams()
//	stream, _ := streams.NewStream(ctx, peerID, protocol)
//
//	// Discovery 服务（成员发现）
//	discovery := realm.Discovery()
//	peers, _ := discovery.FindPeers(ctx, 10)
//
// ## Realm 切换
//
//	// 加入第一个 Realm
//	realm1, _ := manager.Join(ctx, "realm1", psk1)
//
//	// 切换到第二个 Realm（自动离开 realm1）
//	realm2, _ := manager.Join(ctx, "realm2", psk2)
//
//	// 显式离开当前 Realm
//	manager.Leave(ctx)
//
// # 协作流程
//
// ## Join Realm 流程
//
//  1. 验证输入（realmID、PSK）
//  2. 如果已在其他 Realm，先 Leave
//  3. 创建子模块实例：
//     Auth → Member → Routing → Gateway（依赖链）
//  4. 依次启动子模块
//  5. 注册到 Rendezvous（/dep2p/realm/<realmID>）
//  6. 发现并同步成员
//  7. 同步状态：Auth → Member → Routing → Gateway
//  8. 设置为 current Realm
//
// ## 子模块协调
//
//	Auth 验证 ────> Member 添加 ────> Routing 更新 ────> Gateway 同步
//	           │                │                 │
//	           │                │                 │
//	           └─ PSK 验证通过   └─ 成员上线/下线   └─ 可达节点同步
//
// ## Leave Realm 流程
//
//  1. 检查是否在 Realm 中
//  2. 依次停止子模块（逆序）：
//     Gateway → Routing → Member → Auth
//  3. 注销 Rendezvous
//  4. 清理 current Realm
//
// # 性能指标
//
//   - Join 操作延迟：< 500ms
//   - Member 同步延迟：< 100ms
//   - Routing 查询延迟：< 50ms
//   - Gateway 中继延迟：< 200ms
//   - 状态同步周期：30 秒
//
// # 线程安全
//
// 所有公共方法都是线程安全的：
//   - Manager：使用 sync.RWMutex 保护 realms 映射表
//   - Realm：使用 sync.RWMutex 保护子模块引用
//   - 原子操作：使用 atomic.Bool 管理状态标志
//
// # 错误处理
//
//	var (
//	    ErrNotStarted      = errors.New("manager not started")
//	    ErrAlreadyInRealm  = errors.New("already in a realm")
//	    ErrNotInRealm      = errors.New("not in any realm")
//	    ErrRealmNotFound   = errors.New("realm not found")
//	    ErrInvalidRealmID  = errors.New("invalid realm id")
//	    ErrInvalidPSK      = errors.New("invalid psk")
//	)
//
// # 依赖关系
//
// 直接依赖（子模块）：
//   - internal/realm/auth：PSK 认证
//   - internal/realm/member：成员管理
//   - internal/realm/routing：域内路由
//   - internal/realm/gateway：域网关
//
// 外部依赖：
//   - internal/core/host：Host 接口
//   - internal/core/peerstore：Peerstore 接口
//   - internal/discovery/coordinator：发现服务
//   - internal/core/identity：节点身份
//   - internal/core/eventbus：事件总线
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/realm/
//   - design/_discussions/20260113-implementation-plan.md
//
// # 版本历史
//
//   - v1.2.0 (2026-01-16)：重构为工厂模式，Protocol 服务在 JoinRealm 时动态创建
//   - v1.1.0 (2026-01-13)：完整实施 realm manager，整合 4 个子模块
package realm

import (
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("realm/manager")
