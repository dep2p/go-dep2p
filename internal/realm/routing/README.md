# Realm Routing - 域内路由

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Realm Layer

---

## 概述

`routing` 提供 Realm 层的智能路由功能，基于 DHT 路由表实现延迟感知的多跳路由。

**核心功能**:
- 🗺️ 路由表管理 - 基于 DHT 的节点管理
- ⚡ 智能路由 - 延迟感知选择
- 🔀 多跳路径 - Dijkstra 最短路径
- ⚖️ 负载均衡 - 加权轮询
- 💾 路由缓存 - LRU + TTL

---

## 快速开始

```go
import "github.com/dep2p/go-dep2p/internal/realm/routing"

// 创建配置
config := routing.DefaultConfig()
config.CacheSize = 1000
config.DefaultPolicy = interfaces.PolicyMixed

// 创建路由器
router := routing.NewRouter("realm-id", dht, config)
defer router.Close()

// 启动路由器
if err := router.Start(ctx); err != nil {
    log.Fatal(err)
}

// 查找路由
route, err := router.FindRoute(ctx, "target-peer")
if err != nil {
    log.Fatal(err)
}
log.Printf("Next hop: %s, Latency: %v", route.NextHop, route.Latency)

// 查找多条路由（负载均衡）
routes, err := router.FindRoutes(ctx, "target-peer", 3)

// 选择最佳路由
best, err := router.SelectBestRoute(ctx, routes, interfaces.PolicyLoadBalance)
```

---

## 核心组件

| 组件 | 说明 |
|------|------|
| **Router** | 路由核心，协调决策 |
| **RouteTable** | 基于 DHT 的路由表 |
| **PathFinder** | Dijkstra 路径查找 |
| **LoadBalancer** | 加权轮询负载均衡 |
| **LatencyProber** | 延迟测量与预测 |
| **GatewayAdapter** | 与 gateway 协作 |

---

## 路由算法

### Dijkstra 最短路径

使用 Dijkstra 算法查找最短路径，权重为节点延迟。

**时间复杂度**: O(E log V)

### 路径评分公式

```
score = latency * 0.5 + hops * 0.3 + load * 0.2
```

| 因素 | 权重 | 说明 |
|------|------|------|
| latency | 50% | 网络延迟 |
| hops | 30% | 跳数 |
| load | 20% | 节点负载 |

---

## 延迟测量

```go
// 创建延迟探测器
prober := routing.NewLatencyProber(host)
if err := prober.Start(ctx); err != nil {
    log.Fatal(err)
}

// 测量延迟
latency, err := prober.MeasureLatency(ctx, "peer-id")
log.Printf("Latency: %v", latency)

// 获取延迟统计
stats := prober.GetStats("peer-id")
log.Printf("Avg: %v, P95: %v, P99: %v", stats.Avg, stats.P95, stats.P99)
```

---

## 与 Gateway 协作

`routing` 与 `gateway` 协作完成 Realm 内部路由：

| 模块 | 职责 |
|------|------|
| **routing** | 路由选择、负载均衡、路径查找 |
| **gateway** | 中继转发、带宽控制、连接管理 |

---

## 性能指标

| 指标 | 目标 |
|------|------|
| 路由查询延迟 | < 10ms |
| 缓存命中率 | > 90% |
| 路径发现时间 | < 100ms |
| 负载均衡偏差 | < 15% |
| 内存占用 | < 50MB (1000节点) |

---

## 测试

```bash
go test -v ./internal/realm/routing/...
go test -cover ./internal/realm/routing/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档

---

**最后更新**: 2026-01-20
