# Discovery Bootstrap 模块

> **版本**: v1.2.0  
> **更新日期**: 2026-01-16  
> **定位**: 引导节点发现（Discovery Layer）

---

## 模块概述

discovery_bootstrap 负责通过预配置的引导节点进行初始节点发现，是网络启动的第一步。

| 属性 | 值 |
|------|-----|
| **架构层** | Discovery Layer |
| **代码位置** | `internal/discovery/bootstrap/` |
| **Fx 模块** | `fx.Module("discovery/bootstrap")` |
| **状态** | ✅ 已实现 |
| **依赖** | core_host |

---

## 极简配置原则（ADR-0009）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Bootstrap 极简配置原则                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ★ 核心理念：用户只需决定"开不开"，系统处理一切细节                         │
│                                                                             │
│  用户配置：                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  dep2p.EnableBootstrap(true)     // 唯一需要设置的                  │   │
│  │                                                                     │   │
│  │  ✗ 不需要：MaxNodes、ProbeInterval、DiscoveryInterval               │   │
│  │  ✗ 不需要：PersistPath、DatabaseType、ExpireTime                    │   │
│  │  ✗ 不需要：ResponseK、NodeCount 等                                  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  设计决策：                                                                  │
│  • 所有运营参数使用经过调优的内置默认值                                     │
│  • 用户无法通过配置文件或环境变量修改这些参数                               │
│  • 这是"能力开关"而非"节点角色"                                             │
│  • 同一节点可同时启用 Bootstrap + Relay（能力融合）                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 使用方式

```go
// 方式 1：启动时启用（推荐）
node, _ := dep2p.NewNode(ctx, dep2p.EnableBootstrap(true))

// 方式 2：运行时启用/禁用
node.EnableBootstrap(ctx)   // 启用 Bootstrap 能力
node.DisableBootstrap(ctx)  // 禁用 Bootstrap 能力

// 方式 3：查询状态
if node.IsBootstrapEnabled() {
    stats := node.BootstrapStats()
    fmt.Printf("存储节点: %d\n", stats.TotalNodes)
}
```

### 前置条件

启用 Bootstrap 能力需满足：
- **公网可达**：必须有公网可直连的地址（非 NAT 后）
- **稳定运行**：建议 7×24 小时在线

---

## ★ 基础设施节点融合设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    基础设施节点融合设计（设计建议，非强制）                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ⚠️ 这是推荐的设计方向，不是强制约束。实际部署可根据需求选择。                │
│                                                                             │
│  分离设计（传统）：                                                          │
│  ──────────────────                                                         │
│  • Bootstrap 节点：DHT 引导                                                 │
│  • STUN 服务器：外部地址发现                                                │
│  • Relay 节点：数据转发                                                     │
│  特点：职责清晰，但部署复杂                                                 │
│                                                                             │
│  融合设计（推荐）：                                                          │
│  ──────────────────                                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  基础设施节点 = DHT 引导 + 地址发现 + 数据转发 + 打洞协调             │   │
│  │                                                                      │   │
│  │  可选提供（按需启用）：                                              │   │
│  │  • DHT 引导（让新节点加入网络）                                      │   │
│  │  • 地址发现（连接时提供观察地址 + 地址簿查询）                       │   │
│  │  • 数据转发（当打洞失败时）                                          │   │
│  │  • 打洞协调通道（在 Relay 连接上进行）                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ★ 参考：iroh 的设计将 Relay 同时提供地址发现和数据转发                     │
│  ★ 注意：不是所有 Bootstrap 都必须承担 Relay/identify，取决于部署配置      │
│                                                                             │
│  启用方式：                                                                  │
│  ──────────                                                                 │
│  dep2p.NewNode(ctx,                                                        │
│      dep2p.EnableBootstrap(true),  // 启用 Bootstrap 能力                  │
│      dep2p.EnableRelay(true),      // 启用 Relay 能力                      │
│  )                                                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    discovery_bootstrap 职责                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 引导连接                                                                 │
│     • 连接预配置的引导节点                                                  │
│     • 获取初始节点列表                                                      │
│     • 引导网络加入流程                                                      │
│                                                                             │
│  2. 引导节点管理                                                             │
│     • 提供默认引导节点列表                                                  │
│     • 支持自定义引导节点                                                    │
│     • 引导节点健康检查                                                      │
│                                                                             │
│  3. 网络初始化                                                               │
│     • 触发 DHT 路由表填充（启动时自动触发 DHT.Bootstrap()）                │
│     • 启动节点发现流程                                                      │
│                                                                             │
│  ★ Bootstrap 与 DHT.Bootstrap 职责边界：                                    │
│  ────────────────────────────────────────                                   │
│  • Bootstrap 服务：初始网络入口，并发连接预配置的引导节点                   │
│  • DHT.Bootstrap()：填充路由表，复用 Bootstrap 已连接的节点                 │
│  • 启动时自动触发：DHT 启动后异步调用 DHT.Bootstrap()                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

