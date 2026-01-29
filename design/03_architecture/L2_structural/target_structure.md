# 目标目录结构 (Target Structure)

> **版本**: v1.4.0  
> **更新日期**: 2026-01-19  
> **定位**: DeP2P 五层架构的目标目录结构

---

## 概述

本文档定义 DeP2P 的**目标目录结构**，基于**五层软件架构**：

**五层软件架构**：
- **API Layer** - 用户入口门面
- **Protocol Layer** - 用户级应用协议
- **Realm Layer** - 业务隔离层
- **Core Layer** - P2P 网络核心能力
- **Discovery Layer** - 节点发现服务

> **参考**: 五层架构详见 [layer_model.md](layer_model.md)

> **实现简化说明**（2026-01-13 更新）
> 
> **接口策略**：
> - **公共接口**：统一在 `pkg/interfaces/` 定义，所有模块直接实现
> - **内部接口**：`internal/*/interfaces/` 子目录为**可选**，仅在模块内部有多个子组件需要相互依赖时使用
> - **事件类型**：统一在 `pkg/types/events.go` 定义，不分散在各模块
> 
> 大多数模块**不需要** `interfaces/` 子目录，直接实现 `pkg/interfaces/` 中的公共接口即可。

---

## 根目录结构

```
dep2p/
├── dep2p.go                     # API 入口 (API Layer)
├── node.go                      # Node 实现 (API Layer)
├── options.go                   # 配置选项 (API Layer)
├── errors.go                    # 公共错误
├── presets.go                   # 预设配置
├── config.go                    # 配置结构
├── doc.go                       # 包文档
│
├── config/                      # 配置管理（横切关注点）
│   ├── config.go                # 主 Config 结构体（嵌入所有子配置）
│   ├── defaults.go              # DefaultXXXConfig() 工厂函数
│   ├── validate.go              # 配置校验
│   ├── identity.go              # IdentityConfig 子配置
│   ├── transport.go             # TransportConfig 子配置
│   ├── security.go              # SecurityConfig 子配置
│   ├── nat.go                   # NATConfig 子配置
│   ├── relay.go                 # RelayConfig 子配置
│   ├── discovery.go             # DiscoveryConfig 子配置
│   ├── connmgr.go               # ConnManagerConfig 子配置
│   ├── messaging.go             # MessagingConfig 子配置
│   ├── realm.go                 # RealmConfig 子配置
│   ├── resource.go              # ResourceConfig 子配置
│   ├── storage.go               # StorageConfig 子配置
│   ├── bandwidth.go             # BandwidthConfig 子配置 ★ 新增
│   ├── pathhealth.go            # PathHealthConfig 子配置 ★ 新增
│   ├── recovery.go              # RecoveryConfig 子配置 ★ 新增
│   ├── connection_health.go     # ConnectionHealthConfig 子配置 ★ 新增
│   └── README.md                # 配置说明
│
├── internal/                    # 内部实现（五层）
│   ├── protocol/                # Protocol Layer
│   ├── realm/                   # Realm Layer
│   ├── core/                    # Core Layer
│   └── discovery/               # Discovery Layer
│
├── pkg/                         # 公共包
│   ├── types/                   # 公共类型
│   ├── interfaces/              # 接口定义（扁平结构）
│   ├── proto/                   # Protobuf 协议定义
│   │   ├── generate.go          # go:generate 指令
│   │   └── addressbook/         # 地址簿协议消息
│   │       ├── addressbook.proto
│   │       └── addressbook.pb.go
│   ├── log/                     # 日志接口
│   └── protocolids/             # 协议 ID
│
├── cmd/                         # 命令行工具（可选）
├── examples/                    # 示例代码
├── docs/                        # 用户文档
└── design/                      # 设计文档
```

---

## Protocol Layer (协议层) 结构

用户级应用协议。

