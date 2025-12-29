# DeP2P 迭代计划：面向业务的 P2P 基础库演进

**基于文档**: `REQUIREMENTS_GAP_ANALYSIS.md`  
**计划目标**: 补齐 P2P 基础库层面的真实业务需求缺口，使 DeP2P 可作为生产级 P2P Runtime  
**计划周期**: 3 个迭代（每迭代 2-3 周）  
**核心原则**: 保持"库"定位，所有新增能力默认不启用，显式开启

---

## 📋 迭代总览

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        DeP2P 演进路线图                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Iteration 1              Iteration 2              Iteration 3          │
│  ─────────────            ─────────────            ─────────────         │
│  可观测性基础              运行时可控性             连接策略增强           │
│                                                                         │
│  ┌─────────────┐         ┌─────────────┐         ┌─────────────┐        │
│  │ Prometheus  │         │ DHT Mode    │         │ ForceConnect│        │
│  │ Exporter    │─────────│ Query       │─────────│ 策略        │        │
│  │             │         │             │         │             │        │
│  │ Swarm Stats │         │ Discovery   │         │ 关键节点    │        │
│  │ 补齐        │         │ Trigger     │         │ 保活        │        │
│  │             │         │             │         │             │        │
│  │ Resource    │         │ Reachability│         │ 连接优先级  │        │
│  │ Limits 配置 │         │ 便捷 API    │         │ 管理        │        │
│  └─────────────┘         └─────────────┘         └─────────────┘        │
│                                                                         │
│  ● 生产监控就绪           ● 运行时可诊断           ● 业务级连接策略        │
│  ● 资源可配置             ● 主动发现控制           ● 可选模块化            │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 🚀 Iteration 1: 可观测性基础（优先级：高）

**目标**: 使 DeP2P 具备生产级可观测能力，支持资源配置

**周期**: 2 周

### 1.1 Prometheus 指标导出

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 设计指标体系 | 定义核心指标命名空间与标签规范 | `design/observability/metrics-spec.md` |
| 实现 MetricsCollector | 收集连接/带宽/DHT/Relay 核心指标 | `internal/core/metrics/collector.go` |
| 实现 PrometheusExporter | 可选启用的 `/metrics` 端点 | `internal/core/metrics/prometheus.go` |
| 配置入口 | `WithMetrics()` / `WithMetricsAddr()` | `options.go` 更新 |

**核心指标清单**:

```go
// 连接指标
dep2p_connections_total{direction="inbound|outbound"}
dep2p_connections_active
dep2p_streams_total
dep2p_streams_active

// 带宽指标
dep2p_bandwidth_bytes_total{direction="in|out"}
dep2p_bandwidth_rate_bytes_per_second{direction="in|out"}

// DHT 指标
dep2p_dht_routing_table_size
dep2p_dht_queries_total{type="find_peer|find_closest|provide"}
dep2p_dht_mode{mode="client|server|auto|lan"}

// Relay 指标
dep2p_relay_reservations_active
dep2p_relay_bytes_forwarded_total
dep2p_relay_rate_limit_hits_total

// 资源指标
dep2p_resource_memory_bytes
dep2p_resource_fd_count
```

**验收标准**:
- [ ] `WithMetrics(true)` 后可访问 `/metrics` 获取 Prometheus 格式指标
- [ ] 默认不启用，不影响现有 API
- [ ] 指标与 `DiagnosticReport` 数据一致

---

### 1.2 Swarm 核心统计补齐

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 扩展 ConnectionDiagnostics | 添加 `NumStreams`、入/出站连接数 | `pkg/interfaces/endpoint/endpoint.go` |
| 实现统计收集 | 从 libp2p Host/Swarm 采集 | `internal/core/endpoint/stats.go` |
| 更新 DiagnosticReport | 整合新字段 | `endpoint_impl.go` 更新 |
| 更新 Introspect API | `/debug/introspect/connections` 返回新字段 | `internal/core/introspect/server.go` |

**新增字段**:

```go
type ConnectionDiagnostics struct {
    // 现有字段...
    
    // 新增
    TotalConnections   int `json:"total_connections"`
    InboundConnections int `json:"inbound_connections"`
    OutboundConnections int `json:"outbound_connections"`
    TotalStreams       int `json:"total_streams"`
    InboundStreams     int `json:"inbound_streams"`
    OutboundStreams    int `json:"outbound_streams"`
}
```

