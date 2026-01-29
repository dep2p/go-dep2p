# Core Metrics 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 监控指标（Core Layer）

---

## 模块概述

core_metrics 提供统一的监控指标收集和导出功能，支持 Prometheus 和 OpenTelemetry。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/metrics/` |
| **Fx 模块** | `fx.Module("metrics")` |
| **状态** | ✅ 已实现 |
| **依赖** | swarm, discovery |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       core_metrics 职责                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 指标收集                                                                 │
│     • 连接指标 (连接数、连接时长、失败率)                                   │
│     • 流指标 (流数、流吞吐量)                                               │
│     • 带宽指标 (入站/出站流量)                                              │
│     • 发现指标 (DHT 查询、节点发现)                                         │
│     • 协议指标 (协议使用率)                                                 │
│                                                                             │
│  2. 指标导出                                                                 │
│     • Prometheus 格式                                                       │
│     • OpenTelemetry 格式                                                    │
│     • 自定义导出器                                                          │
│                                                                             │
│  3. 带宽统计                                                                 │
│     • 全局带宽计数                                                          │
│     • 每协议带宽                                                            │
│     • 每节点带宽                                                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 指标分类

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DeP2P 监控指标                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  连接指标 (dep2p_swarm_*)                                                    │
│  ├─ dep2p_swarm_connections_total{dir="inbound|outbound"}                   │
│  ├─ dep2p_swarm_connections_active                                          │
│  ├─ dep2p_swarm_dial_attempts_total                                         │
│  ├─ dep2p_swarm_dial_errors_total{error="timeout|refused|..."}              │
│  └─ dep2p_swarm_connection_duration_seconds                                 │
│                                                                             │
│  流指标 (dep2p_stream_*)                                                     │
│  ├─ dep2p_stream_opened_total{dir="inbound|outbound"}                       │
│  ├─ dep2p_stream_active                                                     │
│  └─ dep2p_stream_duration_seconds                                           │
│                                                                             │
│  带宽指标 (dep2p_bandwidth_*)                                                │
│  ├─ dep2p_bandwidth_bytes_total{dir="in|out"}                               │
│  ├─ dep2p_bandwidth_bytes_per_peer{peer="...",dir="in|out"}                 │
│  └─ dep2p_bandwidth_bytes_per_protocol{protocol="...",dir="in|out"}         │
│                                                                             │
│  发现指标 (dep2p_discovery_*)                                                │
│  ├─ dep2p_discovery_dht_queries_total                                       │
│  ├─ dep2p_discovery_dht_query_duration_seconds                              │
│  ├─ dep2p_discovery_peers_found_total{source="dht|mdns|bootstrap"}          │
│  └─ dep2p_discovery_routing_table_size                                      │
│                                                                             │
│  Identify 指标 (dep2p_identify_*)                                            │
│  ├─ dep2p_identify_pushes_sent_total                                        │
│  ├─ dep2p_identify_pushes_received_total                                    │
│  └─ dep2p_identify_duration_seconds                                         │
│                                                                             │
│  NAT 指标 (dep2p_nat_*)                                                      │
│  ├─ dep2p_nat_reachability{status="public|private|unknown"}                 │
│  └─ dep2p_nat_holepunch_success_total                                       │
│                                                                             │
│  Relay 指标 (dep2p_relay_*)                                                  │
│  ├─ dep2p_relay_reservations_active                                         │
│  ├─ dep2p_relay_connections_total                                           │
│  └─ dep2p_relay_bytes_relayed_total                                         │
│                                                                             │
│  资源指标 (dep2p_resource_*)                                                 │
│  ├─ dep2p_resource_memory_usage_bytes                                       │
│  ├─ dep2p_resource_fd_usage                                                 │
│  └─ dep2p_resource_blocked_total{scope="system|peer|conn|stream"}           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

```go
// pkg/interfaces/metrics.go

// MetricsCollector 指标收集器接口
type MetricsCollector interface {
    // 带宽统计
    LogSentMessage(size int64)
    LogRecvMessage(size int64)
    LogSentMessageStream(size int64, protocol types.ProtocolID, peer types.PeerID)
    LogRecvMessageStream(size int64, protocol types.ProtocolID, peer types.PeerID)
    
    // 获取统计
    GetBandwidthForPeer(peer types.PeerID) Stats
    GetBandwidthForProtocol(proto types.ProtocolID) Stats
    GetBandwidthTotals() Stats
    
    // Prometheus 注册
    Register(registry prometheus.Registerer) error
}

// Stats 带宽统计
type Stats struct {
    TotalIn  int64
    TotalOut int64
    RateIn   float64
    RateOut  float64
}

// BandwidthCounter 带宽计数器
type BandwidthCounter interface {
    LogSentMessage(size int64)
    LogRecvMessage(size int64)
    GetBandwidthTotals() Stats
}
```

---

## Prometheus 集成

```go
// 启用 Prometheus 指标
node, _ := dep2p.New(
    dep2p.WithMetrics(prometheus.DefaultRegisterer),
)

// 或使用自定义注册器
registry := prometheus.NewRegistry()
node, _ := dep2p.New(
    dep2p.WithMetrics(registry),
)

// 暴露指标端点
http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
```

---

## 参考实现

### go-libp2p Metrics

```
github.com/libp2p/go-libp2p/core/metrics/
├── bandwidth.go      # 带宽计数器
└── reporter.go       # 带宽报告器

github.com/libp2p/go-libp2p/p2p/net/swarm/
└── metrics.go        # Swarm 指标

github.com/libp2p/go-libp2p/p2p/protocol/identify/
└── metrics.go        # Identify 指标
```

### iroh Metrics

```
iroh/src/metrics.rs                # 指标定义
iroh/src/magicsock/metrics.rs      # MagicSock 指标
iroh/src/net_report/metrics.rs     # 网络报告指标
```

---

## Grafana 仪表板

go-libp2p 提供预定义的 Grafana 仪表板：

| 仪表板 | 说明 |
|--------|------|
| swarm | 连接和流指标 |
| identify | Identify 协议指标 |
| autonat | NAT 检测指标 |
| holepunch | 打洞指标 |
| autorelay | 自动中继指标 |
| relaysvc | 中继服务指标 |
| eventbus | 事件总线指标 |
| resource-manager | 资源管理指标 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_swarm](../core_swarm/) | 连接群管理 |
| [discovery_dht](../discovery_dht/) | DHT 发现 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
