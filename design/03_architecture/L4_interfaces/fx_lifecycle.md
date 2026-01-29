# Fx + Lifecycle 模式 (Fx + Lifecycle Pattern)

> 定义 DeP2P 的依赖注入和生命周期管理模式

---

## 概述

DeP2P 使用 Uber Fx 进行依赖注入和生命周期管理。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Fx 核心概念                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Module：模块定义，封装一组 Provider                                          │
│  Provider：提供依赖的函数                                                     │
│  Invoke：应用启动时执行的函数                                                  │
│  Lifecycle：生命周期钩子 (OnStart/OnStop)                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Fx + Lifecycle 混合模式

### 模块目录结构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     Fx + Lifecycle 混合模式                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  每个模块结构：                                                               │
│                                                                             │
│  internal/<layer>/<module>/                                                 │
│  ├── doc.go              # 包文档                                            │
│  ├── module.go           # Fx 模块定义                                       │
│  ├── <module>.go         # 核心实现                                          │
│  ├── interfaces/         # 内部接口（可选）                                   │
│  │   └── <module>.go                                                        │
│  └── ...                 # 其他实现文件                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Fx 模块定义模板

```
// Fx 模块定义模板（伪代码）
// 位置：internal/<layer>/<module>/module.go

Module "<layer>.<module>" {
    // 提供构造函数
    provide: [
        New,                            // 创建实例
        New as interfaces.XXX           // 接口绑定
    ],
    
    // 生命周期钩子
    invoke: registerLifecycle
}

// registerLifecycle 注册生命周期钩子
function registerLifecycle(lifecycle, service):
    lifecycle.append(
        OnStart: (ctx) → service.Start(ctx),
        OnStop:  (ctx) → service.Stop(ctx)
    )
```

---

## 模块定义规范

### 标准模块结构

每个组件模块应包含 `module.go` 文件：

```
// 位置：internal/protocol/messaging/module.go

Module MessagingModule {
    // 提供实现
    provide: [
        NewService
    ]
    
    // 绑定到公共接口（外部使用）
    bind: [
        NewService → MessagingService
    ]
    
    // 注意：大多数模块不需要内部接口
    // 仅在模块内部有多个子组件需要相互依赖时，才需要内部接口
}
```

### 模块命名规范

| 域 | 模块 | 名称 | 说明 |
|----|------|------|------|
| **App** | `node.Module` | `"node"` | 用户入口 |
| **App** | `messaging.Module` | `"messaging"` | 消息服务 |
| **App** | `pubsub.Module` | `"pubsub"` | 发布订阅 |
| **App** | `streams.Module` | `"streams"` | 流管理 |
| **App** | `liveness.Module` | `"liveness"` | 存活检测 |
| **Biz** | `realm.Module` | `"realm"` | Realm 管理 |
| **Core** | `host.Module` | `"host"` | 网络主机 |
| **Core** | `transport.Module` | `"transport"` | 传输层 |
| **Core** | `security.Module` | `"security"` | 安全层 |
| **Core** | `muxer.Module` | `"muxer"` | 流复用 |
| **Core** | `connmgr.Module` | `"connmgr"` | 连接管理 |
| **Core** | `identity.Module` | `"identity"` | 身份管理 |
| **Core** | `relay.Module` | `"relay"` | 中继服务 |
| **Core** | `nat.Module` | `"nat"` | NAT 穿透 |
| **Discovery** | `coordinator.Module` | `"coordinator"` | 发现协调器 |
| **Discovery** | `dht.Module` | `"dht"` | DHT 发现 |
| **Discovery** | `bootstrap.Module` | `"bootstrap"` | 引导节点 |
| **Discovery** | `mdns.Module` | `"mdns"` | 局域网发现 |
| **Discovery** | `rendezvous.Module` | `"rendezvous"` | 命名空间发现 |

---

## 构造函数规范

### 基本构造函数

