# Relay 架构说明

本文档详细说明 relay 包的内部架构、组件职责和协作关系。

---

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────┐
│                               Manager                                    │
│                     (统一入口，协调所有 Relay 功能)                        │
│                                                                          │
│  职责:                                                                   │
│  • DialWithPriority() - 按优先级连接                                     │
│  • EnableRelay()/DisableRelay() - 启用/禁用 Relay 服务能力               │
│  • SetRelayAddr()/RelayAddr() - 配置/获取中继地址                        │
│  • DialViaRelay() - 通过中继拨号                                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────┐          ┌─────────────────────────────────┐   │
│  │    RelayService     │          │          AutoRelay              │   │
│  │   (单连接管理)       │          │       (多 Relay 策略)            │   │
│  │                     │          │                                 │   │
│  │  状态机:            │          │  候选管理:                       │   │
│  │  None → Configured  │          │  • candidates map              │   │
│  │       → Connecting  │          │  • activeRelays map            │   │
│  │       → Connected   │          │  • preferredRelays set         │   │
│  │                     │          │                                 │   │
│  │  核心方法:          │          │  核心方法:                       │   │
│  │  • SetRelay()      │          │  • Start()/Stop()              │   │
│  │  • Connect()       │          │  • AddCandidate()              │   │
│  │  • DialViaRelay()  │          │  • SelectRelay()               │   │
│  │  • State()         │          │  • RefreshReservations()       │   │
│  └─────────────────────┘          └─────────────────────────────────┘   │
│                                                                          │
│                           ┌──────────────────────┐                      │
│                           │   client.Client      │                      │
│                           │  (底层协议客户端)     │                      │
│                           │                      │                      │
│                           │  • Reserve()         │                      │
│                           │  • Connect()         │                      │
│                           │  • Close()           │                      │
│                           └──────────────────────┘                      │
└─────────────────────────────────────────────────────────────────────────┘

                           ┌──────────────────────┐
                           │   server.Server      │
                           │  (中继服务端)         │
                           │                      │
                           │  • Start()           │
                           │  • HandleReserve()   │
                           │  • HandleConnect()   │
                           └──────────────────────┘
```

---

## 组件详细说明

### 1. Manager (manager.go)

**职责**: 统一入口，协调所有 Relay 相关功能。

**字段**:
```go
type Manager struct {
    config     *Config
    swarm      pkgif.Swarm
    eventbus   pkgif.EventBus
    peerstore  pkgif.Peerstore
    host       pkgif.Host
    
    relay        *RelayService  // 单连接管理
    holePuncher  pkgif.HolePuncher
}
```

**核心方法**:

| 方法 | 说明 |
|------|------|
| `DialWithPriority()` | 按优先级尝试连接：直连 → 打洞 → 中继 |
| `DialViaRelay()` | 直接通过中继拨号（跳过直连和打洞） |
| `EnableRelay()` | 启用 Relay 服务能力（成为中继节点） |
| `DisableRelay()` | 禁用 Relay 服务能力 |
| `SetRelayAddr()` | 配置中继地址（惰性连接，不立即连接） |
| `RelayAddr()` | 获取当前配置的中继地址 |
| `HasRelay()` | 检查是否已配置/连接中继 |

**连接优先级**:
```
1. 直连 (dialDirect)
   ↓ 失败
2. 打洞 (dialHolePunch)
   ↓ 失败
3. 中继 (DialViaRelay)
```

---

### 2. RelayService (service.go)

**职责**: 底层单连接管理，负责与单个 Relay 节点的交互。

**状态机**:
```
┌──────────┐  SetRelay()  ┌────────────┐  Connect()  ┌────────────┐
│   None   │ ──────────→ │ Configured │ ──────────→ │ Connecting │
└──────────┘              └────────────┘              └────────────┘
                                                           │
                                                           │ 成功
                                                           ↓
                                                     ┌────────────┐
                                                     │ Connected  │
                                                     └────────────┘
                                                           │
                                                           │ 断开/移除
                                                           ↓
                                                     ┌──────────┐
                                                     │   None   │
                                                     └──────────┘
