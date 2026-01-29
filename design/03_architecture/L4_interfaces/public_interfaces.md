# 公共接口设计 (Public Interfaces)

> 定义 DeP2P 对外暴露的稳定接口契约 (pkg/interfaces/)

---

## 概述

公共接口是 DeP2P 对外的稳定 API 契约，位于 `pkg/interfaces/`。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          公共接口特性                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  • 对外可见：可被外部项目导入                                                  │
│  • 稳定性高：遵循语义化版本                                                    │
│  • 向后兼容：变更需要版本兼容考虑                                               │
│  • 文档完整：每个方法都有完整文档                                               │
│  • 最小原则：只暴露必要的方法                                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 重要说明：系统接口 vs 用户 API

**本文档定义的是系统接口（System Interfaces）**，位于 `pkg/interfaces/`，用于：
- 模块间契约定义
- 依赖注入
- 技术完备性

**用户 API（User API）** 在根目录（`dep2p.go`, `realm_api.go` 等），对系统接口进行包装：
- 只暴露用户需要的方法
- 隐藏内部细节（Router, Gateway, PSK 等）
- 返回具体类型（`*Node`, `*Realm`）而非接口

```
用户代码
    ↓ 使用
用户 API (根目录: dep2p.Node, dep2p.Realm, ...)
    ↓ 包装
系统接口 (pkg/interfaces/: interfaces.Realm, interfaces.Host, ...) ← 本文档
    ↓ 定义契约
内部实现 (internal/: realm/, core/host, ...)
```

**示例**：
- 用户调用：`realm.Messaging()` → 返回 `*dep2p.Messaging`
- 系统接口：`interfaces.Realm.Messaging()` → 返回 `interfaces.Messaging`
- 用户不能调用：`realm.Router()`, `realm.PSK()` （这些是系统接口的内部方法）

详见：[L4_interfaces/README.md](README.md) 的"系统接口 vs 用户 API"章节。

---

## 公共接口目录结构

```
pkg/interfaces/
│
├── ═══════════════════ API Layer 接口 ═══════════════════
│
├── node.go                      # 用户入口 Facade
│
├── ═══════════════════ Protocol Layer 接口 ══════════════
│
├── messaging.go                 # 点对点消息
├── pubsub.go                    # 发布订阅
├── streams.go                   # 流管理
├── liveness.go                  # 存活检测
│
├── ═══════════════════ Realm Layer 接口 ═════════════════
│
├── realm.go                     # Realm 管理
│
├── ═══════════════════ Core Layer 接口 ══════════════════
│
├── host.go                      # 网络主机（核心门面）
├── transport.go                 # 传输层
├── security.go                  # 安全层
├── muxer.go                     # 流复用
├── connmgr.go                   # 连接管理（含水位线、裁剪控制）
├── identity.go                  # 身份管理
├── peerstore.go                 # 节点存储
├── swarm.go                     # 连接群
├── protocol.go                  # 协议路由
├── upgrader.go                  # 连接升级
├── resource.go                  # 资源管理
├── eventbus.go                  # 事件总线
├── metrics.go                   # 指标接口
├── storage.go                   # ★ 存储引擎（Engine 基础接口）
├── bandwidth.go                 # ★ 带宽统计（按 Peer/Protocol）
├── netmon.go                    # ★ 连接健康监控（状态机）
├── pathhealth.go                # ★ 路径健康管理（RTT/评分/切换）
├── recovery.go                  # ★ 网络恢复管理
│
├── ═══════════════════ Discovery Layer 接口 ═════════════
│
├── discovery.go                 # 发现协调器和 DHT
│
├── ═══════════════════ 可观测性接口 ═════════════════════
│
└── health.go                    # 健康检查
```

---

## API Layer 接口

### Node 接口

Node 是 DeP2P 的用户入口 Facade，封装所有顶层操作。

