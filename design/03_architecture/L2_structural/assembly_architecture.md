# 组件装配架构 (Component Assembly Architecture)

> **版本**: v1.1.0  
> **更新日期**: 2026-01-23  
> **定位**: DeP2P 组件装配、启动流程与生命周期管理

---

## 概述

本文档定义 DeP2P 的**组件装配架构**，包括：

- **模块分类与注册策略** - 核心/条件/扩展模块的装配规则
- **启动阶段划分** - Initialize → Prepare → Start → Ready
- **健康检查机制** - 组件状态监控与诊断
- **优雅关闭流程** - 反向顺序停止与资源清理
- **用户扩展点** - 自定义模块注入

---

## 设计原则

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         组件装配设计原则                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  1. 按需加载 (Lazy Loading)                                             │
│     只加载用户启用的模块，减少内存占用和启动时间                           │
│                                                                         │
│  2. 配置验证前置 (Early Validation)                                     │
│     在启动前验证配置，快速失败，避免资源浪费                               │
│                                                                         │
│  3. 显式依赖 (Explicit Dependencies)                                    │
│     通过 Fx 依赖注入明确模块间依赖关系                                     │
│                                                                         │
│  4. 阶段化启动 (Phased Startup)                                         │
│     将启动过程分为多个阶段，便于监控和故障排查                             │
│                                                                         │
│  5. 优雅降级 (Graceful Degradation)                                     │
│     非关键组件失败不影响核心功能（未来）                                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 模块分类

### 模块分类体系

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            模块分类                                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  核心模块 (Core Modules) - 必须加载                                      │
│  ────────────────────────────────────                                   │
│  • Identity         - 身份管理                                           │
│  • EventBus         - 事件总线                                           │
│  • Peerstore        - 节点存储                                           │
│  • Protocol         - 协议注册表                                         │
│  • Metrics          - 指标系统（如启用）                                  │
│                                                                         │
│  传输层模块 (Transport Modules) - 条件加载                                │
│  ────────────────────────────────────────────                           │
│  • Transport        - QUIC/TCP 传输（任一启用时加载）                     │
│  • Security         - TLS/Noise 安全层                                   │
│  • Muxer            - 流多路复用                                         │
│  • Upgrader         - 连接升级器                                         │
│  • Swarm            - 连接池                                             │
│  • Host             - 网络主机                                           │
│                                                                         │
│  网络层模块 (Network Modules) - 条件加载                                  │
│  ────────────────────────────────────────────                           │
│  • ConnMgr          - 连接管理（HighWater > 0）                          │
│  • ResourceMgr      - 资源管理（启用时）                                  │
│  • NAT              - NAT 穿透（任一 NAT 功能启用）                       │
│  • Relay            - 中继服务（客户端/服务端启用时）                      │
│                                                                         │
│  发现层模块 (Discovery Modules) - 条件加载                                │
│  ────────────────────────────────────────────                           │
│  • DHT              - DHT 发现（EnableDHT）                              │
│  • mDNS             - 局域网发现（EnableMDNS）                            │
│  • Bootstrap        - 引导节点（EnableBootstrap）                        │
│  • DNS              - DNS 发现（EnableDNS）                              │
│  • Rendezvous       - 命名空间发现（EnableRendezvous）                   │
│  • Discovery        - 发现协调器（任一发现服务启用）                       │
│                                                                         │
│  Realm 层模块 (Realm Modules) - 条件加载                                 │
│  ────────────────────────────────────────────                           │
│  • Auth             - PSK 认证（EnableAuth）                             │
│  • Member           - 成员管理（EnableMember）                           │
│  • Routing          - Realm 路由（EnableRouting）                       │
│  • Gateway          - Realm 网关（EnableGateway）                       │
│  • RealmManager     - Realm 管理器（任一 Realm 功能启用）                │
│                                                                         │
│  协议层模块 (Protocol Modules) - 条件加载                                 │
│  ────────────────────────────────────────────                           │
│  • Messaging        - 点对点消息（默认启用）                              │
│  • PubSub           - 发布订阅（EnablePubSub）                           │
│  • Streams          - 流管理（EnableStreams）                            │
│  • Liveness         - 存活检测（EnableLiveness）                         │
│                                                                         │
│  扩展模块 (Extension Modules) - 用户提供                                  │
│  ────────────────────────────────────────────                           │
│  • UserFxOptions    - 用户自定义 Fx 模块                                 │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 条件加载逻辑

