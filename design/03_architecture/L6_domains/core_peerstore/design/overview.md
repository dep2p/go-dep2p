# 整体设计

> **版本**: v1.2.0  
> **更新日期**: 2026-01-23  
> **状态**: ✅ 已完善

---

## 一、设计目标

### 1.1 核心目标

1. **高性能**: 支持毫秒级地址/密钥/协议查询
2. **并发安全**: 支持多协程并发读写，无数据竞态
3. **自动管理**: 地址 TTL 自动过期，GC 自动清理
4. **扩展性**: 模块化设计，易于扩展持久化存储

### 1.2 非目标

1. ❌ **不支持分布式存储**：仅本地内存存储
2. ❌ **不支持跨进程共享**：单进程内使用
3. ❌ **不持久化数据**：默认内存存储（可扩展）

---

## 二、架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          core_peerstore                                      │
│                         (节点信息统一存储)                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                       Peerstore (主入口)                               │  │
│  │                                                                       │  │
│  │  • 聚合 4 个子簿                                                      │  │
│  │  • 实现 pkg/interfaces/peerstore.go 接口                              │  │
│  │  • Fx 模块集成                                                        │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                        │
│                  ┌─────────────────┼─────────────────┐                      │
│                  │                 │                 │                      │
│                  ↓                 ↓                 ↓                      │
│  ┌─────────────────────┐ ┌─────────────────┐ ┌─────────────────────┐      │
│  │    AddrBook         │ │    KeyBook      │ │    ProtoBook        │      │
│  │                     │ │                 │ │                     │      │
│  │  • 地址存储         │ │  • 公钥存储     │ │  • 协议列表         │      │
│  │  • TTL 管理         │ │  • PeerID 验证  │ │  • 能力查询         │      │
│  │  • GC 清理          │ │  • 自动提取     │ │  • 协议移除         │      │
│  │  • 地址流           │ │                 │ │                     │      │
│  └─────────────────────┘ └─────────────────┘ └─────────────────────┘      │
│                                    │                                        │
│                                    ↓                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                       MetadataStore                                    │  │
│  │                                                                       │  │
│  │  • 键值对存储                                                         │  │
│  │  • 任意类型值                                                         │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 模块职责

| 模块 | 职责 | 存储内容 |
|------|------|----------|
| **Peerstore** | 主入口，聚合子簿 | - |
| **AddrBook** | 地址管理 | `PeerID → [](Multiaddr, TTL, Expiry)` |
| **KeyBook** | 密钥管理 | `PeerID → PubKey` |
| **ProtoBook** | 协议管理 | `PeerID → []ProtocolID` |
| **MetadataStore** | 元数据管理 | `PeerID → map[string]interface{}` |

---

## 三、核心设计

### 3.1 地址簿 (AddrBook) 设计

#### 3.1.1 数据结构

```go
// AddrBook 地址簿
type AddrBook struct {
    mu sync.RWMutex
    
    // addrs 地址映射：PeerID → addresses
    addrs map[types.PeerID]map[string]*expiringAddr
    
    // expiringHeap 过期地址堆（用于 GC）
    expiringHeap expiringAddrHeap
    
    // gcCtx GC 上下文
    gcCtx context.Context
    gcCancel context.CancelFunc
}

// expiringAddr 过期地址
type expiringAddr struct {
    Addr      types.Multiaddr  // 地址
    TTL       time.Duration    // TTL
    Expiry    time.Time        // 过期时间
    PeerID    types.PeerID     // 所属节点
    heapIndex int              // 堆索引
}
```

#### 3.1.2 TTL 常量定义

```go
const (
    // PermanentAddrTTL 永久地址（如引导节点）
    PermanentAddrTTL = math.MaxInt64 - 1
    
    // ConnectedAddrTTL 连接成功的地址
    ConnectedAddrTTL = 30 * time.Minute
    
    // RecentlyConnectedAddrTTL 最近连接的地址
    RecentlyConnectedAddrTTL = 15 * time.Minute
    
    // DiscoveredAddrTTL DHT/Rendezvous 发现的地址
    DiscoveredAddrTTL = 10 * time.Minute
    
    // LocalAddrTTL mDNS 发现的地址
    LocalAddrTTL = 5 * time.Minute
    
    // TempAddrTTL 临时地址
    TempAddrTTL = 2 * time.Minute
)
```