```

**字段**:
```go
type RelayService struct {
    addr    types.Multiaddr  // 配置的中继地址
    state   RelayState       // 连接状态
    swarm   pkgif.Swarm
    host    pkgif.Host
    client  *client.Client   // 中继客户端
    server  *server.Server   // 中继服务端
    limiter *RelayLimiter    // 统一限流器
    serverEnabled atomic.Bool // 是否启用为 Relay 服务端
}
```

**核心方法**:

| 方法 | 说明 |
|------|------|
| `SetRelay()` | 配置中继地址（仅保存，不连接） |
| `Relay()` | 获取当前配置的中继地址 |
| `RemoveRelay()` | 移除中继配置 |
| `Connect()` | 建立中继连接 |
| `State()` | 获取当前状态 |
| `DialViaRelay()` | 通过中继拨号到目标节点 |
| `Enable()` | 启用 Relay 服务能力 |
| `Disable()` | 禁用 Relay 服务能力 |

---

### 3. AutoRelay (client/autorelay.go)

**职责**: 上层多 Relay 策略，实现自动化中继管理。

**设计目标**:
- 自动发现可用 Relay 候选
- 维护多个活跃 Relay 预留（冗余）
- 故障自动切换
- 首选中继优先

**字段**:
```go
type AutoRelay struct {
    config    AutoRelayConfig
    client    pkgif.RelayClient
    host      pkgif.Host
    peerstore pkgif.Peerstore
    
    // 活跃中继 (已建立预留)
    activeRelays   map[string]*activeRelay
    
    // 候选中继 (已发现，待建立预留)
    candidates     map[string]*relayCandidate
    
    // 黑名单 (连接失败的 Relay)
    blacklist      map[string]time.Time
    
    // 首选中继列表
    preferredRelays map[string]struct{}
    
    // 地址变更回调
    onAddrsChanged func([]string)
}
```

**配置**:
```go
type AutoRelayConfig struct {
    MinRelays          int           // 最小活跃中继数 (默认 2)
    MaxRelays          int           // 最大活跃中继数 (默认 4)
    RefreshInterval    time.Duration // 预留刷新间隔
    BootDelay          time.Duration // 启动延迟
    BlacklistDuration  time.Duration // 黑名单时长
}
```

**核心方法**:

| 方法 | 说明 |
|------|------|
| `Start()` | 启动 AutoRelay（开始候选发现和预留管理） |
| `Stop()` | 停止 AutoRelay |
| `AddCandidate()` | 添加候选中继 |
| `AddPreferredRelay()` | 添加首选中继（优先使用） |
| `SelectRelay()` | 选择最佳中继用于连接 |
| `RefreshReservations()` | 刷新所有预留 |
| `GetActiveRelays()` | 获取所有活跃中继 |

---

### 4. client.Client (client/client.go)

**职责**: 底层中继协议客户端，实现 Circuit Relay v2 协议。

**字段**:
```go
type Client struct {
    swarm      pkgif.Swarm
    relayPeer  types.PeerID
    relayAddr  types.Multiaddr
    reservation *Reservation
}
```

**核心方法**:

| 方法 | 说明 |
|------|------|
| `Reserve()` | 向 Relay 请求预留（获取中继地址） |
| `Connect()` | 通过 Relay 连接到目标节点 |
| `Close()` | 关闭客户端，释放资源 |

---

### 5. server.Server (server/server.go)

**职责**: 中继服务端，接受预留请求和连接请求。

**字段**:
```go
type Server struct {
    swarm   pkgif.Swarm
    limiter Limiter
    
    // 活跃预留
    reservations map[string]*reservation
    
    // 活跃电路
    circuits     map[string]*circuit
}
```

**核心方法**:

| 方法 | 说明 |
|------|------|
| `Start()` | 启动服务端（注册协议处理器） |
| `Stop()` | 停止服务端 |
| `HandleReserve()` | 处理预留请求 |
| `HandleConnect()` | 处理连接请求 |

---

## 协作流程

### 场景 1: 客户端通过 Relay 连接

```
用户代码
   │
   │ manager.DialWithPriority(ctx, target)
   ↓
Manager
   │
   │ 1. dialDirect() 失败
   │ 2. dialHolePunch() 失败
   │ 3. DialViaRelay()
   ↓
RelayService
   │
   │ 使用 client.Client
   ↓
client.Client
   │
   │ 1. Reserve() - 获取中继地址
   │ 2. Connect() - 通过中继连接
   ↓
Relay Server
   │
   │ 建立电路，转发数据
   ↓
Target Node
```

### 场景 2: 自动中继管理

```
AutoRelay.Start()
   │
   │ 1. 加载首选中继
   │ 2. 从 DHT 发现候选
   │ 3. 建立预留（MinRelays 个）
   ↓
后台循环
   │
   │ • 定期刷新预留
   │ • 监控活跃中继
   │ • 故障时自动切换
   ↓
当需要连接时
   │
   │ SelectRelay() 选择最佳中继
   ↓
使用选中的中继
```

---

## 配置说明

### RelayLimiter

统一资源限制器，控制中继资源使用：

```go
type RelayLimiterConfig struct {
    MaxReservations      int           // 最大预留数
    MaxCircuits          int           // 最大电路数（0 表示不限制）
    MaxReservationsPerIP int           // 每 IP 最大预留数
    MaxCircuitsPerPeer   int           // 每节点最大电路数（0 表示不限制）
    ReservationTTL       time.Duration // 预留有效期
    MaxCircuitDuration   time.Duration // 单次中继最长时间
    MaxCircuitData       int64         // 单次中继最大数据量
}
```

---

## 设计决策

### 为什么分离 RelayService 和 AutoRelay？

1. **单一职责**: RelayService 专注单连接管理，AutoRelay 专注策略
2. **可测试性**: 可独立测试单连接和多 Relay 策略
3. **灵活性**: 用户可只使用 RelayService（手动管理）或 AutoRelay（自动管理）

### 为什么保留 Relay 连接（打洞成功后）？

1. **信令通道**: 后续打洞可能需要
2. **快速恢复**: 直连断开时可快速切换
3. **低成本**: 空闲连接资源占用少

---

## 相关文档

- [NAT 穿透与中继概念澄清](../../../design/_discussions/20260123-nat-relay-concept-clarification.md)
- [Relay 概念实现跟踪](../../../design/_discussions/20260124-relay-concept-implementation-tracking.md)