### Node 级别接口（Bootstrap 能力开关）

```go
// pkg/interfaces/node.go (Bootstrap 能力部分)

type Node interface {
    // ... 其他方法 ...
    
    // Bootstrap 能力开关
    EnableBootstrap(ctx context.Context) error   // 启用 Bootstrap 能力
    DisableBootstrap(ctx context.Context) error  // 禁用 Bootstrap 能力
    IsBootstrapEnabled() bool                    // 查询是否已启用
    BootstrapStats() BootstrapStats              // 获取统计信息
}

// BootstrapStats Bootstrap 统计信息
type BootstrapStats struct {
    Enabled        bool          // 是否启用
    TotalNodes     int           // 存储节点总数
    OnlineNodes    int           // 在线节点数
    LastProbe      time.Time     // 最后探测时间
    LastDiscovery  time.Time     // 最后主动发现时间
}
```

### Discovery 层接口（内部使用）

```go
// pkg/interfaces/discovery.go (Bootstrap 部分)

// BootstrapDiscovery 引导发现接口
type BootstrapDiscovery interface {
    Discovery
    
    // Bootstrap 执行引导流程
    Bootstrap(ctx context.Context) error
    
    // BootstrapPeers 返回引导节点列表
    BootstrapPeers() []types.PeerInfo
    
    // SetBootstrapPeers 设置引导节点列表
    SetBootstrapPeers(peers []types.PeerInfo)
}
```

---

## 默认引导节点

DeP2P 官方引导节点配置在**统一配置系统**中（`config/discovery.go`）。

```go
// 位置：config/discovery.go

func DefaultDiscoveryConfig() DiscoveryConfig {
    return DiscoveryConfig{
        Bootstrap: BootstrapConfig{
            Peers: []string{
                // DeP2P 官方引导节点（占位符）
                "/dnsaddr/bootstrap1.dep2p.io/p2p/12D3KooWBootstrap1Placeholder",
                "/dnsaddr/bootstrap2.dep2p.io/p2p/12D3KooWBootstrap2Placeholder",
                "/dnsaddr/bootstrap-asia.dep2p.io/p2p/12D3KooWBootstrap3Placeholder",
                "/dnsaddr/bootstrap-eu.dep2p.io/p2p/12D3KooWBootstrap4Placeholder",
            },
            MinPeers: 4,
            Timeout:  30 * time.Second,
        },
    }
}
```

**配置特点**：
- 默认值在 `config/` 包中定义（统一配置管理）
- 使用 DNS 地址（dnsaddr），便于后续更换 IP
- 当前为占位符 PeerID，待正式部署后填充
- 用户可通过 JSON 配置文件或代码完全覆盖

---

## 配置来源

Bootstrap 配置支持两种来源，按优先级排序：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    配置来源与优先级                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  配置来源：                                                                  │
│  • 默认配置：config.DefaultDiscoveryConfig().Bootstrap.Peers                │
│  • 用户覆盖：通过 JSON 配置文件或代码修改                                   │
│                                                                             │
│  优先级：                                                                    │
│  1. 用户 JSON 配置文件                                                       │
│  2. 用户代码配置                                                             │
│  3. 系统默认配置（config/discovery.go）                                     │
│                                                                             │
│  设计原则：                                                                  │
│  • 所有默认值在 config/ 包中定义                                            │
│  • internal/ 模块不硬编码配置                                               │
│  • 用户可通过配置系统完全控制                                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**配置示例**：

```go
// 1. 使用默认引导节点（来自 config/discovery.go）
cfg := config.NewConfig()
// cfg.Discovery.Bootstrap.Peers 已包含默认节点

// 2. 覆盖为自定义引导节点
cfg := config.NewConfig()
cfg.Discovery.Bootstrap.Peers = []string{
    "/ip4/10.0.0.1/tcp/4001/p2p/QmCustomBootstrap1",
    "/ip4/10.0.0.2/tcp/4001/p2p/QmCustomBootstrap2",
}

// 3. 清空引导节点列表
cfg := config.NewConfig()
cfg.Discovery.Bootstrap.Peers = []string{}

// 4. 禁用引导节点功能
cfg := config.NewConfig()
cfg.Discovery.EnableBootstrap = false

// 5. 从 JSON 配置文件加载
cfg, _ := config.LoadFromFile("config.json")
```

---

## 配置转换

配置系统使用 `[]string`（multiaddr 格式），内部模块使用 `[]types.PeerInfo`。

