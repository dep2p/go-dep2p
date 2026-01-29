# L6: 模块设计 (Domain Modules)

> **版本**: v1.3.0  
> **更新日期**: 2026-01-25  
> **定位**: 模块开发者视角：单个组件的内部架构、接口、配置、实现状态

---

## L3/L6 边界说明

L6_domains 与 [L3_behavioral](../L3_behavioral/) 互补，共同构成完整的设计文档体系。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    L3 vs L6 职责边界                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  L3_behavioral（行为设计）                                                   │
│  ════════════════════════                                                   │
│  职责: 描述"谁在什么时候做什么"                                              │
│  内容: 流程时序、组件协作、决策逻辑、状态流转                               │
│  视角: 系统行为、跨组件交互                                                 │
│  示例: "冷启动时先 STUN 再发布 DHT"、"连接优先级：直连→打洞→Relay"          │
│                                                                             │
│  L6_domains（模块设计）                                                      │
│  ═══════════════════════                                                    │
│  职责: 描述"内部怎么做"                                                     │
│  内容: 接口定义、数据结构、API 签名、配置参数                               │
│  视角: 组件内部实现、可扩展性                                               │
│  示例: "Bootstrap 接口定义"、"PeerRecord 数据结构"、"AddressBook API"       │
│                                                                             │
│  引用关系：                                                                  │
│  ──────────                                                                 │
│  • L3 引用 L6 获取具体接口和数据结构                                        │
│  • L6 引用 L3 了解组件在系统中的行为                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### L3/L6 对照表

| L3 行为文档 | 对应 L6 模块文档 |
|------------|-----------------|
| [lifecycle_overview.md](../L3_behavioral/lifecycle_overview.md) | Node ReadyLevel、DHT 架构（v2.0） → [discovery_coordinator](discovery_coordinator/) |
| [discovery_flow.md](../L3_behavioral/discovery_flow.md) | Bootstrap/DHT 行为 → [discovery_coordinator](discovery_coordinator/)、[discovery_dht](discovery_dht/) |
| [relay_flow.md](../L3_behavioral/relay_flow.md) | Relay 三大职责 → [core_relay](core_relay/) |
| [connection_flow.md](../L3_behavioral/connection_flow.md) | 连接管理 → [core_swarm](core_swarm/)、[core_connmgr](core_connmgr/) |
| [realm_flow.md](../L3_behavioral/realm_flow.md) | Realm 管理 → [realm](realm/) |

---

## 目录结构

```
L6_domains/
├── README.md                  # 本文件
│
├── api_node/                  # API Layer (入口层)
│   └── api_node/              # Node 入口
│
├── protocol_*/                # Protocol Layer (协议层)
│   ├── protocol_messaging/    # 消息传递
│   ├── protocol_pubsub/       # 发布订阅
│   ├── protocol_streams/      # 双向流
│   └── protocol_liveness/     # 存活检测
│
├── realm/                     # Realm Layer (Realm 层)
│   └── realm/                 # Realm 管理（★ 含连接器、地址簿）
│
├── core_*/                    # Core Layer (核心层)
│   │
│   │  ── Services ──
│   ├── core_host/             # 网络主机
│   ├── core_swarm/            # 连接群管理 ★ 新增
│   ├── core_peerstore/        # 节点存储 ★ 新增
│   ├── core_eventbus/         # 事件总线 ★ 新增
│   ├── core_resourcemgr/      # 资源管理 ★ 新增
│   ├── core_metrics/          # 监控指标 ★ 新增
│   ├── core_storage/          # 存储服务（BadgerDB）★ 新增
│   │
│   │  ── Network Stack ──
│   ├── core_connmgr/          # 连接管理
│   ├── core_upgrader/         # 连接升级 ★ 新增
│   │
│   │  ── Pluggable Components ──
│   ├── core_transport/        # 传输层 (QUIC/TCP)
│   ├── core_security/         # 安全层 (TLS/Noise)
│   ├── core_muxer/            # 多路复用
│   ├── core_identity/         # 身份管理
│   │
│   │  ── Protocols & Services ──
│   ├── core_protocol/         # 协议注册与路由 ★ 新增
│   ├── core_relay/            # 中继服务
│   └── core_nat/              # NAT 穿透
│
├── discovery_*/               # Discovery Layer (发现层)
│   ├── discovery_coordinator/ # 发现协调器
│   ├── discovery_dht/         # DHT 发现
│   ├── discovery_bootstrap/   # 引导节点发现
│   ├── discovery_mdns/        # 局域网发现
│   ├── discovery_rendezvous/  # 命名空间发现
│   └── discovery_dns/         # DNS 发现 ★ 新增
│
└── pkg_types/                 # 公共类型
```

