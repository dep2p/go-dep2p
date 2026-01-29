# Core PeerStore 模块

> **版本**: v1.3.0  
> **更新日期**: 2026-01-23  
> **定位**: 节点信息存储（Core Layer）

---

## 模块概述

core_peerstore 是节点信息的统一存储组件，管理已知节点的地址、公钥、协议支持和元数据。它是节点发现和连接建立的基础设施。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/peerstore/` |
| **Fx 模块** | `fx.Module("peerstore")` |
| **状态** | ✅ 已实现 |
| **依赖** | identity |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       core_peerstore 职责                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 地址簿 (AddrBook)                                                        │
│     • 存储节点的多地址                                                      │
│     • 地址 TTL 管理                                                         │
│     • 地址优先级排序                                                        │
│     • 过期地址 GC                                                           │
│                                                                             │
│  2. 密钥簿 (KeyBook)                                                         │
│     • 存储节点公钥                                                          │
│     • PeerID ↔ PublicKey 映射                                              │
│     • 密钥验证                                                              │
│                                                                             │
│  3. 协议簿 (ProtoBook)                                                       │
│     • 存储节点支持的协议                                                    │
│     • 协议能力查询                                                          │
│                                                                             │
│  4. 元数据簿 (MetadataBook)                                                  │
│     • 存储节点元数据 (Agent, ProtocolVersion)                               │
│     • 自定义键值对                                                          │
│                                                                             │
│  5. 持久化 (v1.1.0+ 强制)                                                    │
│     • Fx 模块自动配置 BadgerDB 持久化存储                                   │
│     • 通过存储引擎实现数据持久化                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 架构设计

### 子组件结构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           PeerStore                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │    AddrBook     │  │    KeyBook      │  │   ProtoBook     │             │
│  │                 │  │                 │  │                 │             │
│  │ PeerID → Addrs  │  │ PeerID → PubKey │  │ PeerID → Protos │             │
│  │ TTL 管理        │  │ 密钥验证        │  │ 协议能力        │             │
│  │ GC 清理         │  │                 │  │                 │             │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘             │
│                                                                             │
│  ┌─────────────────┐  ┌─────────────────────────────────────┐             │
│  │  MetadataBook   │  │             Store Backend           │             │
│  │                 │  │                                     │             │
│  │ PeerID → KV     │  │  Memory (default)  │  Persistent    │             │
│  │ 自定义元数据    │  │                                     │             │
│  └─────────────────┘  └─────────────────────────────────────┘             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 地址 TTL 管理

```
地址类型与 TTL：

  ┌─────────────────────────┬───────────────┬─────────────────────────┐
  │       地址来源          │     TTL       │         说明            │
  ├─────────────────────────┼───────────────┼─────────────────────────┤
  │  直连成功               │   30 分钟     │  ConnectedAddrTTL       │
  │  DHT 发现               │   10 分钟     │  DiscoveredAddrTTL      │
  │  Rendezvous 发现        │   10 分钟     │  DiscoveredAddrTTL      │
  │  mDNS 发现              │    5 分钟     │  LocalAddrTTL           │
  │  永久地址（配置）       │   永不过期    │  PermanentAddrTTL       │
  └─────────────────────────┴───────────────┴─────────────────────────┘
