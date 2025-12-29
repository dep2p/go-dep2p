# 带宽统计模块 (Bandwidth)

## 概述

带宽统计模块提供全面的流量统计能力，帮助监控 P2P 节点的网络使用情况。

## 架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Bandwidth 模块                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                      Counter (计数器)                         │   │
│  ├──────────────────────────────────────────────────────────────┤   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐             │   │
│  │  │ totalIn    │  │ totalOut   │  │            │             │   │
│  │  │ (Meter)    │  │ (Meter)    │  │            │             │   │
│  │  └────────────┘  └────────────┘  │            │             │   │
│  │                                   │            │             │   │
│  │  ┌────────────────────────────┐  │ Reporter   │             │   │
│  │  │ peerIn/Out (MeterRegistry) │  │            │             │   │
│  │  └────────────────────────────┘  │            │             │   │
│  │                                   │            │             │   │
│  │  ┌────────────────────────────┐  │            │             │   │
│  │  │ protocolIn/Out (Registry)  │  │            │             │   │
│  │  └────────────────────────────┘  └────────────┘             │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## 文件结构

```
internal/core/bandwidth/
├── module.go           # fx 模块定义
├── counter.go          # BandwidthCounter 实现
├── meter.go            # 流量计量器（EWMA 速率计算）
├── README.md           # 本文档
└── bandwidth_test.go   # 单元测试
```

## 核心功能

### 1. 流量统计维度

| 维度 | 说明 |
|------|------|
| 总量 | 所有流量的汇总 |
| 按 Peer | 每个节点的流量统计 |
| 按 Protocol | 每个协议的流量统计 |

### 2. 速率计算

使用指数加权移动平均 (EWMA) 算法实时计算速率：
- 平滑因子 α = 0.25
- 更新间隔 1 秒
- 自动衰减历史数据

### 3. Stream 集成

自动在 `endpoint.Stream` 的 `Read()` 和 `Write()` 中记录流量。

## 使用示例

### 基本使用

```go
import (
    "github.com/dep2p/go-dep2p/internal/core/bandwidth"
    bandwidthif "github.com/dep2p/go-dep2p/pkg/interfaces/bandwidth"
)

// 创建计数器
config := bandwidthif.DefaultConfig()
counter := bandwidth.NewCounter(config)

// 手动记录流量
counter.LogSentMessage(1024)
counter.LogRecvMessage(2048)

// 获取统计
total := counter.GetBandwidthTotals()
fmt.Printf("出站: %s (%s)\n",
    bandwidth.FormatBytes(total.TotalOut),
    bandwidth.FormatRate(total.RateOut))
```

### 按 Peer 统计

```go
// 记录流上的流量
counter.LogSentMessageStream(1024, "/chat/1.0", peerID)

// 获取该节点的统计
peerStats := counter.GetBandwidthForPeer(peerID)
fmt.Printf("与 %s 的流量: 入 %s, 出 %s\n",
    peerID.ShortString(),
    bandwidth.FormatBytes(peerStats.TotalIn),
    bandwidth.FormatBytes(peerStats.TotalOut))

// 获取流量最大的节点
topPeers := counter.TopPeers(10)
for _, p := range topPeers {
    fmt.Printf("  %s: %s\n", p.Peer.ShortString(),
        bandwidth.FormatBytes(p.Stats.TotalBytes()))
}
```

### 按协议统计

```go
// 获取协议统计
protoStats := counter.GetBandwidthForProtocol("/gossip/1.0")
fmt.Printf("GossipSub 流量: %s\n",
    bandwidth.FormatBytes(protoStats.TotalBytes()))

// 获取流量最大的协议
topProtocols := counter.TopProtocols(5)
```

### 定期报告

```go
reporter := bandwidth.NewReporter(counter, logger)
reporter.Start(time.Minute)
defer reporter.Stop()
```

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `Enabled` | true | 是否启用统计 |
| `TrackByPeer` | true | 是否按 Peer 统计 |
| `TrackByProtocol` | true | 是否按协议统计 |
| `IdleTimeout` | 1h | 空闲条目超时 |
| `TrimInterval` | 10m | 清理间隔 |
| `ReportInterval` | 1m | 报告间隔 |

## 统计数据结构

```go
type Stats struct {
    TotalIn  int64   // 入站字节数
    TotalOut int64   // 出站字节数
    RateIn   float64 // 入站速率 (bytes/sec)
    RateOut  float64 // 出站速率 (bytes/sec)
}
```

## 性能考虑

- 使用原子操作保证线程安全
- 使用 `sync.Map` 存储动态键值
- 定期清理空闲条目节省内存
- 速率计算使用 EWMA 避免频繁更新

## 相关文档

- [Endpoint 模块](../endpoint/README.md)
- [libp2p BandwidthCounter](https://github.com/libp2p/go-libp2p/tree/master/core/metrics)

