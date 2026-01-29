# pkg_interfaces 设计概述

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 设计目标

pkg_interfaces 的设计目标是提供**清晰、稳定、可测试**的系统组件接口，实现依赖倒置原则（DIP）。

### 核心原则

1. **接口优先**：先定义契约，再实现
2. **依赖倒置**：高层依赖抽象，不依赖实现
3. **分层清晰**：严格的 Tier 分层，无循环依赖
4. **可替换性**：允许多种实现，支持 Mock 测试

---

## 架构设计

### 依赖倒置原则（DIP）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          依赖倒置原则                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   高层模块 (internal/realm)                                                 │
│       ↓ 依赖                                                                │
│   抽象接口 (pkg/interfaces/host)                                            │
│       ↑ 实现                                                                │
│   低层模块 (internal/core/host)                                             │
│                                                                             │
│   好处：                                                                    │
│   • 高层不依赖低层实现                                                       │
│   • 低层可以替换实现                                                         │
│   • 接口成为稳定的契约                                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 分层架构

### Tier 分层设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Tier 分层架构                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Tier 4: Realm（领域层）                                                    │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │ Realm, RealmManager                                              │     │
│   └──────────────────────────────────────────────────────────────────┘     │
│                                ↓ 依赖                                       │
│   Tier 3: Network Services（网络服务层）                                     │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │ Discovery, Messaging, PubSub, Peerstore, ConnManager, etc.      │     │
│   └──────────────────────────────────────────────────────────────────┘     │
│                                ↓ 依赖                                       │
│   Tier 2: Transport（传输层）                                                │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │ Transport, Security, Muxer                                       │     │
│   └──────────────────────────────────────────────────────────────────┘     │
│                                ↓ 依赖                                       │
│   Tier 1: Core（核心层）                                                     │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │ Host, Node, Connection, Stream                                   │     │
│   └──────────────────────────────────────────────────────────────────┘     │
│                                ↓ 依赖                                       │
│   Tier 0-1: Identity（身份层）                                               │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │ Identity, PublicKey, PrivateKey                                  │     │
│   └──────────────────────────────────────────────────────────────────┘     │
│                                ↓ 依赖                                       │
│   Tier -1: Types（类型层）                                                   │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │ pkg/types - 零依赖纯类型                                          │     │
│   └──────────────────────────────────────────────────────────────────┘     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**依赖规则**：
- ✅ 上层可以依赖下层
- ❌ 下层不能依赖上层
- ❌ 禁止跨层依赖
- ❌ 禁止循环依赖

---

## 接口与实现映射

### 映射关系

| pkg/interfaces | internal/core | 说明 |
|----------------|---------------|------|
| identity/ | identity/ | 身份管理 |
| host.go | host/ | P2P 主机 |
| node.go | node/ | 顶层 API |
| transport.go | transport/ | 传输层 |
| security.go | security/ | 安全握手 |
| muxer.go | muxer/ | 流复用 |
| discovery.go | discovery/ | 节点发现 |
| messaging.go | messaging/ | 消息服务 |
| pubsub.go | pubsub/ | 发布订阅 |
| realm.go | realm/ | Realm 管理 |
| peerstore.go | peerstore/ | 节点存储 |
| connmgr.go | connmgr/ | 连接管理 |
| swarm.go | swarm/ | 连接池 |

### Fx 模块集成

```go
// internal/core/host/module.go
package host

import (
    "go.uber.org/fx"
    pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// 编译时验证接口实现
var _ pkgif.Host = (*hostImpl)(nil)

// Fx 模块定义
var Module = fx.Module("host",
    fx.Provide(
        fx.Annotate(
            NewHost,
            fx.As(new(pkgif.Host)),
        ),
    ),
)
```

---

## 接口设计模式

### 1. 单一职责接口

**原则**：每个接口只负责一个功能领域

```go
// ✅ 好：职责单一
type Discovery interface {
    FindPeers(ctx context.Context, ns string, opts ...DiscoveryOption) (<-chan PeerInfo, error)
    Advertise(ctx context.Context, ns string, opts ...DiscoveryOption) (time.Duration, error)
}

// ❌ 坏：职责混杂
type SuperInterface interface {
    FindPeers(...) error
    SendMessage(...) error  // 应该在 Messaging 接口
    CreateRealm(...) error  // 应该在 RealmManager 接口
}
```

