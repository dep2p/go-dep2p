// Package peerstore 实现节点信息存储。
//
// peerstore 是节点信息的统一存储组件，管理已知节点的地址、公钥、协议支持和元数据。
// 它是节点发现和连接建立的基础设施。
//
// # 核心职责
//
// 1. 地址簿 (AddrBook): 存储节点多地址，TTL 管理，GC 清理
//
// 2. 密钥簿 (KeyBook): 存储节点公钥，PeerID 验证
//
// 3. 协议簿 (ProtoBook): 存储节点支持的协议
//
// 4. 元数据簿 (MetadataStore): 存储节点元数据
//
// # 使用示例
//
//	import "github.com/dep2p/go-dep2p/internal/core/peerstore"
//
//	// 创建 Peerstore
//	ps := peerstore.NewPeerstore()
//	defer ps.Close()
//
//	// 添加地址（连接成功后）
//	peerID := types.PeerID("QmXXX...")
//	addrs := []types.Multiaddr{
//	    types.Multiaddr("/ip4/127.0.0.1/tcp/4001"),
//	}
//	ps.AddAddrs(peerID, addrs, peerstore.ConnectedAddrTTL)
//
//	// 查询地址
//	retrievedAddrs := ps.Addrs(peerID)
//
//	// 添加公钥
//	pubKey := crypto.Ed25519PublicKey{...}
//	ps.AddPubKey(peerID, pubKey)
//
//		// 设置支持的协议
	//	import "github.com/dep2p/go-dep2p/pkg/protocol"
	//	ps.SetProtocols(peerID, types.ProtocolID(protocol.DHT))
//
//	// 存储元数据
//	ps.Put(peerID, "agent", "dep2p/v1.0.0")
//
// # 地址 TTL 常量
//
// 不同来源的地址使用不同的 TTL：
//
//	PermanentAddrTTL       永久地址（引导节点）
//	ConnectedAddrTTL       连接成功的地址（30 分钟）
//	RecentlyConnectedAddrTTL 最近连接的地址（15 分钟）
//	DiscoveredAddrTTL      DHT/Rendezvous 发现的地址（10 分钟）
//	LocalAddrTTL           mDNS 发现的地址（5 分钟）
//	TempAddrTTL            临时地址（2 分钟）
//
// # GC 清理
//
// Peerstore 会自动启动后台 GC 任务，定期清理过期地址：
//
//	// GC 配置
//	cfg := peerstore.Config{
//	    EnableGC:    true,
//	    GCInterval:  1 * time.Minute,
//	    GCLookahead: 10 * time.Second,
//	}
//
// # 并发安全
//
// Peerstore 及其子簿都是并发安全的，可以在多协程中安全使用：
//
//	// 协程 1：添加地址
//	go func() {
//	    ps.AddAddrs(peerID, addrs, ConnectedAddrTTL)
//	}()
//
//	// 协程 2：查询地址
//	go func() {
//	    addrs := ps.Addrs(peerID)
//	}()
//
// # Fx 模块集成
//
//	import "go.uber.org/fx"
//
//	app := fx.New(
//	    peerstore.Module(),
//	    fx.Invoke(func(ps *peerstore.Peerstore) {
//	        // 使用 peerstore
//	    }),
//	)
//
// # 架构设计
//
// Peerstore 采用组合模式，聚合 4 个子簿：
//
//	┌───────────────────────┐
//	│     Peerstore         │
//	├───────────────────────┤
//	│ • AddrBook            │
//	│ • KeyBook             │
//	│ • ProtoBook           │
//	│ • MetadataStore       │
//	└───────────────────────┘
//
// 每个子簿独立管理其数据，使用独立的 RWMutex 保证并发安全。
//
// # 设计文档
//
//   - design/03_architecture/L6_domains/core_peerstore/
//   - pkg/interfaces/peerstore.go
package peerstore