```
internal/protocol/
├── README.md
│
├── messaging/                   # 点对点消息
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── service.go               # 直接实现 pkg/interfaces/messaging.go
│   ├── request.go
│   ├── send.go
│   └── handler.go
│
├── pubsub/                      # 发布订阅
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── service.go               # 直接实现 pkg/interfaces/pubsub.go
│   └── gossipsub/               # GossipSub 实现
│       ├── router.go
│       ├── mesh.go
│       ├── scoring.go
│       ├── topic.go
│       └── subscription.go
│
├── streams/                     # 流管理
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── service.go               # 直接实现 pkg/interfaces/streams.go
│   └── stream.go
│
└── liveness/                    # 存活检测
    ├── module.go
    ├── doc.go
    ├── README.md
    ├── service.go               # 直接实现 pkg/interfaces/liveness.go
    └── ping.go
```

---

## Realm Layer (Realm 层) 结构

业务隔离层。

```
internal/realm/
├── module.go                    # Fx 模块定义（聚合子模块）
├── doc.go
├── README.md
├── realm.go                     # Realm 实现（直接实现 pkg/interfaces/realm.go）
│
├── manager/                     # Realm 管理（子组件）
│   └── manager.go               # 管理器实现
│
├── auth/                        # PSK 认证（子组件）
│   ├── auth.go                  # PSK 认证实现
│   └── derive.go                # PSK 派生
│
├── member/                      # 成员管理（子组件）
│   └── member.go                # 成员管理实现
│
# 注：Relay 统一在 Core Layer (internal/core/relay/)
# v2.0 三大职责：缓存加速层 + 打洞协调信令 + 数据通信保底（DHT 是权威目录）
```

---

## Core Layer (核心层) 结构

P2P 网络核心能力。

