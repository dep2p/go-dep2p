# Endpoint 聚合入口模块

## 概述

**层级**: Tier 4  
**职责**: 聚合所有子模块，提供统一的用户 API 入口，实现 Realm 多租户支持。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [Realm 协议](../../../docs/01-design/protocols/application/04-realm.md) | 多租户隔离设计 |
| [消息传递协议](../../../docs/01-design/protocols/application/03-messaging.md) | 通信模式设计 |
| [连接协议](../../../docs/01-design/protocols/transport/04-connection.md) | 连接建立/心跳/重连/质量监控 |
| [地址管理协议](../../../docs/01-design/protocols/network/04-address-management.md) | 地址排序/缺失恢复/刷新通知 |

## 能力清单

### 核心能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 身份聚合 | ✅ 已实现 | 暴露 ID() 和 PublicKey() |
| 连接管理 | ✅ 已实现 | Connect/Disconnect/Connections |
| 协议处理 | ✅ 已实现 | SetProtocolHandler |
| 地址管理 | ✅ 已实现 | ListenAddrs/AdvertisedAddrs |
| 监听/接受 | ✅ 已实现 | Listen/Accept |
| 子系统访问 | ✅ 已实现 | Discovery/NAT/Relay 访问 |

### 连接协议能力（必须落地到某个组件）

> 连接协议里大量"策略/状态机/指标"属于跨组件能力：Endpoint 负责编排，Address/ConnMgr/Discovery/Transport/NAT/Relay 分别落地实现。

| 能力 | 状态 | 推荐落点（主） | 说明 |
|------|------|----------------|------|
| 地址发现阶段 | ✅ 已实现 | `discovery/` + `address/` | 缓存→DHT→mDNS→Bootstrap |
| 地址排序阶段 | ✅ 已实现 | `address/` | 公网>局域网>NAT映射>中继 + 分数模型 |
| 连接尝试编排 | ✅ 已实现 | `endpoint/` | 直连→打洞→中继，多路径尝试 |
| 心跳保活 (Ping/Pong) | ✅ 已实现 | `liveness/` | `/dep2p/ping/1.0`，超时判定断连 |
| 连接抖动容错 | ✅ 已实现 | `connmgr/` + `endpoint/` | JitterTolerance/StateHoldTime |
| 重连策略/指数退避 | ✅ 已实现 | `connmgr/` | 指数退避重连 |
| 连接质量监控 | ✅ 已实现 | `connmgr/` | AvgRTT/SuccessRate/QualityScore |

### Realm 能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| Realm 加入 | ✅ 已实现 | JoinRealm(realmID) |
| Realm 离开 | ✅ 已实现 | LeaveRealm() (v1.1 无参数) |
| ~~多 Realm 支持~~ | ~~已移除~~ | v1.1 严格单 Realm 模型 |
| Realm 感知连接 | ✅ 已实现 | 连接时指定 Realm |
| Realm 隔离 | ✅ 已实现 | 不同 Realm 节点互不可见 |
| ~~跨 Realm 通信~~ | ~~已移除~~ | v1.1 不支持跨 Realm (Gateway 已删除) |

### 消息传递能力 (通过 Messaging 模块)

| 能力 | 状态 | 说明 |
|------|------|------|
| Stream 模式 | ✅ 已实现 | 双向流通信 |
| Request-Response | ✅ 已实现 | 一问一答模式 |
| Pub-Sub | ✅ 已实现 | 发布订阅模式 |

## 依赖关系

### 接口依赖

```
pkg/types/              → NodeID, ProtocolID, Address, RealmID
pkg/interfaces/core/    → Endpoint, Connection, Stream 接口
pkg/interfaces/crypto/  → PublicKey 接口
```

### 模块依赖

```
identity    → 节点身份
transport   → 网络传输
security    → 安全层
muxer       → 多路复用
nat         → NAT 穿透
discovery   → 节点发现
relay       → 中继服务
protocol    → 协议管理
connmgr     → 连接管理
address     → 地址管理
messaging   → 消息服务
```

## 目录结构

```
endpoint/
├── README.md            # 本文件
├── module.go            # fx 模块定义
├── endpoint.go          # Endpoint 主实现
├── connection.go        # Connection 包装
├── stream.go            # Stream 包装
└── realm/               # Realm 支持子模块
    ├── manager.go       # Realm 管理器
    └── types.go         # Realm 相关类型
```

## 公共接口

实现 `pkg/interfaces/core/` 中的 Endpoint 接口：

