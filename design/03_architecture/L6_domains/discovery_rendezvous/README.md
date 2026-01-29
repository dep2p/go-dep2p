# Discovery Rendezvous 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 命名空间发现（Discovery Layer）

---

## 模块概述

discovery_rendezvous 通过命名空间实现节点发现，节点可以在特定命名空间下注册和发现彼此。

| 属性 | 值 |
|------|-----|
| **架构层** | Discovery Layer |
| **代码位置** | `internal/discovery/rendezvous/` |
| **协议** | `/dep2p/sys/rendezvous/1.0.0` |
| **Fx 模块** | `fx.Module("discovery/rendezvous")` |
| **状态** | ✅ 已实现 |
| **依赖** | core_host |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    discovery_rendezvous 职责                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 命名空间注册                                                             │
│     • 在命名空间注册本节点                                                  │
│     • TTL 管理                                                              │
│     • 自动续期                                                              │
│                                                                             │
│  2. 命名空间发现                                                             │
│     • 发现命名空间内的节点                                                  │
│     • 分页查询                                                              │
│     • 异步发现                                                              │
│                                                                             │
│  3. Rendezvous 服务端                                                        │
│     • 存储注册信息                                                          │
│     • 处理发现请求                                                          │
│     • 过期清理                                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

```go
// pkg/interfaces/discovery.go (Rendezvous 部分)

// RendezvousDiscovery Rendezvous 发现接口
type RendezvousDiscovery interface {
    Discovery
    
    // Register 在命名空间注册
    Register(ctx context.Context, namespace string, ttl time.Duration) error
    
    // Unregister 取消注册
    Unregister(namespace string) error
    
    // Discover 发现命名空间内的节点
    Discover(ctx context.Context, namespace string, limit int) ([]types.PeerInfo, error)
    
    // DiscoverAsync 异步发现
    DiscoverAsync(ctx context.Context, namespace string) (<-chan types.PeerInfo, error)
}
```

---

## 使用场景

- Realm 内节点发现（命名空间 = RealmID）
- 应用级节点分组
- 服务发现

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `DefaultTTL` | 2h | 默认注册 TTL |
| `RefreshInterval` | 1h | 续期间隔 |
| `MaxRegistrations` | 1000 | 服务端最大注册数 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [discovery_coordinator](../discovery_coordinator/) | 发现协调器 |
| [realm](../realm/) | Realm 使用 Rendezvous 发现成员 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