```
// 位置：internal/protocol/messaging/service.go

class Service {
    host: InternalHost
    realm: InternalRealm
    config: Config
    handlers: map<string, Handler>
    ctx: Context
    cancel: CancelFunc
}

// 构造函数
function NewService(
    lifecycle: Lifecycle,
    host: InternalHost,       // 依赖内部接口
    realm: InternalRealm,     // 依赖内部接口
    config: Config
) → Service {
    ctx, cancel = context.WithCancel(background())
    
    service = new Service {
        host: host,
        realm: realm,
        config: config,
        handlers: {},
        ctx: ctx,
        cancel: cancel
    }
    
    // 注册生命周期钩子
    lifecycle.Append(Hook {
        OnStart: service.start,
        OnStop: service.stop
    })
    
    return service
}

// 确保实现接口
assert Service implements InternalMessagingService
```

### 构造函数规则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          构造函数规则                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  规则 1: 依赖内部接口                                                        │
│  ──────────────────────                                                     │
│  内部组件间依赖使用内部接口（interfaces.*）                                   │
│  公共接口只用于对外暴露                                                       │
│                                                                             │
│  规则 2: 第一个参数是 Lifecycle                                              │
│  ───────────────────────────────                                            │
│  需要生命周期管理的组件                                                       │
│                                                                             │
│  规则 3: 配置通过参数注入                                                     │
│  ────────────────────────                                                   │
│  不要在构造函数中读取全局配置                                                  │
│                                                                             │
│  规则 4: 不要在构造函数中阻塞                                                  │
│  ──────────────────────────────                                             │
│  初始化逻辑放到 OnStart 中                                                   │
│                                                                             │
│  规则 5: 接口类型断言                                                         │
│  ────────────────────                                                       │
│  使用类型断言确保实现接口                                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 生命周期管理

### 生命周期钩子

```
// 标准生命周期钩子实现

function (service *Service) start(ctx: Context) → error {
    // 1. 初始化资源
    service.handlers = {}
    
    // 2. 注册协议处理器
    service.host.SetStreamHandler(protocolID, service.handleStream)
    
    // 3. 启动后台任务
    go service.backgroundLoop(service.ctx)
    
    return nil
}

function (service *Service) stop(ctx: Context) → error {
    // 1. 取消后台任务
    service.cancel()
    
    // 2. 等待任务完成
    service.wg.Wait()
    
    // 3. 释放资源
    service.host.RemoveStreamHandler(protocolID)
    
    return nil
}
```

### 启动顺序

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          启动顺序（按依赖）                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ═══════════════════════════════════════════════════════════════════════   │
│  Phase 1: Node Initialization (Fx 启动时)                                   │
│  ═══════════════════════════════════════════════════════════════════════   │
│                                                                             │
│  构造阶段（同步）：                                                          │
│  ─────────────────                                                          │
│                                                                             │
│  [Core] → Identity, Security, Transport, Muxer, ConnMgr, Host              │
│      ↓       Relay (System), NAT                                            │
│      ↕                                                                      │
│  [Discovery] → DHT, Bootstrap, mDNS, Rendezvous, Coordinator               │
│      ↓                                                                      │
│  [RealmManager] → 空实例（等待 JoinRealm）                                  │
│      ↓                                                                      │
│  [Node] → 返回给用户                                                        │
│                                                                             │
│  ★ 注意：Realm 内部组件和协议服务此时未创建                                  │
│                                                                             │
│  启动阶段（按依赖顺序）：                                                     │
│  ─────────────────────                                                      │
│                                                                             │
│  Core.OnStart ↔ Discovery.OnStart → RealmManager.OnStart → Node.OnStart    │
│                                                                             │
│  ═══════════════════════════════════════════════════════════════════════   │
│  Phase 2: Realm Creation (用户调用 JoinRealm 时)                            │
│  ═══════════════════════════════════════════════════════════════════════   │
│                                                                             │
│  RealmManager.JoinRealm(psk) 内部创建：                                     │
│  ─────────────────────────────────────                                      │
│                                                                             │
│  [Realm Core] → Auth, Member, Routing, Gateway (由 RealmManager 创建)       │
│      ↓                                                                      │
│  [Protocol] → Messaging, PubSub, Streams, Liveness (绑定 RealmID)          │
│      ↓                                                                      │
│  [Realm] → 返回给用户                                                       │
│                                                                             │
│  ═══════════════════════════════════════════════════════════════════════   │
│  停止阶段（反向顺序）：                                                       │
│  ═══════════════════════════════════════════════════════════════════════   │
│                                                                             │
│  Realm.Close → RealmManager.OnStop → Discovery.OnStop ↔ Core.OnStop        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