---

## 五层软件架构对应

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    L6 模块与五层架构对应                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  API Layer (入口层)                                                          │
│  └── api_node               → 根目录 (dep2p.go, node.go)                    │
│                                                                             │
│  Protocol Layer (协议层)                                                     │
│  ├── protocol_messaging     → internal/protocol/messaging/                  │
│  ├── protocol_pubsub        → internal/protocol/pubsub/                     │
│  ├── protocol_streams       → internal/protocol/streams/                    │
│  └── protocol_liveness      → internal/protocol/liveness/                   │
│                                                                             │
│  Realm Layer (Realm 层)                                                      │
│  └── realm                  → internal/realm/                               │
│      └── connector          → internal/realm/connector/     ★ 仅 ID 连接    │
│                                                                             │
│  Note: AddressBook 实现位于 internal/core/relay/addressbook/                 │
│                                                                             │
│  Core Layer (核心层)                                                         │
│  │                                                                          │
│  │  Services:                                                               │
│  ├── core_host              → internal/core/host/                           │
│  ├── core_swarm             → internal/core/swarm/           ★ 新增        │
│  ├── core_peerstore         → internal/core/peerstore/       ★ 新增        │
│  ├── core_eventbus          → internal/core/eventbus/        ★ 新增        │
│  ├── core_resourcemgr       → internal/core/resourcemgr/     ★ 新增        │
│  ├── core_metrics           → internal/core/metrics/         ★ 新增        │
│  ├── core_storage           → internal/core/storage/         ★ 新增        │
│  │                                                                          │
│  │  Network Stack:                                                          │
│  ├── core_connmgr           → internal/core/connmgr/                        │
│  ├── core_upgrader          → internal/core/upgrader/        ★ 新增        │
│  │                                                                          │
│  │  Pluggable Components:                                                   │
│  ├── core_transport         → internal/core/transport/                      │
│  ├── core_security          → internal/core/security/                       │
│  ├── core_muxer             → internal/core/muxer/                          │
│  ├── core_identity          → internal/core/identity/                       │
│  │                                                                          │
│  │  Protocols & Services:                                                   │
│  ├── core_protocol          → internal/core/protocol/        ★ 新增        │
│  ├── core_relay             → internal/core/relay/                          │
│  └── core_nat               → internal/core/nat/                            │
│                                                                             │
│  Discovery Layer (发现层)                                                    │
│  ├── discovery_coordinator  → internal/discovery/coordinator/               │
│  ├── discovery_dht          → internal/discovery/dht/                       │
│  ├── discovery_bootstrap    → internal/discovery/bootstrap/                 │
│  ├── discovery_mdns         → internal/discovery/mdns/                      │
│  ├── discovery_rendezvous   → internal/discovery/rendezvous/                │
│  └── discovery_dns          → internal/discovery/dns/        ★ 新增        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 模块清单与实现状态

### 状态说明

| 状态 | 符号 | 说明 |
|------|------|------|
| 已实现 | ✅ | 代码已完成并测试 |
| 进行中 | 🚧 | 正在开发 |
| 规划中 | 📋 | 已设计但未开始 |
| 待规划 | ⏳ | 新架构规划，目录待创建 |

### API Layer (入口层)

| 模块 | 目标位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [api_node](api_node/) | 根目录 | ✅ | ✅ | Node 入口门面 |

### Protocol Layer (协议层)

面向应用开发者的业务通信协议。

