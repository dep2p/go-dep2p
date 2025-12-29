# Metrics 指标工具

## 概述

提供统一的指标收集和导出能力，支持 Prometheus 格式。

## 目录结构

```
metrics/
├── README.md        # 本文件
└── metrics.go       # 指标实现
```

## 接口定义

```go
// Metrics 指标接口
type Metrics interface {
    // Counter 计数器
    Counter(name string, labels ...string) Counter
    
    // Gauge 仪表盘
    Gauge(name string, labels ...string) Gauge
    
    // Histogram 直方图
    Histogram(name string, buckets []float64, labels ...string) Histogram
    
    // Summary 摘要
    Summary(name string, objectives map[float64]float64, labels ...string) Summary
}

// Counter 计数器接口
type Counter interface {
    Inc()
    Add(float64)
    WithLabelValues(lvs ...string) Counter
}

// Gauge 仪表盘接口
type Gauge interface {
    Set(float64)
    Inc()
    Dec()
    Add(float64)
    Sub(float64)
    WithLabelValues(lvs ...string) Gauge
}

// Histogram 直方图接口
type Histogram interface {
    Observe(float64)
    WithLabelValues(lvs ...string) Histogram
}
```

## 预定义指标

```go
var (
    // 连接指标
    ConnectionsTotal = Counter("dep2p_connections_total", "direction")
    ConnectionsCurrent = Gauge("dep2p_connections_current", "direction")
    
    // 流量指标
    BytesSent = Counter("dep2p_bytes_sent_total", "protocol")
    BytesReceived = Counter("dep2p_bytes_received_total", "protocol")
    
    // 延迟指标
    RequestLatency = Histogram("dep2p_request_latency_seconds", 
        []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
        "protocol",
    )
    
    // DHT 指标
    DHTQueries = Counter("dep2p_dht_queries_total", "type")
    DHTRoutingTableSize = Gauge("dep2p_dht_routing_table_size")
    
    // NAT 指标
    NATType = Gauge("dep2p_nat_type")
    HolePunchAttempts = Counter("dep2p_holepunch_attempts_total", "result")
)
```

## 实现

基于 `prometheus/client_golang` 的指标实现：

```go
type prometheusMetrics struct {
    namespace string
    subsystem string
}

func NewMetrics(namespace, subsystem string) Metrics {
    return &prometheusMetrics{
        namespace: namespace,
        subsystem: subsystem,
    }
}

func (m *prometheusMetrics) Counter(name string, labels ...string) Counter {
    opts := prometheus.CounterOpts{
        Namespace: m.namespace,
        Subsystem: m.subsystem,
        Name:      name,
    }
    
    if len(labels) > 0 {
        counter := prometheus.NewCounterVec(opts, labels)
        prometheus.MustRegister(counter)
        return &counterVec{counter}
    }
    
    counter := prometheus.NewCounter(opts)
    prometheus.MustRegister(counter)
    return &simpleCounter{counter}
}
```

## HTTP 导出

```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

// 启动指标 HTTP 服务
http.Handle("/metrics", promhttp.Handler())
http.ListenAndServe(":2112", nil)
```

## 使用示例

```go
// 记录连接
metrics.ConnectionsTotal.WithLabelValues("inbound").Inc()
metrics.ConnectionsCurrent.WithLabelValues("inbound").Inc()

// 记录流量
metrics.BytesSent.WithLabelValues("dht").Add(float64(n))

// 记录延迟
start := time.Now()
// ... 操作 ...
metrics.RequestLatency.WithLabelValues("dht").Observe(time.Since(start).Seconds())
```

## 依赖

- `github.com/prometheus/client_golang` - Prometheus Go 客户端