```
// 位置：pkg/interfaces/node.go

interface Node {
    // ═══════════════════════════════════════════════════════════════════════
    // 身份信息
    // ═══════════════════════════════════════════════════════════════════════
    
    ID() → NodeID                              // 节点唯一标识
    ListenAddrs() → []Multiaddr                // 监听地址列表
    AdvertisedAddrs() → []Multiaddr            // 对外公告地址
    
    // ═══════════════════════════════════════════════════════════════════════
    // Realm 操作
    // ═══════════════════════════════════════════════════════════════════════
    
    JoinRealm(ctx, name: string, psk: PSK, opts...) → Realm, error
        // 使用 PSK 加入指定 Realm，如已加入其他 Realm 会先离开
    
    CurrentRealm() → Realm | nil               // 返回当前 Realm
    
    // ═══════════════════════════════════════════════════════════════════════
    // ★ Relay 能力（v2.0 统一设计）
    // ═══════════════════════════════════════════════════════════════════════
    //
    // v2.0 三层架构：
    //   Layer 1: DHT（★ 权威目录）— 存储签名 PeerRecord
    //   Layer 2: 缓存加速层 — Peerstore / MemberList / Relay 地址簿
    //   Layer 3: 连接策略 — 直连 → 打洞 → Relay 兜底
    //
    // Relay 三大职责：
    //   1. 缓存加速层：维护地址簿，作为 DHT 本地缓存（非权威目录）
    //   2. 打洞协调信令：打洞协调的必要前提（来自显式配置的 Relay）
    //   3. 数据通信保底：直连/打洞失败时转发数据
    //
    // ★ 核心原则：DHT 是权威目录，Relay 地址簿是缓存加速层
    // ★ 显式配置原则（ADR-0010）：Relay 地址需要显式配置，不支持自动发现
    //
    // 连接优先级（INV-003）：
    //   直连 → 打洞（需信令通道）→ Relay
    //
    // 打洞成功后保留 Relay：
    //   作为备份，直连断开时可快速切换
    //
    // 详见 ADR-0010: Relay 明确配置
    //
    
    // 作为 Relay 服务器
    EnableRelay(ctx) → error                   // 启用（无参数，内置默认值）
    DisableRelay(ctx) → error                  // 禁用
    IsRelayEnabled() → bool                    // 查询状态
    RelayStats() → RelayStats                  // 统计信息
    
    // 作为 Relay 客户端（使用中继）
    SetRelayAddr(addr: Multiaddr) → error      // 设置中继地址
    RemoveRelayAddr() → error                  // 移除中继地址
    RelayAddr() → Multiaddr, bool              // 获取当前中继地址
    
    // ═══════════════════════════════════════════════════════════════════════
    // Bootstrap 能力
    // ═══════════════════════════════════════════════════════════════════════
    //
    // ★ 核心理念：Bootstrap 采用极简开关设计，无需配置参数
    // 详见 ADR-0009: Bootstrap 极简配置
    //
    
    // 作为 Bootstrap 服务器（项目方）
    EnableBootstrap(ctx) → error               // 启用（无参数，内置默认值）
    DisableBootstrap(ctx) → error              // 禁用
    IsBootstrapEnabled() → bool                // 查询状态
    BootstrapStats() → BootstrapStats          // 统计信息
    
    // 作为 Bootstrap 客户端（使用引导节点）
    AddBootstrapPeer(addr: Multiaddr) → error  // 添加引导节点
    RemoveBootstrapPeer(id: NodeID)            // 移除引导节点
    BootstrapPeers() → []Multiaddr             // 获取引导节点列表
    Bootstrap(ctx) → error                     // 手动触发 Bootstrap
    
    // ═══════════════════════════════════════════════════════════════════════
    // 网络变化处理
    // ═══════════════════════════════════════════════════════════════════════
    
    NetworkChange()                            // 通知节点网络可能已变化
    OnNetworkChange(callback: NetworkChangeCallback)  // 注册网络变化回调
    
    // ═══════════════════════════════════════════════════════════════════════
    // 生命周期
    // ═══════════════════════════════════════════════════════════════════════
    
    Close() → error                            // 关闭节点
}

type NetworkChangeCallback = (event: NetworkChangeEvent) → void

// ═══════════════════════════════════════════════════════════════════════
// 内置默认值（用户不可配置）
// ═══════════════════════════════════════════════════════════════════════
//
// Bootstrap 内置默认值:
//   MaxNodes: 50000                    // 最大存储节点数
//   PersistPath: ${DataDir}/bootstrap.db  // 持久化路径
//   ProbeInterval: 5 min               // 存活探测间隔
//   DiscoveryInterval: 10 min          // 主动发现间隔
//   NodeExpireTime: 24h                // 节点过期时间
//   ResponseK: 20                      // FIND_NODE 返回节点数
//
// Relay 内置默认值:
//   MaxReservations: 100               // 最大预留连接数
//   MaxDuration: 60s                   // 单次中继最大时长
//   KeepConnection: true               // 打洞成功后保留 Relay 作为备份
//   IdleTimeout: 30s                   // 空闲超时
//   ReservationTTL: 1h                 // ★ 预留有效期
//   RenewalInterval: 30min             // ★ 续租间隔（TTL/2）
//   RenewalWindow: 5min                // ★ 续租窗口（TTL 过期前开始尝试）
//   MaxRenewalFailures: 3              // ★ 最大续租失败次数
//
// DHT 地址发布参数:
//   DHTAddressTTL: 24h                 // ★ DHT 地址 TTL
//   DHTRefreshInterval: 12h            // ★ DHT 刷新间隔（TTL/2）
//
// 重试策略参数:
//   RetryInitialInterval: 1s           // ★ 初始重试间隔
//   RetryMultiplier: 2                 // ★ 退避因子
//   RetryMaxInterval: 30s              // ★ 最大重试间隔
//   RetryMaxAttempts: 5                // ★ 最大重试次数
//   RetryJitter: 0.2                   // ★ 抖动因子（±20%）
//

// 统计类型
struct RelayStats {
    ActiveConnections: int
    TotalConnections: uint64
    BytesForwarded: uint64
    Uptime: Duration
}
```

### Messaging 接口

Messaging 提供点对点消息传递。

```
// 位置：pkg/interfaces/messaging.go

interface MessagingService {
    Send(ctx, peer: NodeID, data: bytes) → error
        // 发送单向消息，不等待响应
    
    SendWithProto(ctx, peer: NodeID, proto: string, data: bytes) → error
        // 发送带协议标识的消息
    
    Request(ctx, peer: NodeID, data: bytes) → bytes, error
        // 发送请求并等待响应
    
    RequestWithProto(ctx, peer: NodeID, proto: string, data: bytes) → bytes, error
        // 发送带协议标识的请求
    
    OnMessage(proto: string, handler: MessageHandler)
        // 注册单向消息处理器
    
    OnRequest(proto: string, handler: RequestHandler)
        // 注册请求处理器
    
    RemoveHandler(proto: string)
        // 移除处理器
}

// 处理器类型
type MessageHandler = (ctx, from: NodeID, data: bytes) → void
type RequestHandler = (ctx, from: NodeID, data: bytes) → bytes, error
```

### PubSub 接口

PubSub 提供发布订阅能力。