```
internal/core/
├── README.md
│
├── host/                        # 网络主机（门面）
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── host.go                  # 直接实现 pkg/interfaces/host.go
│   └── options.go               # Host 选项
│
├── identity/                    # 身份管理
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── identity.go              # 直接实现 pkg/interfaces/identity.go
│   ├── keypair.go
│   ├── device.go                # ★ 设备身份与证书管理
│   └── errors.go
│
├── transport/                   # 传输层
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── quic/                    # QUIC 传输实现
│   │   ├── module.go
│   │   ├── listener.go
│   │   ├── connection.go        # 实现 pkg/interfaces/transport.go Connection
│   │   └── errors.go
│   └── tcp/                     # TCP 传输实现（可选）
│       ├── module.go
│       ├── listener.go
│       └── connection.go
│
├── security/                    # 安全层
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── tls/                     # TLS 1.3 实现
│   │   ├── module.go
│   │   ├── transport.go         # 实现 pkg/interfaces/security.go
│   │   ├── cert.go
│   │   └── config.go
│   └── noise/                   # Noise 协议实现
│       ├── module.go
│       ├── transport.go
│       ├── handshake.go
│       └── conn.go
│
├── muxer/                       # 多路复用
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   └── yamux/                   # Yamux 实现
│       ├── module.go
│       ├── transport.go         # 实现 pkg/interfaces/muxer.go
│       ├── session.go
│       └── stream.go
│
├── upgrader/                    # 连接升级器
│   ├── module.go
│   └── upgrader.go              # 实现 pkg/interfaces/upgrader.go
│
├── swarm/                       # 连接池管理
│   ├── module.go
│   ├── swarm.go                 # 实现 pkg/interfaces/swarm.go
│   ├── dial.go
│   └── listener.go
│
├── protocol/                    # 协议注册
│   ├── module.go
│   └── registry.go              # 实现 pkg/interfaces/protocol.go
│
├── peerstore/                   # 节点存储
│   ├── module.go
│   ├── peerstore.go             # 实现 pkg/interfaces/peerstore.go
│   ├── addrbook.go
│   ├── keybook.go
│   └── metrics.go
│
├── metrics/                     # 度量系统
│   ├── module.go
│   ├── bandwidth.go             # 实现 pkg/interfaces/metrics.go
│   └── rate.go
│
├── eventbus/                    # 事件总线
│   ├── module.go
│   └── eventbus.go              # 实现 pkg/interfaces/eventbus.go
│
├── resourcemgr/                 # 资源管理
│   ├── module.go
│   ├── manager.go               # 实现 pkg/interfaces/resource.go
│   └── limits.go
│
├── connmgr/                     # 连接管理
│   ├── module.go
│   ├── connmgr.go               # 实现 pkg/interfaces/connmgr.go
│   ├── limiter.go
│   └── decayer.go
│
├── relay/                       # ★ 统一中继（三大职责 v2.0）
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── manager.go               # 中继管理器
│   ├── client/                  # 中继客户端
│   │   └── client.go
│   ├── server/                  # 中继服务端
│   │   └── server.go
│   └── addressbook/             # ★ 成员地址簿（缓存加速层，非权威目录）
│       ├── doc.go               # 包文档
│       ├── errors.go            # 错误定义
│       ├── addressbook.go       # MemberAddressBook 聚合根
│       ├── store_memory.go      # MemoryStore 内存存储
│       ├── store_badger.go      # ★ BadgerStore 持久化存储
│       ├── entry.go             # Proto 转换工具
│       └── addressbook_test.go  # 单元测试
│
├── storage/                     # ★ 存储服务（统一持久化基础设施）
│   ├── module.go                # Fx 模块
│   ├── doc.go                   # 模块文档
│   ├── config.go                # 存储配置
│   ├── errors.go                # 错误定义
│   ├── engine/                  # 存储引擎抽象
│   │   ├── engine.go            # InternalEngine/Batch/Iterator 内部接口
│   │   ├── config.go            # 引擎配置
│   │   ├── errors.go            # 引擎错误
│   │   └── badger/              # BadgerDB 实现
│   │       ├── db.go            # 主引擎实现
│   │       ├── batch.go         # 批量操作
│   │       ├── iter.go          # 迭代器
│   │       ├── txn.go           # 事务
│   │       └── db_test.go       # 单元测试
│   └── kv/                      # KV 抽象层（带前缀隔离）
│       ├── store.go             # KVStore 实现
│       └── store_test.go        # 单元测试
│
├── bandwidth/                   # ★ 带宽统计
│   ├── module.go                # Fx 模块
│   ├── doc.go                   # 模块文档
│   ├── counter.go               # 实现 pkg/interfaces/bandwidth.go
│   ├── meter.go                 # EWMA 计量器
│   └── counter_test.go          # 单元测试
│
├── netmon/                      # ★ 连接健康监控
│   ├── module.go                # Fx 模块
│   ├── doc.go                   # 模块文档
│   ├── config.go                # 配置
│   ├── monitor.go               # 实现 pkg/interfaces/netmon.go
│   ├── error_counter.go         # 错误计数器
│   └── monitor_test.go          # 单元测试
│
├── pathhealth/                  # ★ 路径健康管理
│   ├── module.go                # Fx 模块
│   ├── doc.go                   # 模块文档
│   ├── config.go                # 配置
│   ├── manager.go               # 实现 pkg/interfaces/pathhealth.go
│   ├── path.go                  # 路径信息和 EWMA
│   └── manager_test.go          # 单元测试
│
├── recovery/                    # ★ 网络恢复管理
│   ├── module.go                # Fx 模块
│   ├── doc.go                   # 模块文档
│   ├── config.go                # 配置
│   ├── manager.go               # 实现 pkg/interfaces/recovery.go
│   ├── bridge.go                # 监控桥接器
│   ├── manager_test.go          # 单元测试
│   └── bridge_test.go           # 桥接器测试
│
├── reachability/                # ★ 可达性管理（地址状态机）
│   ├── module.go                # Fx 模块
│   ├── doc.go                   # 模块文档
│   ├── coordinator.go           # 可达性协调器
│   ├── state.go                 # ★ 地址状态机（Candidate→Validating→Reachable→Published）
│   ├── dialback.go              # 回拨验证服务（AutoNAT）
│   ├── store.go                 # 可达地址存储
│   └── addrmgmt/                # 地址管理子模块
│       ├── handler.go           # 地址记录处理
│       └── scheduler.go         # 地址刷新调度
│
├── nat/                         # NAT 穿透（外部地址发现 + 打洞）
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── service.go               # NAT 服务（实现 interfaces.NATService）
│   ├── config.go                # NAT 配置（含 IPv6NATMode）
│   ├── stun/                    # STUN 客户端（外部地址发现）
│   │   ├── stun.go              # STUN 基础客户端
│   │   ├── nattype.go           # ★ RFC 3489 NAT 类型检测
│   │   └── nattype_test.go      # NAT 类型检测测试
│   ├── upnp/                    # UPnP 端口映射
│   │   └── upnp.go
│   ├── natpmp/                  # NAT-PMP
│   │   └── natpmp.go
│   └── holepunch/               # ★ 打洞协调（需要 Relay 信令通道）
│       ├── holepunch.go         # 打洞服务
│       └── signaling.go         # ★ 信令通道管理（复用 Relay 连接）
```