```pseudocode
// 传输层条件
if cfg.Transport.EnableQUIC or cfg.Transport.EnableTCP:
    modules.append(transport.Module, security.Module, ...)

// NAT 穿透条件
if cfg.NAT.EnableAutoNAT or cfg.NAT.EnableUPnP or cfg.NAT.EnableHolePunch:
    modules.append(nat.Module)

// ★ 打洞需要信令通道（Relay 连接提供）
// 若启用 HolePunch 但未启用 Relay Client，发出警告或自动启用
if cfg.NAT.EnableHolePunch and not cfg.Relay.EnableClient:
    log.Warn("HolePunch 依赖 Relay 连接作为信令通道，建议启用 Relay Client")

// 中继服务条件
// Relay 三大职责 (v2.0)：缓存加速层 + 打洞协调信令 + 数据通信保底
// ★ DHT 是权威目录，Relay 地址簿是缓存
if cfg.Relay.EnableClient or cfg.Relay.EnableServer:
    modules.append(relay.Module)

// 发现服务条件
if cfg.Discovery.EnableDHT:
    modules.append(dht.Module)

// Discovery 协调器条件（至少一个发现服务启用）
if hasAnyDiscovery(cfg):
    modules.append(discovery.Module)
```

---

## 启动阶段

### 阶段定义

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         启动阶段流程图                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  New(opts...)                                                           │
│      │                                                                  │
│      ├─► 配置构建 (Configuration)                                        │
│      │   • 应用 Preset                                                  │
│      │   • 应用 Functional Options                                      │
│      │   • 配置验证 (Validate)                                           │
│      │                                                                  │
│      ├─► 模块装配 (Assembly)                                             │
│      │   • 核心模块（必须）                                              │
│      │   • 条件模块（按配置）                                            │
│      │   • 扩展模块（用户提供）                                          │
│      │   • 构建 Fx App                                                  │
│      │                                                                  │
│      └─► 返回 Node 实例 (未启动)                                         │
│                                                                         │
│  Start(ctx)                                                             │
│      │                                                                  │
│      ├─► Phase 1: Initialize (初始化)                                   │
│      │   • 状态: StateInitializing                                      │
│      │   • 加载身份密钥                                                  │
│      │   • 分配资源                                                      │
│      │   • 调用 Fx app.Start()                                          │
│      │   • 超时: 10s                                                    │
│      │                                                                  │
│      ├─► Phase 2: Prepare (准备)                                        │
│      │   • 状态: StateStarting                                          │
│      │   • 创建内部组件                                                  │
│      │   • 注册协议处理器                                                │
│      │   • 初始化存储                                                    │
│      │                                                                  │
│      ├─► Phase 3: Start (启动)                                          │
│      │   • 开始网络监听                                                  │
│      │   • 连接引导节点                                                  │
│      │   • ★ 建立 Relay 连接（若配置）                                   │
│      │     - 缓存加速层：注册到 Relay 地址簿（非权威，DHT 为权威）        │
│      │     - 打洞协调信令：打洞协调的前置依赖                             │
│      │     - 惰性连接：配置 ≠ 立即连接，按需建立                          │
│      │   • 启动发现服务                                                  │
│      │                                                                  │
│      ├─► Phase 4: Ready Check (就绪检查)                                │
│      │   • 等待监听地址就绪                                              │
│      │   • ★ 外部地址发现 → 可达性验证 → 可发布地址                       │
│      │   • 健康检查通过                                                  │
│      │   • 超时: 5s                                                     │
│      │                                                                  │
│      └─► Phase 5: Running (运行中)                                      │
│          • 状态: StateRunning                                           │
│          • 发出 EventNodeReady                                          │
│          • 接受用户请求                                                  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 状态机