```

### ★ 候选地址 vs 已验证地址（v1.3.0 新增）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    地址状态分层（★ 关键）                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  地址状态标签（AddressState）：                                              │
│  ═════════════════════════════                                              │
│                                                                             │
│  1. Candidate（候选地址）                                                   │
│     ─────────────────────                                                   │
│     • 来源：STUN、UPnP、Observed Address                                    │
│     • 状态：未验证，可能不可达                                              │
│     • TTL：3 分钟（CandidateAddrTTL）                                       │
│     • 用途：打洞尝试、Reachability 验证输入                                 │
│     • ⚠️ 不能用于 DHT 发布或对外公告                                       │
│                                                                             │
│  2. Verified（已验证地址）                                                  │
│     ───────────────────────                                                 │
│     • 来源：Reachability/AutoNAT dialback 验证通过                          │
│     • 状态：已验证，确认可达                                                │
│     • TTL：30 分钟（VerifiedAddrTTL）                                       │
│     • 用途：DHT 发布、Relay 地址簿注册、对外公告                            │
│     • ✅ 可以对外公布                                                       │
│                                                                             │
│  3. Connected（已连接地址）                                                 │
│     ─────────────────────                                                   │
│     • 来源：直连成功的地址                                                  │
│     • 状态：实际已使用                                                      │
│     • TTL：30 分钟（ConnectedAddrTTL）                                      │
│     • 用途：优先连接地址                                                    │
│                                                                             │
│  状态转换：                                                                  │
│  ──────────                                                                 │
│  Candidate ──(验证通过)──▶ Verified ──(连接成功)──▶ Connected              │
│      │                        │                                             │
│      │(过期/失败)             │(过期/失效)                                  │
│      ▼                        ▼                                             │
│   [清除]                   [降级为 Candidate]                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### ★ Observed Address 使用警告

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Observed Address 使用注意                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ⚠️ Observed Address 仅为候选地址，不能直接作为可达地址使用！               │
│                                                                             │
│  来源：                                                                      │
│  • Identify 协议中对端告知的地址                                            │
│  • STUN 探测返回的地址                                                      │
│  • 其他节点告知的"我看到的你的地址"                                         │
│                                                                             │
│  风险：                                                                      │
│  • 对端可能告知错误地址（恶意或错误）                                       │
│  • NAT 映射可能已过期                                                       │
│  • 可能是私网地址（对端与本节点在同一 NAT 后）                              │
│                                                                             │
│  正确使用：                                                                  │
│  1. 存入 PeerStore 时标记为 Candidate                                       │
│  2. 多源验证（至少 2 个不同节点报告相同地址）                               │
│  3. 或通过 Reachability/AutoNAT 验证后才能升级为 Verified                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

```go
// pkg/interfaces/peerstore.go

// PeerStore 节点存储接口
type PeerStore interface {
    // 地址管理
    AddAddrs(peer types.PeerID, addrs []types.Multiaddr, ttl time.Duration)
    SetAddrs(peer types.PeerID, addrs []types.Multiaddr, ttl time.Duration)
    Addrs(peer types.PeerID) []types.Multiaddr
    ClearAddrs(peer types.PeerID)
    AddrStream(ctx context.Context, peer types.PeerID) <-chan types.Multiaddr
    
    // 密钥管理
    PubKey(peer types.PeerID) (crypto.PubKey, error)
    AddPubKey(peer types.PeerID, key crypto.PubKey) error
    
    // 协议管理
    GetProtocols(peer types.PeerID) ([]types.ProtocolID, error)
    AddProtocols(peer types.PeerID, protos ...types.ProtocolID) error
    SetProtocols(peer types.PeerID, protos ...types.ProtocolID) error
    SupportsProtocols(peer types.PeerID, protos ...types.ProtocolID) ([]types.ProtocolID, error)
    
    // 元数据管理
    Get(peer types.PeerID, key string) (interface{}, error)
    Put(peer types.PeerID, key string, val interface{}) error
    
    // 节点查询
    Peers() []types.PeerID
    PeerInfo(peer types.PeerID) types.PeerInfo
    
    // 生命周期
    Close() error
}
```

---

## 参考实现

### go-libp2p PeerStore

```
github.com/libp2p/go-libp2p/core/peerstore/
├── peerstore.go          # 接口定义
└── helpers.go            # 辅助函数

github.com/libp2p/go-libp2p-peerstore/
├── pstoremem/            # 内存实现
│   └── peerstore.go
└── pstoreds/             # 持久化实现
    └── peerstore.go
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_identity](../core_identity/) | 身份管理 |
| [core_swarm](../core_swarm/) | 连接群管理 |
| [core_reachability](../core_reachability/) | 地址可达性验证 |
| [core_nat](../core_nat/) | NAT 穿透（候选地址来源） |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-23