**验收标准**:
- [ ] `DiagnosticReport()` 返回完整的连接/流统计
- [ ] Introspect JSON 输出包含所有新字段
- [ ] 单元测试覆盖

---

### 1.3 资源限额配置面

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 设计 ResourceLimits 结构 | 内存/FD/连接/流限额 | `pkg/interfaces/config/resource.go` |
| 实现配置选项 | `WithResourceLimits()` | `options.go` |
| 集成 libp2p ResourceManager | 转换配置到 libp2p rcmgr | `internal/core/resource/manager.go` |
| 暴露限额状态 | 在 DiagnosticReport 中显示 | 更新 `endpoint_impl.go` |

**配置结构**:

```go
type ResourceLimits struct {
    // 内存限额
    MemoryLimitMB int `json:"memory_limit_mb"`
    
    // 文件描述符限额
    MaxFileDescriptors int `json:"max_file_descriptors"`
    
    // 连接限额（覆盖 HighWater/LowWater）
    MaxConnections         int `json:"max_connections"`
    MaxConnectionsPerPeer  int `json:"max_connections_per_peer"`
    
    // 流限额
    MaxStreams         int `json:"max_streams"`
    MaxStreamsPerPeer  int `json:"max_streams_per_peer"`
    MaxStreamsPerConn  int `json:"max_streams_per_conn"`
}
```

**验收标准**:
- [ ] `WithResourceLimits(limits)` 正确应用到 libp2p ResourceManager
- [ ] 超限时正确拒绝连接/流，并记录指标
- [ ] DiagnosticReport 显示当前资源使用 vs 限额

---

### Iteration 1 里程碑验收

```bash
# 1. 启动带 metrics 的节点
endpoint, _ := dep2p.New(
    dep2p.WithMetrics(true),
    dep2p.WithMetricsAddr(":9090"),
    dep2p.WithResourceLimits(dep2p.ResourceLimits{
        MemoryLimitMB:      512,
        MaxFileDescriptors: 1024,
        MaxConnections:     100,
    }),
)

# 2. 验证 Prometheus 指标
curl http://localhost:9090/metrics | grep dep2p_

# 3. 验证诊断报告
curl http://localhost:6060/debug/introspect | jq '.connections'
```

---

## 🔧 Iteration 2: 运行时可控性（优先级：中）

**目标**: 提升运行时可诊断性与主动控制能力

**周期**: 2 周

### 2.1 DHT Mode 查询

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 扩展 DHT 接口 | 添加 `Mode() DHTMode` 方法 | `pkg/interfaces/discovery/dht.go` |
| 实现查询 | 从底层 Kademlia DHT 获取当前模式 | `internal/core/discovery/dht/service.go` |
| 添加到诊断 | DiscoveryDiagnostics 包含 DHT 模式 | 更新 `endpoint_impl.go` |

**接口变更**:

```go
type DHTMode string

const (
    DHTModeClient DHTMode = "client"
    DHTModeServer DHTMode = "server"
    DHTModeAuto   DHTMode = "auto"
    DHTModeLAN    DHTMode = "lan"
)

type DHT interface {
    // 现有方法...
    
    // 新增
    Mode() DHTMode
}
```

**验收标准**:
- [ ] `dht.Mode()` 返回当前运行模式
- [ ] DiagnosticReport 包含 `dht_mode` 字段
- [ ] Prometheus 指标 `dep2p_dht_mode` 正确设置

---

### 2.2 Discovery Trigger 方法

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 设计 Trigger API | 统一的"主动发现"入口 | `pkg/interfaces/discovery/discovery.go` |
| 实现触发逻辑 | 立即执行一轮 Announce + DiscoverPeers | `internal/core/discovery/service.go` |
| 添加触发原因日志 | 记录触发来源便于排障 | 日志增强 |

**接口变更**:

```go
type DiscoveryService interface {
    // 现有方法...
    
    // 新增：主动触发一轮发现
    // reason 用于日志/指标标记（如 "low_peers", "manual", "reconnect"）
    Trigger(ctx context.Context, reason string) error
}
```