```pseudocode
// NodeState 节点状态
enum NodeState:
    Idle         = 0  // 空闲（未启动）
    Initializing = 1  // 初始化中
    Starting     = 2  // 启动中
    Running      = 3  // 运行中
    Stopping     = 4  // 停止中
    Stopped      = 5  // 已停止

// 状态转换规则
// Idle → Initializing → Starting → Running
// Running → Stopping → Stopped
```

### 启动超时

| 阶段 | 超时时间 | 失败处理 |
|------|----------|----------|
| Initialize | 10s | 清理资源，返回错误 |
| Ready Check | 5s | 停止 Fx App，返回错误 |

---

## 配置验证

### 验证时机

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        配置验证时机                                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  buildFxApp(cfg, node) {                                                │
│      // 1. 配置验证（前置）                                              │
│      if err := cfg.config.Validate(); err != nil {                      │
│          return nil, fmt.Errorf("config validation: %w", err)           │
│      }                                                                   │
│                                                                         │
│      // 2. 条件模块装配                                                  │
│      modules := []fx.Option{...}                                        │
│                                                                         │
│      // 3. 构建 Fx App                                                  │
│      return fx.New(modules...), nil                                     │
│  }                                                                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 验证规则

```pseudocode
function Config.Validate() -> error:
    // 1. 连接管理验证
    if c.ConnMgr.HighWater < c.ConnMgr.LowWater:
        return error("ConnMgr.HighWater must >= LowWater")
    
    // 2. 传输层验证
    if not c.Transport.EnableQUIC and not c.Transport.EnableTCP and not c.Transport.EnableWebSocket:
        return error("at least one transport must be enabled")
    
    // 3. 安全层验证
    if not c.Security.EnableTLS and not c.Security.EnableNoise:
        return error("at least one security transport must be enabled")
    
    // 4. Bootstrap 验证
    if c.Discovery.EnableBootstrap and len(c.Discovery.Bootstrap.Peers) == 0:
        return error("bootstrap enabled but no peers configured")
    
    // 5. 资源限制验证
    if c.Resource.EnableResourceManager:
        if c.Resource.System.MaxConnections <= 0:
            return error("invalid resource limit: MaxConnections must > 0")
    
    return nil
```

---

## 健康检查

### 健康检查接口

```pseudocode
// pkg/interfaces/health.go

// HealthChecker 健康检查接口
interface HealthChecker:
    // Check 执行健康检查
    Check(ctx: Context) -> HealthStatus
    
    // Name 组件名称
    Name() -> string

// HealthStatus 健康状态
struct HealthStatus:
    Status: Status           // 状态
    Message: string          // 状态描述
    Details: Map<string, any>  // 详细信息
    Timestamp: Time          // 检查时间

// Status 状态枚举
enum Status:
    Healthy   = 0  // 健康
    Degraded  = 1  // 降级
    Unhealthy = 2  // 不健康
```

### Node 健康检查实现