#### ★ 3.1.3 地址来源（多层次）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    地址来源（多层次）                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. STUN/外部探测 — 获取外部地址与 NAT 类型                                 │
│  2. 观察地址交换 — 连接时对端告知的地址                                     │
│  3. 本地缓存 — 最近成功连接过的地址（PeerStore）                            │
│  4. 成员同步 — 通过 Gossip 等协议同步的地址                                │
│  5. ★ Relay 地址簿 — 向 Relay 查询目标地址                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### ★ 3.1.4 地址发现优先级

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    地址发现优先级                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  优先级 1: 本地缓存（PeerStore）                                            │
│  ════════════════════════════════                                           │
│  • 最近连接过的地址，零网络开销                                              │
│  • 需要 TTL 管理，避免使用过期地址                                          │
│                                                                             │
│  优先级 2: 成员列表/Gossip 同步                                             │
│  ═══════════════════════════════                                            │
│  • 通过协议同步的成员地址                                                   │
│  • 相对实时，但可能不完整                                                   │
│                                                                             │
│  优先级 3: DHT 查询（★ 权威来源）                                           │
│  ═══════════════════════════════════════                                    │
│  • 查询 Realm 作用域的 DHT                                                  │
│  • 返回签名 PeerRecord（防投毒、防伪造）                                    │
│  • ★ DHT 是权威目录，其他来源是缓存                                        │
│                                                                             │
│  优先级 4: Relay 地址簿（缓存回退）                                         │
│  ═══════════════════════════════════                                        │
│  • 向 Relay 查询目标地址（作为 DHT 本地缓存）                               │
│  • ★ Relay 地址簿是缓存，不是权威目录                                      │
│  • DHT 失败时作为回退                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### ★ 3.1.5 地址更新触发条件

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    地址更新触发条件                                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  触发条件：                                                                  │
│  ──────────                                                                 │
│  • 网络变化 — 4G/WiFi 切换、IP 变更                                        │
│  • STUN 结果变化 — 公网地址/端口变化                                       │
│  • 观察地址变化 — 对端告知的地址变更                                       │
│  • 本地监听地址变更 — 本地绑定地址更新                                      │
│  • 连接成功 — 更新地址 TTL 并提高优先级                                    │
│                                                                             │
│  地址变化后应该：                                                            │
│  ─────────────────                                                          │
│  1. 更新本地缓存（PeerStore）                                               │
│  2. 通知 Relay（更新地址簿）                                                │
│  3. 广播给相关节点（Realm 成员）                                            │
│  4. 更新 DHT（如适用）                                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 3.1.3 GC 清理机制