```
配置转换流程：

  config/discovery.go
  DefaultDiscoveryConfig() 定义默认引导节点
               ↓
  用户可覆盖（JSON/代码）
               ↓
  config.Discovery.Bootstrap.Peers ([]string)
               ↓
  ConfigFromUnified() 转换
               ↓
  parseBootstrapPeers()
  (string → types.PeerInfo)
               ↓
  bootstrap.Config.Peers ([]types.PeerInfo)
               ↓
  Bootstrap.Bootstrap() - 并发连接
```

**转换函数**：

```go
// 位置：internal/discovery/bootstrap/module.go

func ConfigFromUnified(cfg *config.Config) *Config {
    if cfg == nil || !cfg.Discovery.EnableBootstrap {
        return &Config{Enabled: false}
    }
    
    // 转换配置中的引导节点（已包含默认值或用户覆盖的值）
    var peers []types.PeerInfo
    if len(cfg.Discovery.Bootstrap.Peers) > 0 {
        peers = parseBootstrapPeers(cfg.Discovery.Bootstrap.Peers)
    }
    
    return &Config{
        Peers:      peers,
        MinPeers:   cfg.Discovery.Bootstrap.MinPeers,
        Timeout:    cfg.Discovery.Bootstrap.Timeout,
        MaxRetries: 3,
        Interval:   cfg.Discovery.Bootstrap.Interval,
        Enabled:    cfg.Discovery.EnableBootstrap,
    }
}

// parseBootstrapPeers 将 multiaddr 字符串解析为 PeerInfo
func parseBootstrapPeers(addrs []string) []types.PeerInfo {
    var peers []types.PeerInfo
    for _, addr := range addrs {
        addrInfo, err := types.AddrInfoFromString(addr)
        if err != nil {
            continue
        }
        peerInfo := addrInfo.ToPeerInfo()
        peerInfo.Source = types.SourceBootstrap
        peers = append(peers, peerInfo)
    }
    return peers
}
```

**关键点**：
- `cfg.Discovery.Bootstrap.Peers` 来自统一配置系统
- 已包含默认值（来自 `config/discovery.go`）或用户覆盖的值
- `internal/` 模块只负责类型转换，不定义默认值

---

## 内置默认值（用户不可配置）

Bootstrap 能力启用后，以下参数使用内置默认值，**用户无法修改**：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Bootstrap 内置默认值                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  存储参数：                                                                  │
│  ├── MaxNodes        = 50,000          # 最大存储节点数                     │
│  ├── PersistPath     = "~/.dep2p/bootstrap.db"  # 持久化路径               │
│  └── StorageBackend  = BadgerDB        # 存储后端（内置）                   │
│                                                                             │
│  探测参数：                                                                  │
│  ├── ProbeInterval   = 5 分钟          # 存活探测间隔                       │
│  ├── ProbeBatchSize  = 100             # 每批探测数量                       │
│  └── ProbeTimeout    = 10 秒           # 单次探测超时                       │
│                                                                             │
│  发现参数：                                                                  │
│  ├── DiscoveryInterval = 10 分钟       # 主动发现间隔                       │
│  └── DiscoveryWalkLen  = 20            # Random Walk 步数                   │
│                                                                             │
│  过期参数：                                                                  │
│  ├── NodeExpireTime  = 24 小时         # 节点过期时间                       │
│  └── OfflineThreshold = 3 次           # 连续失败阈值                       │
│                                                                             │
│  响应参数：                                                                  │
│  └── ResponseK       = 20              # 返回最近 K 个节点                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 设计理由

| 参数 | 值 | 理由 |
|------|-----|------|
| MaxNodes | 50,000 | 覆盖中小型网络，内存占用约 50MB |
| ProbeInterval | 5 分钟 | 平衡探测频率与网络负载 |
| DiscoveryInterval | 10 分钟 | 避免 DHT 压力过大 |
| NodeExpireTime | 24 小时 | 允许节点短暂离线后恢复 |
| ResponseK | 20 | Kademlia 标准值，足够触发 DHT 迭代 |

---

## 配置参数（用户可配置）

以下参数用户**可以配置**（连接引导节点时使用）：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `Peers` | DefaultBootstrapPeers() | 引导节点列表（用户配置覆盖） |
| `Timeout` | 30s | 单个引导节点连接超时 |
| `MinPeers` | 4 | 最少成功连接数 |
| `Interval` | 5min | 引导间隔（定期检查连接数） |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [ADR-0009: Bootstrap 极简配置](../../../01_context/decisions/ADR-0009-bootstrap-simplified.md) | 配置简化决策 |
| [discovery_coordinator](../discovery_coordinator/) | 发现协调器 |
| [discovery_dht](../discovery_dht/) | DHT 发现 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-23