```pseudocode
// node.go

// Health 获取节点健康状态
function Node.Health(ctx: Context) -> Map<string, HealthStatus>:
    results = Map<string, HealthStatus>{}
    
    // 1. Host 健康检查
    if n.host != nil:
        addrs = n.host.Addrs()
        results["host"] = HealthStatus{
            Status:    statusFromBool(len(addrs) > 0),
            Message:   "listening on {len(addrs)} addresses",
            Details:   {"addresses": addrs},
            Timestamp: Now()
        }
    else:
        results["host"] = HealthStatus{
            Status:    Unhealthy,
            Message:   "host not initialized",
            Timestamp: Now()
        }
    
    // 2. 连接状态
    connCount := n.ConnectionCount()
    results["connections"] = HealthStatus{
        Status:    statusFromBool(connCount >= 0), // 0 连接也是健康的
        Message:   fmt.Sprintf("%d active connections", connCount),
        Details:   map[string]interface{}{"count": connCount},
        Timestamp: time.Now(),
    }
    
    // 3. Discovery 健康检查
    if n.discovery != nil {
        // 假设 Discovery 实现了 HealthChecker
        if hc, ok := n.discovery.(HealthChecker); ok {
            results["discovery"] = hc.Check(ctx)
        }
    }
    
    // 4. Realm 健康检查
    if n.currentRealm != nil {
        results["realm"] = HealthStatus{
            Status:    StatusHealthy,
            Message:   fmt.Sprintf("joined realm: %s", n.currentRealm.ID()),
            Timestamp: time.Now(),
        }
    }
    
    return results
}

// statusFromBool 从布尔值转换状态
function statusFromBool(ok: bool) -> Status:
    if ok:
        return Healthy
    return Unhealthy
```

### 健康检查端点

```pseudocode
// cmd/dep2p/main.go

// 提供 HTTP 健康检查端点
http.HandleFunc("/health", handler(w, r):
    ctx = withTimeout(r.Context(), 5s)
    
    health = node.Health(ctx)
    
    // 判断整体健康状态
    allHealthy = true
    for status in health:
        if status.Status != Healthy:
            allHealthy = false
            break
    
    // 设置 HTTP 状态码
    if allHealthy:
        w.WriteHeader(200)
    else:
        w.WriteHeader(503)
    
    // 返回 JSON
    json.Encode(w, health)
)
```

---

## 优雅关闭

### 关闭流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         关闭流程                                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Stop(ctx) / Close()                                                    │
│      │                                                                  │
│      ├─► 状态检查                                                        │
│      │   • 已关闭 → 返回 nil（幂等）                                     │
│      │   • 运行中 → 继续关闭流程                                         │
│      │                                                                  │
│      ├─► 状态转换: StateStopping                                         │
│      │                                                                  │
│      ├─► Fx 反向停止                                                     │
│      │   • app.Stop(ctx)                                                │
│      │   • 按反向依赖顺序调用 OnStop                                      │
│      │   • 超时: 30s                                                    │
│      │                                                                  │
│      ├─► 协议层停止                                                      │
│      │   • Liveness → Streams → PubSub → Messaging                     │
│      │                                                                  │
│      ├─► Realm 层停止                                                    │
│      │   • RealmManager → Gateway → Routing → Member → Auth            │
│      │                                                                  │
│      ├─► Discovery 层停止                                                │
│      │   • Discovery → Rendezvous → DNS → Bootstrap → mDNS → DHT       │
│      │                                                                  │
│      ├─► Core 层停止                                                     │
│      │   • Host → Relay → NAT → ConnMgr → Swarm → Upgrader            │
│      │   • → Muxer → Security → Transport → ResourceMgr                │
│      │                                                                  │
│      ├─► 基础组件停止                                                    │
│      │   • Peerstore → EventBus → Identity                             │
│      │                                                                  │
│      └─► 状态转换: StateStopped                                          │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 停止超时

| 操作 | 超时时间 | 失败处理 |
|------|----------|----------|
| app.Stop() | 30s | 强制返回，记录警告 |
| 单个组件 OnStop | 5s | 继续下一个组件 |

---

## 用户扩展点

### Fx 扩展选项

```pseudocode
// options.go

// WithFxOption 添加自定义 Fx 模块
//
// 允许用户注入自定义 Fx 模块到 DEP2P 的依赖注入容器中。
//
// 示例：
//   node = NewNode(ctx,
//       WithFxOption(Provide(NewCustomService)),
//       WithFxOption(Invoke((cs: CustomService) => { /* 使用自定义服务 */ }))
//   )
function WithFxOption(opt: FxOption) -> Option:
    return function(cfg: nodeConfig) -> error:
        cfg.userFxOptions.append(opt)
        return nil

// WithFxOptions 添加多个自定义 Fx 模块
function WithFxOptions(opts: ...FxOption) -> Option:
    return function(cfg: nodeConfig) -> error:
        cfg.userFxOptions.append(opts...)
        return nil
```

