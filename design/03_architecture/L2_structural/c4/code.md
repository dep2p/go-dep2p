# C4 Level 4: 代码结构 (Code)

> DeP2P 实际代码目录结构和关键文件说明

---

## 项目根目录

```
dep2p/
├── node.go                 # Node 结构体和核心方法
├── options.go              # Option 模式配置
├── config.go               # 配置结构定义
├── presets.go              # 预设配置
├── errors.go               # 公共错误定义
│
├── config/                 # 配置管理（横切关注点，独立于五层）
│   ├── config.go           # 主配置（嵌入所有子配置）
│   ├── defaults.go         # 默认值工厂函数
│   ├── validate.go         # 配置校验
│   ├── identity.go         # IdentityConfig
│   ├── transport.go        # TransportConfig
│   ├── security.go         # SecurityConfig
│   ├── nat.go              # NATConfig
│   ├── relay.go            # RelayConfig
│   ├── discovery.go        # DiscoveryConfig
│   ├── connmgr.go          # ConnManagerConfig
│   ├── messaging.go        # MessagingConfig
│   └── realm.go            # RealmConfig
│
├── internal/               # 内部实现（不可被外部导入）
│   ├── core/               # 核心层
│   ├── discovery/          # 发现层
│   ├── realm/              # Realm 层
│   └── protocol/           # 协议层
│
├── pkg/                    # 公共包（可被外部导入）
│   ├── interfaces/         # 接口定义（扁平结构）
│   ├── types/              # 类型定义
│   ├── log/                # 日志接口
│   └── protocolids/        # 协议 ID
│
├── cmd/                    # 命令行工具
├── examples/               # 示例代码
├── tests/                  # 测试
├── docs/                   # 对外文档
├── design/                 # 内部设计文档
└── scripts/                # 脚本工具
```

---

## internal/core/ 结构

**Core Layer** - 核心网络功能

```
internal/core/
│
├── identity/                   # 身份管理
│   ├── module.go               # Fx 模块
│   ├── identity.go             # Identity 实现
│   ├── keypair.go              # 密钥对管理
│   └── errors.go               # 错误定义
│
├── transport/                  # 传输层
│   ├── module.go
│   ├── quic/                   # QUIC 实现
│   │   ├── module.go
│   │   ├── listener.go         # 监听器
│   │   ├── connection.go       # 连接实现
│   │   └── errors.go
│   └── tcp/                    # TCP 实现（可选）
│
├── security/                   # 安全层
│   ├── module.go
│   ├── tls/                    # TLS 1.3
│   │   ├── module.go
│   │   ├── transport.go        # TLS 传输
│   │   ├── cert.go             # 证书管理
│   │   └── config.go
│   └── noise/                  # Noise 协议
│       ├── module.go
│       ├── transport.go
│       ├── handshake.go
│       └── conn.go
│
├── muxer/                      # 多路复用
│   ├── module.go
│   └── yamux/                  # Yamux 实现
│       ├── module.go
│       ├── transport.go
│       ├── session.go
│       └── stream.go
│
├── connmgr/                    # 连接管理
│   ├── module.go
│   ├── connmgr.go              # 连接管理器
│   ├── limiter.go              # 连接限制
│   └── decayer.go              # 连接衰减
│
├── upgrader/                   # 连接升级器
│   ├── module.go
│   └── upgrader.go             # Security + Muxer 升级
│
├── swarm/                      # 连接池管理
│   ├── module.go
│   ├── swarm.go                # Swarm 实现
│   ├── dial.go                 # 拨号逻辑
│   └── listener.go             # 监听逻辑
│
├── protocol/                   # 协议注册
│   ├── module.go
│   └── registry.go             # 协议注册表
│
├── peerstore/                  # 节点存储
│   ├── module.go
│   ├── peerstore.go            # Peerstore 实现
│   ├── addrbook.go             # 地址簿
│   ├── keybook.go              # 密钥簿
│   └── metrics.go              # 节点度量
│
├── metrics/                    # 度量系统
│   ├── module.go
│   ├── bandwidth.go            # 带宽统计
│   └── rate.go                 # 速率计算
│
├── eventbus/                   # 事件总线
│   ├── module.go
│   └── eventbus.go             # EventBus 实现
│
├── resourcemgr/                # 资源管理
│   ├── module.go
│   ├── manager.go              # 资源管理器
│   └── limits.go               # 资源限制
│
├── host/                       # Host 门面（核心抽象）
│   ├── module.go
│   ├── host.go                 # Host 实现
│   ├── options.go              # Host 选项
│   └── doc.go                  # 文档
│
├── relay/                      # ★ 统一中继（三大职责 v2.0）
│   ├── module.go
│   ├── manager.go              # 中继管理器
│   ├── client/                 # 中继客户端
│   │   └── client.go
│   ├── server/                 # 中继服务端
│   │   └── server.go
│   └── addressbook/            # ★ 地址簿（缓存加速层，非权威目录）
│       └── addressbook.go
│
├── nat/                        # NAT 穿透（外部地址发现 + 打洞）
│   ├── module.go
│   ├── manager.go              # NAT 管理器
│   ├── stun/                   # STUN 客户端（外部地址发现）
│   │   └── stun.go
│   ├── upnp/                   # UPnP 端口映射
│   │   └── upnp.go
│   ├── natpmp/                 # NAT-PMP
│   │   └── natpmp.go
│   └── holepunch/              # ★ 打洞（需要 Relay 信令通道）
│       └── holepunch.go
│
└── reachability/               # ★ 可达性验证
    ├── module.go
    └── reachability.go         # 候选地址 → 可达性验证 → 可发布地址
```

