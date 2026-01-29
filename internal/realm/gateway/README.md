# Realm Gateway - 域网关

> **版本**: v1.0.0  
> **状态**: 已完成  
> **架构层**: Realm Layer

---

## 概述

`gateway` 实现 Realm Gateway（域网关），负责执行 Realm 内部的中继转发，与 `routing` 子模块协作形成完整的路由与中继系统。

**核心功能**:
- 中继转发 - Realm 内部中继服务
- PSK 认证 - 保证 Realm 隔离性
- 连接池 - 复用连接提升性能
- 带宽限流 - Token Bucket 限流

---

## 快速开始

```go
import "github.com/dep2p/go-dep2p/internal/realm/gateway"

// 创建配置
config := gateway.DefaultConfig()
config.MaxBandwidth = 100 * 1024 * 1024 // 100 MB/s
config.MaxConcurrent = 1000

// 创建 Gateway
gw := gateway.NewGateway("realm-id", host, auth, config)
defer gw.Close()

// 启动 Gateway
if err := gw.Start(ctx); err != nil {
    log.Fatal(err)
}

// 启动中继服务
go gw.ServeRelay(ctx)

// 处理中继请求
req := &interfaces.RelayRequest{
    SourcePeerID: "peer1",
    TargetPeerID: "peer2",
    Protocol:     "/dep2p/realm/my-realm/messaging",
    RealmID:      "my-realm",
}
if err := gw.Relay(ctx, req); err != nil {
    log.Printf("Relay failed: %v", err)
}
```

---

## 核心组件

| 组件 | 说明 |
|------|------|
| **Gateway** | 网关核心，协调各子组件 |
| **RelayService** | 处理中继请求，双向流转发 |
| **ConnectionPool** | 连接复用，LRU 淘汰 |
| **BandwidthLimiter** | Token Bucket 限流 |
| **ProtocolValidator** | 协议前缀验证 |
| **RouterAdapter** | 与 routing 协作适配器 |

---

## 协议验证规则

| 协议前缀 | 处理方式 |
|----------|---------|
| `/dep2p/realm/<realmID>/*` | Gateway 处理 |
| `/dep2p/app/<realmID>/*` | Gateway 处理 |
| `/dep2p/sys/*` | 节点级 Relay 处理 |

**验证流程**：
1. 提取协议中的 `<realmID>`
2. 检查 `<realmID>` == 本 Gateway 服务的 RealmID
3. 验证请求方持有该 Realm 的 PSK 证明
4. 全部通过则转发，否则拒绝

---

## 与 Routing 协作

路由与网关协作架构：

| 模块 | 职责 |
|------|------|
| **routing** | 路由选择、负载均衡、路径查找 |
| **gateway** | 中继转发、带宽控制、连接管理 |

**交互流程**：
1. `routing.FindRoute()` 发现目标需要中继
2. `routing.QueryReachable()` 查询可用网关
3. `routing.SelectNode()` 选择最优网关
4. `routing.RequestRelay()` 请求中继
5. `gateway.Relay()` 执行实际转发
6. `gateway.ReportState()` 报告状态

---

## 性能指标

| 指标 | 目标 |
|------|------|
| 转发吞吐量 | > 10 MB/s |
| 中继延迟增加 | < 5ms |
| 连接池命中率 | > 85% |
| 带宽限流精度 | +/- 5% |
| 最大并发会话 | 1000 |
| 内存占用 | < 100MB (1000会话) |

---

## 测试

```bash
go test -v ./internal/realm/gateway/...
go test -cover ./internal/realm/gateway/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档

---

**最后更新**: 2026-01-24