### 使用示例

```pseudocode
// 示例：添加自定义监控服务

struct CustomMetrics:
    node: Node

function NewCustomMetrics(node: Node) -> CustomMetrics:
    return CustomMetrics{node: node}

function CustomMetrics.Start():
    // 定期收集指标

// 创建节点时注入
node = NewNode(ctx,
    WithPreset(PresetServer),
    WithFxOption(
        Provide(NewCustomMetrics),
        Invoke((m: CustomMetrics) => { async m.Start() })
    )
)
```

---

## 装配代码示例

### 完整的 buildFxApp 实现

```pseudocode
// fx.go

function buildFxApp(cfg: nodeConfig, node: Node) -> FxApp:
    // ════════════════════════════════════════════════════════════════
    // 1. 配置验证（前置）
    // ════════════════════════════════════════════════════════════════
    if err = cfg.config.Validate(); err != nil:
        return error("config validation: " + err)
    
    // ════════════════════════════════════════════════════════════════
    // 2. 核心模块（必须）
    // ════════════════════════════════════════════════════════════════
    modules = [
        // 配置注入
        Supply(cfg),
        Supply(cfg.config),
        
        // 基础组件
        identity.Module,
        eventbus.Module,
        peerstore.Module,
        protocol.Module
    ]
    
    // 指标系统（如启用资源管理）
    if cfg.config.Resource.EnableResourceManager:
        modules.append(metrics.Module)
    
    // ════════════════════════════════════════════════════════════════
    // 3. 传输层（条件）
    // ════════════════════════════════════════════════════════════════
    if cfg.config.Transport.EnableQUIC or cfg.config.Transport.EnableTCP:
        modules.append(
            transport.Module,
            security.Module,
            muxer.Module,
            upgrader.Module,
            swarm.Module
        )
        
        // Host 依赖传输层
        modules.append(host.Module)
    else:
        return error("no transport enabled")
    
    // ════════════════════════════════════════════════════════════════
    // 4. 网络层（条件）
    // ════════════════════════════════════════════════════════════════
    if cfg.config.ConnMgr.HighWater > 0:
        modules.append(connmgr.Module)
    
    if cfg.config.Resource.EnableResourceManager:
        modules.append(resourcemgr.Module)
    
    if cfg.config.NAT.EnableAutoNAT or cfg.config.NAT.EnableUPnP or 
       cfg.config.NAT.EnableHolePunch:
        modules.append(nat.Module)
    
    if cfg.config.Relay.EnableClient or cfg.config.Relay.EnableServer:
        modules.append(relay.Module)
    
    // ════════════════════════════════════════════════════════════════
    // 5. 发现层（条件）
    // ════════════════════════════════════════════════════════════════
    if cfg.config.Discovery.EnableDHT:
        modules.append(dht.Module)
    if cfg.config.Discovery.EnableMDNS:
        modules.append(mdns.Module)
    if cfg.config.Discovery.EnableBootstrap:
        modules.append(bootstrap.Module)
    if cfg.config.Discovery.EnableDNS:
        modules.append(dns.Module)
    if cfg.config.Discovery.EnableRendezvous:
        modules.append(rendezvous.Module)
    
    // Discovery 协调器（如有任何发现服务）
    if hasAnyDiscovery(cfg.config):
        modules.append(discovery.Module)
    
    // ════════════════════════════════════════════════════════════════
    // 6. Realm 层（条件）
    // ════════════════════════════════════════════════════════════════
    if hasAnyRealm(cfg.config):
        if cfg.config.Realm.EnableAuth:
            modules.append(auth.Module)
        if cfg.config.Realm.EnableMember:
            modules.append(member.Module)
        if cfg.config.Realm.EnableRouting:
            modules.append(routing.Module)
        if cfg.config.Realm.EnableGateway:
            modules.append(gateway.Module)
        modules.append(realm.Module)
    
    // ════════════════════════════════════════════════════════════════
    // 7. 协议层（条件）
    // ════════════════════════════════════════════════════════════════
    // Messaging 默认启用
    modules.append(messaging.Module)
    
    if cfg.config.Messaging.EnablePubSub:
        modules.append(pubsub.Module)
    if cfg.config.Messaging.EnableStreams:
        modules.append(streams.Module)
    if cfg.config.Messaging.EnableLiveness:
        modules.append(liveness.Module)
    
    // ════════════════════════════════════════════════════════════════
    // 8. 用户扩展
    // ════════════════════════════════════════════════════════════════
    modules.append(cfg.userFxOptions...)
    
    // ════════════════════════════════════════════════════════════════
    // 9. Node 注入
    // ════════════════════════════════════════════════════════════════
    modules.append(Invoke(injectNode(node)))
    
    // ════════════════════════════════════════════════════════════════
    // 10. Fx 配置
    // ════════════════════════════════════════════════════════════════
    modules.append(WithLogger(NopLogger))
    
    return FxNew(modules...)

// 辅助函数
function hasAnyDiscovery(cfg: Config) -> bool:
    return cfg.Discovery.EnableDHT or
           cfg.Discovery.EnableMDNS or
           cfg.Discovery.EnableBootstrap or
           cfg.Discovery.EnableDNS or
           cfg.Discovery.EnableRendezvous

function hasAnyRealm(cfg: Config) -> bool:
    return cfg.Realm.EnableAuth or
           cfg.Realm.EnableMember or
           cfg.Realm.EnableRouting or
           cfg.Realm.EnableGateway

function injectNode(node: Node) -> Injector:
    return function(host: Host, realmManager: RealmManager, discovery: Discovery):
        node.host = host
        node.realmManager = realmManager
        node.discovery = discovery
```