---

## Discovery Layer (发现层) 结构

节点发现与广播。

```
internal/discovery/
├── README.md
│
├── coordinator/                 # 发现协调器
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── coordinator.go           # 多发现机制协调
│   ├── finder.go                # ★ PeerFinder 多级缓存查找
│   └── announcer.go             # ★ AddressAnnouncer 地址刷新公告
│
├── dht/                         # Kademlia DHT 发现
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   ├── dht.go                  # 实现 pkg/interfaces/discovery.go DHT接口
│   ├── dht_lifecycle.go
│   ├── dht_query.go
│   ├── dht_providers.go
│   ├── dht_peerrecord.go
│   ├── network_adapter.go      # 网络适配器（使用 Host）
│   ├── handler.go
│   ├── routing_table.go
│   └── message.go
│
├── bootstrap/                   # 引导节点发现
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   └── bootstrap.go            # 实现 pkg/interfaces/discovery.go Discovery接口
│
├── mdns/                        # 局域网 mDNS 发现
│   ├── module.go
│   ├── doc.go
│   ├── README.md
│   └── mdns.go                 # 实现 pkg/interfaces/discovery.go Discovery接口
│
└── rendezvous/                  # 命名空间发现
    ├── module.go
    ├── doc.go
    ├── README.md
    ├── service.go              # Rendezvous 服务
    ├── client.go
    └── server.go
```

---

## Package 结构

公共包，可被外部项目导入。

```
pkg/
├── types/                       # 公共类型
│   ├── doc.go                   # 包文档
│   ├── ids.go                   # PeerID, NodeID, RealmID, StreamID
│   ├── enums.go                 # Direction, Connectedness, KeyType
│   ├── stats.go                 # 统计类型
│   ├── peer.go                  # PeerInfo
│   ├── multiaddr.go             # Multiaddr 封装
│   └── errors.go                # 通用错误
│
├── interfaces/                  # 接口定义（扁平结构）
│   ├── doc.go                   # 包文档
│   │
│   │  # ═══════════ API Layer 接口 ═══════════
│   ├── node.go                  # Node 接口
│   │
│   │  # ═══════════ Protocol Layer 接口 ═══════════
│   ├── messaging.go             # Messaging 接口
│   ├── pubsub.go                # PubSub, Topic 接口
│   ├── streams.go               # Streams 接口
│   ├── liveness.go              # Liveness 接口
│   │
│   │  # ═══════════ Realm Layer 接口 ═══════════
│   ├── realm.go                 # Realm, RealmManager 接口
│   │
│   │  # ═══════════ Core Layer 接口 ═══════════
│   ├── host.go                  # Host 接口（核心门面）
│   ├── identity.go              # Identity 接口
│   ├── transport.go             # Transport, Connection, Stream 接口
│   ├── security.go              # Security 接口
│   ├── muxer.go                 # Muxer 接口
│   ├── upgrader.go              # Upgrader 接口
│   ├── swarm.go                 # Swarm 接口
│   ├── protocol.go              # Protocol 接口
│   ├── peerstore.go             # Peerstore 接口
│   ├── metrics.go               # Metrics 接口
│   ├── eventbus.go              # EventBus 接口
│   ├── resource.go              # ResourceManager 接口
│   ├── connmgr.go               # ConnMgr 接口
│   ├── relay.go                 # Relay 接口
│   ├── nat.go                   # NAT 接口
│   ├── storage.go               # ★ Storage 接口（Engine 基础接口）
│   │
│   │  # ═══════════ Discovery Layer 接口 ═══════════
│   └── discovery.go             # Discovery, DHT, RoutingTable 接口
│
├── log/                         # 日志接口
│   └── log.go                   # Logger 接口
│
└── protocolids/                 # 协议 ID
    ├── doc.go
    ├── sys.go                   # 系统协议 ID
    └── app.go                   # 应用协议 ID 生成
```