参见: [INV-002 Realm 成员资格](../../01_context/decisions/invariants/INV-002-realm-membership.md)

---

## 条件模块注册

### 按需加载模式

DEP2P 采用条件模块注册，只加载用户启用的功能模块：

```
// 条件模块注册（伪代码）

function buildFxApp(config) → FxApp, error:
    
    // 1. 配置验证（前置）
    if config.Validate() fails:
        return error("config validation failed")
    
    // 2. 核心模块（必须加载）
    modules = [
        IdentityModule,
        EventBusModule,
        PeerstoreModule
    ]
    
    // 3. 传输层（条件加载）
    if config.Transport.EnableQUIC or config.Transport.EnableTCP:
        modules += [TransportModule, SecurityModule, MuxerModule, 
                    UpgraderModule, SwarmModule, HostModule]
    
    // 4. 发现层（条件加载）
    if config.Discovery.EnableDHT:
        modules += [DHTModule]
    if config.Discovery.EnableMDNS:
        modules += [mDNSModule]
    if hasAnyDiscovery(config):
        modules += [CoordinatorModule]
    
    // 5. NAT 穿透（条件加载）
    // ★ 打洞需要信令通道（通常由 Relay 连接提供）
    if config.NAT.EnableAutoNAT or config.NAT.EnableUPnP:
        modules += [NATModule]
    
    // 6. 中继服务（条件加载）
    // ★ v2.0 Relay 三大职责：缓存加速 + 打洞协调信令 + 数据保底
    // ★ DHT 是权威目录，Relay 地址簿是缓存加速层
    if config.Relay.EnableClient or config.Relay.EnableServer:
        modules += [RelayModule]
    
    // ★ 打洞与信令依赖检查
    if config.NAT.EnableHolePunch and not config.Relay.EnableClient:
        warn("打洞需要 Relay 信令通道，否则只能退化为 Relay 兜底")
    
    // 7. 增强功能模块（条件加载）
    if config.Bandwidth.Enabled:
        modules += [BandwidthModule]
    if config.ConnectionHealth.Enabled:
        modules += [NetMonModule]
    if config.PathHealth.Enabled:
        modules += [PathHealthModule]
    if config.Recovery.Enabled:
        modules += [RecoveryModule]
    
    // 8. RealmManager（始终加载）
    // ★ Realm 内部组件由 JoinRealm() 动态创建（符合 INV-002）
    modules += [RealmManagerModule]
    
    // 9. 用户扩展
    modules += config.userExtensions
    
    return FxApp.New(modules)

// 辅助函数
function hasAnyDiscovery(config) → bool:
    return config.Discovery.EnableDHT or
           config.Discovery.EnableMDNS or
           config.Discovery.EnableBootstrap
```

### ★ Realm 与协议服务的创建时机 (INV-002)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Realm/协议服务的正确创建时机                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ❌ 错误：在 Fx 启动时条件加载 Realm/协议模块                                │
│  ──────────────────────────────────────────────                             │
│  if hasAnyRealm(cfg.config) {                                               │
│      modules = append(modules, auth.Module, member.Module, ...)  // 错误!    │
│  }                                                                          │
│  if cfg.config.Messaging.EnablePubSub {                                     │
│      modules = append(modules, pubsub.Module)  // 错误!                      │
│  }                                                                          │
│                                                                             │
│  问题：                                                                      │
│  • 协议服务需要 RealmID 来构建协议前缀 (/dep2p/app/<realmID>/...)            │
│  • 启动时还没有 PSK，无法派生 RealmID                                        │
│  • 违反 INV-002：业务 API 需要先 JoinRealm                                   │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  ✅ 正确：RealmManager 工厂模式                                              │
│  ──────────────────────────────                                             │
│                                                                             │
│  Phase 1 (Fx 启动时):                                                       │
│  • 只加载 RealmManager（空实例）                                             │
│  • RealmManager 持有 Host、EventBus 等依赖                                  │
│                                                                             │
│  Phase 2 (用户调用 JoinRealm 时):                                           │
│  • RealmManager.JoinRealm(psk) 内部创建 Realm 组件                          │
│  • 派生 RealmID                                                             │
│  • 创建 Auth、Member、Routing、Gateway                                      │
│  • 创建 Messaging、PubSub、Streams、Liveness（绑定 RealmID）                 │
│  • 注册 Realm 级协议处理器                                                   │
│                                                                             │
│  优点：                                                                      │
│  • 符合 INV-002 语义                                                        │
│  • 协议 ID 正确携带 RealmID                                                 │
│  • 节点启动不依赖 Realm 配置                                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### RealmManager 工厂模式示例

