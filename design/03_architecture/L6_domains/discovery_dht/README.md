# Discovery DHT 模块

> **版本**: v1.2.0  
> **更新日期**: 2026-01-23  
> **定位**: Kademlia DHT 分布式发现（Discovery Layer）

---

## 模块概述

discovery_dht 基于 Kademlia 协议实现分布式节点发现，是 DeP2P 的核心发现组件。

**★ 核心目的**：DHT 发布的目的是"让别人能连上我"，因此只能发布可达地址。

| 属性 | 值 |
|------|-----|
| **架构层** | Discovery Layer |
| **代码位置** | `internal/discovery/dht/` |
| **协议** | `/dep2p/sys/dht/1.0.0` |
| **Fx 模块** | `fx.Module("discovery_dht")` |
| **状态** | ✅ **已实现** |
| **依赖** | core_host, core_peerstore, ★ core_reachability（发布前置条件） |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       discovery_dht 职责                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 路由表管理                                                               │
│     • 维护 Kademlia K-Bucket                                                │
│     • 路由表刷新                                                            │
│     • 节点距离计算 (XOR Metric)                                             │
│                                                                             │
│  2. 节点发现                                                                 │
│     • FindPeer 查找特定节点                                                 │
│     • FindClosestPeers 查找最近节点                                         │
│     • 迭代查询算法                                                          │
│                                                                             │
│  3. 内容路由                                                                 │
│     • Provide 宣告内容提供者                                                │
│     • FindProviders 查找内容提供者                                          │
│     • Provider Record 管理                                                  │
│                                                                             │
│  4. 值存储 (可选)                                                            │
│     • PutValue 存储键值                                                     │
│     • GetValue 获取键值                                                     │
│     • 值验证                                                                │
│                                                                             │
│  5. 启动时自动 Bootstrap（v1.2.1 新增）                                      │
│     • DHT 启动后自动调用 Bootstrap() 填充路由表                             │
│     • 引导节点来源：配置 + Peerstore 合并（配置优先）                        │
│     • 异步执行，不阻塞 Fx 启动流程                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 启动时自动 Bootstrap

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    DHT 启动时自动 Bootstrap（Step A4 对齐）                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  触发条件：                                                                  │
│  ────────────                                                               │
│  • config.Discovery.EnableDHT = true                                        │
│  • config.Discovery.EnableBootstrap = true                                  │
│                                                                             │
│  执行时机：                                                                  │
│  ────────────                                                               │
│  • DHT.Start() 成功后                                                       │
│  • 延迟 500ms 确保 Host 完全就绪                                            │
│  • 异步执行，不阻塞 Fx 启动                                                 │
│                                                                             │
│  引导节点来源（合并策略）：                                                  │
│  ──────────────────────────                                                 │
│  1. 配置中的 BootstrapPeers（优先，从统一配置解析）                         │
│  2. Peerstore 中已知的 peers（补充，排除本地节点）                          │
│  • 以 PeerID 为键去重，配置优先                                             │
│                                                                             │
│  超时与错误处理：                                                            │
│  ────────────────                                                           │
│  • 单次 Bootstrap 超时：30 秒                                               │
│  • 失败只记录日志，不影响节点启动                                           │
│  • 后续可通过连接事件逐步填充路由表                                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## DHT 发布（v2.0）

> 详见 [DHT-Realm 架构重构方案](../../../_discussions/20260126-dht-realm-architecture-redesign.md)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DHT 发布（v2.0）                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  功能说明：                                                                  │
│  ──────────                                                                 │
│  JoinRealm 时立即发布到 DHT（先发布后发现）。                                │
│  发布完成后启动后台发现循环，发现其他成员后进行 PSK 双向认证。               │
│                                                                             │
│  DHT Key 格式（v2.0）：                                                      │
│  ────────────────────                                                       │
│  /dep2p/v2/node/<NodeID>                    — Node 级 PeerRecord             │
│  /dep2p/v2/realm/<H(RealmID)>/members       — Provider Record（成员发现）    │
│  /dep2p/v2/realm/<H(RealmID)>/peer/<NodeID> — Realm 成员 PeerRecord          │
│                                                                             │
│  调用流程（JoinRealm）：                                                     │
│  ────────────────────────                                                   │
│  1. 派生 RealmID = HKDF(psk, "dep2p-realm-id")                              │
│  2. DHT.ProvideRealmMembership(realmID) — 发布 Provider Record              │
│  3. DHT.PublishRealmPeerRecord(realmID, record) — 发布成员地址              │
│  4. 返回 Realm 对象（非阻塞）                                               │
│  5. 启动后台发现循环（goroutine，指数退避）                                 │
│                                                                             │
│  接口定义（v2.0）：                                                          │
│  ──────────────────                                                         │
│  // pkg/interfaces/dht.go                                                   │
│  ProvideRealmMembership(ctx, realmID) error        // 发布 Provider         │
│  FindRealmMembers(ctx, realmID) ([]peer.ID, error) // 查询成员列表          │
│  PublishRealmPeerRecord(ctx, realmID, record) error                         │
│  FindRealmPeerRecord(ctx, realmID, nodeID) (*SignedRealmPeerRecord, error)  │
│                                                                             │
│  错误处理：                                                                  │
│  ──────────                                                                 │
│  • 发布失败只记录日志，不阻塞 JoinRealm 返回                                │
│  • 后续可通过发现循环重试                                                   │
│  • 指数退避策略：2s → 4s → 8s → ... → 60s（最大）                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 架构设计