**验收标准**:
- [ ] 调用 `Trigger("low_peers")` 后立即执行发现流程
- [ ] 日志/指标记录触发原因
- [ ] 不影响正常的后台发现调度

---

### 2.3 Reachability 便捷 API

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 添加 Endpoint.Reachability() | 返回当前可达性状态 | `pkg/interfaces/endpoint/endpoint.go` |
| 整合 NAT/Relay 状态 | 综合判断可达性 | `internal/core/endpoint/reachability.go` |
| 添加到诊断 | 顶层 Reachability 字段 | 更新 `DiagnosticReport` |

**接口变更**:

```go
type ReachabilityStatus string

const (
    ReachabilityPublic  ReachabilityStatus = "public"   // 公网直连
    ReachabilityPrivate ReachabilityStatus = "private"  // NAT 后，可打洞
    ReachabilityRelayed ReachabilityStatus = "relayed"  // 仅通过 Relay 可达
    ReachabilityUnknown ReachabilityStatus = "unknown"  // 检测中
)

type Endpoint interface {
    // 现有方法...
    
    // 新增
    Reachability() ReachabilityStatus
}
```

**验收标准**:
- [ ] `endpoint.Reachability()` 返回准确的可达性状态
- [ ] 状态变化时触发事件（可选）
- [ ] DiagnosticReport 顶层显示可达性

---

### 2.4 DCUTR 配置暴露

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 添加 WithHolePunch() | 显式控制打洞功能开关 | `options.go` |
| 添加打洞统计 | 成功/失败/进行中计数 | `DiagnosticReport.NATDiagnostics` |

**验收标准**:
- [ ] `WithHolePunch(false)` 可禁用打洞
- [ ] 诊断报告包含打洞统计

---

### Iteration 2 里程碑验收

```go
// 1. 查询 DHT 模式
mode := endpoint.DHT().Mode()
fmt.Printf("DHT Mode: %s\n", mode)

// 2. 主动触发发现
err := endpoint.Discovery().Trigger(ctx, "low_peers")

// 3. 查询可达性
status := endpoint.Reachability()
fmt.Printf("Reachability: %s\n", status)
```

---

## 🎯 Iteration 3: 连接策略增强（优先级：低，可选）

**目标**: 提供业务级连接管理策略，作为可选模块

**周期**: 2-3 周

**注意**: 此迭代内容偏向"策略/编排"层，可作为独立模块实现，不进入核心路径。

### 3.1 业务关键节点配置

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 设计 CriticalPeers 配置 | 需要优先保持连接的节点列表 | `pkg/interfaces/config/peers.go` |
| 实现配置选项 | `WithCriticalPeers([]NodeID)` | `options.go` |
| 集成 ConnManager | 标记为 Protected | `internal/core/connmgr/critical.go` |

**配置结构**:

```go
type CriticalPeersConfig struct {
    // 关键节点列表
    PeerIDs []types.NodeID `json:"peer_ids"`
    
    // 连接丢失时的行为
    ReconnectOnDisconnect bool          `json:"reconnect_on_disconnect"`
    ReconnectInterval     time.Duration `json:"reconnect_interval"`
    MaxReconnectAttempts  int           `json:"max_reconnect_attempts"`
}
```

---

### 3.2 ForceConnect 策略

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 设计 ForceConnect 接口 | 强制连接并保活 | `pkg/interfaces/endpoint/force_connect.go` |
| 实现保活逻辑 | 后台 goroutine 监控 + 重连 | `internal/core/connmgr/force_connect.go` |
| 添加事件通知 | 连接丢失/恢复事件 | `pkg/interfaces/events/connection.go` |

**接口设计**:

```go
type ForceConnector interface {
    // 添加强制连接目标
    Add(nodeID types.NodeID, opts ...ForceConnectOption) error
    
    // 移除强制连接目标
    Remove(nodeID types.NodeID) error
    
    // 列出所有强制连接目标及其状态
    List() []ForceConnectStatus
}

type ForceConnectStatus struct {
    NodeID            types.NodeID
    Connected         bool
    LastConnected     time.Time
    LastDisconnected  time.Time
    ReconnectAttempts int
    LastError         string
}
```

---

### 3.3 连接优先级管理