```
// RealmManager 工厂模式（伪代码）
// 位置：internal/realm/manager/manager.go

struct Manager {
    host: Host
    eventBus: EventBus
    storage: Storage
    current: Realm | nil    // 当前 Realm（单 Realm 模式）
}

// JoinRealm 加入 Realm（内部创建所有组件）
function Manager.JoinRealm(ctx, psk: bytes) → Realm, error:
    lock()
    defer unlock()
    
    if current is not nil:
        return error(ErrAlreadyJoined)
    
    // 1. 派生 RealmID
    realmID = deriveRealmID(psk)
    
    // 2. 创建 Realm 核心组件（不是 Fx 模块）
    authSvc = Auth.New(host, psk)
    memberSvc = Member.New(realmID, host, eventBus)
    routingSvc = Routing.New(realmID, host)
    
    // 3. 创建协议服务（绑定 RealmID）
    msgSvc = Messaging.New(host, realmID)
    pubsubSvc = PubSub.New(host, realmID)
    streamsSvc = Streams.New(host, realmID)
    livenessSvc = Liveness.New(host, realmID)
    
    // 4. 组装 Realm
    realm = Realm {
        id: realmID,
        auth: authSvc,
        member: memberSvc,
        routing: routingSvc,
        messaging: msgSvc,
        pubsub: pubsubSvc,
        streams: streamsSvc,
        liveness: livenessSvc
    }
    
    // 5. 启动服务
    if realm.Start(ctx) fails:
        return error
    
    current = realm
    return realm
```

参见 connection_flow.md 中的启动流程描述。

### 模块条件注册模板

```
// 模块定义模板（伪代码）
// 位置：internal/<layer>/<module>/module.go

Module "<layer>.<module>" {
    provide: NewFromParams
}

struct Params {
    UnifiedConfig: Config [optional]
    // 其他依赖...
}

function ConfigFromUnified(config) → ModuleConfig | nil:
    // 检查功能是否启用
    if config is nil or not config.EnableXXX:
        return nil  // 禁用时返回 nil
    
    // 映射配置字段
    return ModuleConfig {
        Field1: config.XXX.Field1,
        Field2: config.XXX.Field2
    }

function NewFromParams(params: Params) → Component | nil, error:
    config = ConfigFromUnified(params.UnifiedConfig)
    
    // 功能禁用时返回 nil（不创建实例）
    if config is nil:
        return nil
    
    // 创建并返回实例
    return New(config)
```

---

## 启动阶段钩子

### 阶段化启动

```
// 节点状态（伪代码）

enum NodeState {
    Idle,           // 空闲（未启动）
    Initializing,   // 初始化中
    Starting,       // 启动中
    Running,        // 运行中
    Stopping,       // 停止中
    Stopped         // 已停止
}

// Node.Start 启动节点（阶段化启动）
function Node.Start(ctx) → error:
    lock()
    defer unlock()
    
    if node.closed:
        return ErrNodeClosed
    if node.started:
        return ErrAlreadyStarted
    
    // ═══════════════════════════════════════════════════════════
    // Phase 1: Initialize（超时 10s）
    // ═══════════════════════════════════════════════════════════
    setState(Initializing)
    
    initCtx = ctx.WithTimeout(10s)
    
    // 启动 Fx App（调用所有 OnStart）
    if app.Start(initCtx) fails:
        return error("initialize failed")
    
    // ═══════════════════════════════════════════════════════════
    // Phase 2: Ready Check（超时 5s）
    // ═══════════════════════════════════════════════════════════
    setState(Starting)
    
    // 等待关键组件就绪
    if waitForReady(ctx) fails:
        app.Stop()  // 启动失败，停止 Fx App
        return error("ready check failed")
    
    // ═══════════════════════════════════════════════════════════
    // Phase 3: Running
    // ═══════════════════════════════════════════════════════════
    setState(Running)
    node.started = true
    
    // 发布就绪事件
    eventBus.Emit(EventNodeReady)
    
    return nil

// waitForReady 等待关键组件就绪
function waitForReady(ctx) → error:
    if host is nil:
        return error("host not initialized")
    
    // 等待至少一个监听地址（超时 5s）
    deadline = 5s
    
    loop with 100ms interval until deadline:
        if ctx.Done():
            return ctx.Err()
        if timeout:
            return error("timeout waiting for listen addresses")
        if host.Addrs().length > 0:
            return nil  // 就绪

// setState 设置节点状态
function setState(state: NodeState):
    node.state = state
    // 可选：发布状态变更事件
```

