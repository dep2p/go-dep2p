// Package dep2p 提供简洁可靠的 P2P 网络库
//
// DeP2P (Decentralized P2P) 是一个模块化、可扩展的 P2P 网络库，
// 融合了 Iroh 的简洁性和 libp2p 的功能性。
//
// # 核心概念
//
// DeP2P 围绕三个核心概念构建：
//
//   - Node: P2P 节点，用户交互的主入口
//   - Realm: 业务域，提供 PSK 准入控制和协议隔离
//   - Services: 通信服务（Messaging、PubSub、Streams、Liveness）
//
// # 快速开始
//
//	import "github.com/dep2p/go-dep2p"
//
//	// 1. 创建并启动节点
//	node, err := dep2p.Start(ctx,
//	    dep2p.WithPreset(dep2p.PresetDesktop),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer node.Close()
//
//	// 2. 加入业务域
//	realmKey := []byte("my-secret-realm-key")
//	realm, err := node.JoinRealm(ctx, realmKey)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 3. 使用通信服务
//	messaging := realm.Messaging()
//	messaging.RegisterHandler("chat", chatHandler)
//	resp, _ := messaging.Send(ctx, peerID, "chat", []byte("hello"))
//
// # API 层次结构
//
//	┌─────────────────────────────────────────────────────────────────┐
//	│  入口层                                                          │
//	│  ┌─────────┐                                                     │
//	│  │  Node   │  dep2p.New() / dep2p.Start()                       │
//	│  └─────────┘                                                     │
//	├─────────────────────────────────────────────────────────────────┤
//	│  域层                                                            │
//	│  ┌─────────┐                                                     │
//	│  │  Realm  │  node.JoinRealm()                                  │
//	│  └─────────┘                                                     │
//	├─────────────────────────────────────────────────────────────────┤
//	│  协议层                                                          │
//	│  ┌───────────┐ ┌────────┐ ┌─────────┐ ┌──────────┐              │
//	│  │ Messaging │ │ PubSub │ │ Streams │ │ Liveness │              │
//	│  └───────────┘ └────────┘ └─────────┘ └──────────┘              │
//	│  realm.Messaging() / realm.PubSub() / ...                       │
//	└─────────────────────────────────────────────────────────────────┘
//
// # 文件组织
//
// 本包按功能领域组织代码：
//
//	dep2p/
//	├── dep2p.go              # 包文档、版本信息
//	│
//	# ════════════════════════════════════════════════════════════════
//	#                          入口层（Node）
//	# ════════════════════════════════════════════════════════════════
//	├── node.go               # Node 结构定义、New()、基本信息
//	├── node_lifecycle.go     # Start、Stop、Close、ReadyLevel
//	├── node_address.go       # AdvertisedAddrs、ShareableAddrs、ConnectionTicket
//	├── node_connect.go       # Connect、Disconnect、ConnectedPeers、GetPeerInfo
//	├── node_observe.go       # 事件回调、带宽统计、健康检查、网络变化
//	├── node_capabilities.go  # EnableBootstrap、EnableRelay 能力开关
//	├── node_diagnostics.go   # 网络诊断、种子恢复、自省服务
//	│
//	# ════════════════════════════════════════════════════════════════
//	#                          域层（Realm）
//	# ════════════════════════════════════════════════════════════════
//	├── realm.go              # Realm 结构、成员管理、获取协议服务
//	│
//	# ════════════════════════════════════════════════════════════════
//	#                          协议层（Services）
//	# ════════════════════════════════════════════════════════════════
//	├── messaging.go          # 点对点消息（请求-响应模式）
//	├── pubsub.go             # 发布订阅（Topic、Subscription）
//	├── streams.go            # 双向流（BiStream）
//	├── liveness.go           # 存活检测（Ping、状态监控）
//	│
//	# ════════════════════════════════════════════════════════════════
//	#                          支撑层
//	# ════════════════════════════════════════════════════════════════
//	├── options.go            # WithXxx 配置选项
//	├── presets.go            # 预设配置（Mobile、Desktop、Server）
//	├── types.go              # 公共类型（NodeState、BandwidthSnapshot 等）
//	├── errors.go             # 错误定义
//	└── helpers.go            # 内部辅助函数
//
// # 预设配置
//
// DeP2P 提供四种预设配置：
//
//	dep2p.PresetMobile   移动端优化，低资源占用
//	dep2p.PresetDesktop  桌面端默认配置（推荐）
//	dep2p.PresetServer   服务器优化，高性能
//	dep2p.PresetMinimal  最小配置，仅用于测试
//
// # 五层软件架构
//
//	┌─────────────────────────────────────────────────────────────┐
//	│  1. API Layer                                               │
//	│     dep2p.New(), dep2p.Start()                              │
//	│     用户入口，配置选项                                        │
//	├─────────────────────────────────────────────────────────────┤
//	│  2. Protocol Layer                                          │
//	│     Messaging, PubSub, Streams, Liveness                    │
//	│     用户级应用协议                                            │
//	├─────────────────────────────────────────────────────────────┤
//	│  3. Realm Layer                                             │
//	│     Manager, Auth, Member, Relay                            │
//	│     业务隔离，成员管理                                        │
//	├─────────────────────────────────────────────────────────────┤
//	│  4. Core Layer                                              │
//	│     Host, Transport, Security, NAT, Relay                   │
//	│     P2P 网络核心能力                                         │
//	├─────────────────────────────────────────────────────────────┤
//	│  5. Discovery Layer                                         │
//	│     DHT, Bootstrap, mDNS                                    │
//	│     节点发现服务                                             │
//	└─────────────────────────────────────────────────────────────┘
//
// # 更多资源
//
//   - 设计文档: design/
//   - 使用示例: examples/
//   - 用户文档: docs/
//
// # 版本
//
// 当前版本: v0.2.0-beta.1
//
// 更多信息请访问: https://github.com/dep2p/go-dep2p
package dep2p