```
// 位置：pkg/interfaces/pubsub.go

interface PubSubService {
    Join(topic: string) → Topic, error         // 加入主题
    Publish(ctx, topic: string, data: bytes) → error  // 发布消息
    Subscribe(topic: string) → Subscription, error    // 订阅主题
    Topics() → []string                        // 已加入的主题列表
}

interface Topic {
    Name() → string                            // 主题名称
    Publish(ctx, data: bytes) → error          // 发布消息
    Subscribe() → Subscription, error          // 订阅此主题
    Peers() → []NodeID                         // 主题中的节点
    Close() → error                            // 离开主题
}

interface Subscription {
    Topic() → string                           // 订阅的主题名称
    Next(ctx) → Message, error                 // 获取下一条消息
    Cancel()                                   // 取消订阅
}

struct Message {
    From: NodeID                               // 发送者
    Topic: string                              // 主题
    Data: bytes                                // 数据
    Timestamp: int64                           // 时间戳
    SeqNo: bytes                               // 序列号
}
```

### Streams 接口

Streams 提供双向流管理。

```
// 位置：pkg/interfaces/streams.go

interface StreamsService {
    Open(ctx, peer: NodeID, proto: string) → Stream, error
        // 打开到目标节点的流
    
    SetHandler(proto: string, handler: StreamHandler)
        // 设置协议处理器
    
    RemoveHandler(proto: string)
        // 移除协议处理器
}

interface Stream extends ReadWriteCloser {
    Protocol() → string                        // 流的协议标识
    RemotePeer() → NodeID                      // 远程节点 ID
    Reset() → error                            // 强制重置流
    
    // ★ 半关闭能力 (v1.3.0 修复)
    // 相关讨论：20260118-stream-reliability-design.md
    CloseWrite() → error                       // 关闭写端，发送 FIN，仍可读取
    CloseRead() → error                        // 关闭读端，停止接收数据
    
    // 超时控制
    SetDeadline(t: Time) → error               // 设置读写超时
    SetReadDeadline(t: Time) → error
    SetWriteDeadline(t: Time) → error
    
    // 状态查询
    IsClosed() → bool                          // 是否已完全关闭
    State() → StreamState                      // 流状态 (Open/HalfClosedLocal/...)
    Stat() → StreamStat                        // 流统计（字节数、打开时间等）
}

// Stream 状态枚举
enum StreamState {
    Open              // 双向打开
    HalfClosedLocal   // 本地关闭写
    HalfClosedRemote  // 远程关闭写
    Closed            // 完全关闭
    Reset             // 已重置
}

type StreamHandler = (stream: Stream) → void
```

> **★ 重要**：`CloseWrite()` 是实现请求-响应模式的关键。发送方使用它表示请求发送完毕，但仍可接收响应。
> 使用 `Close()` 会完全关闭流，导致无法接收响应。

### Liveness 接口

Liveness 提供节点存活检测。

```
// 位置：pkg/interfaces/liveness.go

interface LivenessService {
    Ping(ctx, peer: NodeID) → Duration, error  // 探测节点，返回 RTT
    IsAlive(peer: NodeID) → bool               // 节点是否存活
    LastSeen(peer: NodeID) → Time, bool        // 最后看到节点的时间
    OnStatusChange(callback: StatusCallback)   // 注册状态变化回调
    
    // ★ 增强查询 (v1.3.0 新增)
    GetStatus(peer: NodeID) → LivenessStatus, bool   // 获取详细状态
    GetStatuses() → map[NodeID]LivenessStatus        // 获取所有节点状态
}

// ★ 增强的状态结构 (v1.3.0)
struct LivenessStatus {
    // 基础字段
    Alive: bool                                // 是否存活
    LastSeen: Time                             // 最后确认时间
    LastRTT: Duration                          // 最后一次 RTT
    AvgRTT: Duration                           // 平均 RTT（滑动窗口）
    FailCount: int                             // 连续失败次数
    
    // ★ 增强统计字段 (v1.3.0 新增)
    MinRTT: Duration                           // 历史最小 RTT（最佳延迟）
    MaxRTT: Duration                           // 历史最大 RTT（最差延迟）
    TotalPings: int                            // 总 Ping 次数
    SuccessCount: int                          // 成功次数
    SuccessRate: float64                       // 成功率 (0.0 - 1.0)
}

type StatusCallback = (peer: NodeID, alive: bool) → void
```

增强字段用途：
- **MinRTT/MaxRTT**: 用于评估网络稳定性和抖动
- **TotalPings/SuccessCount**: 用于计算历史可靠性
- **SuccessRate**: 用于快速判断节点质量

---

## Realm Layer 接口

### Realm 接口

Realm 代表业务隔离域。