| 模块 | 目标位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [protocol_messaging](protocol_messaging/) | `internal/protocol/messaging/` | ✅ | ✅ | 消息传递 |
| [protocol_pubsub](protocol_pubsub/) | `internal/protocol/pubsub/` | ✅ | ✅ | 发布订阅 |
| [protocol_streams](protocol_streams/) | `internal/protocol/streams/` | ✅ | ✅ | 双向流 |
| [protocol_liveness](protocol_liveness/) | `internal/protocol/liveness/` | ✅ | ✅ | 存活检测 |

### Realm Layer (Realm 层)

业务隔离核心能力。

| 模块 | 目标位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [realm](realm/) | `internal/realm/` | ✅ | ✅ | Realm 管理 |
| ★ connector | `internal/realm/connector/` | ✅ | 📋 | "仅 ID 连接"支持 |
| ★ addressbook | `internal/core/relay/addressbook/` | ✅ | ✅ | 成员地址簿（已实现） |

### Core Layer (核心层)

P2P 网络核心能力。

#### Services (核心服务)

| 模块 | 目标位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [core_host](core_host/) | `internal/core/host/` | ✅ | ✅ | 网络主机 |
| [core_swarm](core_swarm/) | `internal/core/swarm/` | ✅ | ✅ | 连接群管理 ★ |
| [core_peerstore](core_peerstore/) | `internal/core/peerstore/` | ✅ | ✅ | 节点存储 ★ |
| [core_eventbus](core_eventbus/) | `internal/core/eventbus/` | ✅ | ✅ | 事件总线 ★ |
| [core_resourcemgr](core_resourcemgr/) | `internal/core/resourcemgr/` | ✅ | ✅ | 资源管理 ★ |
| [core_metrics](core_metrics/) | `internal/core/metrics/` | ✅ | ✅ | 监控指标 ★ |
| [core_storage](core_storage/) | `internal/core/storage/` | ✅ | 📋 | 存储引擎（BadgerDB）★ |

#### Network Stack (网络栈)

| 模块 | 目标位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [core_connmgr](core_connmgr/) | `internal/core/connmgr/` | ✅ | ✅ | 连接管理 |
| [core_upgrader](core_upgrader/) | `internal/core/upgrader/` | ✅ | ✅ | 连接升级 ★ |

#### Pluggable Components (可插拔组件)

| 模块 | 目标位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [core_transport](core_transport/) | `internal/core/transport/` | ✅ | ✅ | QUIC/TCP |
| [core_security](core_security/) | `internal/core/security/` | ✅ | ✅ | TLS/Noise |
| [core_muxer](core_muxer/) | `internal/core/muxer/` | ✅ | ✅ | 多路复用 |
| [core_identity](core_identity/) | `internal/core/identity/` | ✅ | ✅ | 身份管理 |

#### Protocols & Services (协议与服务)

| 模块 | 目标位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [core_protocol](core_protocol/) | `internal/core/protocol/` | ✅ | ✅ | 协议注册与路由 ★ |
| [core_relay](core_relay/) | `internal/core/relay/` | ✅ | ✅ | 中继服务 |
| [core_nat](core_nat/) | `internal/core/nat/` | ✅ | ✅ | NAT 穿透 |

### Discovery Layer (发现层)

节点发现与广播，与 Core 层双向协作。

| 模块 | 目标位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [discovery_coordinator](discovery_coordinator/) | `internal/discovery/` | ✅ | ✅ | 发现协调器 |
| [discovery_dht](discovery_dht/) | `internal/discovery/dht/` | ✅ | ✅ | DHT 发现 |
| [discovery_bootstrap](discovery_bootstrap/) | `internal/discovery/bootstrap/` | ✅ | ✅ | 引导节点 |
| [discovery_mdns](discovery_mdns/) | `internal/discovery/mdns/` | ✅ | ✅ | 局域网 |
| [discovery_rendezvous](discovery_rendezvous/) | `internal/discovery/rendezvous/` | ✅ | ✅ | 命名空间 |
| [discovery_dns](discovery_dns/) | `internal/discovery/dns/` | ✅ | 📋 | DNS-SD ★ |

### 公共包 (pkg_*)

| 模块 | 代码位置 | 文档 | 实现 | 说明 |
|------|----------|:----:|:----:|------|
| [pkg_types](pkg_types/) | `pkg/types/` | ✅ | ✅ | 公共类型定义 |

---