---

## 最佳实践

### 1. 模块开发规范

```pseudocode
// internal/xxx/module.go

// Module 定义 Fx 模块
Module = FxModule("layer.component",
    Provide(NewFromParams)
)

// Params 依赖参数
struct Params:
    UnifiedCfg: Config [optional]
    // 其他依赖...

// ConfigFromUnified 从统一配置提取模块配置
function ConfigFromUnified(cfg: Config) -> Config:
    if cfg == nil or not cfg.EnableXXX:
        return nil  // 禁用时返回 nil
    return Config{
        // 映射配置字段
    }

// NewFromParams 从 Fx 参数创建实例
function NewFromParams(p: Params) -> Component:
    cfg = ConfigFromUnified(p.UnifiedCfg)
    if cfg == nil:
        return nil  // 禁用时返回 nil
    return New(cfg)
```

### 2. 错误处理

```pseudocode
// 配置验证错误
if err = cfg.Validate(); err != nil:
    return error("config validation: " + err)

// 启动超时错误
ctx = withTimeout(ctx, 10s)

if err = n.app.Start(ctx); err != nil:
    if err is DeadlineExceeded:
        return error("initialize timeout: " + err)
    return error("initialize failed: " + err)
```

### 3. 资源清理

```pseudocode
// New 中的资源清理
defer:
    if err != nil:
        if node.app != nil:
            node.app.Stop(backgroundContext())

// Stop 中的超时保护
ctx = withTimeout(backgroundContext(), 30s)

if err = n.app.Stop(ctx); err != nil:
    // 记录但不失败
    log.Warn("stop timeout: " + err)
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [fx_lifecycle.md](../L4_interfaces/fx_lifecycle.md) | Fx 生命周期管理详细规范 |
| [config_architecture.md](config_architecture.md) | 配置管理架构 |
| [module_design.md](module_design.md) | 模块设计规范 |
| [dependency_rules.md](dependency_rules.md) | 依赖关系规则 |

---

**最后更新**: 2026-01-15  
**作者**: DEP2P Team
