// Package dht 实现 Kademlia DHT 分布式节点发现
//
// # 模块概述
//
// dht 基于 Kademlia 协议实现分布式哈希表，是 DeP2P 的核心发现组件。
// 提供节点发现、内容路由、值存储等功能，支持 Realm 隔离。
//
// # 版本 v2.0 新增功能
//
// 1. 权威目录能力
//   - DHT 作为跨 Relay 的唯一权威地址来源
//   - GetAuthoritativePeerRecord: 权威查询（DHT > Relay > Local）
//   - AddressBookProvider: 与 Relay 地址簿集成接口
//
// 2. 完整 PeerRecord 支持
//   - RealmPeerRecord: 包含 NAT 类型、可达性、能力列表
//   - SignedRealmPeerRecord: Ed25519 签名的 PeerRecord
//   - PeerRecordStore: 专用存储，支持 seq 冲突解决
//
// 3. Realm 隔离与 Key 格式
//   - RealmPeerKey: /dep2p/v2/realm/H(RealmID)/peer/NodeID
//   - SHA-256 哈希确保 RealmID 均匀分布
//   - Key 格式验证
//
// 4. 安全校验
//   - PeerRecordValidator: 签名验证、Key 匹配、TTL 校验
//   - seq 冲突选择：选择最大 seq 的有效记录
//   - 防重放攻击
//
// 5. 生命周期管理
//   - republishLoop: 定时续期（默认 TTL/2）
//   - LocalPeerRecordManager: seq 自动递增
//   - AddressChangeDetector: 地址变化检测触发重新发布
//   - DynamicTTLCalculator: 基于 NAT 类型动态调整 TTL
//
// 6. 可达性验证
//   - ReachabilityChecker: 可达性检测接口（依赖倒置）
//   - PublishLocalPeerRecordWithVerification: 发布前验证
//   - MakePublishDecision: 智能发布决策
//
// # 核心功能
//
// 1. 路由表管理
//   - 维护 256 个 K-Bucket（K=20）
//   - XOR 距离度量
//   - LRU 驱逐策略
//   - 替换缓存机制
//
// 2. 节点发现
//   - FindPeer: 查找特定节点
//   - FindPeers: 发现节点（实现 Discovery 接口）
//   - GetPeerRecord: 获取签名的 PeerRecord
//   - GetAuthoritativePeerRecord: 权威查询
//   - 迭代查询算法（Alpha=3）
//   - 查询结果自动缓存到 Peerstore
//
// 3. 内容路由
//   - Provide: 宣告内容提供者
//   - FindProviders: 查找内容提供者
//   - Provider 记录管理（TTL=24h）
//
// 4. 值存储
//   - PutValue: 存储键值对
//   - GetValue: 获取键值对
//   - 值复制机制（ReplicationFactor=3）
//   - TTL 支持（MaxRecordAge=24h）
//
// 5. PeerRecord 管理
//   - PublishPeerRecord: 发布签名的 PeerRecord
//   - PublishLocalPeerRecord: 发布本地 PeerRecord
//   - PublishLocalPeerRecordWithVerification: 带可达性验证的发布
//   - InitializeLocalRecordManager: 初始化本地记录管理器
//
// # 使用示例
//
//	// 创建 DHT
//	config := dht.DefaultConfig()
//	config.BootstrapPeers = []types.PeerInfo{...}
//
//	dht, err := dht.New(host, peerstore, dht.WithBootstrapPeers(peers))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 设置可达性检测器（可选）
//	dht.SetReachabilityChecker(myReachabilityChecker)
//
//	// 设置地址簿提供者（可选，用于 DHT/Relay 协作）
//	dht.SetAddressBookProvider(myAddressBookProvider)
//
//	// 启动 DHT
//	if err := dht.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 初始化本地记录管理器
//	if err := dht.InitializeLocalRecordManager(privKey, realmID); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 发布本地 PeerRecord（带可达性验证）
//	decision, err := dht.PublishLocalPeerRecordWithVerification(ctx)
//	if err != nil {
//	    log.Warn("publish failed:", err)
//	}
//
//	// 权威查询 PeerRecord
//	result, err := dht.GetAuthoritativePeerRecord(ctx, realmID, nodeID)
//	if err == nil && result.Source == dht.SourceDHT {
//	    log.Info("authoritative record found:", result.Record)
//	}
//
// # 架构设计
//
// DHT 采用分层架构：
//
//	┌─────────────────────────────────────────┐
//	│         Discovery 接口层                 │
//	│  FindPeers, Advertise, Start, Stop      │
//	├─────────────────────────────────────────┤
//	│           DHT 接口层                     │
//	│  FindPeer, GetPeerRecord, Provide       │
//	│  GetAuthoritativePeerRecord             │
//	│  PublishLocalPeerRecordWithVerification │
//	├─────────────────────────────────────────┤
//	│          核心组件层                      │
//	│  RoutingTable, ValueStore               │
//	│  ProviderStore, PeerRecordStore         │
//	│  LocalPeerRecordManager                 │
//	│  PeerRecordValidator                    │
//	├─────────────────────────────────────────┤
//	│          集成接口层                      │
//	│  AddressBookProvider (Relay 集成)       │
//	│  ReachabilityChecker (AutoNAT 集成)     │
//	├─────────────────────────────────────────┤
//	│          网络层                          │
//	│  NetworkAdapter, Handler                │
//	└─────────────────────────────────────────┘
//	              ↓
//	┌─────────────────────────────────────────┐
//	│       interfaces.Host 门面               │
//	│  Connect, NewStream, SetStreamHandler   │
//	└─────────────────────────────────────────┘
//
// # 权威目录层级
//
// 地址发现优先级（从高到低）：
//
//  1. DHT（权威）: SignedPeerRecord，最高权威性
//  2. Relay 地址簿（缓存）: DHT 的缓存层
//  3. Peerstore（本地）: 本地缓存，可能已过期
//
// # NetworkAdapter 防递归设计
//
// 为避免 "DHT→Connect→Discovery→DHT" 递归依赖，NetworkAdapter 采用特殊设计：
//
//  1. 优先从 DHT 路由表获取节点地址（不触发 discovery）
//  2. 其次从 Peerstore 获取地址
//  3. 使用 Host.Connect 直接拨号
//  4. 通过 Host.NewStream 创建 DHT 协议流
//
// # Layer1 安全机制
//
// DHT 实现了 Layer1 安全验证：
//
//  1. 速率限制
//     - PeerRecord: 10/min per sender
//     - Provider: 50/min per sender
//
//  2. SignedPeerRecord 验证
//     - Ed25519 签名验证
//     - seq 单调递增检查（防重放）
//     - Key 中的 RealmID/NodeID 必须与记录一致
//     - TTL 有效性验证
//
//  3. 地址验证
//     - 拒绝私网地址（10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16）
//     - 拒绝回环地址（127.0.0.0/8）
//     - 拒绝链路本地地址（169.254.0.0/16）
//     - 仅允许公网可达地址（可配置 AllowPrivateAddrs）
//
// # 协议
//
// 协议 ID: /dep2p/sys/dht/1.0.0
//
// 消息类型（10种）：
//   - FIND_NODE: 查找节点
//   - FIND_VALUE: 查找值
//   - STORE: 存储值
//   - PING: 心跳检测
//   - ADD_PROVIDER: 添加提供者
//   - GET_PROVIDERS: 获取提供者
//   - REMOVE_PROVIDER: 移除提供者
//   - PUT_PEER_RECORD: 存储 PeerRecord（v2.0 新增）
//   - GET_PEER_RECORD: 查询 PeerRecord（v2.0 新增）
//
// 编码格式: JSON
//
// # 参数
//
//   - K (BucketSize): 20 - K-桶容量
//   - Alpha: 3 - 并发查询参数
//   - QueryTimeout: 30s - 查询超时
//   - RefreshInterval: 1h - 路由表刷新间隔
//   - ReplicationFactor: 3 - 值复制因子
//   - MaxRecordAge: 24h - 记录最大存活时间
//   - ProviderTTL: 24h - Provider 记录 TTL
//   - PeerRecordTTL: 1h - PeerRecord TTL（v2.0: 支持动态调整）
//   - CleanupInterval: 10min - 清理间隔
//   - RepublishInterval: 20min - PeerRecord 重新发布间隔（建议 TTL/2）
//
// # 动态 TTL 调整
//
// 根据 NAT 类型自动调整 PeerRecord TTL：
//
//   - NATTypeNone（公网）: 最大 TTL（24h）
//   - NATTypeFullCone: 较长 TTL（2h）
//   - NATTypeRestrictedCone/PortRestricted: 基础 TTL（1h）
//   - NATTypeSymmetric: 较短 TTL（30min）
//   - Unknown: 较短 TTL（30min）
//
// # 生命周期
//
// DHT 运行 5 个后台循环：
//
//  1. bootstrap(): 引导流程，连接 BootstrapPeers
//  2. bootstrapRetryLoop(): 引导重试（最多10次）
//  3. refreshLoop(): 路由表刷新（1小时）
//  4. cleanupLoop(): 过期清理（10分钟）
//  5. republishLoop(): PeerRecord 续期（默认 TTL/2，支持地址变化检测）
//
// # 并发安全
//
// 所有公共方法都是并发安全的：
//   - 路由表使用 RWMutex 保护
//   - ValueStore 使用 RWMutex 保护
//   - ProviderStore 使用 RWMutex 保护
//   - PeerRecordStore 使用 RWMutex 保护
//   - LocalPeerRecordManager 使用 RWMutex + atomic 保护
//   - 状态标志使用 atomic 操作
//
// # 依赖倒置接口
//
// v2.0 定义了以下接口支持依赖倒置：
//
//   - AddressBookProvider: Relay 地址簿集成
//   - ReachabilityChecker: 可达性检测集成（AutoNAT/DialBack）
//   - PeerRecordValidator: PeerRecord 验证器
//
// # 依赖
//
//   - interfaces.Host: 网络主机门面
//   - interfaces.Peerstore: 节点信息存储（用于缓存）
//   - pkg/lib/crypto: 签名和验证
//
// # 设计文档
//
//   - Kademlia: A Peer-to-peer Information System Based on the XOR Metric
//   - design/_discussions/20260123-nat-relay-concept-clarification.md
//   - design/_discussions/20260124-dht-implementation-tracking.md
package dht