### ★ Connect 成功 = 可通信（连接语义保证）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                   Connect 成功 = 可通信（核心语义保证）                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  当 Realm.Connect(targetID) 返回成功时，必须保证：                          │
│                                                                             │
│    1. 传输层连接已建立                                                       │
│       - 直连成功，或                                                        │
│       - 打洞成功（需信令通道），或                                          │
│       - Relay 中继成功                                                      │
│                                                                             │
│    2. Realm 认证已完成                                                       │
│       - PSK 验证通过                                                        │
│       - 双方确认为同一 Realm 成员                                           │
│                                                                             │
│    3. 协议协商已完成                                                         │
│       - 支持的协议已确定                                                    │
│       - 可立即进行通信                                                      │
│                                                                             │
│  ★ 这意味着用户不会遇到"连接成功但无法通信"的情况（避免抽象泄漏）            │
│                                                                             │
│  Connect 内部流程：                                                          │
│  ─────────────────                                                          │
│                                                                             │
│    1. 地址发现（v2.0 优先级）                                               │
│       Peerstore → MemberList → DHT（★ 权威）→ Relay.AddressBook（缓存）    │
│                                                                             │
│    2. 建立连接（优先级 INV-003）                                            │
│       直连 → 打洞（需 Relay 信令通道）→ Relay 保底                         │
│                                                                             │
│    3. Realm 认证                                                             │
│       PSK 验证 → 成员确认                                                   │
│                                                                             │
│    4. 协议协商                                                               │
│       协商支持的协议版本                                                    │
│                                                                             │
│    5. 返回成功                                                               │
│       所有步骤完成 → 返回 Connection                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 启动超时配置

| 阶段 | 超时时间 | 失败处理 |
|------|----------|----------|
| Initialize (Fx App Start) | 10s | 返回错误 |
| Ready Check | 5s | 停止 Fx，返回错误 |

---

## 健康检查集成

### 健康检查接口

```
// 健康检查接口（伪代码）
// 位置：pkg/interfaces/health.go

interface HealthChecker {
    Check(ctx) → HealthStatus     // 执行健康检查
    Name() → string               // 组件名称
}

struct HealthStatus {
    Status: Status                // Healthy, Degraded, Unhealthy
    Message: string               // 状态描述
    Details: map[string]any       // 详细信息
    Timestamp: Time               // 检查时间
}

enum Status {
    Healthy,      // 健康
    Degraded,     // 降级
    Unhealthy     // 不健康
}
```

### Node 健康检查实现

```
// Node.Health 获取节点健康状态（伪代码）

function Node.Health(ctx) → map[string]HealthStatus:
    results = {}
    
    // 1. 节点状态
    results["node"] = HealthStatus {
        Status: nodeHealthStatus(),
        Message: "state: {node.state}"
    }
    
    // 2. Host 健康检查
    if host is not nil:
        addrs = host.Addrs()
        results["host"] = HealthStatus {
            Status: Healthy if addrs.length > 0 else Unhealthy,
            Message: "listening on {addrs.length} addresses",
            Details: { addresses: addrs }
        }
    
    // 3. 连接状态
    connCount = ConnectionCount()
    results["connections"] = HealthStatus {
        Status: Healthy,  // 0 连接也是健康的
        Message: "{connCount} active connections"
    }
    
    // 4. Discovery 健康检查
    if discovery implements HealthChecker:
        results["discovery"] = discovery.Check(ctx)
    
    // 5. Realm 健康检查
    if currentRealm is not nil:
        results["realm"] = HealthStatus {
            Status: Healthy,
            Message: "joined realm: {currentRealm.ID()}"
        }
    
    return results

// nodeHealthStatus 根据节点状态返回健康状态
function nodeHealthStatus() → Status:
    switch node.state:
        case Running:       return Healthy
        case Initializing:  return Degraded
        case Starting:      return Degraded
        default:            return Unhealthy

func statusFromBool(ok bool) Status {
    if ok {
        return StatusHealthy
    }
    return StatusUnhealthy
}
```