| 任务 | 描述 | 交付物 |
|------|------|--------|
| 设计优先级标签 | 为连接添加优先级标签 | `pkg/interfaces/endpoint/priority.go` |
| 集成 ConnManager | 低水位裁剪时优先保留高优先级 | 更新 `internal/core/connmgr` |

**优先级枚举**:

```go
type ConnectionPriority int

const (
    PriorityCritical ConnectionPriority = 100  // 绝不断开
    PriorityHigh     ConnectionPriority = 75   // 最后断开
    PriorityNormal   ConnectionPriority = 50   // 默认
    PriorityLow      ConnectionPriority = 25   // 优先断开
)
```

---

### Iteration 3 里程碑验收

```go
// 1. 配置关键节点
endpoint, _ := dep2p.New(
    dep2p.WithCriticalPeers(dep2p.CriticalPeersConfig{
        PeerIDs: []types.NodeID{bootstrapNode, relayNode},
        ReconnectOnDisconnect: true,
        ReconnectInterval: 5 * time.Second,
    }),
)

// 2. 运行时添加强制连接
endpoint.ForceConnector().Add(importantPeer)

// 3. 查看状态
for _, status := range endpoint.ForceConnector().List() {
    fmt.Printf("Peer %s: connected=%v, attempts=%d\n",
        status.NodeID, status.Connected, status.ReconnectAttempts)
}
```

---

## 📊 迭代优先级与依赖关系

```
                    ┌──────────────────┐
                    │   Iteration 1    │
                    │  可观测性基础     │
                    │                  │
                    │ • Prometheus     │
                    │ • Swarm Stats    │
                    │ • Resource Limits│
                    └────────┬─────────┘
                             │
                             │ 依赖：指标基础
                             ▼
                    ┌──────────────────┐
                    │   Iteration 2    │
                    │  运行时可控性     │
                    │                  │
                    │ • DHT Mode       │
                    │ • Discovery Trig │
                    │ • Reachability   │
                    └────────┬─────────┘
                             │
                             │ 可选：策略层
                             ▼
                    ┌──────────────────┐
                    │   Iteration 3    │
                    │  连接策略增强     │
                    │  (可选模块)       │
                    │                  │
                    │ • Critical Peers │
                    │ • ForceConnect   │
                    │ • Priority Mgmt  │
                    └──────────────────┘
```

---

## 🎯 交付检查清单

### Iteration 1 完成标准

- [ ] Prometheus `/metrics` 端点可用
- [ ] 核心指标（连接/带宽/DHT/Relay）完整
- [ ] `DiagnosticReport` 包含 Swarm 核心统计
- [ ] `WithResourceLimits()` 配置生效
- [ ] 文档更新：`docs/zh/how-to/observability.md`

### Iteration 2 完成标准

- [ ] `DHT.Mode()` 返回当前模式
- [ ] `DiscoveryService.Trigger()` 可用
- [ ] `Endpoint.Reachability()` 返回准确状态
- [ ] `WithHolePunch()` 配置生效
- [ ] 文档更新：`docs/zh/reference/api-defaults.md`

### Iteration 3 完成标准（可选）

- [ ] `WithCriticalPeers()` 配置生效
- [ ] `ForceConnector` 接口实现
- [ ] 连接优先级在 ConnManager 中生效
- [ ] 模块化设计，不影响核心路径
- [ ] 文档更新：`docs/zh/how-to/connection-management.md`

---

## 📅 时间线（建议）

| 迭代 | 开始日期 | 结束日期 | 主要交付 |
|------|----------|----------|----------|
| Iteration 1 | Week 1 | Week 2 | Prometheus + Stats + ResourceLimits |
| Iteration 2 | Week 3 | Week 4 | DHT Mode + Trigger + Reachability |
| Iteration 3 | Week 5 | Week 7 | ForceConnect + Priority（可选） |

---

## 🔗 相关文档

- [需求满足度分析](./REQUIREMENTS_GAP_ANALYSIS.md)
- [P2P 网络需求分析](./P2P_REQUIREMENTS_ANALYSIS.md)
- [架构层设计](../architecture/layers.md)
- [不变量规范](../invariants/README.md)

---

**计划版本**: v1.0  
**创建日期**: 2025-12-29  
**维护者**: DeP2P Core Team