### 2. 接口组合

**原则**：通过组合构建复杂接口

```go
// 基础接口
type Discoverer interface {
    FindPeers(ctx context.Context, ns string, opts ...DiscoveryOption) (<-chan PeerInfo, error)
}

type Advertiser interface {
    Advertise(ctx context.Context, ns string, opts ...DiscoveryOption) (time.Duration, error)
}

// 组合接口
type Discovery interface {
    Discoverer
    Advertiser
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

### 3. 选项模式

**原则**：使用函数选项提供灵活配置

```go
// 选项函数类型
type DiscoveryOption func(*DiscoveryOptions)

// 选项结构
type DiscoveryOptions struct {
    Limit int
    TTL   time.Duration
}

// 选项函数
func WithLimit(limit int) DiscoveryOption {
    return func(o *DiscoveryOptions) {
        o.Limit = limit
    }
}

// 使用
peers, err := discovery.FindPeers(ctx, "test-ns", WithLimit(10))
```

---

## 与 go-libp2p 的关系

### 兼容接口

DeP2P 核心接口与 go-libp2p 保持兼容：

| DeP2P | go-libp2p | 兼容性 |
|-------|-----------|--------|
| Host | core/host.Host | ✅ 核心方法对齐 |
| Transport | core/transport.Transport | ✅ 方法签名兼容 |
| Security | core/sec.SecureTransport | ✅ 兼容 |
| Muxer | core/mux.Muxer | ✅ 兼容 |
| Peerstore | core/peerstore.Peerstore | ✅ 部分兼容 |
| Discovery | core/discovery.Discovery | ✅ 兼容 |

**兼容好处**：
- ✅ 可复用 go-libp2p 生态组件
- ✅ 降低学习曲线
- ✅ 参考成熟设计模式

### 扩展接口

DeP2P 特有接口（go-libp2p 中不存在）：

| 接口 | 用途 |
|------|------|
| **Realm** | 业务隔离单元（DeP2P 核心创新） |
| **RealmManager** | Realm 生命周期管理 |

---

## 关键设计决策

### 1. 接口粒度

**决策**：中等粒度接口

**理由**：
- ✅ 不过于细粒度（避免接口爆炸）
- ✅ 不过于粗粒度（保持职责单一）
- ✅ 适合 Fx 依赖注入

**示例**：
```go
// ✅ 好：中等粒度
type Host interface {
    ID() string
    Connect(...) error
    NewStream(...) (Stream, error)
    Peerstore() Peerstore
    // ... 7-10 个核心方法
}

// ❌ 坏：过于细粒度
type IDProvider interface { ID() string }
type Connector interface { Connect(...) error }
type StreamCreator interface { NewStream(...) (Stream, error) }
// ... 需要组合太多接口

// ❌ 坏：过于粗粒度
type Everything interface {
    // ... 50+ 个方法
}
```

### 2. 错误定义位置

**决策**：错误常量定义在接口文件中

**理由**：
- ✅ 错误是接口契约的一部分
- ✅ 使用方和实现方共享错误定义
- ✅ 便于文档和测试

**示例**：
```go
// pkg/interfaces/realm.go
package interfaces

var (
    ErrAlreadyInRealm = errors.New("already in a realm")
    ErrNotInRealm = errors.New("not in any realm")
)

type Realm interface {
    Join(ctx context.Context) error  // 可能返回 ErrAlreadyInRealm
    Leave(ctx context.Context) error // 可能返回 ErrNotInRealm
}
```

### 3. 接口返回值

**决策**：优先返回具体类型，必要时返回接口

**理由**：
- ✅ 具体类型更灵活（可访问额外方法）
- ✅ 接口返回值用于替换性（如工厂方法）

**示例**：
```go
// ✅ 好：返回具体类型
type Host interface {
    Addrs() []string  // 具体类型
}

// ✅ 好：返回接口（工厂方法）
type PubSub interface {
    Join(topic string, opts ...TopicOption) (Topic, error)  // 返回接口
}
```

---

## 相关文档

- [tier_structure.md](tier_structure.md) - 分层结构详解
- [../requirements/requirements.md](../requirements/requirements.md) - 需求说明
- [../coding/guidelines.md](../coding/guidelines.md) - 接口设计指南

---

**最后更新**：2026-01-13