### 组件级健康检查示例

```
// Discovery 健康检查实现（伪代码）
// 位置：internal/discovery/discovery.go

// 实现 HealthChecker 接口
function Discovery.Check(ctx) → HealthStatus:
    activeServices = 0
    details = {}
    
    // 检查 DHT
    if dht is not nil:
        activeServices++
        details["dht"] = "active"
    
    // 检查 mDNS
    if mdns is not nil:
        activeServices++
        details["mdns"] = "active"
    
    return HealthStatus {
        Status: Healthy if activeServices > 0 else Unhealthy,
        Message: "{activeServices} discovery services active",
        Details: details
    }

function Discovery.Name() → string:
    return "discovery"
```

---

## 模块组合

### 模块集定义

```
// 位置：bootstrap/modules.go

// Core 模块集
CoreModules = [
    identity.Module,
    security.Module,
    transport.Module,
    muxer.Module,
    connmgr.Module,
    host.Module,
    relay.Module,
    nat.Module
]

// Discovery 模块集
DiscoveryModules = [
    dht.Module,
    bootstrap.Module,
    mdns.Module,
    rendezvous.Module,
    coordinator.Module
]

// Biz 模块集
BizModules = [
    realm.Module
]

// App 模块集
AppModules = [
    messaging.Module,
    pubsub.Module,
    streams.Module,
    liveness.Module,
    node.Module
]

// 所有模块（按启动顺序）
AllModules = [
    CoreModules,
    DiscoveryModules,
    BizModules,
    AppModules
]
```

### 应用启动

```
// 位置：bootstrap/bootstrap.go

function NewApp(options...) → App {
    opts = [
        // 基础模块
        AllModules,
        
        // 配置
        provide(config.Load),
        
        // 日志
        withLogger(fxLogger)
    ]
    
    // 添加用户选项
    opts = append(opts, options...)
    
    return fx.New(opts...)
}

function Run(app: App) → error {
    startCtx = context.WithTimeout(background(), 30 * Second)
    
    if err = app.Start(startCtx); err != nil {
        return err
    }
    
    // 等待停止信号
    <-app.Done()
    
    stopCtx = context.WithTimeout(background(), 30 * Second)
    return app.Stop(stopCtx)
}
```

---

## 接口绑定模式

### 双接口绑定

每个模块同时绑定公共接口和内部接口：

```
// 位置：internal/core/host/module.go

Module HostModule {
    provide: [NewHost]
    
    // 绑定到公共接口（外部/App 层使用）
    bind: [
        HostImpl → Host (public)
    ]
    
    // 绑定到内部接口（内部组件使用）
    bind: [
        HostImpl → InternalHost (internal)
    ]
}
```

### 依赖选择

```
// App 层依赖内部 Host 接口
struct MessagingParams {
    Host: InternalHost       // 内部接口，有更多方法
    Realm: InternalRealm
}

// 对外提供公共接口
struct NodeResult {
    Node: Node               // 公共接口
}
```

---

## 配置注入

### 配置提供

```
// 位置：config/provider.go

Module ConfigModule {
    provide: [
        Load,                    // 加载主配置
        NewMessagingConfig,      // 消息配置
        NewPubSubConfig,         // 发布订阅配置
        NewRelayConfig,          // 中继配置
        NewHostConfig            // Host 配置
    ]
}

function Load() → Config, error {
    return loadFromEnv()
}

function NewMessagingConfig(cfg: Config) → MessagingConfig {
    return cfg.Messaging
}

function NewHostConfig(cfg: Config) → HostConfig {
    return cfg.Host
}
```

### 配置结构

