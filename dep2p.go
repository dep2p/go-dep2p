// Package dep2p 提供简洁可靠的 P2P 网络库
//
// DeP2P (Decentralized P2P) 是一个模块化、可扩展的 P2P 网络库，
// 融合了 Iroh 的简洁性和 libp2p 的功能性。
//
// # 核心创新：Realm 业务隔离
//
// Realm 是 DeP2P 的核心创新，提供独立的 P2P 子网络：
//   - PSK 准入控制：通过预共享密钥验证成员身份
//   - 协议级隔离：不同 Realm 的节点无法互相通信
//   - 独立发现：节点只发现同 Realm 的其他节点
//
// # 快速开始
//
// 最简单的使用方式：
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
//	// 3. 使用协议层服务（多协议支持）
//
//	// Messaging - 点对点消息（多协议）
//	messaging := realm.Messaging()
//	messaging.RegisterHandler("chat", chatHandler)
//	resp, _ := messaging.Send(ctx, peerID, "chat", []byte("hello"))
//
//	// PubSub - 发布订阅（多主题）
//	pubsub := realm.PubSub()
//	topic, _ := pubsub.Join("my-topic")
//	topic.Publish(ctx, []byte("world"))
//
//	// Streams - 双向流（多协议）
//	streams := realm.Streams()
//	stream, _ := streams.Open(ctx, peerID, "file-transfer")
//	stream.Write(fileData)
//
// # 预设配置
//
// DeP2P 提供四种预设配置：
//
//   - PresetMobile: 移动端优化，低资源占用
//   - PresetDesktop: 桌面端默认配置（推荐）
//   - PresetServer: 服务器优化，高性能
//   - PresetMinimal: 最小配置，仅用于测试
//
// 使用预设：
//
//	node, err := dep2p.New(ctx,
//	    dep2p.WithPreset(dep2p.PresetServer),
//	    dep2p.WithListenPort(4001),
//	)
//
// # 自定义配置
//
// 通过 Option 函数自定义配置：
//
//	node, err := dep2p.New(ctx,
//	    dep2p.WithListenPort(4001),
//	    dep2p.WithIdentityFromFile("~/.dep2p/identity.key"),
//	    dep2p.WithBootstrapPeers(
//	        "/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMG...",
//	    ),
//	    dep2p.WithRelay(true),
//	    dep2p.WithNAT(true),
//	    dep2p.WithConnectionLimits(50, 100),
//	)
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
// # 依赖方向
//
// # API → Protocol → Realm → Core ↔ Discovery
//
// # 更多文档
//
//   - 设计文档: /design
//   - 架构指南: /design/03_architecture
//   - 使用示例: /examples
//
// # 版本信息
//
// 当前版本: v0.2.0-beta.1
//
// 更多信息请访问: https://github.com/dep2p/go-dep2p
package dep2p

// Version 当前版本
// 更新此版本号时，请同步更新 version.json
const Version = "v0.2.0-beta.1"

// BuildInfo 构建信息（通过 ldflags 注入）
var (
	// GitCommit Git 提交哈希
	GitCommit string

	// BuildDate 构建日期
	BuildDate string

	// GoVersion Go 版本
	GoVersion string
)

// VersionInfo 返回完整版本信息字符串
func VersionInfo() string {
	info := "DeP2P " + Version
	if GitCommit != "" {
		info += " (" + GitCommit[:min(8, len(GitCommit))] + ")"
	}
	if BuildDate != "" {
		info += " built " + BuildDate
	}
	return info
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ════════════════════════════════════════════════════════════════════════════
//                              类型别名
// ════════════════════════════════════════════════════════════════════════════

// Endpoint 是 Node 的类型别名
//
// 为兼容旧 API 提供，新代码应直接使用 *Node。
// Endpoint 提供网络端点功能：
//   - ID() 获取节点 ID
//   - ListenAddrs() 获取监听地址
//   - ConnectionCount() 获取连接数
//   - Close() 关闭端点
type Endpoint = *Node
