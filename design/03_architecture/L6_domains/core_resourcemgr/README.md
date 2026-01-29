# Core ResourceMgr 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 资源管理（Core Layer）

---

## 模块概述

core_resourcemgr 提供资源限制和配额管理，防止资源耗尽攻击，确保节点稳定运行。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/resourcemgr/` |
| **Fx 模块** | `fx.Module("resourcemgr")` |
| **状态** | ✅ 已实现 |
| **依赖** | 无（底层组件） |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     core_resourcemgr 职责                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 资源限制                                                                 │
│     • 连接数限制（入站/出站）                                               │
│     • 流数限制（每连接/每节点）                                             │
│     • 内存使用限制                                                          │
│     • 文件描述符限制                                                        │
│                                                                             │
│  2. 资源作用域 (Scope)                                                       │
│     • 系统级作用域 (System)                                                 │
│     • 传输级作用域 (Transient)                                              │
│     • 服务级作用域 (Service)                                                │
│     • 协议级作用域 (Protocol)                                               │
│     • 节点级作用域 (Peer)                                                   │
│     • 连接级作用域 (Connection)                                             │
│     • 流级作用域 (Stream)                                                   │
│                                                                             │
│  3. 资源预留与释放                                                           │
│     • 预留资源（阻塞/非阻塞）                                               │
│     • 释放资源                                                              │
│     • 资源使用统计                                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 资源限制层次

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         资源限制层次                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  System Scope (系统级)                                                       │
│  ├─ 最大连接数: 1000                                                        │
│  ├─ 最大流数: 10000                                                         │
│  ├─ 最大内存: 1GB                                                           │
│  └─ 最大 FD: 900                                                            │
│                                                                             │
│  Transient Scope (临时资源)                                                  │
│  ├─ 正在建立的连接资源                                                      │
│  └─ 握手期间的临时资源                                                      │
│                                                                             │
│  Peer Scope (每节点)                                                         │
│  ├─ 最大连接数: 10                                                          │
│  ├─ 最大流数: 100                                                           │
│  └─ 最大内存: 64MB                                                          │
│                                                                             │
│  Connection Scope (每连接)                                                   │
│  └─ 最大流数: 50                                                            │
│                                                                             │
│  Stream Scope (每流)                                                         │
│  └─ 最大内存: 16MB                                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

```go
// pkg/interfaces/resource.go

// ResourceManager 资源管理接口
type ResourceManager interface {
    // 作用域获取
    ViewSystem() (ResourceScope, error)
    ViewTransient() (ResourceScope, error)
    ViewService(service string) (ResourceScope, error)
    ViewProtocol(proto types.ProtocolID) (ResourceScope, error)
    ViewPeer(peer types.PeerID) (ResourceScope, error)
    
    // 作用域打开
    OpenConnection(dir Direction, usefd bool, addr types.Multiaddr) (ConnectionScope, error)
    OpenStream(peer types.PeerID, dir Direction) (StreamScope, error)
    
    // 生命周期
    Close() error
}

// ResourceScope 资源作用域
type ResourceScope interface {
    ReserveMemory(size int, pri uint8) error
    ReleaseMemory(size int)
    Stat() ScopeStat
    BeginSpan() (ResourceScopeSpan, error)
    Done()
}

// ScopeStat 作用域统计
type ScopeStat struct {
    NumStreamsInbound  int
    NumStreamsOutbound int
    NumConnsInbound    int
    NumConnsOutbound   int
    NumFD              int
    Memory             int64
}
```

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `SystemMaxConns` | 1000 | 系统最大连接数 |
| `SystemMaxStreams` | 10000 | 系统最大流数 |
| `SystemMaxMemory` | 1GB | 系统最大内存 |
| `PeerMaxConns` | 10 | 每节点最大连接数 |
| `PeerMaxStreams` | 100 | 每节点最大流数 |
| `ConnMaxStreams` | 50 | 每连接最大流数 |

---

## 参考实现

### go-libp2p ResourceManager

```
github.com/libp2p/go-libp2p/p2p/host/resource-manager/
├── rcmgr.go          # 资源管理实现
├── scope.go          # 作用域实现
├── limit.go          # 限制定义
├── trace.go          # 追踪日志
├── metrics.go        # 指标收集
└── allowlist.go      # 白名单

关键特性：
• 层次化资源限制
• 作用域嵌套
• 自动限制配置
• 资源统计
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_swarm](../core_swarm/) | 连接群管理 |
| [core_connmgr](../core_connmgr/) | 连接管理 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