```
// 位置：pkg/interfaces/realm.go

interface Realm {
    // ═══════════════════════════════════════════════════════════════════════
    // 基本信息
    // ═══════════════════════════════════════════════════════════════════════
    
    Name() → string                            // Realm 显示名称
    ID() → RealmID                             // Realm 唯一标识（从 PSK 派生）
    
    // ═══════════════════════════════════════════════════════════════════════
    // ★ "仅 ID 连接"支持（核心特性）
    // ═══════════════════════════════════════════════════════════════════════
    //
    // Realm 内支持使用纯 NodeID 连接，系统自动发现地址
    // v2.0 地址发现优先级：Peerstore → MemberList → DHT（★ 权威）→ Relay 地址簿（缓存）
    //
    // ★ Connect 成功 = 可通信（核心语义保证）
    // ─────────────────────────────────────
    // Connect 返回成功时，必须保证：
    //   1. 传输层连接已建立（直连 / 打洞 / Relay）
    //   2. Realm 认证已完成（PSK 验证）
    //   3. 协议协商已完成（可立即通信）
    // 
    // 这意味着用户不会遇到"连接成功但无法通信"的情况（避免抽象泄漏）
    //
    
    Connect(ctx, target: NodeID) → Connection, error
        // 使用纯 NodeID 连接 Realm 成员
        // 系统自动发现地址，尝试直连 → 打洞 → Relay 保底
        // ★ 返回成功时保证可通信（传输 + 认证 + 协议协商完成）
    
    // ═══════════════════════════════════════════════════════════════════════
    // 成员管理
    // ═══════════════════════════════════════════════════════════════════════
    
    Members() → []NodeID                       // 已知成员列表
    MemberCount() → int                        // 成员数量
    IsMember(id: NodeID) → bool                // 检查是否为成员
    
    // ═══════════════════════════════════════════════════════════════════════
    // 服务入口
    // ═══════════════════════════════════════════════════════════════════════
    
    Messaging() → MessagingService             // 点对点消息服务
    PubSub() → PubSubService                   // 发布订阅服务
    Streams() → StreamsService                 // 流管理服务
    Liveness() → LivenessService               // 存活检测服务
    Discovery() → DiscoveryService             // Realm 内发现服务
    
    // ═══════════════════════════════════════════════════════════════════════
    // 生命周期
    // ═══════════════════════════════════════════════════════════════════════
    
    Leave(ctx) → error                         // 离开当前 Realm
    Context() → Context                        // Realm 上下文
    
    // ═══════════════════════════════════════════════════════════════════════
    // 注意：Relay 能力已统一到 Node 级别
    // ═══════════════════════════════════════════════════════════════════════
    //
    // Realm 内连接使用 Node 级 Relay：
    //   node.SetRelayAddr(addr)   // 设置 Relay
    //   realm.Connect(targetID)   // 连接自动使用 Node 的 Relay
    //
    // v2.0 Relay 三大职责：
    //   1. 缓存加速层（地址簿是 DHT 本地缓存，非权威目录）
    //   2. 打洞协调信令（打洞协调，需显式配置 Relay）
    //   3. 数据通信保底（直连/打洞失败时）
    //
    // ★ DHT 是权威目录，Relay 地址簿是缓存加速层
    //
}

interface RealmManager {
    Join(ctx, name: string, psk: PSK, opts...) → Realm, error
    Current() → Realm | nil
    Leave(ctx) → error
}

// ═══════════════════════════════════════════════════════════════════════
// Realm 内置默认值（用户不可配置）
// ═══════════════════════════════════════════════════════════════════════
//
//   MaxMembers: 1000                  // 最大成员数
//   MemberTTL: 24h                    // 成员信息过期时间
//   AddressBookSize: 10000            // 地址簿大小
//
```

---

## Core 核心域接口

### Host 接口

Host 是网络主机抽象。

```
// 位置：pkg/interfaces/host.go

interface Host {
    // ═══════════════════════════════════════════════════════════════════════
    // 身份
    // ═══════════════════════════════════════════════════════════════════════
    
    ID() → NodeID                              // 主机节点标识
    Addrs() → []Multiaddr                      // 监听地址
    
    // ═══════════════════════════════════════════════════════════════════════
    // 连接管理
    // ═══════════════════════════════════════════════════════════════════════
    
    Connect(ctx, peer: PeerInfo) → error       // 连接到节点
    Disconnect(peer: NodeID) → error           // 断开连接
    Peers() → []NodeID                         // 已连接节点列表
    Connectedness(peer: NodeID) → Connectedness  // 连接状态
    
    // ═══════════════════════════════════════════════════════════════════════
    // 流操作
    // ═══════════════════════════════════════════════════════════════════════
    
    NewStream(ctx, peer: NodeID, proto: string) → Stream, error
    SetStreamHandler(proto: string, handler: StreamHandler)
    RemoveStreamHandler(proto: string)
    
    // ═══════════════════════════════════════════════════════════════════════
    // 生命周期
    // ═══════════════════════════════════════════════════════════════════════
    
    Close() → error
}

struct PeerInfo {
    ID: NodeID
    Addrs: []Multiaddr
}

enum Connectedness {
    NotConnected,   // 未连接
    Connected,      // 已连接
    CanConnect,     // 可以连接（有地址）
    CannotConnect   // 无法连接
}
```

### Transport 接口

Transport 定义传输层抽象。

```
// 位置：pkg/interfaces/transport.go

interface Transport {
    Dial(ctx, addr: Multiaddr, peer: NodeID) → Connection, error
    Listen(addr: Multiaddr) → Listener, error
    Protocols() → []string                     // 支持的协议
    CanDial(addr: Multiaddr) → bool            // 是否可拨号
    Close() → error
}

interface Connection {
    LocalPeer() → NodeID
    RemotePeer() → NodeID
    LocalAddr() → Multiaddr
    RemoteAddr() → Multiaddr
    OpenStream(ctx) → Stream, error
    AcceptStream() → Stream, error
    IsClosed() → bool
    Close() → error
}

interface Listener {
    Accept() → Connection, error
    Addr() → Multiaddr
    Close() → error
}
```

### Security 接口

Security 提供安全握手和加密能力。

```
// 位置：pkg/interfaces/security.go

interface SecureTransport {
    SecureInbound(ctx, conn: NetConn, expectedPeer: NodeID) → SecureConn, error
    SecureOutbound(ctx, conn: NetConn, expectedPeer: NodeID) → SecureConn, error
    ID() → string                              // 安全协议标识
}

interface SecureConn extends NetConn {
    LocalPeer() → NodeID
    RemotePeer() → NodeID
    RemotePublicKey() → PublicKey
}
```

### Muxer 接口

Muxer 提供流多路复用能力。

```
// 位置：pkg/interfaces/muxer.go

interface Muxer {
    NewConn(conn: NetConn, isServer: bool) → MuxedConn, error
    ID() → string                              // 多路复用协议标识
}

interface MuxedConn {
    OpenStream(ctx) → Stream, error
    AcceptStream() → Stream, error
    Close() → error
    IsClosed() → bool
}
```