```go
// Endpoint 端点接口
type Endpoint interface {
    // 身份
    ID() types.NodeID
    PublicKey() crypto.PublicKey
    
    // 连接管理
    Connect(ctx context.Context, nodeID types.NodeID) (Connection, error)
    ConnectWithAddrs(ctx context.Context, nodeID types.NodeID, addrs []Address) (Connection, error)
    Disconnect(nodeID types.NodeID) error
    Connections() []Connection
    Connection(nodeID types.NodeID) (Connection, bool)
    ConnectionCount() int
    
    // 监听
    Listen(ctx context.Context) error
    Accept(ctx context.Context) (Connection, error)
    
    // 协议
    SetProtocolHandler(protocolID ProtocolID, handler ProtocolHandler)
    RemoveProtocolHandler(protocolID ProtocolID)
    Protocols() []ProtocolID
    
    // 地址
    ListenAddrs() []Address
    AdvertisedAddrs() []Address
    AddAdvertisedAddr(addr Address)
    
    // 子系统
    Discovery() DiscoveryService
    NAT() NATService
    Relay() RelayClient
    AddressBook() AddressBook
    EventBus() EventBus
    
    // 生命周期
    Close() error
}
```

### Realm 扩展接口

```go
// RealmEndpoint Realm 感知的端点接口 (v1.1)
//
// v1.1 变更:
//   - 严格单 Realm 模型（节点同时只能加入一个 Realm）
//   - 移除 Realms() 方法（改用 CurrentRealm()）
//   - LeaveRealm() 改为无参数
//   - 移除 CrossRealmConnect（Gateway 不再支持）
type RealmEndpoint interface {
    Endpoint
    
    // Realm 管理
    JoinRealm(ctx context.Context, realmID types.RealmID, opts ...JoinOption) error
    LeaveRealm() error         // v1.1: 无参数
    CurrentRealm() types.RealmID  // v1.1: 替代 Realms()
    IsMember() bool            // v1.1: 检查是否已加入 Realm
    
    // Realm 感知连接
    ConnectInRealm(ctx context.Context, realmID types.RealmID, nodeID types.NodeID) (Connection, error)
}
```

## 关键算法

### Realm ID 生成 (来自设计文档)

```go
// RealmID = SHA256(CreatorPublicKey || RealmName)
func GenerateRealmID(creatorPubKey crypto.PublicKey, realmName string) types.RealmID {
    data := append(creatorPubKey.Bytes(), []byte(realmName)...)
    hash := sha256.Sum256(data)
    var realmID types.RealmID
    copy(realmID[:], hash[:])
    return realmID
}
```

### 连接建立流程

```go
func (e *Endpoint) Connect(ctx context.Context, nodeID types.NodeID) (Connection, error) {
    // 1. 检查现有连接
    if conn, ok := e.Connection(nodeID); ok {
        return conn, nil
    }
    
    // 2. 获取地址
    addrs := e.addressBook.Addrs(nodeID)
    if len(addrs) == 0 {
        // 通过发现服务查找
        var err error
        addrs, err = e.discovery.FindPeer(ctx, nodeID)
        if err != nil {
            return nil, fmt.Errorf("无法找到节点地址: %w", err)
        }
    }
    
    // 3. 尝试连接
    return e.ConnectWithAddrs(ctx, nodeID, addrs)
}

func (e *Endpoint) ConnectWithAddrs(ctx context.Context, nodeID types.NodeID, addrs []Address) (Connection, error) {
    // 尝试每个地址
    for _, addr := range addrs {
        // 1. 建立传输连接
        rawConn, err := e.transport.Dial(ctx, addr, nodeID)
        if err != nil {
            continue
        }
        
        // 2. 安全升级 (QUIC 内置 TLS)
        secureConn, err := e.security.SecureOutbound(ctx, rawConn, nodeID)
        if err != nil {
            rawConn.Close()
            continue
        }
        
        // 3. 创建多路复用器
        mux, err := e.muxer.NewMuxer(secureConn, false)
        if err != nil {
            secureConn.Close()
            continue
        }
        
        // 4. 包装为 Connection
        conn := newConnection(secureConn, mux, nodeID)
        e.addConnection(nodeID, conn)
        
        // 5. 通知连接管理器
        e.connManager.NotifyConnected(conn)
        
        return conn, nil
    }
    
    return nil, ErrConnectionFailed
}
```

### 重连策略（来自连接协议）

```
容忍窗口:
JitterTolerance = 5s
ReconnectGrace  = 10s
StateHoldTime   = 30s

重连优先级:
P1 缓存地址重连（2s）
P2 所有已知地址（5s）
P3 DHT 重发现（10s）
失败则指数退避，最多 5 次，最大间隔 30s
```

### Realm 加入流程 (来自设计文档)

```go
func (e *Endpoint) JoinRealm(ctx context.Context, realmID types.RealmID, opts ...JoinOption) error {
    options := &joinOptions{}
    for _, opt := range opts {
        opt(options)
    }
    
    // Public Realm: 直接查询 DHT 获取元数据
    if !options.hasJoinKey {
        meta, err := e.discovery.GetRealmMeta(ctx, realmID)
        if err != nil {
            return err
        }
        if meta.AccessLevel != AccessPublic {
            return ErrJoinKeyRequired
        }
    } else {
        // Protected/Private Realm: 需要 JoinKey
        proof := e.signJoinProof(realmID, options.joinKey)
        if err := e.verifyJoinProof(realmID, proof); err != nil {
            return err
        }
    }
    
    // 注册到 Realm
    e.realms[realmID] = &realmState{
        id:       realmID,
        joinedAt: time.Now(),
    }
    
    // 在 DHT 中注册
    if err := e.discovery.AnnounceToRealm(ctx, realmID); err != nil {
        return err
    }
    
    return nil
}
```