**重要说明**:
- `pkg/interfaces/` 采用**扁平结构**，不使用层前缀
- 所有接口文件直接放在 `pkg/interfaces/` 目录下
- 不使用子目录
- 采用 go-libp2p 风格，通过单一包暴露所有接口

---

## 目录结构原则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          目录结构原则                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  原则 1: 五层架构                                                            │
│  ──────────────────                                                         │
│  internal/ 按架构层组织：protocol/, realm/, core/, discovery/                │
│  体现五层软件架构                                                            │
│                                                                             │
│  原则 2: 组件自包含                                                          │
│  ────────────────────                                                       │
│  每个组件根目录包含：module.go, doc.go, README.md                             │
│  直接实现 pkg/interfaces/ 中的公共接口，无需内部接口层                         │
│                                                                             │
│  原则 3: 禁止使用 impl/ 目录                                                  │
│  ────────────────────────────                                               │
│  所有实现目录必须使用具体的实现名称                                            │
│  ❌ transport/quic/impl/                                                    │
│  ✅ transport/quic/                                                         │
│                                                                             │
│  原则 4: 接口扁平命名                                                         │
│  ──────────────────────                                                     │
│  公共接口使用功能名称，不使用层前缀：host.go, messaging.go, discovery.go    │
│  采用 go-libp2p 风格的扁平接口结构                                           │
│                                                                             │
│  原则 5: 依赖方向                                                            │
│  ──────────────────                                                         │
│  API → Protocol → Realm → Core ↔ Discovery                                  │
│  禁止反向依赖                                                                │
│                                                                             │
│  原则 6: 横切关注点独立                                                       │
│  ──────────────────────                                                     │
│  config/ 在根目录，日志/指标直接使用                                          │
│  不需要 infra 层                                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [layer_model.md](layer_model.md) | 五层软件架构 |
| [module_design.md](module_design.md) | 模块划分 |
| [dependency_rules.md](dependency_rules.md) | 依赖规则 |
| [../L1_overview/domain_map.md](../L1_overview/domain_map.md) | 领域映射 |

---

---

## 废弃的命名约定

**早期设计** 曾考虑使用层前缀命名接口文件（如 `core_host.go`, `protocol_messaging.go`），但在实施阶段决定采用 **go-libp2p 风格的扁平命名**。

| 废弃命名 | 当前命名 | 说明 |
|----------|----------|------|
| `core_host.go` | `host.go` | Host 接口 |
| `core_identity.go` | `identity.go` | Identity 接口 |
| `protocol_messaging.go` | `messaging.go` | Messaging 接口 |
| `protocol_pubsub.go` | `pubsub.go` | PubSub 接口 |
| `discovery_dht.go` | `discovery.go` | Discovery/DHT 接口 |

---

**最后更新**：2026-01-23  
**架构版本**：v1.1.0（Host 门面 + 扁平接口）