```
struct Config {
    Identity: IdentityConfig
    Host: HostConfig
    Transport: TransportConfig
    Messaging: MessagingConfig
    PubSub: PubSubConfig
    Relay: RelayConfig
}

struct HostConfig {
    ListenAddrs: []string
    LowWater: int
    HighWater: int
}

struct MessagingConfig {
    MaxMessageSize: int
    RequestTimeout: Duration
}
```

---

## 可选依赖

### 使用标签

```
// 可选依赖示例

struct ServiceParams {
    Host: InternalHost
    Realm: InternalRealm
    Metrics: MetricsReporter [optional]  // 可选依赖
}

function NewService(params: ServiceParams) → Service {
    service = new Service {
        host: params.Host,
        realm: params.Realm
    }
    
    // 可选依赖检查
    if params.Metrics != nil {
        service.metrics = params.Metrics
    }
    
    return service
}
```

### 使用命名依赖

```
// 命名依赖示例

// ★ 统一 Relay（不再区分 System/Realm）
Module RelayModule {
    provide: [
        NewRelay [name: "relay"]
    ]
}

// 使用命名依赖
struct ServiceParams {
    Relay: RelayService [name: "relay"]
}

// v2.0 Relay 三大职责：
//   1. 缓存加速层（非权威目录，DHT 才是权威）
//   2. 打洞协调信令
//   3. 数据通信保底
```

---

## 最佳实践

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Fx + Lifecycle 最佳实践                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 保持构造函数简单                                                          │
│  ──────────────────────                                                     │
│  构造函数只做依赖注入和结构初始化                                               │
│  复杂初始化放到 OnStart 中                                                   │
│                                                                             │
│  2. 优雅处理停止                                                             │
│  ────────────────                                                           │
│  OnStop 中要处理所有清理逻辑                                                  │
│  使用 Context 超时，避免无限等待                                               │
│                                                                             │
│  3. 避免循环依赖                                                             │
│  ────────────────                                                           │
│  如果出现循环依赖，考虑：                                                      │
│  - 提取公共接口                                                              │
│  - 使用事件驱动                                                              │
│  - 延迟注入                                                                  │
│                                                                             │
│  4. 使用模块分组                                                             │
│  ────────────────                                                           │
│  按功能域分组模块                                                             │
│  便于测试时替换部分模块                                                        │
│                                                                             │
│  5. 配置与服务分离                                                            │
│  ──────────────────                                                         │
│  配置通过单独的 Provider 提供                                                 │
│  便于测试时使用不同配置                                                        │
│                                                                             │
│  6. 双接口绑定                                                               │
│  ──────────────                                                             │
│  同时绑定公共接口和内部接口                                                    │
│  公共接口对外，内部接口供内部使用                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 测试支持

### 模拟模块替换

```
// 测试中替换模块

function TestService(t: Test) {
    app = fxtest.New(t,
        // 使用真实模块
        messaging.Module,
        
        // 替换依赖为 Mock
        replace: [
            MockHost → InternalHost,
            MockRealm → InternalRealm
        ]
    )
    
    app.RequireStart()
    defer app.RequireStop()
    
    // 测试逻辑
}
```

### 部分模块测试

```
// 只测试特定层的模块

function TestCoreLayer(t: Test) {
    app = fxtest.New(t,
        // 只使用 Core + Discovery
        CoreModules,
        DiscoveryModules,
        
        // 提供测试配置
        provide: [testConfig]
    )
    
    app.RequireStart()
    defer app.RequireStop()
}
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [public_interfaces.md](public_interfaces.md) | 公共接口设计 |
| [internal_interfaces.md](internal_interfaces.md) | 内部接口设计 |
| [component_interface_map.md](component_interface_map.md) | 组件-接口映射 |
| [../L2_structural/module_design.md](../L2_structural/module_design.md) | 模块划分 |
| [../L2_structural/target_structure.md](../L2_structural/target_structure.md) | 目标目录结构 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [assembly_architecture.md](../L2_structural/assembly_architecture.md) | 组件装配架构 |
| [config_architecture.md](../L2_structural/config_architecture.md) | 配置管理架构 |

---

**最后更新**：2026-01-24（v2.0 DHT 权威模型对齐）