### 监听和接受

```go
func (e *Endpoint) Listen(ctx context.Context) error {
    for _, addr := range e.config.ListenAddrs {
        listener, err := e.transport.Listen(ctx, addr)
        if err != nil {
            return err
        }
        e.listeners = append(e.listeners, listener)
        
        // 启动接受循环
        go e.acceptLoop(ctx, listener)
    }
    return nil
}

func (e *Endpoint) acceptLoop(ctx context.Context, listener Listener) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }
        
        rawConn, err := listener.Accept()
        if err != nil {
            continue
        }
        
        go e.handleInbound(ctx, rawConn)
    }
}

func (e *Endpoint) handleInbound(ctx context.Context, rawConn transport.Conn) {
    // 1. 安全升级
    secureConn, err := e.security.SecureInbound(ctx, rawConn)
    if err != nil {
        rawConn.Close()
        return
    }
    
    // 2. 获取对方身份
    remoteID := secureConn.RemotePeer()
    
    // 3. 检查连接管理器是否允许
    if !e.connManager.AllowConnection(remoteID, DirectionInbound) {
        secureConn.Close()
        return
    }
    
    // 4. 创建多路复用器
    mux, err := e.muxer.NewMuxer(secureConn, true)
    if err != nil {
        secureConn.Close()
        return
    }
    
    // 5. 包装并注册连接
    conn := newConnection(secureConn, mux, remoteID)
    e.addConnection(remoteID, conn)
    e.connManager.NotifyConnected(conn)
    
    // 6. 启动流接受循环
    go e.streamAcceptLoop(ctx, conn, mux)
}
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    
    // 核心依赖
    Identity  identityif.Identity   `name:"identity"`
    Transport transportif.Transport `name:"transport"`
    
    // 安全和多路复用
    Security     securityif.SecureTransport `name:"secure_transport" optional:"true"`
    MuxerFactory muxerif.MuxerFactory       `name:"muxer_factory" optional:"true"`
    
    // 可选依赖
    Discovery      coreif.DiscoveryService  `name:"discovery" optional:"true"`
    NAT            natif.NATService         `name:"nat" optional:"true"`
    Relay          coreif.RelayClient       `name:"relay" optional:"true"`
    AddressBook    addressif.AddressBook    `name:"address_book" optional:"true"`
    AddressManager addressif.AddressManager `name:"address_manager" optional:"true"`
    
    // 协议路由器 - 用于统一管理协议处理器
    ProtocolRouter protocolif.Router `name:"protocol_router" optional:"true"`
    
    // 连接管理 - 水位线控制和连接保护
    ConnManager connmgrif.ConnectionManager `name:"conn_manager" optional:"true"`
    
    // 连接门控 - 黑名单和连接拦截
    ConnGater connmgrif.ConnectionGater `name:"conn_gater" optional:"true"`
    
    // 配置
    Config *Config `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    Endpoint core.Endpoint `name:"endpoint"`
}

func Module() fx.Option {
    return fx.Module("endpoint",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

**注意**：以上 `ModuleInput` 结构与实际实现一致（2025-12-21 更新）。以下依赖现已集成：
- `ProtocolRouter`: 协议路由器，用于统一管理协议处理器。如果注入，`SetProtocolHandler` 会使用它；否则使用内部 handlers map。
- `ConnManager`: 连接管理器，在连接建立/断开时自动调用 `NotifyConnected`/`NotifyDisconnected`，支持水位线控制。
- `ConnGater`: 连接门控，在出站拨号前调用 `InterceptPeerDial`，在入站连接时调用 `InterceptAccept` 和 `InterceptSecured`。

## 生命周期

```go
func registerLifecycle(lc fx.Lifecycle, ep core.Endpoint) {
    lc.Append(fx.Hook{
        OnStart: func(ctx context.Context) error {
            return ep.Listen(ctx)
        },
        OnStop: func(ctx context.Context) error {
            return ep.Close()
        },
    })
}
```

## Realm 访问级别

| 级别 | 值 | 说明 |
|------|-----|------|
| Public | 0 | 任何人可加入，节点可被发现 |
| Protected | 1 | 需要 JoinKey，节点可被同 Realm 发现 |
| Private | 2 | 需要 JoinKey，节点不可被外部发现 |

## 相关文档

- [Realm 协议](../../../docs/01-design/protocols/application/04-realm.md)
- [消息传递协议](../../../docs/01-design/protocols/application/03-messaging.md)
- [连接协议](../../../docs/01-design/protocols/transport/04-connection.md)
- [pkg/interfaces/core/endpoint.go](../../../pkg/interfaces/core/endpoint.go)