---

## internal/discovery/ 结构

**Discovery Layer** - 节点发现

```
internal/discovery/
│
├── coordinator/                # 发现协调器
│   ├── module.go
│   └── coordinator.go          # 多发现机制协调
│
├── dht/                        # Kademlia DHT
│   ├── module.go
│   ├── dht.go                  # DHT 核心
│   ├── dht_lifecycle.go        # 生命周期
│   ├── dht_query.go            # 查询逻辑
│   ├── dht_providers.go        # Provider 管理
│   ├── dht_peerrecord.go       # PeerRecord
│   ├── network_adapter.go      # 网络适配器（使用 Host）
│   ├── handler.go              # 协议处理器
│   ├── routing_table.go        # 路由表
│   └── message.go              # 消息编解码
│
├── mdns/                       # mDNS 本地发现
│   ├── module.go
│   └── mdns.go                 # mDNS 实现
│
├── bootstrap/                  # Bootstrap 发现
│   ├── module.go
│   └── bootstrap.go            # Bootstrap 实现
│
└── rendezvous/                 # Rendezvous 发现
    ├── module.go
    └── rendezvous.go           # Rendezvous 实现
```

---

## internal/realm/ 结构

**Realm Layer** - 业务隔离域

```
internal/realm/
│
├── module.go                   # Fx 模块
├── manager.go                  # Realm 管理器
├── realm.go                    # Realm 实现
├── auth.go                     # 成员认证
├── membership.go               # 成员管理
├── psk.go                      # PSK 派生
├── errors.go                   # 错误定义
└── doc.go                      # 文档
```

---

## internal/protocol/ 结构

**Protocol Layer** - 应用协议

```
internal/protocol/
│
├── messaging/                  # 点对点消息
│   ├── module.go
│   ├── service.go              # 消息服务
│   ├── request.go              # 请求响应
│   └── ack.go                  # 确认机制
│
└── pubsub/                     # 发布订阅
    ├── module.go
    ├── pubsub.go               # PubSub 服务
    ├── topic.go                # Topic 管理
    ├── subscription.go         # 订阅管理
    └── gossipsub/              # GossipSub 实现
        ├── router.go           # 消息路由
        ├── mesh.go             # Mesh 维护
        ├── scoring.go          # Peer Scoring
        └── heartbeat.go        # 心跳机制
```

---

## pkg/ 结构

**公共包** - 对外暴露的接口和类型

