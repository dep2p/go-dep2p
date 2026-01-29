# Core ConnMgr 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 连接管理（Core Layer）

---

## 模块概述

core_connmgr 负责连接池管理、连接优先级和保护策略。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/connmgr/` |
| **Fx 模块** | `fx.Module("connmgr")` |
| **状态** | ✅ 已实现 |
| **依赖** | core_peerstore, core_eventbus |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        core_connmgr 职责                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 连接池管理                                                              │
│     • 水位控制（LowWater/HighWater）                                        │
│     • 连接回收（Trim）                                                      │
│     • 优先级排序                                                            │
│                                                                             │
│  2. 连接保护                                                                │
│     • 标签保护（Tag）                                                       │
│     • 防止关键连接被回收                                                    │
│     • 衰减标签（Decay）                                                     │
│                                                                             │
│  3. 连接门控（Gater）                                                       │
│     • 拨号拦截（InterceptPeerDial）                                        │
│     • 入站拦截（InterceptAccept）                                          │
│     • 安全拦截（InterceptSecured）                                         │
│     • 升级拦截（InterceptUpgraded）                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 连接优先级

```
连接优先级计算：

  Priority = BaseScore + Σ(TagScores)

  BaseScore:
    • 入站连接: 0
    • 出站连接: 10

  TagScores（示例）:
    • "realm-member": +100
    • "bootstrap": +50
    • "relay": +50
    • "dht-server": +30
```

---

## 公共接口

```go
// pkg/interfaces/connmgr.go

// ConnManager 连接管理器接口
type ConnManager interface {
    // TagPeer 为节点添加标签
    TagPeer(peer types.PeerID, tag string, value int)
    
    // UntagPeer 移除标签
    UntagPeer(peer types.PeerID, tag string)
    
    // Protect 保护连接
    Protect(peer types.PeerID, tag string)
    
    // Unprotect 取消保护
    Unprotect(peer types.PeerID, tag string) bool
    
    // IsProtected 检查是否受保护
    IsProtected(peer types.PeerID, tag string) bool
    
    // TrimOpenConns 修剪连接
    TrimOpenConns(ctx context.Context)
    
    // Notifee 获取通知器
    Notifee() Notifiee
}

// ConnectionGater 连接门控器接口
type ConnectionGater interface {
    InterceptPeerDial(peer types.PeerID) bool
    InterceptAddrDial(peer types.PeerID, addr types.Multiaddr) bool
    InterceptAccept(conn Connection) bool
    InterceptSecured(dir Direction, peer types.PeerID, conn Connection) bool
    InterceptUpgraded(conn Connection) (bool, ControlReason)
}
```

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `LowWater` | 100 | 低水位（开始回收） |
| `HighWater` | 400 | 高水位（停止接受新连接） |
| `GracePeriod` | 20s | 新连接保护期 |
| `DecayInterval` | 1min | 衰减间隔 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_swarm](../core_swarm/) | 连接群管理 |
| [core_resourcemgr](../core_resourcemgr/) | 资源管理 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
