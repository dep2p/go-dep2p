# REQ-OPS-001: 可观测性需求

## 1. 元数据

| 属性 | 值 |
|------|---|
| **ID** | REQ-OPS-001 |
| **标题** | 可观测性需求 |
| **类型** | generic |
| **层级** | NF: 非功能 |
| **优先级** | P1 |
| **状态** | draft |
| **创建日期** | 2026-01-11 |
| **更新日期** | 2026-01-11 |

---

## 2. 需求描述

DeP2P 必须提供完善的可观测性能力，包括结构化日志、Prometheus 指标、诊断接口，支持生产环境的监控和故障排查。

---

## 3. 背景与动机

### 3.1 问题陈述

P2P 网络运维面临以下挑战：

1. **状态不透明**：难以了解节点内部状态
2. **故障难定位**：分布式环境下问题难追踪
3. **性能难优化**：缺乏性能数据

### 3.2 目标

构建**三位一体**的可观测性体系：
- **日志 (Logs)**：事件追踪
- **指标 (Metrics)**：性能监控
- **诊断 (Diagnostics)**：状态检查

### 3.3 竞品参考

| 产品 | 日志 | 指标 | 诊断 |
|------|------|------|------|
| **iroh** | tracing | 无 | introspect HTTP |
| **go-libp2p** | log | OpenTelemetry | pprof |

---

## 4. 需求详情

### 4.1 日志要求

#### 4.1.1 日志级别

| 级别 | 用途 | 示例 |
|------|------|------|
| `ERROR` | 错误事件 | 连接失败、协议错误 |
| `WARN` | 警告事件 | 资源不足、重试 |
| `INFO` | 重要事件 | 连接建立、Realm 加入 |
| `DEBUG` | 调试信息 | 消息收发、状态变化 |
| `TRACE` | 详细追踪 | 包级别日志 |

#### 4.1.2 日志格式

```go
// 结构化日志
logger.Info("connection established",
    "peer_id", peerID,
    "addr", addr,
    "latency_ms", latency.Milliseconds(),
)

// 输出 JSON
{
    "level": "info",
    "msg": "connection established",
    "peer_id": "12D3KooW...",
    "addr": "/ip4/1.2.3.4/udp/4001/quic-v1",
    "latency_ms": 42,
    "time": "2026-01-11T12:00:00Z"
}
```

### 4.2 指标要求

#### 4.2.1 指标分类

| 类别 | 指标示例 | 类型 |
|------|----------|------|
| **连接** | `dep2p_connections_total` | Gauge |
| **带宽** | `dep2p_bytes_sent_total` | Counter |
| **延迟** | `dep2p_connection_latency_seconds` | Histogram |
| **Realm** | `dep2p_realm_members` | Gauge |
| **发现** | `dep2p_discovery_peers_found` | Counter |

#### 4.2.2 Prometheus 接口

```go
// 指标注册
var (
    connectionsTotal = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "dep2p_connections_total",
            Help: "Number of active connections",
        },
        []string{"direction", "transport"},
    )
    
    bytesSentTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "dep2p_bytes_sent_total",
            Help: "Total bytes sent",
        },
        []string{"peer_id", "protocol"},
    )
)

// HTTP 端点
// GET /metrics
```

#### 4.2.3 关键指标

```prometheus
# 连接指标
dep2p_connections_total{direction="inbound",transport="quic"} 42
dep2p_connections_total{direction="outbound",transport="quic"} 38

# 带宽指标
dep2p_bytes_sent_total{protocol="/dep2p/app/msg/1.0"} 1234567
dep2p_bytes_received_total{protocol="/dep2p/app/msg/1.0"} 2345678

# 延迟指标
dep2p_connection_latency_seconds_bucket{le="0.01"} 100
dep2p_connection_latency_seconds_bucket{le="0.05"} 150
dep2p_connection_latency_seconds_bucket{le="0.1"} 180

# Realm 指标
dep2p_realm_members{realm="my-app"} 25

# 发现指标
dep2p_discovery_peers_found_total{source="dht"} 500
dep2p_discovery_peers_found_total{source="mdns"} 10
```