```
pkg/
│
├── interfaces/                 # 接口定义（扁平结构）
│   ├── doc.go                  # 包文档
│   ├── host.go                 # Host 接口（核心门面）
│   ├── node.go                 # Node 接口（顶层 API）
│   ├── identity.go             # Identity 接口
│   ├── transport.go            # Transport, Connection, Stream 接口
│   ├── security.go             # Security 接口
│   ├── muxer.go                # Muxer 接口
│   ├── discovery.go            # Discovery, DHT 接口
│   ├── peerstore.go            # Peerstore 接口
│   ├── connmgr.go              # ConnMgr 接口
│   ├── swarm.go                # Swarm 接口
│   ├── realm.go                # Realm 接口
│   ├── messaging.go            # Messaging 接口
│   ├── pubsub.go               # PubSub 接口
│   ├── streams.go              # Streams 接口
│   ├── liveness.go             # Liveness 接口
│   ├── eventbus.go             # EventBus 接口
│   ├── metrics.go              # Metrics 接口
│   ├── resource.go             # ResourceManager 接口
│   ├── protocol.go             # Protocol 接口
│   └── upgrader.go             # Upgrader 接口
│
├── types/                      # 类型定义
│   ├── doc.go
│   ├── ids.go                  # PeerID, NodeID, RealmID, StreamID
│   ├── enums.go                # Direction, Connectedness, KeyType
│   ├── stats.go                # 统计类型
│   ├── peer.go                 # PeerInfo
│   ├── multiaddr.go            # Multiaddr 封装
│   └── errors.go               # 通用错误
│
├── log/                        # 日志接口
│   └── log.go                  # Logger 接口
│
└── protocolids/                # 协议 ID
    ├── doc.go
    ├── sys.go                  # 系统协议 ID
    └── app.go                  # 应用协议 ID 生成
```

**重要说明**:
- `pkg/interfaces/` 是**扁平结构**，所有接口文件直接放在该目录下
- 不使用子目录
- 采用 go-libp2p 风格，通过单一包暴露所有接口

---

## 模块标准结构

每个核心模块遵循标准结构：

```
internal/{layer}/{module}/
├── module.go              # Fx 模块定义（必需）
├── doc.go                 # 包文档（推荐）
├── README.md              # 模块文档（推荐）
├── {main}.go              # 主实现文件
├── {feature}.go           # 功能文件
├── errors.go              # 错误定义
└── {sub}/                 # 子模块（可选）
```

### module.go 示例

```pseudocode
// internal/realm/module.go

Module realm = FxModule("realm",
    Provide(
        NewManager,
        NewAuthService
    )
)

// NewManager 构造函数
function NewManager(host: Host, cfg: RealmConfig) -> Manager:
    return Manager{
        host: host,
        cfg:  cfg
    }
```

---

## 关键文件说明

### 入口文件

| 文件 | 职责 |
|------|------|
| `node.go` | Node 结构体，New()，JoinRealm() |
| `options.go` | With* Option 函数 |
| `config.go` | Config 结构体定义 |

### 核心门面

| 文件 | 职责 |
|------|------|
| `internal/core/host/host.go` | Host 门面，聚合 Swarm、Protocol、Peerstore 等 |
| `internal/core/swarm/swarm.go` | 连接池管理 |

### 层级管理器

| 文件 | 职责 |
|------|------|
| `internal/realm/manager.go` | Realm 生命周期管理 |
| `internal/core/relay/manager.go` | 统一 Relay（缓存加速 + 信令 + 保底） |
| `internal/core/connmgr/connmgr.go` | 连接池管理 |
| `internal/discovery/coordinator/coordinator.go` | 多发现机制协调 |

### 协议实现

| 文件 | 职责 |
|------|------|
| `internal/protocol/pubsub/gossipsub/router.go` | GossipSub 消息路由 |
| `internal/core/relay/client/client.go` | Relay 客户端（信令 + 缓存加速） |
| `internal/discovery/dht/dht.go` | Kademlia DHT 核心 |

---

## 架构层与目录映射

| 架构层 | 目录 | 说明 |
|--------|------|------|
| **API Layer** | 根目录 | `node.go`, `options.go` |
| **Realm Layer** | `internal/realm/` | Realm 隔离域 |
| **Protocol Layer** | `internal/protocol/` | 应用协议（Messaging, PubSub） |
| **Discovery Layer** | `internal/discovery/` | 节点发现（DHT, mDNS, Bootstrap） |
| **Core Layer** | `internal/core/` | 核心网络功能（Host, Transport, Security） |
| **Public Interfaces** | `pkg/interfaces/` | 公共接口（扁平结构） |
| **Public Types** | `pkg/types/` | 公共类型 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [component.md](component.md) | 组件图 |
| [../module_design.md](../module_design.md) | 模块划分 |
| [../../L6_domains/](../../L6_domains/) | 模块详细设计 |
| [../../L4_interfaces/](../../L4_interfaces/) | 接口设计 |
| `pkg/interfaces/README.md` | 接口结构说明 |

---

**最后更新**：2026-01-23  
**架构版本**：v1.1.0（Host 门面 + 扁平接口）