```
GC 流程：
┌─────────────────────────────────────────────────────────────────┐
│                         GC 后台任务                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. 每隔 1 分钟触发一次 GC                                       │
│  2. 从 expiringHeap 中取出过期地址                               │
│  3. 删除过期地址                                                 │
│  4. 如果节点没有任何地址，从 addrs map 中删除该节点               │
│                                                                 │
│  ┌───────────────────────────────────────────────────────┐     │
│  │  expiringHeap (最小堆)                                 │     │
│  │                                                       │     │
│  │  [Expiry1] → [Expiry2] → [Expiry3] → ...             │     │
│  │    ↑ 最早过期                                         │     │
│  └───────────────────────────────────────────────────────┘     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

#### 3.1.4 地址流 (AddrStream)

```go
// AddrStream 返回地址更新通道
func (ab *AddrBook) AddrStream(ctx context.Context, peerID types.PeerID) <-chan types.Multiaddr {
    ch := make(chan types.Multiaddr, 16)
    
    go func() {
        defer close(ch)
        
        // 1. 发送当前所有地址
        for _, addr := range ab.Addrs(peerID) {
            select {
            case ch <- addr:
            case <-ctx.Done():
                return
            }
        }
        
        // 2. 监听新地址（通过事件总线）
        // TODO: 集成 core_eventbus
    }()
    
    return ch
}
```

### 3.2 密钥簿 (KeyBook) 设计

#### 3.2.1 数据结构

```go
// KeyBook 密钥簿
type KeyBook struct {
    mu sync.RWMutex
    
    // pubKeys 公钥映射
    pubKeys map[types.PeerID]crypto.PubKey
    
    // privKeys 私钥映射（仅本地节点）
    privKeys map[types.PeerID]crypto.PrivKey
}
```

#### 3.2.2 PeerID 验证

```go
// AddPubKey 添加公钥（带验证）
func (kb *KeyBook) AddPubKey(peerID types.PeerID, pubKey crypto.PubKey) error {
    // 1. 验证 PeerID 是否由 pubKey 派生
    if !peerID.MatchesPublicKey(pubKey) {
        return ErrInvalidPublicKey
    }
    
    kb.mu.Lock()
    defer kb.mu.Unlock()
    
    // 2. 存储公钥
    kb.pubKeys[peerID] = pubKey
    return nil
}
```

#### 3.2.3 自动提取公钥

```go
// PubKey 获取公钥（自动提取）
func (kb *KeyBook) PubKey(peerID types.PeerID) (crypto.PubKey, error) {
    kb.mu.RLock()
    pubKey, ok := kb.pubKeys[peerID]
    kb.mu.RUnlock()
    
    if ok {
        return pubKey, nil
    }
    
    // 尝试从 PeerID 中提取内嵌公钥（Ed25519）
    pubKey, err := peerID.ExtractPublicKey()
    if err != nil {
        return nil, ErrNotFound
    }
    
    // 缓存提取的公钥
    kb.mu.Lock()
    kb.pubKeys[peerID] = pubKey
    kb.mu.Unlock()
    
    return pubKey, nil
}
```

### 3.3 协议簿 (ProtoBook) 设计

#### 3.3.1 数据结构

```go
// ProtoBook 协议簿
type ProtoBook struct {
    mu sync.RWMutex
    
    // protocols 协议映射
    protocols map[types.PeerID][]types.ProtocolID
}
```

#### 3.3.2 协议能力查询

```go
// SupportsProtocols 查询节点支持的协议
func (pb *ProtoBook) SupportsProtocols(peerID types.PeerID, protos ...types.ProtocolID) ([]types.ProtocolID, error) {
    pb.mu.RLock()
    defer pb.mu.RUnlock()
    
    peerProtos, ok := pb.protocols[peerID]
    if !ok {
        return nil, nil
    }
    
    // 查找交集
    var supported []types.ProtocolID
    for _, proto := range protos {
        for _, peerProto := range peerProtos {
            if proto == peerProto {
                supported = append(supported, proto)
                break
            }
        }
    }
    
    return supported, nil
}
```

### 3.4 元数据簿 (MetadataStore) 设计

#### 3.4.1 数据结构

```go
// MetadataStore 元数据存储
type MetadataStore struct {
    mu sync.RWMutex
    
    // data 元数据映射：PeerID → key → value
    data map[types.PeerID]map[string]interface{}
}
```

#### 3.4.2 常用元数据

| 键 | 值类型 | 说明 |
|----|--------|------|
| `agent` | `string` | 客户端版本（如 "dep2p/v1.0.0"） |
| `protocol_version` | `string` | 协议版本 |
| `observed_addr` | `types.Multiaddr` | 观察到的外部地址 |
| `latency` | `time.Duration` | 延迟 |

---

## 四、并发安全设计

### 4.1 锁策略

```
┌─────────────────────────────────────────────────────────────────┐
│                        锁策略                                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. 每个子簿独立 RWMutex                                         │
│     • AddrBook.mu                                               │
│     • KeyBook.mu                                                │
│     • ProtoBook.mu                                              │
│     • MetadataStore.mu                                          │
│                                                                 │
│  2. 读多写少场景，优先使用 RLock                                 │
│                                                                 │
│  3. 避免嵌套锁（防止死锁）                                       │
│                                                                 │
│  4. 锁粒度：按子簿粒度，不按 PeerID 粒度                         │
│     （避免锁竞争，简化实现）                                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 竞态检测

所有测试必须通过 `go test -race`：

```bash
go test -race -v ./internal/core/peerstore/...
```

---

## 五、Fx 模块集成

### 5.1 模块定义

```go
// Module 返回 Fx 模块
func Module() fx.Option {
    return fx.Module("peerstore",
        fx.Provide(
            NewConfig,      // 配置
            NewPeerstore,   // Peerstore 实例
        ),
        fx.Invoke(registerLifecycle), // 生命周期
    )
}

// Config Peerstore 配置
type Config struct {
    // GC 配置
    EnableGC       bool          // 是否启用 GC
    GCInterval     time.Duration // GC 间隔（默认 1 分钟）
    GCLookahead    time.Duration // GC 提前量（默认 10 秒）
}

// 生命周期注册
func registerLifecycle(lc fx.Lifecycle, ps *Peerstore) {
    lc.Append(fx.Hook{
        OnStart: func(ctx context.Context) error {
            // 启动 GC
            ps.StartGC()
            return nil
        },
        OnStop: func(ctx context.Context) error {
            // 停止 GC，关闭存储
            return ps.Close()
        },
    })
}
```

### 5.2 依赖注入

```go
// NewPeerstore 创建 Peerstore
func NewPeerstore(cfg Config) (*Peerstore, error) {
    return &Peerstore{
        addrBook:  addrbook.New(cfg.GCInterval, cfg.GCLookahead),
        keyBook:   keybook.New(),
        protoBook: protobook.New(),
        metadata:  metadata.New(),
    }, nil
}
```