## 实现状态概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        实现状态概览 (v1.1.0)                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  API Layer (入口层)                                                          │
│  └── Node                ✅  (用户入口门面)                                  │
│                                                                             │
│  Protocol Layer (协议层)                                                     │
│  ├── Messaging           ✅  (请求/响应)                                     │
│  ├── PubSub              ✅  (GossipSub)                                     │
│  ├── Streams             ✅  (双向流)                                        │
│  └── Liveness            ✅  (存活检测)                                      │
│                                                                             │
│  Realm Layer (Realm 层)                                                      │
│  ├── Realm               ✅  (Manager + Auth + Member + Gateway)             │
│  ├── Connector           📋  ("仅 ID 连接"支持)             ★ 新增文档      │
│  └── AddressBook         📋  (成员地址簿)                   ★ 新增文档      │
│                                                                             │
│  Core Layer (核心层)                                                         │
│  │                                                                          │
│  │  Services:                                                               │
│  ├── Host                ✅  (节点聚合)                                      │
│  ├── Swarm               ✅  (连接池、拨号、流管理)         ★ 新增文档      │
│  ├── PeerStore           ✅  (地址簿、密钥簿、协议簿)       ★ 新增文档      │
│  ├── EventBus            ✅  (事件发布订阅)                 ★ 新增文档      │
│  ├── ResourceMgr         ✅  (资源限制、配额管理)           ★ 新增文档      │
│  ├── Metrics             ✅  (Prometheus、带宽统计)         ★ 新增文档      │
│  ├── Storage             📋  (BadgerDB 持久化引擎)          ★ 新增文档      │
│  │                                                                          │
│  │  Network Stack:                                                          │
│  ├── ConnMgr             ✅  (连接生命周期、GC)                              │
│  ├── Upgrader            ✅  (安全握手、复用协商)           ★ 新增文档      │
│  │                                                                          │
│  │  Pluggable Components:                                                   │
│  ├── Transport           ✅  (QUIC + TCP + WebSocket)                        │
│  ├── Security            ✅  (TLS 1.3 + Noise)                               │
│  ├── Muxer               ✅  (yamux)                                         │
│  ├── Identity            ✅  (Ed25519, Secp256k1, RSA)                       │
│  │                                                                          │
│  │  Protocols & Services:                                                   │
│  ├── Protocol            ✅  (注册、路由、系统协议)         ★ 新增文档      │
│  │   ├── Identify        ✅  (/dep2p/sys/identify/1.0.0)                    │
│  │   ├── Ping            ✅  (/dep2p/sys/ping/1.0.0)                        │
│  │   ├── AutoNAT         ✅  (/dep2p/sys/autonat/1.0.0)                     │
│  │   ├── HolePunch       ✅  (/dep2p/sys/holepunch/1.0.0)                   │
│  │   └── Relay           ✅  (/dep2p/relay/1.0.0/{hop,stop})                │
│  ├── Relay (Service)     ✅  (Client + Server + ★ AddressBook)              │
│  └── NAT                 ✅  (STUN + UPnP + NAT-PMP)                         │
│                                                                             │
│  Discovery Layer (发现层)                                                    │
│  ├── Coordinator         ✅  (统一调度)                                      │
│  ├── DHT                 ✅  (Kademlia)                                      │
│  ├── Bootstrap           ✅  (引导节点)                                      │
│  ├── mDNS                ✅  (局域网)                                        │
│  ├── Rendezvous          ✅  (命名空间)                                      │
│  └── DNS                 📋  (DNS-SD + dnsaddr)             ★ 新增文档      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

