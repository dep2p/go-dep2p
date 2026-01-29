# C4 Level 3: 组件 (Component)

> 各域内部的组件结构和依赖关系

---

## 组件全景图

```mermaid
flowchart TB
    subgraph APILayer["API Layer (根目录)"]
        Node["Node<br/>节点入口"]
    end
    
    subgraph ProtocolLayer["Protocol Layer (internal/protocol/)"]
        Messaging["messaging<br/>消息传递"]
        PubSub["pubsub<br/>发布订阅"]
        Streams["streams<br/>流管理"]
        Liveness["liveness<br/>存活检测"]
    end
    
    subgraph RealmLayer["Realm Layer (internal/realm/)"]
        Realm["realm<br/>业务隔离"]
    end
    
    subgraph CoreLayer["Core Layer (internal/core/)"]
        Host["host<br/>网络主机"]
        Identity["identity<br/>身份管理"]
        Transport["transport<br/>QUIC/TCP 传输"]
        Security["security<br/>TLS/Noise 安全"]
        Muxer["muxer<br/>多路复用"]
        ConnMgr["connmgr<br/>连接管理"]
        Relay["relay<br/>中继服务"]
        NAT["nat<br/>NAT 穿透"]
    end
    
    subgraph DiscoveryLayer["Discovery Layer (internal/discovery/)"]
        Coordinator["coordinator<br/>发现协调"]
        DHT["dht<br/>DHT 发现"]
        Bootstrap["bootstrap<br/>引导节点"]
        mDNS["mdns<br/>局域网发现"]
        Rendezvous["rendezvous<br/>命名空间发现"]
    end
    
    Node --> Realm
    Node --> Messaging
    Realm --> Messaging
    Realm --> PubSub
    Messaging --> Host
    PubSub --> Host
    Streams --> Host
    Liveness --> Host
    
    Host --> Transport
    Host --> Security
    Host --> Muxer
    Host --> ConnMgr
    Host --> Identity
    
    Transport --> Security
    Security --> Identity
    
    Host <--> Coordinator
    Coordinator --> DHT
    Coordinator --> Bootstrap
    Coordinator --> mDNS
    Coordinator --> Rendezvous
    
    Relay --> Host
    NAT --> Host
```

---

## Core 核心域组件

### identity - 身份管理

```mermaid
flowchart LR
    subgraph Identity["identity/"]
        KeyPair["KeyPair<br/>密钥对管理"]
        NodeID["NodeID<br/>节点标识"]
        Signer["Signer<br/>签名服务"]
    end
    
    KeyPair --> NodeID
    KeyPair --> Signer
```

| 组件 | 职责 |
|------|------|
| **KeyPair** | Ed25519 密钥对生成和管理 |
| **NodeID** | NodeID = SHA256(公钥) |
| **Signer** | 消息签名和验证 |

### transport - 传输层

```mermaid
flowchart LR
    subgraph Transport["transport/"]
        QUIC["quic/<br/>QUIC 实现"]
        TCP["tcp/<br/>TCP 实现"]
        Listener["Listener<br/>连接监听"]
        Dialer["Dialer<br/>连接拨号"]
        Upgrader["Upgrader<br/>连接升级"]
    end
    
    Listener --> QUIC
    Listener --> TCP
    Dialer --> QUIC
    Dialer --> TCP
    Upgrader --> QUIC
    Upgrader --> TCP
```

| 组件 | 职责 |
|------|------|
| **QUIC** | quic-go 封装 |
| **TCP** | TCP 传输封装 |
| **Listener** | 监听传入连接 |
| **Dialer** | 主动拨号连接 |
| **Upgrader** | 协议升级 |

### security - 安全握手

```mermaid
flowchart LR
    subgraph Security["security/"]
        TLS["tls/<br/>TLS 1.3"]
        Noise["noise/<br/>Noise 协议"]
    end
```

| 组件 | 职责 |
|------|------|
| **TLS** | TLS 1.3 握手，证书绑定 NodeID |
| **Noise** | Noise 协议握手 |

### muxer - 多路复用

```mermaid
flowchart LR
    subgraph Muxer["muxer/"]
        Yamux["yamux/<br/>Yamux 实现"]
        Factory["Factory<br/>复用器工厂"]
    end
    
    Factory --> Yamux
```

### connmgr - 连接管理

```mermaid
flowchart LR
    subgraph ConnMgr["connmgr/"]
        Manager["Manager<br/>连接管理器"]
        Limiter["Limiter<br/>连接限制"]
        Protector["Protector<br/>连接保护"]
    end
    
    Manager --> Limiter
    Manager --> Protector
```

| 组件 | 职责 |
|------|------|
| **Manager** | 连接池管理 |
| **Limiter** | 连接数限制 |
| **Protector** | 连接保护（Protect/Unprotect） |

### relay - 中继服务（v2.0 更新）

> **v2.0 核心变化**：DHT 是权威目录，Relay 地址簿是缓存加速层