### ConnMgr 接口

ConnMgr 管理连接池。

```
// 位置：pkg/interfaces/connmgr.go

interface ConnectionManager {
    TagPeer(peer: NodeID, tag: string, value: int)
    UntagPeer(peer: NodeID, tag: string)
    Protect(peer: NodeID, tag: string)
    Unprotect(peer: NodeID, tag: string) → bool
    IsProtected(peer: NodeID, tag: string) → bool
    TrimOpenConns(ctx)
    GetInfo() → ConnMgrInfo
}

interface ConnectionGater {
    InterceptPeerDial(peer: NodeID) → bool
    InterceptAddrDial(peer: NodeID, addr: Multiaddr) → bool
    InterceptAccept(conn: NetConn) → bool
    InterceptSecured(direction: Direction, peer: NodeID, conn: NetConn) → bool
}

struct ConnMgrInfo {
    LowWater: int
    HighWater: int
    ConnCount: int
}
```

### Identity 接口

Identity 提供身份管理。

```
// 位置：pkg/interfaces/identity.go

interface Identity {
    ID() → NodeID
    PrivateKey() → PrivateKey
    PublicKey() → PublicKey
}

interface IdentityManager extends Identity {
    Sign(data: bytes) → bytes, error
    Verify(peer: NodeID, data: bytes, sig: bytes) → bool, error
}

interface PublicKey {
    Bytes() → bytes
    Type() → KeyType
    Verify(data: bytes, sig: bytes) → bool, error
}

interface PrivateKey {
    Bytes() → bytes
    Type() → KeyType
    Sign(data: bytes) → bytes, error
    Public() → PublicKey
}

enum KeyType {
    Ed25519,
    Secp256k1,
    RSA
}
```

### Relay 接口

Relay 提供中继服务（v2.0 三大职责：缓存加速 + 信令通道 + 数据保底）。

```
// 位置：pkg/interfaces/relay.go

// ★ v2.0 三层架构：DHT 是权威目录，Relay 是缓存/信令/保底
//
// Relay 三大职责：
//   1. 缓存加速层：维护地址簿，作为 DHT 本地缓存（非权威目录）
//   2. 打洞协调信令：打洞协调的必要前提（来自显式配置的 Relay）
//   3. 数据通信保底：直连/打洞失败时转发数据
//
// ★ 显式配置原则（ADR-0010）：Relay 地址需要显式配置，不支持自动发现

interface RelayService {
    DialViaRelay(ctx, relay: NodeID, target: NodeID) → Connection, error
    IsConnected(relay: NodeID) → bool
    Reserve(ctx, relay: NodeID) → error
    
    // 缓存加速层：查询地址（非权威，DHT 才是权威目录）
    QueryAddress(ctx, target: NodeID) → []Multiaddr, error
}

interface RelayManager {
    SetRelay(addr: Multiaddr) → error        // 设置 Relay
    RemoveRelay() → error                    // 移除 Relay
    Relay() → Multiaddr, bool                // 获取当前 Relay
    KeepConnection() → bool                  // 是否保留 Relay（打洞成功后）
}

interface RelayServer {
    Start(ctx) → error
    Stop() → error
    Stats() → RelayStats
    
    // 地址簿管理
    AddressBook() → AddressBook
}

// ★ AddressBook 说明（v2.0）：
// - 这是 Relay 服务器端的内部接口，不直接暴露给用户
// - ★ Relay 地址簿是缓存加速层，DHT 是权威目录
// - 客户端通过协议消息（AddressRegister/AddressQuery）与 Relay 交互
// - 详见 L6_domains/core_relay/design/overview.md 的协议定义
interface AddressBook {
    Register(id: NodeID, addrs: []Multiaddr) → error
    Query(id: NodeID) → []Multiaddr, error  // 缓存查询，非权威
    Remove(id: NodeID) → error
}
```

### NAT 接口

NAT 提供 NAT 穿透能力。

```
// 位置：pkg/interfaces/nat.go

// ★ NAT 三层能力说明：
//   1. 外部地址发现（STUN/观察地址）→ 仅知道地址，不保证可达
//   2. 打洞（直连建立）→ 需要信令通道（通常由 Relay 连接提供）
//   3. 中继（兜底通信）→ 由 Relay 提供
//
// GetExternalAddr() 返回的是"候选地址"，需经 Reachability 验证后才能发布到 DHT

interface NATService {
    GetExternalAddr() → Multiaddr, error
        // ★ 返回候选地址（非可发布地址）
        // 必须调用 ReachabilityService.Verify() 验证后才能发布到 DHT
    
    MapPort(ctx, proto: string, port: int) → int, error
    UnmapPort(proto: string, port: int) → error
    Reachability() → Reachability
    
    // ★ 打洞能力
    HolePunch(ctx, target: NodeID) → error
        // 打洞需要信令通道（通常由 Relay 连接提供）
        // 若无信令通道，返回 ErrNoSignalingChannel
}

enum Reachability {
    Unknown,        // 未知
    Public,         // 公网可达
    Private         // 内网（需 NAT）
}

// ★ 可达性验证服务
interface ReachabilityService {
    Verify(ctx, addr: Multiaddr) → bool, error
        // 验证地址是否真实可达
        // 通常使用 AutoNAT 或外部探测
    
    PublishableAddrs() → []Multiaddr
        // 返回经过验证的可发布地址
        // DHT 发布必须使用此方法的输出
}
```

### Storage 接口

Storage 提供统一的持久化存储能力。