图例: ✅ 已实现  📋 规划中  🚧 进行中  ★ 本次新增
```

---

## 组件设计原则

| 原则 | 说明 |
|------|------|
| **自包含** | 组件文档包含从需求到测试的完整信息 |
| **可追溯** | 每个设计决策可追溯到需求 |
| **边界清晰** | 实现设计（L6）不越界到概念模型（L5） |
| **可扩展** | 目录结构允许根据组件复杂度新增内容 |
| **五层架构** | API、Protocol、Realm、Core、Discovery 分工明确 |

---

## Fx 模块依赖图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      DeP2P v1.1.0 Fx 模块依赖图                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  dep2p.New()                                                                │
│      │                                                                      │
│      ├── fx.Module("identity")      ← 最底层，无依赖                        │
│      │                                                                      │
│      ├── fx.Module("eventbus")      ← 无依赖                                │
│      │                                                                      │
│      ├── fx.Module("resourcemgr")   ← 无依赖                                │
│      │                                                                      │
│      ├── fx.Module("storage")       ← ★ 无依赖（可选持久化）               │
│      │                                                                      │
│      ├── fx.Module("peerstore")     ← 依赖 identity, storage (可选)        │
│      │                                                                      │
│      ├── fx.Module("transport")     ← 依赖 identity                        │
│      │                                                                      │
│      ├── fx.Module("security")      ← 依赖 identity                        │
│      │                                                                      │
│      ├── fx.Module("muxer")         ← 依赖 transport, security             │
│      │                                                                      │
│      ├── fx.Module("connmgr")       ← 依赖 peerstore, eventbus             │
│      │                                                                      │
│      ├── fx.Module("upgrader")      ← 依赖 security, muxer                 │
│      │                                                                      │
│      ├── fx.Module("swarm")         ← 依赖 transport, upgrader, connmgr   │
│      │                                                                      │
│      ├── fx.Module("protocol/system")                                      │
│      │   ├── identify              ← 依赖 peerstore, swarm                │
│      │   ├── ping                  ← 依赖 swarm                           │
│      │   ├── autonat               ← 依赖 swarm                           │
│      │   ├── holepunch             ← 依赖 swarm, relay                    │
│      │   └── relay                 ← 依赖 swarm                           │
│      │                                                                      │
│      ├── fx.Module("nat")           ← 依赖 swarm                           │
│      │                                                                      │
│      ├── fx.Module("relay")         ← 依赖 swarm, nat                      │
│      │                                                                      │
│      ├── fx.Module("host")          ← 聚合: swarm, protocol, nat, relay   │
│      │                                                                      │
│      ├── fx.Module("discovery")     ← 依赖 host                            │
│      │   ├── coordinator                                                   │
│      │   ├── dht                                                           │
│      │   ├── mdns                                                          │
│      │   ├── bootstrap                                                     │
│      │   ├── rendezvous                                                    │
│      │   └── dns                                                           │
│      │                                                                      │
│      ├── fx.Module("realm")         ← 依赖 host, discovery                 │
│      │                                                                      │
│      ├── fx.Module("metrics")       ← 依赖 swarm, discovery                │
│      │                                                                      │
│      └── fx.Module("protocol/app")  ← 依赖 realm (可选), host              │
│          ├── messaging                                                     │
│          ├── pubsub                                                        │
│          ├── streams                                                       │
│          └── liveness             ← 内部使用 system/ping                   │
│                                                                            │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 日志与指标说明

日志和指标不作为独立模块，直接使用标准库：

| 能力 | 处理方式 |
|------|----------|
| **Logging** | 直接使用标准库 `log/slog` |
| **Metrics** | 直接使用 `prometheus`，由 core_metrics 封装 |

---

## 相关文档

### L3 行为文档

| 文档 | 说明 |
|------|------|
| [L3: lifecycle_overview.md](../L3_behavioral/lifecycle_overview.md) | ★ 节点生命周期横切面 |
| [L3: discovery_flow.md](../L3_behavioral/discovery_flow.md) | 发现行为流程 |
| [L3: relay_flow.md](../L3_behavioral/relay_flow.md) | Relay 行为流程 |
| [L3: connection_flow.md](../L3_behavioral/connection_flow.md) | 连接行为流程 |
| [L3: state_machines.md](../L3_behavioral/state_machines.md) | 状态机定义 |

### 架构文档

| 文档 | 说明 |
|------|------|
| [../L2_structural/layer_model.md](../L2_structural/layer_model.md) | 五层软件架构 |
| [../L2_structural/module_design.md](../L2_structural/module_design.md) | 模块划分 |
| [../L4_interfaces/component_interface_map.md](../L4_interfaces/component_interface_map.md) | 组件接口映射 |
| [../L5_models/](../L5_models/) | 领域模型 |

---

**最后更新**：2026-01-25（添加 L3/L6 边界说明）