### Kademlia 路由表

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Kademlia 路由表                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  K-Bucket 结构 (k = 20)                                                     │
│  ─────────────────────                                                      │
│                                                                             │
│  Distance: XOR(LocalID, RemoteID)                                           │
│                                                                             │
│  Bucket[0]:   距离 2^0   (最近邻)                                           │
│  Bucket[1]:   距离 2^1                                                      │
│  Bucket[2]:   距离 2^2                                                      │
│  ...                                                                        │
│  Bucket[255]: 距离 2^255 (最远)                                             │
│                                                                             │
│  每个 Bucket 最多存储 k 个节点，按 LRU 排序                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 迭代查找流程

```
FindPeer(targetID) 流程：

1. 从路由表选取 α 个最近节点
2. 并发向这些节点发送 FIND_NODE
3. 收集返回的更近节点
4. 继续向更近节点查询
5. 直到找到目标或无更近节点

参数：
• α (Alpha) = 3 并发度
• k = 20 最大返回数
```

---

## 公共接口

```go
// pkg/interfaces/discovery.go (DHT 部分)

// DHTRouter DHT 路由接口
type DHTRouter interface {
    Discovery
    
    // 节点发现
    FindPeer(ctx context.Context, peer types.PeerID) (types.PeerInfo, error)
    FindClosestPeers(ctx context.Context, key []byte, count int) ([]types.PeerID, error)
    
    // 内容路由
    Provide(ctx context.Context, key []byte) error
    FindProviders(ctx context.Context, key []byte, count int) ([]types.PeerInfo, error)
    
    // 值存储 (可选)
    PutValue(ctx context.Context, key string, value []byte) error
    GetValue(ctx context.Context, key string) ([]byte, error)
    
    // 路由表
    RoutingTable() RoutingTable
}

// RoutingTable 路由表接口
type RoutingTable interface {
    Size() int
    ListPeers() []types.PeerID
    NearestPeers(key []byte, count int) []types.PeerID
}
```

---

## DHT 模式

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| **Client** | 只查询不响应 | 移动设备、短暂节点 |
| **Server** | 响应查询请求 | 长期在线节点 |
| **Auto** | 自动切换 | 默认模式 |

---

## 参考实现

### go-libp2p-kad-dht

```
github.com/libp2p/go-libp2p-kad-dht/
├── dht.go            # DHT 主结构
├── routing.go        # 路由实现
├── lookup.go         # 查找算法
├── providers.go      # Provider 管理
├── records.go        # Record 处理
└── routing_table/    # K-Bucket 实现

关键参数：
• K = 20 (Bucket 大小)
• Alpha = 3 (并发度)
• RefreshInterval = 10 分钟
```

### iroh DHT

```
iroh/src/discovery/pkarr/dht.rs
• Pkarr DHT 实现
• DNS 友好的密钥发布
```

---