---

## 六、错误处理

### 6.1 错误定义

```go
// errors.go

var (
    // ErrNotFound 节点或数据未找到
    ErrNotFound = errors.New("peer not found")
    
    // ErrInvalidPublicKey 公钥与 PeerID 不匹配
    ErrInvalidPublicKey = errors.New("invalid public key for peer")
    
    // ErrInvalidAddr 无效地址
    ErrInvalidAddr = errors.New("invalid address")
    
    // ErrClosed 存储已关闭
    ErrClosed = errors.New("peerstore closed")
)
```

### 6.2 错误处理原则

1. **查询不存在的节点**：返回空切片，不返回错误
2. **添加无效公钥**：返回 `ErrInvalidPublicKey`
3. **存储关闭后操作**：返回 `ErrClosed`

---

## 七、测试策略

### 7.1 测试文件结构

```
internal/core/peerstore/
├── peerstore_test.go      # 主测试
├── concurrent_test.go     # 并发测试
├── integration_test.go    # 集成测试
├── module_test.go         # Fx 模块测试
├── addrbook/
│   └── addrbook_test.go   # 地址簿测试
├── keybook/
│   └── keybook_test.go    # 密钥簿测试
├── protobook/
│   └── protobook_test.go  # 协议簿测试
└── metadata/
    └── metadata_test.go   # 元数据测试
```

### 7.2 关键测试场景

| 测试场景 | 测试文件 | 说明 |
|---------|---------|------|
| 地址 TTL 过期 | addrbook_test.go | 验证过期地址不返回 |
| GC 清理 | addrbook_test.go | 验证定期清理生效 |
| PeerID 验证 | keybook_test.go | 验证公钥与 PeerID 匹配 |
| 并发读写 | concurrent_test.go | 竞态检测 |
| Fx 模块加载 | integration_test.go | 依赖注入 |

---

## 八、性能优化

### 8.1 内存优化

```
优化策略：
1. 使用 map[string]*expiringAddr 而非 []expiringAddr
   • 避免重复地址
   • O(1) 查找

2. GC 只清理过期地址，不清理 map entry
   • 节点可能有多个地址，只清理过期的

3. 使用 heap 管理过期时间
   • O(log n) 插入
   • O(1) 查找最早过期
```

### 8.2 查询优化

```
优化策略：
1. 读操作使用 RLock（允许并发读）

2. 地址查询时过滤过期地址
   • 在返回前检查 Expiry

3. 公钥自动提取缓存
   • 提取后缓存，避免重复提取
```

---

## 九、与 go-libp2p 对比

| 特性 | go-libp2p | DeP2P | 说明 |
|------|-----------|-------|------|
| **地址 TTL** | ✅ | ✅ | 相同常量定义 |
| **GC 清理** | ✅ | ✅ | 基于 heap |
| **公钥验证** | ✅ | ✅ | MatchesPublicKey |
| **协议能力查询** | ✅ | ✅ | SupportsProtocols |
| **地址流** | ✅ | ✅ | AddrStream |
| **Metrics** | ✅ | ❌ | DeP2P 简化实现 |
| **持久化存储** | ✅ (pstoreds) | ❌ | DeP2P 仅内存存储 |
| **依赖** | libp2p 包 | **独立实现** | DeP2P 不依赖 libp2p |

---

## 十、扩展性设计

### 10.1 预留接口

```go
// 预留持久化接口（暂未实现）
type PeerstoreBackend interface {
    // Load 加载数据
    Load() error
    
    // Save 保存数据
    Save() error
    
    // Close 关闭后端
    Close() error
}
```

### 10.2 Metrics 集成

```go
// 预留 Metrics 接口（可选）
type PeerstoreMetrics interface {
    // RecordLatency 记录延迟
    RecordLatency(peer types.PeerID, latency time.Duration)
    
    // LatencyEWMA 查询平均延迟
    LatencyEWMA(peer types.PeerID) time.Duration
}
```

---

## 十一、相关文档

| 文档 | 路径 | 说明 |
|------|------|------|
| 需求文档 | [requirements.md](../requirements/requirements.md) | 功能需求 |
| 编码指南 | [guidelines.md](../coding/guidelines.md) | 编码规范 |
| 测试策略 | [strategy.md](../testing/strategy.md) | 测试策略 |
| 接口定义 | [pkg/interfaces/peerstore.go](../../../../pkg/interfaces/peerstore.go) | 接口契约 |

---

**最后更新**：2026-01-23