### 4.3 诊断要求

#### 4.3.1 诊断接口

```go
// DiagnosticReport 结构化诊断报告
type DiagnosticReport struct {
    // 节点信息
    NodeID       NodeID        `json:"node_id"`
    ListenAddrs  []string      `json:"listen_addrs"`
    Uptime       time.Duration `json:"uptime"`
    
    // Realm 信息
    CurrentRealm *RealmID      `json:"current_realm,omitempty"`
    
    // 连接信息
    Connections  ConnectionDiagnostics `json:"connections"`
    
    // 发现信息
    Discovery    DiscoveryDiagnostics  `json:"discovery"`
    
    // NAT 信息
    NAT          NATDiagnostics        `json:"nat"`
    
    // Relay 信息
    Relay        RelayDiagnostics      `json:"relay"`
}

// 获取诊断报告
report := node.Diagnostics()
```

#### 4.3.2 HTTP 诊断端点

| 端点 | 说明 |
|------|------|
| `GET /debug/introspect` | 完整诊断报告 (JSON) |
| `GET /debug/introspect/connections` | 连接信息 |
| `GET /debug/introspect/discovery` | 发现信息 |
| `GET /debug/introspect/nat` | NAT 信息 |
| `GET /debug/pprof/*` | Go pprof 端点 |

### 4.4 配置选项

```go
// 可观测性配置
type ObservabilityConfig struct {
    // 日志配置
    LogLevel  string // debug, info, warn, error
    LogFormat string // json, text
    
    // 指标配置
    MetricsEnabled bool
    MetricsAddr    string // :9090
    
    // 诊断配置
    DiagnosticsEnabled bool
    DiagnosticsAddr    string // :6060
}

// 使用
node, _ := dep2p.NewNode(
    dep2p.WithLogLevel("info"),
    dep2p.WithMetrics(":9090"),
    dep2p.WithDiagnostics(":6060"),
)
```

---

## 5. 验收标准

### 5.1 日志

- [ ] 支持 5 个日志级别
- [ ] 支持 JSON 结构化输出
- [ ] 可配置日志级别
- [ ] 关键事件有日志

### 5.2 指标

- [ ] 提供 Prometheus 指标端点
- [ ] 连接指标完整
- [ ] 带宽指标完整
- [ ] 延迟直方图可用
- [ ] Realm 指标可用

### 5.3 诊断

- [ ] 提供 JSON 诊断报告
- [ ] HTTP 诊断端点可用
- [ ] pprof 端点可用
- [ ] 诊断信息准确

---

## 6. 非功能要求

| 维度 | 要求 |
|------|------|
| **性能** | 指标收集开销 < 1% CPU |
| **可用性** | 指标端点独立于主服务 |

---

## 7. 关联文档

| 类型 | 链接 |
|------|------|
| **竞品** | [iroh 分析](../../references/individual/iroh.md) |

---

## 8. 实现追踪

### 8.1 代码引用

| 文件 | 符号 | 状态 |
|------|------|------|
| `pkg/observability/metrics.go` | `RegisterMetrics` | ⏳ 待实现 |
| `pkg/observability/diagnostics.go` | `DiagnosticReport` | ⏳ 待实现 |
| `internal/introspect/server.go` | `Server` | ⏳ 待实现 |

### 8.2 测试证据

| 测试文件 | 测试函数 | 状态 |
|----------|----------|------|
| `pkg/observability/metrics_test.go` | `TestMetricsExport` | ⏳ 待实现 |

---

## 9. 变更历史

| 日期 | 版本 | 变更说明 |
|------|------|----------|
| 2026-01-11 | 1.0 | 初始版本 |
