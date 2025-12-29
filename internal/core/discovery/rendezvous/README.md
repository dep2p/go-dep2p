# Rendezvous 发现模块

## 概述

Rendezvous 模块提供基于主题（命名空间）的轻量级节点发现机制。与 DHT 不同，Rendezvous 不需要节点参与分布式哈希表，而是通过中心化的 Rendezvous 服务点来协调节点发现。

## 架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Rendezvous 发现                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────┐                        ┌──────────────┐               │
│  │   Node A     │                        │   Node B     │               │
│  │  (Client)    │                        │  (Client)    │               │
│  └──────┬───────┘                        └───────┬──────┘               │
│         │                                        │                       │
│         │  1. Register("topic/chat")             │                       │
│         ├────────────────────────────┐           │                       │
│         │                            │           │                       │
│         │                            ▼           │                       │
│         │                   ┌──────────────┐     │                       │
│         │                   │  Rendezvous  │     │                       │
│         │                   │    Point     │     │                       │
│         │                   │  (Server)    │     │                       │
│         │                   └──────┬───────┘     │                       │
│         │                          │             │                       │
│         │                          │             │  2. Discover("topic/chat")
│         │                          │◄────────────┤                       │
│         │                          │             │                       │
│         │                          │  3. Return [NodeA]                  │
│         │                          ├────────────►│                       │
│         │                          │             │                       │
│         │                          │             │  4. Connect to NodeA  │
│         │◄─────────────────────────┼─────────────┤                       │
│         │                          │             │                       │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## 文件结构

```
rendezvous/
├── README.md           # 本文档
├── protocol.go         # 协议常量、消息编解码、验证函数
├── store.go            # 注册信息存储（内存）
├── point.go            # Rendezvous Point 服务端
├── rendezvous.go       # Rendezvous Discoverer 客户端
└── rendezvous_test.go  # 单元测试
```

## 核心概念

### 命名空间 (Namespace)

命名空间是 Rendezvous 发现的核心概念，用于组织和隔离不同的节点组。

格式示例：
- 业务主题: `blockchain/mainnet/peers`
- Realm 内: `<RealmID>/topic/<name>`
- 服务发现: `service/<service-type>`

### TTL (Time-To-Live)

注册具有有效期（TTL），过期后自动清除。客户端需要定期续约以保持注册有效。

默认值：
- 默认 TTL: 2 小时
- 最大 TTL: 72 小时
- 续约间隔: TTL/2

### Rendezvous Point

Rendezvous Point 是服务端组件，负责：
- 存储节点注册信息
- 响应发现请求
- 自动清理过期注册

### Discoverer

Discoverer 是客户端组件，负责：
- 注册到 Rendezvous Point
- 自动续约
- 发现命名空间中的节点

## 协议

### 协议 ID

```
/dep2p/sys/rendezvous/1.0.0
```

> 注意：Rendezvous 是系统级协议，使用 `sys` 前缀，与其他系统协议保持一致。

### 消息类型

| 类型 | 说明 |
|------|------|
| `REGISTER` | 注册请求 |
| `REGISTER_RESPONSE` | 注册响应 |
| `UNREGISTER` | 取消注册请求 |
| `DISCOVER` | 发现请求 |
| `DISCOVER_RESPONSE` | 发现响应 |

### Protobuf 定义

消息定义位于 `pkg/proto/rendezvous/rendezvous.proto`。

## 使用示例

### 作为客户端

```go
import (
    "github.com/dep2p/go-dep2p/internal/core/discovery/rendezvous"
)

// 创建 Discoverer
config := rendezvous.DefaultDiscovererConfig()
config.Points = []types.NodeID{rendezvousPointID}

discoverer := rendezvous.NewDiscoverer(endpoint, localID, config, logger)
discoverer.Start(ctx)
defer discoverer.Stop()

// 注册到命名空间
err := discoverer.Register(ctx, "my-app/chat", 2*time.Hour)

// 发现节点
peers, err := discoverer.Discover(ctx, "my-app/chat", 10)

// 取消注册
err = discoverer.Unregister(ctx, "my-app/chat")
```

