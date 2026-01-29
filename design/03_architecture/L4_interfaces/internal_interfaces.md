# 内部接口设计 (Internal Interfaces)

> **状态**: 可选设计  
> **更新日期**: 2026-01-23  
> **定位**: 定义 DeP2P 模块间的内部交互接口 (internal/*/interfaces/)

---

## ⚠️ 重要说明

**内部接口层是可选设计，大多数模块不需要。**

### 实际实现策略

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          简化后的接口策略                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  公共接口 (pkg/interfaces/)                                                 │
│  ─────────────────────────────                                              │
│  • 统一在 pkg/interfaces/ 定义                                              │
│  • 所有模块直接实现公共接口                                                   │
│  • 无需中间层                                                                 │
│                                                                             │
│  内部接口 (internal/*/interfaces/) - 可选                                    │
│  ──────────────────────                                                     │
│  • 仅在模块内部有多个子组件需要相互依赖时使用                                 │
│  • 例如：transport/quic/ 和 transport/tcp/ 需要共同接口                       │
│  • 大多数模块不需要此层                                                       │
│                                                                             │
│  事件类型                                                                     │
│  ────────                                                                   │
│  • 统一在 pkg/types/events.go 定义                                           │
│  • 不分散在各模块的 events/ 目录                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 何时需要内部接口？

**需要内部接口的场景**：
- 模块内部有多个实现（如 `transport/quic/` 和 `transport/tcp/`）
- 这些实现需要共享接口定义
- 模块外部不需要看到这些接口

**不需要内部接口的场景**（大多数情况）：
- 模块只有一个实现
- 直接实现 `pkg/interfaces/` 中的公共接口即可
- 例如：`identity/`, `eventbus/`, `connmgr/` 等

---

## 概述