```mermaid
flowchart TB
    subgraph Relay["relay/"]
        Client["client/<br/>中继客户端"]
        Server["server/<br/>中继服务器"]
        Manager["Manager<br/>中继管理"]
        Transport["Transport<br/>中继传输"]
        AddressBook["AddressBook<br/>地址簿(缓存)"]
    end
    
    Manager --> Client
    Manager --> Server
    Manager --> AddressBook
    Client --> Transport
```

| 组件 | 职责 |
|------|------|
| **Client** | 请求中继连接 |
| **Server** | 提供中继服务 |
| **Manager** | 管理可用中继 |
| **Transport** | 中继传输封装 |
| **AddressBook** | 地址簿（缓存加速层，非权威目录） |

**Relay 三大职责 (v2.0)**：
1. **缓存加速层**：维护地址簿，作为 DHT 本地缓存（非权威）
2. **打洞协调信令**：提供打洞协调的信令通道
3. **数据通信保底**：直连/打洞失败时转发数据

### nat - NAT 穿透

```mermaid
flowchart LR
    subgraph NAT["nat/"]
        Punch["holepunch/<br/>UDP 打洞"]
        UPnP["upnp/<br/>端口映射"]
        STUN["stun/<br/>地址发现"]
    end
```

| 组件 | 职责 |
|------|------|
| **Holepunch** | UDP 打洞协调 |
| **UPnP** | UPnP/NAT-PMP 端口映射 |
| **STUN** | 公网地址发现 |

---

## Discovery 发现域组件

```mermaid
flowchart TB
    subgraph Discovery["discovery/"]
        Coordinator["coordinator/<br/>发现协调"]
        DHT["dht/<br/>Kademlia DHT"]
        MDNS["mdns/<br/>mDNS 局域网"]
        Bootstrap["bootstrap/<br/>引导节点"]
        Rendezvous["rendezvous/<br/>Rendezvous 发现"]
    end
    
    Coordinator --> DHT
    Coordinator --> Bootstrap
    Coordinator --> MDNS
    Coordinator --> Rendezvous
```

| 组件 | 职责 |
|------|------|
| **Coordinator** | 统一调度各发现方式 |
| **DHT** | Kademlia 分布式哈希表 |
| **mDNS** | 局域网多播发现 |
| **Bootstrap** | 初始节点连接 |
| **Rendezvous** | 命名空间发现 |

---

## Realm Layer 组件

### realm - Realm 管理

```mermaid
flowchart TB
    subgraph Realm["realm/"]
        Manager["manager/<br/>Realm 管理器"]
        Auth["auth/<br/>成员认证"]
        PSK["PSK<br/>密钥派生"]
        Member["member/<br/>成员管理"]
    end
    
    Manager --> Auth
    Manager --> PSK
    Manager --> Member
```

| 组件 | 职责 |
|------|------|
| **Manager** | Realm 生命周期管理 |
| **Auth** | PSK 成员认证 |
| **PSK** | RealmID 和 RealmKey 派生 |
| **Member** | 成员列表缓存 |

---

## Protocol Layer 组件

### messaging - 消息传递

```mermaid
flowchart TB
    subgraph Messaging["messaging/"]
        Service["Service<br/>消息服务"]
        Request["Request<br/>请求响应"]
        Delivery["Delivery<br/>消息投递"]
    end
    
    Service --> Request
    Service --> Delivery
```

### pubsub - 发布订阅

```mermaid
flowchart TB
    subgraph PubSub["pubsub/"]
        Service["Service<br/>PubSub 服务"]
        GossipSub["gossipsub/<br/>GossipSub"]
        Topic["Topic<br/>主题管理"]
    end
    
    Service --> GossipSub
    Service --> Topic
```

| 组件 | 职责 |
|------|------|
| **Service** | 发布订阅 API |
| **GossipSub** | GossipSub 协议实现 |
| **Topic** | 主题管理和消息投递 |

---

## 组件依赖矩阵

| 组件 | 依赖的组件 |
|------|-----------|
| **identity** | pkg/types |
| **transport** | identity, security |
| **security** | identity |
| **muxer** | - |
| **connmgr** | - |
| **host** | transport, security, muxer, connmgr, identity |
| **coordinator** | host, dht, bootstrap, mdns, rendezvous |
| **dht** | host |
| **bootstrap** | host |
| **mdns** | - |
| **rendezvous** | host |
| **relay** | host |
| **nat** | host, relay |
| **realm** | host, messaging |
| **messaging** | host |

---

## 组件通信方式

```
组件间通信原则：

  1. 通过 pkg/interfaces/ 定义的接口
  2. 通过 Fx 依赖注入
  3. 禁止直接 import 其他组件的内部包
```

```mermaid
flowchart LR
    A["messaging/"]
    I["pkg/interfaces/realm.go"]
    B["realm/ 实现"]
    
    A -->|"依赖接口"| I
    I -->|"Fx 注入"| B
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [container.md](container.md) | 容器图 |
| [code.md](code.md) | 代码结构 |
| [../module_design.md](../module_design.md) | 模块划分 |

---

**最后更新**：2026-01-24（v2.0 DHT 权威模型对齐）