### 作为服务点

```go
import (
    "github.com/dep2p/go-dep2p/internal/core/discovery/rendezvous"
)

// 创建 Point
config := rendezvous.DefaultPointConfig()
config.MaxRegistrations = 10000
config.MaxNamespaces = 1000

point := rendezvous.NewPoint(endpoint, config, logger)
point.Start(ctx)
defer point.Stop()

// 获取统计信息
stats := point.Stats()
fmt.Printf("注册数: %d, 命名空间数: %d\n", 
    stats.TotalRegistrations, 
    stats.TotalNamespaces)
```

## 配置参数

### DiscovererConfig

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| Points | []NodeID | nil | 已知的 Rendezvous 点 |
| DefaultTTL | Duration | 2h | 默认注册 TTL |
| RenewalInterval | Duration | 1h | 续约间隔 |
| DiscoverTimeout | Duration | 30s | 发现超时 |
| RegisterTimeout | Duration | 30s | 注册超时 |
| MaxRetries | int | 3 | 最大重试次数 |
| RetryInterval | Duration | 5s | 重试间隔 |

### PointConfig

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| MaxRegistrations | int | 10000 | 最大注册总数 |
| MaxNamespaces | int | 1000 | 最大命名空间数 |
| MaxTTL | Duration | 72h | 最大 TTL |
| DefaultTTL | Duration | 2h | 默认 TTL |
| CleanupInterval | Duration | 5m | 清理间隔 |
| MaxRegistrationsPerNamespace | int | 1000 | 每个命名空间最大注册数 |
| MaxRegistrationsPerPeer | int | 100 | 每个节点最大注册数 |
| DefaultDiscoverLimit | int | 100 | 默认发现限制 |

## 与 fx 集成

Rendezvous 模块通过 Discovery 模块的 fx 配置自动集成：

```go
config := discoveryif.DefaultConfig()
config.EnableRendezvous = true
config.ServeRendezvous = true  // 作为服务点
config.RendezvousPoints = []PeerInfo{...}  // 已知服务点
```

## 接口设计

Rendezvous 实现了多个细粒度接口，包括 `NamespaceDiscoverer`、`Rendezvous` 和 `Announcer`：

```go
// Rendezvous Discoverer 实现的接口
var _ discoveryif.Rendezvous = (*Discoverer)(nil)
var _ discoveryif.NamespaceDiscoverer = (*Discoverer)(nil)
var _ discoveryif.Announcer = (*Discoverer)(nil)
```

### Announcer 适配

Rendezvous Discoverer 同时实现了 `Announcer` 接口，可以作为通告器使用：

```go
// Announce 映射到 Register
func (d *Discoverer) Announce(ctx, namespace) error
func (d *Discoverer) AnnounceWithTTL(ctx, namespace, ttl) error

// StopAnnounce 映射到 Unregister
func (d *Discoverer) StopAnnounce(namespace) error
```

这使得 Rendezvous 可以与统一发现 API 无缝集成，在 `DiscoveryService.RegisterService` 时同时注册到 DHT Provider 和 Rendezvous。

### 接口矩阵

不同发现机制实现不同接口：

| 发现器 | PeerFinder | ClosestPeerFinder | NamespaceDiscoverer | Announcer |
|--------|------------|-------------------|---------------------|-----------|
| DHT | ✅ | ✅ | ✅ | ✅ |
| Rendezvous | ❌ | ❌ | ✅ | ✅ |
| mDNS | ❌ | ❌ | ✅ | ❌ |
| Bootstrap | ❌ | ❌ | ❌ | ❌ |

## 相关文档

- [设计文档](../../../../docs/01-design/protocols/network/01-discovery.md#5-rendezvous-发现)
- [Discovery 接口](../../../../pkg/interfaces/discovery/discovery.go)
- [Protobuf 定义](../../../../pkg/proto/rendezvous/rendezvous.proto)