```
// 位置：pkg/interfaces/storage.go

// Engine 存储引擎基础接口
// 允许用户提供自定义存储后端（可选）
interface Engine {
    // 基础操作
    Get(key: bytes) → bytes, error              // 获取值
    Put(key: bytes, value: bytes) → error       // 设置值
    Delete(key: bytes) → error                  // 删除键
    Has(key: bytes) → bool, error               // 检查键是否存在
    
    // 生命周期
    Close() → error                             // 关闭引擎
}

// EngineStats 引擎统计信息
struct EngineStats {
    KeyCount: int64                             // 键数量
    DiskSize: int64                             // 磁盘占用
    CacheHits: int64                            // 缓存命中数
    CacheMisses: int64                          // 缓存未命中数
}
```

> **注意**：Storage 的完整接口（Batch、Iterator、Transaction）在 `internal/core/storage/engine/` 中定义，
> 仅暴露最小的 `Engine` 公共接口，允许用户可选地提供自定义存储后端。

---

### ★ BandwidthCounter 接口

BandwidthCounter 提供多维度的带宽统计能力。

```
// 位置：pkg/interfaces/bandwidth.go

interface BandwidthCounter {
    // ═══════════════════════════════════════════════════════════════════════
    // 记录流量
    // ═══════════════════════════════════════════════════════════════════════
    
    LogSentMessage(size: int64)                       // 记录发送消息
    LogRecvMessage(size: int64)                       // 记录接收消息
    LogSentStream(size: int64, proto: string, peer: string)  // 记录流发送
    LogRecvStream(size: int64, proto: string, peer: string)  // 记录流接收
    
    // ═══════════════════════════════════════════════════════════════════════
    // 获取统计
    // ═══════════════════════════════════════════════════════════════════════
    
    GetTotals() → BandwidthStats                      // 获取总带宽统计
    GetForPeer(peer: string) → BandwidthStats         // 获取 Peer 带宽统计
    GetForProtocol(proto: string) → BandwidthStats    // 获取协议带宽统计
    GetByPeer() → map[string]BandwidthStats           // 获取所有 Peer 统计
    GetByProtocol() → map[string]BandwidthStats       // 获取所有协议统计
    
    // ═══════════════════════════════════════════════════════════════════════
    // 管理
    // ═══════════════════════════════════════════════════════════════════════
    
    Reset()                                           // 重置所有统计
    TrimIdle(since: Time)                             // 清理空闲条目
}

// BandwidthStats 带宽统计快照
struct BandwidthStats {
    TotalIn: int64      // 总入站字节数
    TotalOut: int64     // 总出站字节数
    RateIn: float64     // 入站速率 (EWMA, bytes/sec)
    RateOut: float64    // 出站速率 (EWMA, bytes/sec)
}
```

---

### ★ ConnectionHealthMonitor 接口

ConnectionHealthMonitor 提供连接级别的健康状态监控。

```
// 位置：pkg/interfaces/netmon.go

interface ConnectionHealthMonitor {
    // ═══════════════════════════════════════════════════════════════════════
    // 生命周期
    // ═══════════════════════════════════════════════════════════════════════
    
    Start(ctx) → error                                // 启动监控器
    Stop() → error                                    // 停止监控器
    
    // ═══════════════════════════════════════════════════════════════════════
    // 错误上报
    // ═══════════════════════════════════════════════════════════════════════
    
    OnSendError(peer: string, err: error)             // 上报发送错误
    OnSendSuccess(peer: string)                       // 上报发送成功
    
    // ═══════════════════════════════════════════════════════════════════════
    // 状态查询
    // ═══════════════════════════════════════════════════════════════════════
    
    GetState() → ConnectionHealth                     // 获取当前状态
    GetSnapshot() → ConnectionHealthSnapshot          // 获取状态快照
    
    // ═══════════════════════════════════════════════════════════════════════
    // 订阅
    // ═══════════════════════════════════════════════════════════════════════
    
    Subscribe() → channel<ConnectionHealthChange>     // 订阅状态变更
    Unsubscribe(ch: channel)                          // 取消订阅
    
    // ═══════════════════════════════════════════════════════════════════════
    // 恢复通知
    // ═══════════════════════════════════════════════════════════════════════
    
    TriggerRecoveryState()                            // 手动触发恢复状态
    NotifyRecoverySuccess()                           // 通知恢复成功
    NotifyRecoveryFailed(err: error)                  // 通知恢复失败
    Reset()                                           // 重置监控器状态
}

// ConnectionHealth 连接健康状态
enum ConnectionHealth {
    Healthy,      // 健康：所有连接正常
    Degraded,     // 降级：部分连接失败
    Down,         // 断开：全部连接失败
    Recovering    // 恢复中：正在尝试恢复
}

// ConnectionHealthChange 状态变更事件
struct ConnectionHealthChange {
    PreviousState: ConnectionHealth
    CurrentState: ConnectionHealth
    Reason: StateChangeReason
    Timestamp: Time
    TriggerPeer: string
    TriggerError: error
}
```

---

### ★ PathHealthManager 接口

PathHealthManager 提供路径级别的健康监控和切换决策。