## ★ 地址发布策略（关键约束）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    DHT 地址发布策略（★ 必须遵循）                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  核心原则：DHT 发布的目的是"让别人能连上我"                                 │
│  ════════════════════════════════════════════                               │
│  发布不可达地址 = 浪费其他节点的连接尝试 = 系统效率下降                      │
│                                                                             │
│  发布规则：                                                                  │
│  ──────────                                                                 │
│  ✅ 只发布经 Reachability 验证的可达地址                                    │
│  ❌ 禁止发布候选地址（STUN/Observed 未验证）                                │
│  ❌ 禁止发布私网地址（10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16）          │
│  ❌ 禁止发布回环地址（127.0.0.0/8）                                         │
│                                                                             │
│  不可达时的策略：                                                            │
│  ───────────────                                                            │
│  当 Reachability 验证失败（节点不可达）时：                                  │
│  • ❌ 不发布直连地址                                                        │
│  • ✅ 发布 Relay 地址作为替代                                               │
│  • ★ 前提：需存在显式配置的 Relay 且有有效预留（ADR-0010）                  │
│  • ★ 如果没有配置 Relay，则无法发布任何地址（节点不可被发现）               │
│  • 格式：/p2p/<RelayID>/p2p-circuit/p2p/<MyID>                             │
│  • 确保其他节点总能通过某种方式连接本节点                                   │
│                                                                             │
│  地址来源约束：                                                              │
│  ──────────────                                                             │
│  Reachability 输出 = DHT 发布输入                                           │
│  • DHT 模块只接受 Reachability 模块产出的可达地址                          │
│  • 不自行收集或验证地址                                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## ★ TTL 与刷新策略

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    DHT 记录 TTL 与刷新                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  PeerRecord TTL 与刷新（与澄清文档一致）：                                   │
│  ─────────────────────────────────────────                                  │
│  • TTL = 24 小时（Kademlia 默认，记录在 DHT 中的有效期）                    │
│  • 续约间隔 = 12 小时（提前刷新，确保连续性）                                │
│  • 续约窗口 = TTL 到期前 2 小时                                             │
│                                                                             │
│  Provider TTL 与刷新：                                                      │
│  ────────────────────                                                       │
│  • TTL = 24 小时                                                            │
│  • 续约间隔 = 12 小时                                                       │
│                                                                             │
│  地址变更时：                                                                │
│  ────────────                                                               │
│  • 触发条件：Reachability 产出新的可达地址                                  │
│  • 动作：立即重新发布 PeerRecord（不等待定时刷新）                          │
│  • 新 Seqno：时间戳派生，确保单调递增                                       │
│                                                                             │
│  网络变化时：                                                                │
│  ────────────                                                               │
│  • 触发条件：NetMon 检测到网络变化（4G→WiFi 等）                            │
│  • 动作：                                                                   │
│    1. 重新 STUN 探测（获取新候选地址）                                      │
│    2. Reachability 验证                                                     │
│    3. 发布新地址到 DHT                                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## ★ 优雅关闭与取消发布（Phase D 对齐）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    DHT 优雅关闭（UnpublishPeerRecord）                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  调用时机：                                                                  │
│  ──────────                                                                 │
│  • Node.Close() 时调用 DHT.UnpublishPeerRecord()                            │
│  • Phase D Step D3：优雅关闭前通知网络本节点即将离线                         │
│                                                                             │
│  实现逻辑：                                                                  │
│  ──────────                                                                 │
│  1. 清理 LocalPeerRecordManager 状态（阻止后续续期）                         │
│  2. 从本地 peerRecordStore 删除记录                                          │
│     • Node Key: /dep2p/v2/node/<NodeID>                                     │
│     • Realm Key: /dep2p/v2/realm/<H(RealmID)>/peer/<NodeID>                │
│  3. 可选：向最近的 K 个节点发送 TTL=0 删除通知                               │
│                                                                             │
│  接口定义：                                                                  │
│  ──────────                                                                 │
│  // pkg/interfaces/dht.go                                                   │
│  UnpublishPeerRecord(ctx context.Context) error                             │
│                                                                             │
│  实现文件：                                                                  │
│  ──────────                                                                 │
│  • internal/discovery/dht/dht.go: UnpublishPeerRecord()                     │
│  • internal/discovery/dht/local_record_manager.go: Clear()                  │
│                                                                             │
│  注意事项：                                                                  │
│  ──────────                                                                 │
│  • UnpublishPeerRecord 是幂等操作，多次调用安全                             │
│  • 即使未初始化也不会报错（直接返回 nil）                                   │
│  • 当前实现不主动通知远程节点，依赖 TTL 过期自然清理                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 配置参数

### K-Bucket 路由表参数（实现）

| 参数 | 值 | 说明 |
|-----|-----|------|
| `KeySize` | 256 | 256 位密钥空间（SHA-256） |
| `BucketSize (K)` | 20 | 每个桶最多 20 个节点（Kademlia 标准） |
| `Alpha (α)` | 3 | 并发查询参数（Kademlia 标准） |
| `NodeExpireTime` | 24h | 节点过期时间 |
| `BucketRefreshInterval` | 1h | 桶刷新间隔 |

### DHT 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `BucketSize` | 20 | K-Bucket 大小 |
| `Alpha` | 3 | 并发查询数 |
| `QueryTimeout` | 30s | 单次查询超时 |
| `RefreshInterval` | 1h | 路由表刷新间隔 |
| `ReplicationFactor` | 3 | 值复制因子 |
| `ProviderTTL` | 24h | Provider 记录 TTL |
| `PeerRecordTTL` | 1h | PeerRecord TTL |
| `CleanupInterval` | 10min | 过期记录清理间隔 |
| `RepublishInterval` | 20min | 记录重新发布间隔 |
| `AllowPrivateAddrs` | true | 是否允许私网地址（便于测试） |

---

## 目录导航

| 文档 | 说明 |
|------|------|
| [requirements/requirements.md](requirements/requirements.md) | 需求追溯 |
| [design/overview.md](design/overview.md) | 整体设计 |
| [design/internals.md](design/internals.md) | 内部设计 |
| [coding/guidelines.md](coding/guidelines.md) | 编码指南 |
| [testing/strategy.md](testing/strategy.md) | 测试策略 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [discovery_coordinator](../discovery_coordinator/) | 发现协调器 |
| [discovery_bootstrap](../discovery_bootstrap/) | 引导发现 |
| [core_reachability](../core_reachability/) | 地址可达性验证（发布输入源） |
| [core_relay](../core_relay/) | Relay 地址簿（替代发布） |
| [L3: 发现流程](../../L3_behavioral/discovery_flow.md) | 行为设计 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-25
