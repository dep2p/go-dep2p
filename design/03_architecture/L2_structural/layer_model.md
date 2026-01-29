# 五层软件架构 (Layer Model)

> **版本**: v1.3.0  
> **更新日期**: 2026-01-19  
> **定位**: DeP2P 五层软件架构详解

---

## 架构概述

DeP2P 采用**五层软件架构**，Realm 作为独立架构层突出其核心创新地位：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      DeP2P 五层软件架构                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  1. API Layer (入口层)                                                 │  │
│  │     dep2p.go, node.go, options.go                                     │  │
│  │     用户入口门面，配置选项                                              │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                        ↓                                    │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  2. Protocol Layer (协议层)                                            │  │
│  │     internal/protocol/                                                 │  │
│  │     用户级应用协议：Messaging / PubSub / Streams / Liveness            │  │
│  │     协议前缀：/dep2p/app/<realmID>/*                                   │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                        ↓                                    │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  3. Realm Layer (Realm 层) — DeP2P 核心创新                            │  │
│  │     internal/realm/                                                    │  │
│  │     业务隔离，成员管理：Manager / Auth / Member                        │  │
│  │     协议前缀：/dep2p/realm/<realmID>/*                                 │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                        ↓                                    │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  4. Core Layer (核心层)                                                │  │
│  │     internal/core/                                                     │  │
│  │     P2P 网络核心能力：Host / Identity / Transport / Security / ...    │  │
│  │     协议前缀：/dep2p/sys/*                                             │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│                                  ↕ 双向协作                                  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  5. Discovery Layer (发现层)                                           │  │
│  │     internal/discovery/                                                │  │
│  │     节点发现服务：Coordinator / DHT / Bootstrap / mDNS / Rendezvous    │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  依赖方向：API → Protocol → Realm → Core ↔ Discovery                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 层详细说明

### API Layer (入口层)

用户的第一个接触点，提供节点创建和配置接口。

```
┌─────────────────────────────────────────────────────────────────────────┐
│  API Layer (入口层)                                                      │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  职责：                                                                  │
│  • 提供用户入口门面                                                      │
│  • 定义配置选项                                                          │
│  • 隐藏内部实现复杂性                                                    │
│                                                                         │
│  核心组件：                                                              │
│  • Node              - 节点入口门面                                     │
│  • Options           - 配置选项                                         │
│                                                                         │
│  代码位置：dep2p.go, node.go, options.go                                │
│                                                                         │
│  用户感知：★★★★★ 完全可见                                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

| 组件 | 职责 | 代码位置 |
|------|------|----------|
| **Node** | 节点入口门面 | `dep2p.go`, `node.go` |
| **Options** | 配置选项 | `options.go` |

### Protocol Layer (协议层)

用户级应用协议，必须先加入 Realm 才能使用。

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Protocol Layer (协议层)                                                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  职责：                                                                  │
│  • 提供用户级应用协议                                                    │
│  • 实现消息传递、发布订阅、流式通信                                      │
│                                                                         │
│  核心组件：                                                              │
│  • Messaging         - 点对点消息                                       │
│  • PubSub            - 发布订阅 (GossipSub)                             │
│  • Streams           - 双向流                                           │
│  • Liveness          - 存活检测                                         │
│                                                                         │
│  协议前缀：/dep2p/app/<realmID>/*                                        │
│  代码位置：internal/protocol/                                            │
│                                                                         │
│  约束：必须先 JoinRealm，否则返回 ErrNotMember (INV-002)                 │
│                                                                         │
│  用户感知：★★★★★ 完全可见                                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

| 组件 | 职责 | 代码位置 |
|------|------|----------|
| **Messaging** | 点对点消息 | `internal/protocol/messaging/` |
| **PubSub** | 发布订阅 (GossipSub) | `internal/protocol/pubsub/` |
| **Streams** | 双向流 | `internal/protocol/streams/` |
| **Liveness** | 存活检测 | `internal/protocol/liveness/` |

### Realm Layer (Realm 层)

DeP2P 的核心创新，提供业务隔离能力。

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Realm Layer (Realm 层) — DeP2P 核心创新                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  职责：                                                                  │
│  • 提供业务隔离能力                                                      │
│  • 管理成员资格                                                          │
│  • PSK 准入控制                                                          │
│  • ★ 支持 Realm 内"仅 ID 连接"                                          │
│                                                                         │
│  核心组件：                                                              │
│  • Realm             - 业务隔离域                                       │
│  • Manager           - Realm 生命周期管理                               │
│  • Auth (PSK)        - 准入控制                                         │
│  • Member            - 成员管理                                         │
│                                                                         │
│  协议前缀：/dep2p/realm/<realmID>/*                                      │
│  代码位置：internal/realm/                                               │
│                                                                         │
│  设计原则：                                                              │
│  • 一个 Node 同时只属于一个 Realm                                        │
│  • RealmID 自动植入协议路径，实现协议级隔离                              │
│  • ★ Realm 是"仅 ID 连接"的边界：Realm 内允许，跨 Realm 禁止            │
│                                                                         │
│  用户感知：★★★☆☆ 显式操作                                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

| 组件 | 职责 | 代码位置 |
|------|------|----------|
| **Realm** | 业务隔离域 | `internal/realm/` |
| **Manager** | Realm 生命周期管理 | `internal/realm/manager/` |
| **Auth** | PSK 成员认证 | `internal/realm/auth/` |
| **Member** | 成员管理 | `internal/realm/member/` |

### Core Layer (核心层)

P2P 网络的核心能力，对用户透明。

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Core Layer (核心层)                                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  职责：                                                                  │
│  • 提供 P2P 网络的核心能力                                               │
│  • 管理传输、安全、身份、穿透、中继                                      │
│  • ★ 提供统一的持久化存储基础设施                                        │
│                                                                         │
│  核心组件：                                                              │
│  • Host              - 网络主机                                         │
│  • Identity          - 身份管理、NodeID                                 │
│  • Transport         - QUIC/TCP 传输                                    │
│  • Security          - TLS/Noise 安全                                   │
│  • Muxer             - 多路复用 (Yamux)                                 │
│  • ConnMgr           - 连接管理                                         │
│  • ★ Relay           - 统一中继（三大职责 v2.0）                         │
│  • NAT               - NAT 穿透 (外部地址发现/打洞)                      │
│  • ★ Storage         - 存储引擎（BadgerDB）                              │
│                                                                         │
│  ★ v2.0 三层架构（DHT 权威模型）：                                        │
│  • Layer 1: DHT（★ 权威目录）— 存储签名 PeerRecord                       │
│  • Layer 2: 缓存加速层 — Peerstore / MemberList / Relay 地址簿           │
│  • Layer 3: 连接策略 — 直连 → 打洞 → Relay 兜底                          │
│                                                                         │
│  Relay 三大职责 (v2.0)：                                                  │
│  • 缓存加速层：维护地址簿，作为 DHT 本地缓存（非权威）                   │
│  • 打洞协调信令：提供打洞协调的信令通道                                  │
│  • 数据通信保底：直连/打洞失败时转发数据                                 │
│                                                                         │
│  协议前缀：/dep2p/sys/*                                                  │
│  代码位置：internal/core/                                                │
│                                                                         │
│  用户感知：★☆☆☆☆ 内部透明                                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

| 组件 | 职责 | 代码位置 |
|------|------|----------|
| **Host** | 网络主机 | `internal/core/host/` |
| **Identity** | 身份管理、NodeID | `internal/core/identity/` |
| **★ DeviceIdentity** | 设备身份与证书管理 | `internal/core/identity/device.go` |
| **Transport** | QUIC/TCP 传输 | `internal/core/transport/` |
| **Security** | TLS/Noise 安全 | `internal/core/security/` |
| **Muxer** | 多路复用 | `internal/core/muxer/` |
| **ConnMgr** | 连接管理 | `internal/core/connmgr/` |
| **★ Relay** | 统一中继（三大职责 v2.0: 缓存加速+信令+保底） | `internal/core/relay/` |
| **★ AddressBook** | 地址簿（缓存加速层，非权威目录） | `internal/core/relay/addressbook/` |
| **NAT** | NAT 穿透（外部地址发现 + 打洞） | `internal/core/nat/` |
| **Reachability** | 可达性协调 | `internal/core/reachability/` |
| **★ DirectAddrState** | 直连地址更新状态机 | `internal/core/reachability/direct_state.go` |
| **★ Storage** | 存储引擎（BadgerDB） | `internal/core/storage/` |

### Discovery Layer (发现层)

节点发现与广播，与 Core Layer 双向协作。

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Discovery Layer (发现层)                                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  职责：                                                                  │
│  • 提供节点发现与广播能力                                                │
│  • 管理 DHT、引导节点、局域网、命名空间发现                              │
│                                                                         │
│  核心组件：                                                              │
│  • Coordinator       - 发现协调（统一调度）                              │
│  • DHT               - Kademlia 分布式哈希表                            │
│  • Bootstrap         - 引导节点发现                                     │
│  • mDNS              - 局域网发现                                       │
│  • Rendezvous        - 命名空间发现                                     │
│                                                                         │
│  代码位置：internal/discovery/                                           │
│                                                                         │
│  特点：与 Core Layer 双向协作                                            │
│                                                                         │
│  用户感知：★☆☆☆☆ 内部透明                                              │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

| 组件 | 职责 | 代码位置 |
|------|------|----------|
| **Coordinator** | 发现协调（统一调度） | `internal/discovery/coordinator/` |
| **★ PeerFinder** | 多级缓存节点查找 | `internal/discovery/coordinator/finder.go` |
| **★ AddressAnnouncer** | 地址定期刷新公告 | `internal/discovery/coordinator/announcer.go` |
| **DHT** | Kademlia DHT | `internal/discovery/dht/` |
| **Bootstrap** | 引导节点发现 | `internal/discovery/bootstrap/` |
| **mDNS** | 局域网发现 | `internal/discovery/mdns/` |
| **Rendezvous** | 命名空间发现 | `internal/discovery/rendezvous/` |

---

## 公共包 (Pkg)

公共包，可被外部项目导入。

| 组件 | 职责 | 代码位置 |
|------|------|----------|
| **Interfaces** | 模块间接口定义 | `pkg/interfaces/` |
| **Types** | 公共类型 | `pkg/types/` |
| **Proto** | Protobuf 定义 | `pkg/proto/` |

**约束**：不能依赖 internal，稳定性最高。

---

## 依赖规则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          五层架构依赖规则                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  规则 1: 层间向下依赖                                                        │
│  ───────────────────────────                                                │
│  API → Protocol → Realm → Core ↔ Discovery                                  │
│  上层依赖下层，Core 与 Discovery 双向协作                                    │
│                                                                             │
│  规则 2: Package 无内部依赖                                                  │
│  ─────────────────────────────                                              │
│  Pkg 不能依赖 internal                                                       │
│  确保外部可安全导入                                                           │
│                                                                             │
│  规则 3: 同层通过接口                                                         │
│  ──────────────────────                                                     │
│  同层组件通过 pkg/interfaces 交互                                             │
│  不直接依赖实现                                                              │
│                                                                             │
│  规则 4: 日志/指标直接使用                                                    │
│  ────────────────────────────                                               │
│  日志使用标准库 log/slog                                                     │
│  指标使用 prometheus                                                         │
│  不抽象接口，直接调用                                                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 协议命名空间与架构层映射

| 协议前缀 | 架构层 | 说明 |
|----------|--------|------|
| `/dep2p/sys/*` | Core Layer | 系统协议（DHT、Relay、打洞等） |
| `/dep2p/realm/<realmID>/*` | Realm Layer | Realm 控制协议（认证、成员同步） |
| `/dep2p/app/<realmID>/*` | Protocol Layer | 应用协议（消息、发布订阅等） |

---

## 用户感知与软件架构对照

| 用户感知层 | 软件架构层 | 用户操作 |
|-----------|-----------|----------|
| **应用服务** | Protocol Layer | `realm.Messaging().Send()` |
| **业务隔离** | Realm Layer | `node.JoinRealm()` |
| **系统基础** | API Layer + Core Layer + Discovery Layer | `dep2p.New()` |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [module_design.md](module_design.md) | 模块划分 |
| [dependency_rules.md](dependency_rules.md) | 依赖规则详解 |
| [target_structure.md](target_structure.md) | 目标目录结构 |
| [../L1_overview/abstractions.md](../L1_overview/abstractions.md) | 核心抽象 |
| [../L3_behavioral/state_machines.md](../L3_behavioral/state_machines.md) | 状态机定义 |
| [../L4_interfaces/fx_lifecycle.md](../L4_interfaces/fx_lifecycle.md) | Fx 生命周期管理 |

---

**最后更新**：2026-01-24（v2.0 DHT 权威模型对齐）