```
// 位置：pkg/interfaces/pathhealth.go

interface PathHealthManager {
    // ═══════════════════════════════════════════════════════════════════════
    // 生命周期
    // ═══════════════════════════════════════════════════════════════════════
    
    Start(ctx) → error                                // 启动管理器
    Stop() → error                                    // 停止管理器
    
    // ═══════════════════════════════════════════════════════════════════════
    // 探测上报
    // ═══════════════════════════════════════════════════════════════════════
    
    ReportProbe(peer: string, addr: string, rtt: Duration, err: error)
        // 上报探测结果
    
    // ═══════════════════════════════════════════════════════════════════════
    // 状态查询
    // ═══════════════════════════════════════════════════════════════════════
    
    GetPathStats(peer: string, addr: string) → *PathStats
        // 获取路径统计
    
    GetAllPaths(peer: string) → []PathStats
        // 获取 Peer 所有路径
    
    // ═══════════════════════════════════════════════════════════════════════
    // 路径选择
    // ═══════════════════════════════════════════════════════════════════════
    
    RankAddrs(peer: string, addrs: []string) → []string
        // 按健康状况排序地址
    
    ShouldSwitch(peer: string, currentPath: string) → SwitchDecision
        // 路径切换决策
}

// PathStats 路径统计
struct PathStats {
    PathID: string
    PathType: PathType         // Direct/Relay
    State: PathState           // Unknown/Healthy/Suspect/Dead
    EWMARTT: Duration          // 指数加权移动平均 RTT
    LastRTT: Duration
    SuccessCount: int64
    FailureCount: int64
    ConsecutiveFailures: int
    LastSeen: Time
    Score: float64             // 路径评分（越低越好）
}

// PathState 路径状态
enum PathState {
    Unknown,    // 初始/未知
    Healthy,    // 健康
    Suspect,    // 可疑
    Dead        // 死亡
}

// SwitchDecision 切换决策
struct SwitchDecision {
    ShouldSwitch: bool
    Reason: string
    SuggestedPath: string
}
```

---

### ★ RecoveryManager 接口

RecoveryManager 提供网络故障自动恢复能力。

```
// 位置：pkg/interfaces/recovery.go

interface RecoveryManager {
    // ═══════════════════════════════════════════════════════════════════════
    // 生命周期
    // ═══════════════════════════════════════════════════════════════════════
    
    Start(ctx) → error                                // 启动管理器
    Stop() → error                                    // 停止管理器
    
    // ═══════════════════════════════════════════════════════════════════════
    // 恢复触发
    // ═══════════════════════════════════════════════════════════════════════
    
    TriggerRecovery(ctx, reason: RecoveryReason) → *RecoveryResult
        // 触发恢复流程
    
    IsRecovering() → bool                             // 是否正在恢复
    
    // ═══════════════════════════════════════════════════════════════════════
    // 关键节点管理
    // ═══════════════════════════════════════════════════════════════════════
    
    SetCriticalPeers(peers: []string)                 // 设置关键节点
    SetCriticalPeersWithAddrs(peers, addrs: []string) // 设置带地址的关键节点
    
    // ═══════════════════════════════════════════════════════════════════════
    // 回调
    // ═══════════════════════════════════════════════════════════════════════
    
    OnRecoveryComplete(callback: func(RecoveryResult)) // 恢复完成回调
    
    // ═══════════════════════════════════════════════════════════════════════
    // 依赖注入（可选）
    // ═══════════════════════════════════════════════════════════════════════
    
    SetRebinder(rebinder: Rebinder)                   // 设置传输层重绑定器
    SetAddressDiscoverer(discoverer: AddressDiscoverer) // 设置地址发现器
    SetConnector(connector: RecoveryConnector)        // 设置连接器
}

// RecoveryReason 恢复原因
enum RecoveryReason {
    NetworkUnreachable,     // 网络不可达
    NoRoute,                // 无路由
    ConnectionRefused,      // 连接被拒绝
    AllConnectionsLost,     // 所有连接丢失
    ErrorThreshold,         // 错误达到阈值
    ManualTrigger,          // 手动触发
    NetworkChange           // 网络变更
}

// RecoveryResult 恢复结果
struct RecoveryResult {
    Success: bool
    Reason: RecoveryReason
    StartTime: Time
    EndTime: Time
    Duration: Duration
    ReconnectedPeers: []string
    FailedPeers: []string
    Error: error
}
```

---

### NetworkMonitor 接口

NetworkMonitor 提供网络变化监控能力。

```
// 位置：pkg/interfaces/network.go

interface NetworkMonitor {
    // Start 启动网络监控
    Start(ctx) → error
    
    // Stop 停止网络监控
    Stop() → error
    
    // Subscribe 订阅网络变化事件
    Subscribe() → channel<NetworkChangeEvent>
    
    // NotifyChange 外部通知网络变化（用于 Android 等平台）
    NotifyChange()
    
    // CurrentState 获取当前网络状态
    CurrentState() → NetworkState
}

// NetworkChangeEvent 网络变化事件
struct NetworkChangeEvent {
    Type: NetworkChangeType      // 变化类型
    OldAddrs: []string           // 旧地址列表
    NewAddrs: []string           // 新地址列表
    Timestamp: Time              // 事件时间
}

enum NetworkChangeType {
    Minor,      // IP 地址变化但接口不变
    Major       // 网络接口变化（如 4G→WiFi）
}

// NetworkState 网络状态
struct NetworkState {
    Interfaces: []NetworkInterface   // 活跃的网络接口
    PreferredInterface: string       // 首选接口
    IsOnline: bool                   // 是否在线
}
```

### JitterTolerance 接口

JitterTolerance 提供连接抖动容忍能力。