内部接口用于模块间的内部交互，位于各模块的 `interfaces/` 子包中。**这是可选设计，仅在需要时使用。**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     公共接口 vs 内部接口                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  公共接口 (pkg/interfaces/)                                                  │
│  ─────────────────────────────                                              │
│  • 对外暴露，可被外部项目导入                                                 │
│  • 稳定性要求高，遵循语义化版本                                               │
│  • 只包含最小必要的 API                                                      │
│                                                                             │
│  内部接口 (internal/*/interfaces/)                                           │
│  ──────────────────────                                                     │
│  • 仅内部使用，外部不可导入                                                   │
│  • 可自由演进，无版本兼容要求                                                 │
│  • 继承公共接口 + 添加内部专用方法                                            │
│  • 用于模块间解耦                                                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 接口继承模式

### 核心原则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          接口继承模式                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  层次结构：                                                                  │
│  ──────────                                                                 │
│                                                                             │
│    pkg/interfaces/<domain>/         公共接口（最小 API）                      │
│            │                                                                │
│            │  嵌入继承（embedding）                                          │
│            ▼                                                                │
│    internal/<domain>/<module>/      内部接口（公共 API + 内部 API）          │
│        interfaces/                                                          │
│            │                                                                │
│            │  实现（implements）                                             │
│            ▼                                                                │
│    internal/<domain>/<module>/      具体实现（struct）                       │
│        impl/                                                                │
│                                                                             │
│  好处：                                                                      │
│  ──────                                                                     │
│  • 公共接口保持最小化和稳定                                                   │
│  • 内部接口可以自由添加内部方法                                               │
│  • 实现自动满足公共接口（类型兼容）                                           │
│  • 外部用户只看到公共 API                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 继承示例

```
// ═══════════════════════════════════════════════════════════════════════════
// 步骤 1: 公共接口（对外暴露，最小 API）
// 位置：pkg/interfaces/host.go
// ═══════════════════════════════════════════════════════════════════════════

interface Host {
    ID() → NodeID
    Addrs() → []Multiaddr
    Connect(ctx, peer: PeerInfo) → error
    NewStream(ctx, peer: NodeID, proto: string) → Stream, error
    SetStreamHandler(proto: string, handler: StreamHandler)
    Close() → error
}

// ═══════════════════════════════════════════════════════════════════════════
// 步骤 2: 内部接口（继承公共 + 添加内部方法）
// 位置：internal/core/host/interfaces/host.go
// ═══════════════════════════════════════════════════════════════════════════

interface InternalHost extends Host {
    // ════════════════════ 以下为内部专用方法 ════════════════════
    
    Transport() → Transport           // 返回传输层（内部组件需要）
    Security() → SecureTransport      // 返回安全层
    ConnMgr() → ConnectionManager     // 返回连接管理器
    Muxer() → Muxer                   // 返回流复用器
    Network() → Network               // 返回底层网络
    NotifyConnect(Notifiee)           // 注册连接通知（内部事件）
    NotifyDisconnect(Notifiee)        // 注册断开通知
    InternalDial(ctx, peer: NodeID) → Connection, error  // 内部拨号
}

// ═══════════════════════════════════════════════════════════════════════════
// 步骤 3: 具体实现（实现内部接口，自动满足公共接口）
// 位置：internal/core/host/impl/host.go
// ═══════════════════════════════════════════════════════════════════════════

class HostImpl implements InternalHost {
    // 实现所有方法...
}

// 类型断言验证
assert HostImpl implements InternalHost  // 满足内部接口
assert HostImpl implements Host          // 自动满足公共接口
```

---

## 内部接口目录结构

```
internal/
│
├── protocol/                      # Protocol Layer
│   ├── node/
│   │   └── interfaces/
│   │       └── node.go            # 内部 Node 接口
│   ├── messaging/
│   │   └── interfaces/
│   │       └── messaging.go       # 内部 Messaging 接口
│   ├── pubsub/
│   │   └── interfaces/
│   │       └── pubsub.go          # 内部 PubSub 接口
│   ├── streams/
│   │   └── interfaces/
│   │       └── streams.go         # 内部 Streams 接口
│   └── liveness/
│       └── interfaces/
│           └── liveness.go        # 内部 Liveness 接口
│
├── realm/                         # Realm Layer
│   └── realm/
│       └── interfaces/
│           └── realm.go           # 内部 Realm 接口
│
├── core/                          # Core 核心域
│   ├── host/
│   │   └── interfaces/
│   │       └── host.go            # 内部 Host 接口
│   ├── identity/
│   │   └── interfaces/
│   │       └── identity.go        # 内部 Identity 接口
│   ├── transport/
│   │   └── interfaces/
│   │       └── transport.go       # 内部 Transport 接口
│   ├── security/
│   │   └── interfaces/
│   │       └── security.go        # 内部 Security 接口
│   ├── muxer/
│   │   └── interfaces/
│   │       └── muxer.go           # 内部 Muxer 接口
│   ├── connmgr/
│   │   └── interfaces/
│   │       └── connmgr.go         # 内部 ConnMgr 接口
│   ├── relay/
│   │   └── interfaces/
│   │       └── relay.go           # 内部 Relay 接口
│   ├── nat/
│   │   └── interfaces/
│   │       └── nat.go             # 内部 NAT 接口
│   └── storage/                   # ★ Storage 存储服务
│       └── engine/
│           └── engine.go          # InternalEngine/Batch/Iterator/Transaction
│
└── discovery/                     # Discovery 发现域
    ├── coordinator/
    │   └── interfaces/
    │       └── coordinator.go     # 内部 Coordinator 接口
    ├── dht/
    │   └── interfaces/
    │       └── dht.go             # 内部 DHT 接口
    ├── bootstrap/
    │   └── interfaces/
    │       └── bootstrap.go       # 内部 Bootstrap 接口
    ├── mdns/
    │   └── interfaces/
    │       └── mdns.go            # 内部 mDNS 接口
    └── rendezvous/
        └── interfaces/
            └── rendezvous.go      # 内部 Rendezvous 接口
```

---

## Protocol Layer 内部接口

### Node 内部接口

```
// 位置：根目录 (API Layer 无独立内部接口)

interface InternalNode extends Node {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Host() → InternalHost             // 返回底层 Host（内部组件使用）
    RealmManager() → InternalRealmManager  // 返回 Realm 管理器
    Config() → NodeConfig             // 返回节点配置
    RegisterService(name: string, service: Service) → error
    GetService(name: string) → Service, bool
}

interface Service {
    Start(ctx) → error
    Stop(ctx) → error
}
```

### Messaging 内部接口

```
// 位置：internal/protocol/messaging/interfaces/messaging.go

interface InternalMessagingService extends MessagingService {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Host() → InternalHost             // 返回底层 Host
    SendRaw(ctx, peer: NodeID, stream: Stream, data: bytes) → error
    SetInternalHandler(proto: string, handler: InternalHandler)
}

type InternalHandler = (ctx, stream: Stream, from: NodeID, data: bytes) → error
```

### PubSub 内部接口

```
// 位置：internal/protocol/pubsub/interfaces/pubsub.go

interface InternalPubSubService extends PubSubService {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Host() → InternalHost             // 返回底层 Host
    Router() → Router                 // 返回消息路由器
    Validator() → Validator           // 返回消息验证器
    SetTopicValidator(topic: string, validator: TopicValidator) → error
    Blacklist() → Blacklist           // 获取黑名单
    Whitelist() → Whitelist           // 获取白名单
}

interface Router {
    AddPeer(peer: NodeID, protos: []string)
    RemovePeer(peer: NodeID)
    GetPeers(topic: string) → []NodeID
}

enum ValidationResult {
    Accept,
    Reject,
    Ignore
}
```

---

## Realm Layer 内部接口

### Realm 内部接口

```
// 位置：internal/realm/interfaces/realm.go

interface InternalRealm extends Realm {
    // ════════════════════ 内部专用方法 ════════════════════
    
    PSK() → PSK                       // 返回预共享密钥（内部使用）
    InternalMessaging() → InternalMessagingService
    InternalPubSub() → InternalPubSubService
    Authenticator() → Authenticator   // 返回成员认证器
    MemberCache() → MemberCache       // 返回成员缓存
    NotifyMemberJoin(peer: NodeID)    // 通知成员加入
    NotifyMemberLeave(peer: NodeID)   // 通知成员离开
}

interface InternalRealmManager extends RealmManager {
    InternalJoin(ctx, psk: PSK) → InternalRealm, error
    ValidatePSK(psk: PSK) → error
}

interface Authenticator {
    Challenge(peer: NodeID) → bytes, error
    Respond(challenge: bytes) → bytes, error
    Verify(peer: NodeID, challenge: bytes, response: bytes) → error
}

interface MemberCache {
    Add(peer: NodeID)
    Remove(peer: NodeID)
    IsMember(peer: NodeID) → bool
    Members() → []NodeID
}
```

---

## Core 核心域内部接口

### Host 内部接口

```
// 位置：internal/core/host/interfaces/host.go

interface InternalHost extends Host {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Transport() → Transport
    Security() → SecureTransport
    Muxer() → Muxer
    ConnMgr() → ConnectionManager
    Network() → Network
    Peerstore() → Peerstore
    NotifyConnect(Notifiee)
    NotifyDisconnect(Notifiee)
    InternalDial(ctx, peer: NodeID) → Connection, error
    InternalListen(addr: Multiaddr) → error
}

interface Network {
    Conns() → []Connection
    ConnsToPeer(peer: NodeID) → []Connection
    DialPeer(ctx, peer: NodeID) → Connection, error
    ClosePeer(peer: NodeID) → error
}

interface Notifiee {
    Connected(Connection)
    Disconnected(Connection)
    OpenedStream(Stream)
    ClosedStream(Stream)
}

interface Peerstore {
    AddAddr(peer: NodeID, addr: Multiaddr, ttl: Duration)
    AddAddrs(peer: NodeID, addrs: []Multiaddr, ttl: Duration)
    Addrs(peer: NodeID) → []Multiaddr
    ClearAddrs(peer: NodeID)
    PeersWithAddrs() → []NodeID
}
```

### Transport 内部接口

```
// 位置：internal/core/transport/interfaces/transport.go

interface InternalTransport extends Transport {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Upgrader() → Upgrader             // 返回连接升级器
    Resolver() → Resolver             // 返回地址解析器
    InternalDial(ctx, addr: Multiaddr) → Connection, error
}

interface Upgrader {
    Upgrade(ctx, conn: NetConn, direction: Direction, peer: NodeID) → Connection, error
}

interface Resolver {
    Resolve(ctx, addr: Multiaddr) → []Multiaddr, error
}
```

### Security 内部接口

```
// 位置：internal/core/security/interfaces/security.go

interface InternalSecureTransport extends SecureTransport {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Handshaker() → Handshaker
    Encryptor() → Encryptor
}

interface Handshaker {
    Handshake(ctx, conn: NetConn, isInitiator: bool) → SecureConn, error
}

interface InternalSecureConn extends SecureConn {
    ConnectionState() → ConnectionState
}

struct ConnectionState {
    LocalPeer: NodeID
    RemotePeer: NodeID
    RemoteKey: PublicKey
    Authenticated: bool
}

interface Encryptor {
    Encrypt(plaintext: bytes) → bytes, error
    Decrypt(ciphertext: bytes) → bytes, error
}
```

### Relay 内部接口

```
// 位置：internal/core/relay/interfaces/relay.go

// ★ v2.0 三层架构：DHT 是权威目录，Relay 是缓存/信令/保底
//
// Relay 三大职责内部接口：
//   1. 缓存加速层（AddressBook 管理，非权威目录）
//   2. 打洞协调信令（SignalingChannel 提供，打洞协调必备）
//   3. 数据通信保底（Circuit 管理）
//
// ★ 显式配置原则（ADR-0010）：
//   - Relay 地址需要显式配置，不支持自动发现
//   - 信令通道来自显式配置的 Relay 连接

interface InternalRelayService extends RelayService {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Host() → InternalHost
    CircuitManager() → CircuitManager
    ReservationManager() → ReservationManager
    
    // ★ 地址簿管理（缓存加速层，非权威目录）
    AddressBookManager() → InternalAddressBook
    
    // ★ 信令通道（打洞协调）
    SignalingChannel() → SignalingChannel
}

interface InternalRelayManager extends RelayManager {
    AutoRelay() → AutoRelay
    SelectRelay(ctx) → NodeID, error
    
    // ★ 连接保留（打洞成功后）
    KeepConnection() → bool
    SetKeepConnection(keep: bool)
}

interface CircuitManager {
    CreateCircuit(ctx, src: NodeID, dst: NodeID) → Circuit, error
    GetCircuit(id: string) → Circuit, bool
    CloseCircuit(id: string) → error
    ListCircuits() → []Circuit
}

// ★ RelayCircuit 接口（v0.2.26 新增，参见 ADR-0011）
// 实现 Connection 接口，支持多路复用
interface InternalRelayCircuit extends Connection {
    // 控制通道
    ControlStream() → Stream
    
    // 数据通道 Muxer
    Muxer() → MuxedConn
    
    // 电路状态
    State() → CircuitState
    SetState(state: CircuitState)
    
    // 远程端点
    RelayPeer() → NodeID
    
    // 配额管理
    BytesUsed() → int64
    MaxBytes() → int64
    
    // 心跳管理
    LastActivity() → Time
    SendKeepAlive(ctx) → error
    
    // 流接受循环
    AcceptStreamLoop(host: Host)
    
    // 优雅关闭
    GracefulClose(ctx) → error
}

// ★ 电路状态枚举（v0.2.26 新增）
enum CircuitState {
    CircuitCreating,   // STOP 握手中
    CircuitActive,     // 电路活跃
    CircuitStale,      // 心跳超时
    CircuitClosed      // 已关闭
}

interface Circuit {
    ID() → string
    Source() → NodeID
    Destination() → NodeID
    BytesForwarded() → uint64
    Close() → error
}

interface ReservationManager {
    Reserve(peer: NodeID, ttl: Duration) → Reservation, error
    GetReservation(peer: NodeID) → Reservation, bool
    CancelReservation(peer: NodeID) → error
}

interface AutoRelay {
    Enable() → error
    Disable() → error
    IsEnabled() → bool
    CurrentRelay() → NodeID, bool
}

// ★ 内部地址簿接口（v2.0：Relay 作为缓存加速层，DHT 是权威目录）
interface InternalAddressBook {
    // 注册/更新成员地址（本地缓存）
    Register(id: NodeID, addrs: []Multiaddr, ttl: Duration) → error
    
    // 查询成员地址（缓存查询，非权威）
    Query(id: NodeID) → []Multiaddr, error
    
    // 批量查询（缓存查询，非权威）
    QueryBatch(ids: []NodeID) → map[NodeID][]Multiaddr, error
    
    // 移除成员
    Remove(id: NodeID) → error
    
    // 订阅地址变化
    Subscribe() → channel<AddressChange>
    
    // 统计
    Stats() → AddressBookStats
}

struct AddressChange {
    ID: NodeID
    OldAddrs: []Multiaddr
    NewAddrs: []Multiaddr
    Removed: bool
}

struct AddressBookStats {
    TotalMembers: int
    TotalAddrs: int
    LastUpdate: Time
}
```

### NAT 内部接口

```
// 位置：internal/core/nat/interfaces/nat.go

interface InternalNATService extends NATService {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Host() → InternalHost
    HolePuncher() → HolePuncher
    PortMapper() → PortMapper
    AutoNAT() → AutoNAT
    Reachability() → InternalReachability
}

// ★ 打洞需要信令通道（来自显式配置的 Relay 连接）
//
// 约束（ADR-0010）：
//   - 信令通道默认来自显式配置的 Relay 连接
//   - 不支持自动发现 Relay 作为信令通道
//   - 如果没有配置 Relay，则无信令通道可用，跳过打洞阶段
interface HolePuncher {
    DirectConnect(ctx, peer: NodeID, addrs: []Multiaddr) → error
        // 前置条件：必须有信令通道（来自显式配置的 Relay 连接）
        // 若无信令通道，返回 ErrNoSignalingChannel
    
    AcceptHolePunch(ctx, peer: NodeID) → error
    
    // ★ 信令通道依赖
    SetSignalingChannel(ch: SignalingChannel) → error
    HasSignalingChannel() → bool
}

// ★ 信令通道抽象（打洞协调的必要前提）
//
// 来源（ADR-0010）：
//   - 主要来源：显式配置的 Relay 连接
//   - 不支持：自动发现 Relay 作为信令通道
interface SignalingChannel {
    SendSync(ctx, target: NodeID, msg: HolePunchSync) → error
    ReceiveSync(ctx) → HolePunchSync, error
    Ready() → bool
}

interface PortMapper {
    AddMapping(proto: string, internalPort: int, externalPort: int, desc: string) → error
    DeleteMapping(proto: string, internalPort: int) → error
    GetMapping(proto: string, internalPort: int) → int, bool
    Mappings() → []Mapping
}

interface AutoNAT {
    Status() → AutoNATStatus
    PublicAddr() → Multiaddr, bool
    CheckReachability(ctx) → Reachability, error
}

// ★ 可达性验证（候选地址 → 可发布地址）
interface InternalReachability {
    // 验证地址是否可达
    Verify(ctx, addr: Multiaddr) → bool, error
    
    // 返回已验证的可发布地址
    PublishableAddrs() → []Multiaddr
    
    // 订阅可达性变化
    Subscribe() → channel<ReachabilityChange>
}
```

### NetworkMonitor 内部接口

```
// 位置：internal/core/network/interfaces/monitor.go

interface InternalNetworkMonitor extends NetworkMonitor {
    // ════════════════════ 内部专用方法 ════════════════════
    
    // InterfaceWatcher 返回网络接口监控器
    InterfaceWatcher() → InterfaceWatcher
    
    // ForceCheck 强制检查网络状态（用于测试）
    ForceCheck() → NetworkChangeEvent
    
    // SetRebindCallback 设置 Socket 重绑定回调
    SetRebindCallback(callback: RebindCallback)
}

interface InterfaceWatcher {
    // Watch 开始监控网络接口变化
    Watch(ctx) → channel<InterfaceEvent>, error
    
    // CurrentInterfaces 返回当前活跃接口列表
    CurrentInterfaces() → []NetworkInterface
    
    // Stop 停止监控
    Stop() → error
}

struct InterfaceEvent {
    Type: InterfaceEventType    // Added / Removed / Changed
    Interface: NetworkInterface
    Timestamp: Time
}

enum InterfaceEventType {
    InterfaceAdded,
    InterfaceRemoved,
    InterfaceChanged
}

type RebindCallback = (oldAddrs, newAddrs: []string) → error
```

### JitterTolerance 内部接口

```
// 位置：internal/core/connmgr/interfaces/jitter.go

interface InternalJitterTolerance extends JitterTolerance {
    // ════════════════════ 内部专用方法 ════════════════════
    
    // GetPeerState 获取指定节点的抖动状态
    GetPeerState(peer: NodeID) → DisconnectedPeerState, bool
    
    // SetConfig 动态更新配置
    SetConfig(config: JitterConfig)
    
    // ForceReconnect 强制立即重连（忽略退避）
    ForceReconnect(ctx, peer: NodeID) → error
    
    // CancelReconnect 取消重连计划
    CancelReconnect(peer: NodeID)
    
    // MaintenanceLoop 启动维护循环
    MaintenanceLoop(ctx)
}

struct DisconnectedPeerState {
    NodeID: NodeID
    DisconnectedAt: Time
    ReconnectAttempts: int
    NextReconnectAt: Time
    State: JitterState
    LastError: error
}

enum JitterState {
    StateConnected,     // 已连接
    StateHeld,          // 断连保持
    StateReconnecting,  // 正在重连
    StateRemoved        // 已移除
}
```

### Identity 内部接口

```
// 位置：internal/core/identity/interfaces/identity.go

interface InternalIdentityManager extends IdentityManager {
    // ════════════════════ 内部专用方法 ════════════════════
    
    KeyStore() → KeyStore
    GenerateKey(keyType: KeyType) → PrivateKey, error
    ImportKey(data: bytes) → PrivateKey, error
    ExportKey() → bytes, error
}

interface KeyStore {
    Has(id: string) → bool
    Put(id: string, key: PrivateKey) → error
    Get(id: string) → PrivateKey, error
    Delete(id: string) → error
    List() → []string, error
}
```

### ★ Storage 内部接口

Storage 模块是一个典型的需要内部接口的场景：公共接口 `Engine` 只暴露基础操作，
而内部需要更多能力（Batch、Iterator、Transaction）供其他模块使用。

```
// 位置：internal/core/storage/engine/engine.go

interface InternalEngine extends Engine {
    // ════════════════════ 内部专用方法 ════════════════════
    
    // 批量操作
    NewBatch() → Batch
    Write(batch: Batch) → error
    
    // 迭代器
    NewIterator(opts: *IteratorOptions) → Iterator
    NewPrefixIterator(prefix: bytes) → Iterator
    
    // 事务
    NewTransaction(writable: bool) → Transaction
    
    // 维护
    Start() → error
    Compact() → error
    Sync() → error
    Stats() → *Stats
}

interface Batch {
    Put(key, value: bytes)
    Delete(key: bytes)
    Write() → error
    Reset()
    Size() → int
}

interface Iterator {
    First() → bool
    Next() → bool
    Valid() → bool
    Key() → bytes
    Value() → bytes
    Close()
    Error() → error
}

interface Transaction {
    Get(key: bytes) → bytes, error
    Set(key, value: bytes) → error
    Delete(key: bytes) → error
    Commit() → error
    Discard()
}
```

---

## Discovery 发现域内部接口

### Coordinator 内部接口

```
// 位置：internal/discovery/coordinator/interfaces/coordinator.go

interface InternalCoordinator extends DiscoveryService {
    // ════════════════════ 内部专用方法 ════════════════════
    
    Host() → InternalHost
    DHT() → InternalDHTFinder
    Rendezvous() → InternalRendezvousFinder
    MDNS() → InternalMDNSFinder
    Bootstrap() → InternalBootstrapFinder
    RefreshRoutingTable(ctx) → error
    RunBootstrap(ctx) → error
}
```

### DHT 内部接口

```
// 位置：internal/discovery/dht/interfaces/dht.go

// ★ DHT 发布约束：
//   仅发布经过 Reachability 验证的地址
//   候选地址必须通过可达性验证后才能发布

interface InternalDHTFinder extends DHTFinder {
    // ════════════════════ 内部专用方法 ════════════════════
    
    RoutingTable() → RoutingTable
    Refresh(ctx) → error
    
    // ★ 可达性绑定（必须使用已验证地址）
    SetReachability(r: InternalReachability)
    
    // ★ 地址发布（内部方法）
    AnnounceVerified(ctx) → error
        // 使用 InternalReachability.PublishableAddrs() 获取可发布地址
        // 不使用候选地址
}

interface RoutingTable {
    Size() → int
    NearestPeers(id: NodeID, count: int) → []NodeID
    Update(id: NodeID) → bool
    Remove(id: NodeID)
}
```

### Bootstrap 内部接口

```
// 位置：internal/discovery/bootstrap/interfaces/bootstrap.go

interface InternalBootstrapFinder extends BootstrapFinder {
    // ════════════════════ 内部专用方法 ════════════════════
    
    BootstrapPeers() → []PeerInfo
    SetBootstrapPeers(peers: []PeerInfo)
}
```

---

## 接口设计原则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          内部接口设计原则                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  原则 1: 继承公共接口                                                        │
│  ──────────────────────                                                     │
│  内部接口通过嵌入（embedding）继承公共接口                                     │
│  确保实现自动满足公共接口                                                     │
│                                                                             │
│  原则 2: 内部方法明确标注                                                    │
│  ──────────────────────                                                     │
│  内部专用方法用注释明确标注                                                   │
│  便于区分公共 API 和内部 API                                                 │
│                                                                             │
│  原则 3: 最小内部暴露                                                        │
│  ──────────────────                                                         │
│  只在内部接口中添加真正需要的方法                                             │
│  避免过度暴露内部实现细节                                                     │
│                                                                             │
│  原则 4: 单向依赖                                                            │
│  ────────────────                                                           │
│  内部接口可以依赖公共接口                                                     │
│  公共接口不能依赖内部接口                                                     │
│                                                                             │
│  原则 5: 接口隔离                                                            │
│  ────────────────                                                           │
│  大接口拆分为多个小接口                                                       │
│  依赖方只依赖需要的接口                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [public_interfaces.md](public_interfaces.md) | 公共接口设计 |
| [component_interface_map.md](component_interface_map.md) | 组件-接口映射 |
| [fx_lifecycle.md](fx_lifecycle.md) | Fx + Lifecycle 模式 |

---

**最后更新**：2026-01-27（新增 RelayCircuit 内部接口 - ADR-0011）
