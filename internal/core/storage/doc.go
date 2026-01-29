// Package storage 提供统一的持久化存储服务
//
// Storage 模块基于 BadgerDB 实现，为 DeP2P 提供统一的键值存储后端。
// 从 v1.1.0 开始，所有组件统一使用 BadgerDB 持久化存储，不再提供内存模式。
//
// # 架构
//
// Storage 模块位于 Core Layer，为其他模块提供存储服务：
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                      使用方模块                              │
//	│  Peerstore | DHT | AddressBook | Rendezvous | MemberStore  │
//	└─────────────────────────────────────────────────────────────┘
//	                              │
//	                              ▼
//	┌─────────────────────────────────────────────────────────────┐
//	│                     storage (本包)                          │
//	│  ┌─────────────────────────────────────────────────────┐   │
//	│  │                    KVStore                          │   │
//	│  │              带前缀隔离的 KV 抽象                    │   │
//	│  └─────────────────────────────────────────────────────┘   │
//	│                              │                              │
//	│  ┌─────────────────────────────────────────────────────┐   │
//	│  │                  engine/badger                      │   │
//	│  │                  BadgerDB 实现                       │   │
//	│  └─────────────────────────────────────────────────────┘   │
//	└─────────────────────────────────────────────────────────────┘
//
// # 键空间设计
//
// 各模块使用不同的键前缀实现数据隔离：
//
//	前缀     | 模块           | 说明
//	---------|----------------|------------------
//	p/a/     | Peerstore      | 节点地址
//	p/k/     | Peerstore      | 节点公钥
//	p/p/     | Peerstore      | 支持的协议
//	p/m/     | Peerstore      | 节点元数据
//	d/v/     | DHT            | 值存储
//	d/p/     | DHT            | Provider 记录
//	d/r/     | DHT            | 路由表
//	a/       | AddressBook    | Realm 成员地址
//	r/       | Rendezvous     | 注册记录
//	m/       | MemberStore    | Realm 成员信息
//
// # 使用示例
//
// 使用 Fx 依赖注入（推荐）：
//
//	app := fx.New(
//	    storage.Module(),
//	    // ... 其他模块
//	)
//
// 手动创建：
//
//	cfg := storage.PersistentConfig("/data/dep2p")
//	eng, err := storage.NewEngine(cfg)
//	if err != nil {
//	    return err
//	}
//	defer eng.Close()
//
//	// 创建带前缀的 KVStore
//	peerstore := storage.NewKVStore(eng, []byte("p/"))
//	dht := storage.NewKVStore(eng, []byte("d/"))
//
// # 配置选项
//
// 通过 WithDataDir 配置数据目录：
//
//	node, err := dep2p.Start(ctx,
//	    dep2p.WithDataDir("./data"),
//	)
//
// # 线程安全
//
// 所有公开的类型和方法都是线程安全的。
package storage