```
// 位置：pkg/interfaces/connmgr.go（扩展）

interface JitterTolerance {
    // NotifyDisconnected 通知节点断连
    NotifyDisconnected(peer: NodeID)
    
    // NotifyReconnected 通知节点重连成功
    NotifyReconnected(peer: NodeID)
    
    // SetReconnectCallback 设置重连回调
    SetReconnectCallback(callback: ReconnectCallback)
    
    // OnStateChange 注册状态变化回调
    OnStateChange(callback: StateChangeCallback)
    
    // GetStats 获取统计信息
    GetStats() → JitterStats
    
    // Close 关闭
    Close() → error
}

// JitterConfig 抖动容忍配置
struct JitterConfig {
    Enabled: bool                      // 是否启用
    ToleranceWindow: Duration          // 容错窗口（默认 5s）
    StateHoldTime: Duration            // 状态保持时间（默认 30s）
    ReconnectEnabled: bool             // 是否启用自动重连
    InitialReconnectDelay: Duration    // 初始重连延迟（默认 1s）
    MaxReconnectDelay: Duration        // 最大重连延迟（默认 60s）
    MaxReconnectAttempts: int          // 最大重连次数（默认 5）
    BackoffMultiplier: float64         // 退避乘数（默认 2.0）
}

// JitterStats 抖动统计
struct JitterStats {
    Held: int                          // 保持状态的节点数
    Reconnecting: int                  // 正在重连的节点数
    TotalReconnectAttempts: int        // 总重连尝试次数
}

type ReconnectCallback = (ctx, peer: NodeID) → error
type StateChangeCallback = (peer: NodeID, state: JitterState) → void

enum JitterState {
    Connected,      // 已连接
    Held,           // 断连保持
    Reconnecting,   // 正在重连
    Removed         // 已移除
}
```

---

## Discovery 发现域接口

### DiscoveryService 接口

发现服务统一接口。

```
// 位置：pkg/interfaces/discovery.go

interface DiscoveryService {
    FindPeers(ctx, ns: string, opts...) → channel<PeerInfo>, error
    Advertise(ctx, ns: string, opts...) → Duration, error
}

struct DiscoveryOptions {
    Limit: int
    TTL: Duration
}
```

### Finder 通用接口

各发现组件实现的通用接口。

```
// 位置：pkg/interfaces/discovery.go

interface Finder {
    FindPeers(ctx, ns: string, limit: int) → channel<PeerInfo>, error
    Advertise(ctx, ns: string, ttl: Duration) → error
    Close() → error
}
```

### DHT 接口

DHT 发现接口。

```
// 位置：pkg/interfaces/discovery.go

// ★ DHT 发布约束：
//   仅发布经过 ReachabilityService.Verify() 验证的地址
//   候选地址必须通过可达性验证后才能发布到 DHT
//   违反此约束会导致其他节点无法连接

interface DHTFinder extends Finder {
    FindPeer(ctx, id: NodeID) → PeerInfo, error
    PutValue(ctx, key: string, value: bytes) → error
    GetValue(ctx, key: string) → bytes, error
    GetClosestPeers(ctx, key: string) → []NodeID, error
    
    // ★ 地址发布（必须使用可达地址）
    Announce(ctx, addrs: []Multiaddr) → error
        // 前置条件：addrs 必须来自 ReachabilityService.PublishableAddrs()
        // 违反约束：返回 ErrUnverifiedAddress
}
```

### Bootstrap 接口

引导节点发现接口。

```
// 位置：pkg/interfaces/discovery.go

interface BootstrapFinder extends Finder {
    AddBootstrapPeer(addr: Multiaddr) → error
    RemoveBootstrapPeer(id: NodeID)
    BootstrapPeers() → []Multiaddr
}
```

### mDNS 接口

局域网发现接口。

```
// 位置：pkg/interfaces/discovery.go

interface MDNSFinder extends Finder {
    // 局域网自动发现，无需额外方法
}
```

### Rendezvous 接口

命名空间发现接口。

```
// 位置：pkg/interfaces/discovery.go

interface RendezvousFinder extends Finder {
    Register(ctx, ns: string, ttl: Duration) → error
    Unregister(ctx, ns: string) → error
}
```

---

## 接口契约约束

### 通用约束

| 约束 ID | 约束内容 |
|---------|----------|
| **GEN-001** | 所有可能阻塞的方法第一个参数必须是 `Context` |
| **GEN-002** | 返回 error 的方法，调用者必须检查 error |
| **GEN-003** | Close() 方法必须是幂等的（可多次调用） |
| **GEN-004** | 接口方法必须并发安全 |

### Node 契约

| 方法 | 前置条件 | 后置条件 | 错误 |
|------|----------|----------|------|
| `JoinRealm()` | Node 已启动 | 加入 Realm 或返回错误 | `ErrInvalidPSK` |
| `EnableRelay(ctx)` | 公网可达 | 启动 Relay 服务（三大职责 v2.0） | `ErrNotPubliclyReachable` |
| `EnableBootstrap(ctx)` | 公网可达 | 启动 Bootstrap 服务 | `ErrNotPubliclyReachable` |

### Realm 契约

| 方法 | 前置条件 | 后置条件 | 错误 |
|------|----------|----------|------|
| `Connect()` | 目标是 Realm 成员 | 建立连接或返回错误 | `ErrNotMember`, `ErrConnectionFailed` |
| `Leave()` | 已加入 Realm | 离开 Realm，取消 Context | 无 |

> **注意**：
> - `Connect()` 仅在 Realm 内有效。跨 Realm 连接必须使用 `node.Connect(ctx, multiaddr)` 并提供完整地址。这是 INV-004 "仅 ID 连接"边界的体现。
> - Relay 能力已统一到 Node 级别，Realm 内连接自动使用 Node 的 Relay。

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [internal_interfaces.md](internal_interfaces.md) | 内部接口设计 |
| [component_interface_map.md](component_interface_map.md) | 组件-接口映射 |
| [fx_lifecycle.md](fx_lifecycle.md) | Fx + Lifecycle 模式 |
| [../L1_overview/abstractions.md](../L1_overview/abstractions.md) | 核心抽象 |
| [ADR-0009](../../01_context/decisions/ADR-0009-bootstrap-simplified.md) | Bootstrap 极简配置 |
| [ADR-0010](../../01_context/decisions/ADR-0010-relay-explicit-config.md) | Relay 明确配置 |

---

**最后更新**：2026-01-24（v2.0 DHT 权威模型对齐）
