# DISC-1227-分层中继设计

**日期**：2024-12-27  
**参与者**：@qinglong  
**状态**：✅ 已结论

---

## 背景

在讨论 API 分层设计时，发现中继（Relay）与 Realm 分层存在根本冲突：

```
场景：A 和 B 在 Realm-X，但无法直连，需要中继 C

┌─────────┐                ┌─────────┐                ┌─────────┐
│  Node A │ ──────────────▶│ Relay C │──────────────▶ │  Node B │
│ Realm-X │   业务消息      │ Realm-Y │   业务消息      │ Realm-X │
└─────────┘                └─────────┘                └─────────┘
                                ▲
                                │
                 ❌ 问题：C 不在 Realm-X
                 ❌ 但 C 承担了 Realm-X 的带宽成本
                 ❌ C 甚至可能"看到"（虽然加密）Realm-X 的流量模式
                 ❌ 打破了 Realm 业务隔离的设计初衷
```

**核心问题**：业务消息不应该跨 Realm 进行发送，但是中继打破了这个壁垒，导致非该 Realm 的节点在承担相应的带宽成本。

---

## 问题分析

### 核心矛盾

| 层级 | 设计目标 | 中继的问题 |
|------|---------|-----------|
| **Layer 1** | 系统基础设施，无业务概念 | 中继在这里是"无差别服务" |
| **Layer 2** | 业务隔离，只服务 Realm 成员 | 但连接能力依赖 Layer 1 中继 |
| **Layer 3** | 业务通信 | 消息走了"非 Realm 成员"的中继 |

**结论**：Layer 1 的中继无法区分业务边界，导致 Realm 隔离被"穿透"。

### 问题本质

1. **中继没有分层**：当前中继是"全局服务"，不区分 Realm
2. **成本分配不合理**：非 Realm 成员承担了 Realm 业务的带宽成本
3. **隔离被打破**：业务数据流经非业务成员
4. **安全风险**：虽然数据加密，但流量模式可能泄露业务信息

### 详细问题场景

#### 场景 1：带宽成本问题

```
Realm-A（区块链网络）有 1000 个节点
Realm-B（游戏网络）有 100 个节点

如果使用全局中继：
- Realm-B 的节点可能承担 Realm-A 的带宽成本
- 不公平：Realm-B 成员没有从 Realm-A 受益，却承担成本
```

#### 场景 2：隔离被打破

```
节点 A（Realm-X）想连接节点 B（Realm-X）
但 A 和 B 无法直连，需要通过中继 C（Realm-Y）

问题：
- C 不在 Realm-X，但能看到 Realm-X 的流量
- 虽然数据加密，但流量模式、连接频率等可能泄露信息
- 违反了"业务隔离"的设计原则
```

#### 场景 3：资源滥用

```
如果中继是全局的，没有 Realm 限制：
- 恶意节点可能滥用中继资源
- 无法区分"合法业务流量"和"恶意流量"
- 难以实施 Realm 级别的资源控制
```

---

## 讨论内容

### 解决方案：中继也要分层

**核心思想**：
- **Layer 1 Relay（系统中继）**：只服务系统级通信
- **Layer 2 Relay（Realm 中继）**：只服务 Realm 内业务通信

### 分层中继架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          分层中继设计                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Layer 3: 应用协议层                                                 │    │
│  │           业务消息走 Realm Relay                                     │    │
│  │           协议: /dep2p/app/*                                        │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      ▲                                       │
│                                      │ 业务消息                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Layer 2: Realm 层                                                   │    │
│  │                                                                      │    │
│  │   ┌──────────────────────────────────────────────────────────────┐  │    │
│  │   │  Realm Relay（业务中继）                                      │  │    │
│  │   │  ────────────────────────────────────────────────────────────│  │    │
│  │   │  • 只服务同 Realm 成员                                        │  │    │
│  │   │  • 由 Realm 成员自愿提供                                      │  │    │
│  │   │  • 承担 Realm 内的带宽成本（合理！）                          │  │    │
│  │   │  • 验证双方都是 Realm 成员才转发                              │  │    │
│  │   │  • 允许 /dep2p/app/*、/dep2p/realm/* 协议                     │  │    │
│  │   │  • 带宽/时长限制由提供者配置                                   │  │    │
│  │   └──────────────────────────────────────────────────────────────┘  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      ▲                                       │
│                                      │ 系统消息 / 连接建立                   │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Layer 1: 系统基础层                                                 │    │
│  │                                                                      │    │
│  │   ┌──────────────────────────────────────────────────────────────┐  │    │
│  │   │  System Relay（系统中继）                                     │  │    │
│  │   │  ────────────────────────────────────────────────────────────│  │    │
│  │   │  • 用于 DHT 查询、Bootstrap、NAT 探测                         │  │    │
│  │   │  • 用于帮助打洞、建立直连                                     │  │    │
│  │   │  • 有带宽/时间限制（防止滥用）                                │  │    │
│  │   │  • 不转发业务消息（/dep2p/app/* 协议）                        │  │    │
│  │   │  • 只允许 /dep2p/sys/* 协议                                   │  │    │
│  │   │  • 带宽限制：10 KB/s                                          │  │    │
│  │   │  • 时长限制：60 秒                                            │  │    │
│  │   └──────────────────────────────────────────────────────────────┘  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Layer 1 System Relay（系统中继）

#### 职责与限制

```go
// Layer 1 系统中继的职责与限制
type SystemRelay interface {
    // ─────────────────────────────────────────────────────────────────────
    // 允许的协议（白名单）
    // ─────────────────────────────────────────────────────────────────────
    // /dep2p/sys/ping/1.0.0        ✅ 允许
    // /dep2p/sys/identify/1.0.0    ✅ 允许
    // /dep2p/sys/dht/1.0.0         ✅ 允许
    // /dep2p/sys/relay/1.0.0       ✅ 允许（中继控制协议）
    // /dep2p/sys/holepunch/1.0.0   ✅ 允许（打洞协调）
    // /dep2p/sys/bootstrap/1.0.0   ✅ 允许
    //
    // /dep2p/app/*                 ❌ 拒绝（业务协议）
    // /dep2p/realm/*               ❌ 拒绝（Realm 协议）
    
    // ─────────────────────────────────────────────────────────────────────
    // 资源限制
    // ─────────────────────────────────────────────────────────────────────
    // • 单连接带宽限制：10 KB/s
    // • 单连接时长限制：60 秒（足够完成打洞）
    // • 总带宽限制：根据服务器配置
    
    // ─────────────────────────────────────────────────────────────────────
    // 核心用途
    // ─────────────────────────────────────────────────────────────────────
    // 1. 帮助节点发现自己的公网地址（STUN-like）
    // 2. 协调打洞（Hole Punching Coordination）
    // 3. DHT 查询的临时中转
    // 4. Bootstrap 时的初始连接
}
```

#### 使用场景

1. **STUN-like 地址发现**
   ```
   节点 A（NAT 后）通过 System Relay 发现自己的公网地址
   System Relay 返回：/ip4/203.x.x.x/udp/4001/quic-v1
   ```

2. **打洞协调**
   ```
   节点 A 和 B 想直连，但都在 NAT 后
   通过 System Relay 协调打洞时机
   System Relay 只负责"协调"，不转发业务数据
   ```

3. **DHT 查询中转**
   ```
   节点 A 查询节点 B 的地址
   如果 A 和 B 无法直连，通过 System Relay 中转查询请求
   但只中转查询，不转发业务消息
   ```

### Layer 2 Realm Relay（业务中继）

#### 职责与特点

```go
// Layer 2 Realm 中继
type RealmRelay interface {
    // ─────────────────────────────────────────────────────────────────────
    // 准入控制（基于 PSK 成员证明）
    // ─────────────────────────────────────────────────────────────────────
    // • Relay 本身必须是 Realm 成员（持有 realmKey）
    // • 只接受同 Realm 成员的连接
    // • 验证请求方和目标方都持有相同 realmKey
    // • 非成员的中继请求直接拒绝
    
    // ─────────────────────────────────────────────────────────────────────
    // 允许的协议（植入 RealmID，天然隔离）
    // ─────────────────────────────────────────────────────────────────────
    // /dep2p/app/<realmID>/*       ✅ 允许（本 Realm 应用协议）
    // /dep2p/realm/<realmID>/*     ✅ 允许（本 Realm 控制协议）
    //
    // /dep2p/sys/*                 ❌ 拒绝（系统协议走 System Relay）
    // /dep2p/app/<other-realmID>/* ❌ 拒绝（其他 Realm 协议）
    
    // ─────────────────────────────────────────────────────────────────────
    // 费用/激励模型（可选）
    // ─────────────────────────────────────────────────────────────────────
    // • Realm 内可以有激励机制
    // • 中继节点获得"信誉分"或代币奖励
    // • 使用中继需要消耗相应资源
}
```

#### 验证流程（基于 PSK）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Realm Relay 验证流程（PSK 成员证明）                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   前提：A、B、R 都持有同一 realmKey（加入 Realm 时获得）                       │
│                                                                              │
│   ┌─────────┐         ┌─────────┐         ┌─────────┐                       │
│   │  Node A │ ──(1)──▶│ Relay R │──(4)───▶│  Node B │                       │
│   │ Realm-X │         │ Realm-X │         │ Realm-X │                       │
│   └─────────┘         └─────────┘         └─────────┘                       │
│                            │                                                 │
│                       验证流程                                               │
│                            │                                                 │
│   (1) A → R: 请求中继到 B                                                    │
│       • 携带 PSK 成员证明（peerID = B，绑定到目标）:                          │
│         proof = MAC(HKDF(realmKey, "membership"),                            │
│                     nodeA || realmID || nodeB || nonce || timestamp)         │
│       • 携带目标 B 的 NodeID                                                 │
│       • 携带协议 ID（必须是 /dep2p/app/<realmID>/* 格式）                     │
│                                                                              │
│   (2) R 验证 A:                                                              │
│       • 用本地 realmKey 重算 MAC，验证 A 持有相同密钥                        │
│       • 验证 peerID 与请求目标一致（proof.peerID == 请求中的 targetNodeID）   │
│       • 验证协议前缀匹配本 Realm（/dep2p/app/<本realmID>/）                  │
│       • 如果失败 → 拒绝（返回 ErrNotMember 或 ErrProtocolNotAllowed）        │
│                                                                              │
│   (3) R 验证 B（惰性验证，连接时确认）:                                      │
│       • R 尝试连接 B，B 在握手时同样提供 PSK 证明                            │
│       • 若 B 无法提供有效证明 → 拒绝（返回 ErrTargetNotMember）              │
│                                                                              │
│   (4) R → B: 转发连接请求                                                    │
│       • R 将 A 的证明转发给 B                                                 │
│       • B 验证 A 的证明：                                                     │
│         - 验证 MAC 正确（A 持有 realmKey）                                    │
│         - 验证 proof.peerID == 自己（证明是给我的）                           │
│         - 验证 timestamp 有效（防重放）                                       │
│       • B 可以选择接受或拒绝                                                 │
│                                                                              │
│   (5) 建立双向中继通道                                                       │
│       • 后续 A ↔ B 的消息通过 R 转发                                         │
│       • R 只看到密文（端到端加密，基于 Noise/TLS）                           │
│       • R 可以统计流量，但看不到内容                                         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 协议验证逻辑

```go
// Relay 验证协议是否允许转发
func (r *RealmRelay) validateProtocol(proto string) error {
    expectedPrefix := fmt.Sprintf("/dep2p/app/%s/", r.realmID)
    realmPrefix := fmt.Sprintf("/dep2p/realm/%s/", r.realmID)
    
    if strings.HasPrefix(proto, expectedPrefix) ||
       strings.HasPrefix(proto, realmPrefix) {
        return nil // ✅ 允许
    }
    return ErrProtocolNotAllowed // ❌ 拒绝
}
```

### 两种中继对比

| 维度 | System Relay (Layer 1) | Realm Relay (Layer 2) |
|------|----------------------|----------------------|
| **服务范围** | 全网任意节点 | 仅同 Realm 成员 |
| **提供者** | 公共基础设施 | Realm 成员自愿提供 |
| **协议限制** | 只允许 `/dep2p/sys/*` | 只允许 `/dep2p/app/<realmID>/*` 和 `/dep2p/realm/<realmID>/*` |
| **成员验证** | 无 | PSK 证明（验证双方持有 realmKey） |
| **带宽限制** | 严格限制（10KB/s） | 由提供者配置 |
| **时长限制** | 60 秒 | 无限（或由提供者配置） |
| **用途** | 打洞协调、DHT、Bootstrap | 业务消息转发 |
| **费用** | 免费（公共资源） | 可有激励机制 |
| **成本承担** | 公共基础设施 | Realm 成员（合理！） |

---

## 连接建立流程（修正版）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                A 连接 B 的完整流程（A 和 B 在同一 Realm）                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Step 1: 地址发现                                                           │
│   ─────────────────                                                          │
│   A 通过 DHT 查询 B 的地址                                                   │
│   B 的 AdvertisedAddrs 可能包含：                                            │
│     • 直连地址：/ip4/203.x.x.x/udp/4001/quic-v1                              │
│     • Realm 中继地址：/p2p/<RealmRelay>/p2p-circuit/p2p/<B>                  │
│                                                                              │
│   Step 2: 尝试直连                                                           │
│   ─────────────────                                                          │
│   A 优先尝试直连 B                                                           │
│   ✅ 成功 → 完成（最优路径）                                                  │
│   ❌ 失败 → Step 3                                                           │
│                                                                              │
│   Step 3: 尝试打洞                                                           │
│   ─────────────────                                                          │
│   如果直连失败，通过 Layer 1 System Relay 协调打洞                           │
│   System Relay 只负责"协调"，不转发业务数据                                  │
│   ✅ 成功 → 完成（直连建立）                                                 │
│   ❌ 失败 → Step 4                                                           │
│                                                                              │
│   Step 4: 使用 Realm Relay                                                   │
│   ─────────────────────                                                      │
│   如果打洞失败，使用 Realm Relay 转发                                        │
│   Realm Relay 验证：                                                         │
│     ✓ A 提供有效 PSK 成员证明（持有 realmKey）                                │
│     ✓ B 提供有效 PSK 成员证明（持有 realmKey）                                │
│     ✓ 协议前缀匹配 /dep2p/app/<realmID>/*（不可跨 Realm）                    │
│   ✅ 验证通过 → 建立中继连接                                                 │
│   ❌ 验证失败 → 连接失败                                                     │
│                                                                              │
│   ⚠️ 注意：                                                                  │
│   • 如果 Realm 内没有可用的 Realm Relay → 连接失败                           │
│   • 不会 fallback 到 System Relay 转发业务消息                               │
│   • 这确保了：业务数据永远不会"泄露"到 Realm 外部节点                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 流程图

```
                    A 连接 B（同 Realm）
                           │
                           ▼
                    ┌─────────────┐
                    │  地址发现    │
                    │ (DHT 查询)   │
                    └──────┬──────┘
                           │
                           ▼
                    ┌─────────────┐
                    │  尝试直连    │
                    └──────┬──────┘
                           │
              ┌────────────┴────────────┐
              │                          │
           ✅ 成功                    ❌ 失败
              │                          │
              ▼                          ▼
         ┌─────────┐            ┌─────────────┐
         │  完成   │            │  尝试打洞    │
         └─────────┘            │(System Relay)│
                                └──────┬───────┘
                                       │
                          ┌────────────┴────────────┐
                          │                         │
                       ✅ 成功                   ❌ 失败
                          │                         │
                          ▼                         ▼
                     ┌─────────┐            ┌─────────────┐
                     │  完成   │            │使用Realm Relay│
                     └─────────┘            └──────┬───────┘
                                                   │
                                      ┌────────────┴────────────┐
                                      │                         │
                                  验证通过                  验证失败
                                      │                         │
                                      ▼                         ▼
                                 ┌─────────┐            ┌─────────┐
                                 │  完成   │            │  失败   │
                                 └─────────┘            └─────────┘
```

---

## 接口设计

### Realm 中增加 Relay 服务

```go
// Realm 接口（扩展）
type Realm interface {
    // ... 之前的方法 ...
    
    // ─────────────────────────────────────────────────────────────────────
    // Relay 服务（新增）
    // ─────────────────────────────────────────────────────────────────────
    
    // Relay 获取 Realm 中继服务
    Relay() RealmRelayService
}

// RealmRelayService Realm 级别的中继服务
type RealmRelayService interface {
    // ─────────────────────────────────────────────────────────────────────
    // 成为中继
    // ─────────────────────────────────────────────────────────────────────
    
    // Serve 声明自己愿意为 Realm 提供中继服务
    // 其他 Realm 成员可以通过你中转消息
    Serve(ctx context.Context, opts ...RelayOption) error
    
    // StopServing 停止提供中继服务
    StopServing() error
    
    // IsServing 是否正在提供中继服务
    IsServing() bool
    
    // ─────────────────────────────────────────────────────────────────────
    // 发现中继
    // ─────────────────────────────────────────────────────────────────────
    
    // FindRelays 发现 Realm 内的可用中继节点
    FindRelays(ctx context.Context) ([]NodeID, error)
    
    // ─────────────────────────────────────────────────────────────────────
    // 使用中继
    // ─────────────────────────────────────────────────────────────────────
    
    // Reserve 预留一个中继槽位（用于接收入站连接）
    Reserve(ctx context.Context, relay NodeID) (Reservation, error)
    
    // ─────────────────────────────────────────────────────────────────────
    // 统计
    // ─────────────────────────────────────────────────────────────────────
    
    // Stats 获取中继使用统计
    Stats() RelayStats
}

// RelayOption 中继服务配置
type RelayOption func(*relayConfig)

func WithRelayBandwidthLimit(bytesPerSec int64) RelayOption
func WithRelayMaxConnections(n int) RelayOption
func WithRelayMaxDuration(d time.Duration) RelayOption

// Reservation 中继预留
type Reservation interface {
    // Relay 返回预留的中继节点
    Relay() NodeID
    
    // Addrs 返回可以告诉其他人的中继地址
    Addrs() []Multiaddr
    
    // Refresh 刷新预留（延长有效期）
    Refresh(ctx context.Context) error
    
    // Close 释放预留
    Close() error
}

// RelayStats 中继统计
type RelayStats struct {
    // 作为中继时的统计
    RelayedConnections int64
    RelayedBytes       int64
    
    // 使用中继时的统计
    ConnectionsViaRelay int64
    BytesViaRelay       int64
}
```

---

## 边界情况处理

### Q1: 如果 Realm 内没有可用的中继节点怎么办？

**方案 1：连接失败，明确告知用户（✅ 推荐）**

```go
err := messaging.Send(ctx, peerID, data)
if errors.Is(err, dep2p.ErrNoRelayAvailable) {
    // Realm 内没有可用中继，且无法直连
    // 提示用户：需要有成员成为中继，或改善网络环境
    log.Warn("无法连接：Realm 内没有可用中继节点")
}
```

**方案 2：配置时强制要求中继（对于关键业务）**

```go
realm, err := node.JoinRealm(ctx, "critical-realm",
    dep2p.WithRequireRelay(true), // 如果没有中继可用，JoinRealm 失败
)
if err != nil {
    log.Fatal("关键业务 Realm 必须至少有一个中继节点")
}
```

**方案 3：自动成为中继（如果节点有公网 IP）**

```go
// 节点启动时自动检测
if hasPublicIP {
    realm.Relay().Serve(ctx) // 自动成为中继
}
```

### Q2: 是否允许 Fallback 到 System Relay？

**结论：不允许（保持 Realm 隔离）**

```go
// 默认行为：不允许 fallback
err := messaging.Send(ctx, peerID, data)
// 如果 Realm 内没有中继，且无法直连，返回 ErrNoRelayAvailable
```

**说明**：
1. System Relay 只允许 `/dep2p/sys/*` 控制面协议（打洞协调、DHT 查询、Bootstrap 等），不转发 `/dep2p/app/*` 或 `/dep2p/realm/*` 数据面流量。
2. 因此所谓 “fallback” 在本设计中**仅存在于建连控制面**（例如：直连失败后仍可通过 System Relay 协调打洞），不允许把业务数据“退化”为经由 System Relay 转发。
3. 若业务方确实需要“全网公共中继转发业务数据”的能力，那等价于引入新的信任与成本模型（公共基础设施承担业务成本），应当作为**另一个显式的产品模式**单独设计，而不是作为 Realm 模式的可选开关。


### Q3: 如何激励节点成为中继？

**方案 1：信誉分系统**

```go
// Realm 内维护信誉分
type Reputation struct {
    NodeID      NodeID
    RelayScore  int64  // 提供中继服务获得分数
    UsageScore  int64  // 使用中继消耗分数
}

// 提供中继服务获得信誉分
realm.Relay().Serve(ctx)
// 后台自动累计信誉分
```

**方案 2：代币/积分系统**

```go
// Realm 内可以有代币系统
// 提供中继服务获得代币
// 使用中继消耗代币
```

**方案 3：强制轮换**

```go
// 对于大型 Realm，可以强制成员轮换提供中继
// 确保公平分担成本
```

### Q4: 如何防止中继滥用？

**限制措施**：

1. **带宽限制**：每个中继连接限制带宽
2. **时长限制**：中继连接有最大时长
3. **连接数限制**：每个中继节点限制最大连接数
4. **信誉检查**：只接受信誉良好的节点作为中继
5. **速率限制**：限制中继请求的频率

```go
realm.Relay().Serve(ctx,
    dep2p.WithRelayBandwidthLimit(10 * 1024 * 1024), // 10 MB/s
    dep2p.WithRelayMaxConnections(100),              // 最多 100 个连接
    dep2p.WithRelayMaxDuration(24 * time.Hour),     // 最长 24 小时
)
```

---

## 完整使用示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p"
)

func main() {
    ctx := context.Background()
    
    // ═══════════════════════════════════════════════════════════════════
    // 场景 1：成为 Realm 中继节点
    // ═══════════════════════════════════════════════════════════════════
    
    node, _ := dep2p.StartNode(ctx, 
        dep2p.WithPreset(dep2p.PresetServer), // 服务器预设，有公网 IP
    )
    defer node.Close()
    
    realm, _ := node.JoinRealm(ctx, "blockchain-mainnet")
    defer realm.Leave()
    
    // 声明自己愿意提供中继服务
    err := realm.Relay().Serve(ctx,
        dep2p.WithRelayBandwidthLimit(10 * 1024 * 1024), // 10 MB/s
        dep2p.WithRelayMaxConnections(100),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("🔄 已成为 Realm 中继节点")
    
    // ═══════════════════════════════════════════════════════════════════
    // 场景 2：使用 Realm 中继（NAT 后的节点）
    // ═══════════════════════════════════════════════════════════════════
    
    behindNAT, _ := dep2p.StartNode(ctx,
        dep2p.WithPreset(dep2p.PresetMobile), // 移动设备，可能在 NAT 后
    )
    defer behindNAT.Close()
    
    realmNAT, _ := behindNAT.JoinRealm(ctx, "blockchain-mainnet")
    defer realmNAT.Leave()
    
    // 发现 Realm 内的中继节点
    relays, _ := realmNAT.Relay().FindRelays(ctx)
    fmt.Printf("🔍 发现 %d 个 Realm 中继节点\n", len(relays))
    
    if len(relays) > 0 {
        // 在中继上预留槽位，以便其他人能连接到我
        reservation, _ := realmNAT.Relay().Reserve(ctx, relays[0])
        
        // 现在我的 AdvertisedAddrs 会包含中继地址
        fmt.Printf("📍 我的地址（含中继）: %v\n", behindNAT.AdvertisedAddrs())
        
        // 其他 Realm 成员可以通过这个地址连接我
        // 中继会验证对方也是 Realm 成员
        
        defer reservation.Close()
    }
    
    // ═══════════════════════════════════════════════════════════════════
    // 场景 3：连接时自动处理（用户无感知）
    // ═══════════════════════════════════════════════════════════════════
    
    messaging := realmNAT.Messaging()
    
    // 发送消息时，底层自动处理：
    // 1. 尝试直连
    // 2. 尝试打洞（通过 System Relay 协调）
    // 3. 使用 Realm Relay 转发
    err = messaging.Send(ctx, targetPeerID, []byte("hello"))
    if err != nil {
        if errors.Is(err, dep2p.ErrNoRelayAvailable) {
            log.Fatal("无法连接：Realm 内没有可用中继节点")
        }
        log.Fatal(err)
    }
    
    // 用户不需要关心连接是直连还是中继
    // 但可以通过 Stats 查看
    stats := realmNAT.Relay().Stats()
    fmt.Printf("📊 通过中继发送: %d 字节\n", stats.BytesViaRelay)
}
```

---

## 结论

### 最终设计

| 问题 | 解决方案 |
|------|---------|
| Layer 1 中继是否需要？ | **需要**，但只用于系统级通信（DHT/打洞） |
| Layer 2 中继是否需要？ | **必须**，业务消息只走 Realm Relay |
| 中继的成本由谁承担？ | Realm 成员（合理！因为他们受益于这个业务网络） |
| 如何保证隔离？ | Realm Relay 验证双方都是成员才转发 |
| 没有 Realm Relay 怎么办？ | 连接失败，不 fallback 到 System Relay |
| 如何激励节点成为中继？ | 信誉分/代币系统（可选） |

### 核心原则

1. **业务数据永远不会"泄露"到 Realm 外部节点**
2. **中继成本由受益者承担**（Realm 成员）
3. **系统级中继只用于基础设施**（不转发业务数据）
4. **Realm 中继验证成员身份**（保证隔离）

---

## 后续行动

| 行动 | 负责人 | 状态 |
|------|--------|------|
| 更新中继协议文档 | @qinglong | ✅ |
| 更新 Realm 接口定义 | @qinglong | 📋 |
| 创建 ADR | @qinglong | 📋 |
| 实现 RealmRelayService | @dev | 📋 |
| 实现验证流程 | @dev | 📋 |

---

## 关联文档

- → [中继协议规范](../protocols/transport/relay.md)
- → [ADR-0003-relay-first-connect](../adr/0003-relay-first-connect.md)
- → [讨论记录：API 分层设计](./DISC-1227-api-layer-design.md)
- → [架构总览](../architecture/overview.md)
