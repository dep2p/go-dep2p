# API Node 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: P2P 节点门面，用户入口（API Layer）

---

## 模块概述

Node 是 DeP2P 的用户入口组件，位于 API Layer，提供简洁的 API 来创建和管理 P2P 节点。

| 属性 | 值 |
|------|-----|
| **架构层** | API Layer (入口层) |
| **代码位置** | 根目录 `dep2p.go`, `node.go`, `options.go` |
| **状态** | ✅ 已实现 |
| **依赖** | realm, protocol_*, core_host |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         api_node 职责                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 节点创建与配置                                                          │
│     • StartNode(opts...) 创建节点                                           │
│     • Option 模式配置                                                       │
│     • 预设配置 (Presets)                                                    │
│                                                                             │
│  2. 节点身份                                                                │
│     • 提供 NodeID                                                           │
│     • 提供监听地址                                                          │
│                                                                             │
│  3. Realm 操作                                                              │
│     • JoinRealm() 加入 Realm                                               │
│     • CurrentRealm() 获取当前 Realm                                        │
│                                                                             │
│  4. 生命周期                                                                │
│     • 节点启动                                                              │
│     • 节点关闭                                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

```go
// pkg/interfaces/node.go
type Node interface {
    // 身份信息
    ID() types.NodeID
    ListenAddrs() []types.Multiaddr
    
    // Realm 操作
    JoinRealm(ctx context.Context, name string, psk types.PSK) (Realm, error)
    CurrentRealm() Realm
    
    // 生命周期
    Close() error
}
```

---

## 依赖关系

| 依赖 | 架构层 | 说明 |
|------|--------|------|
| realm | Realm Layer | Realm 管理 |
| protocol_* | Protocol Layer | 应用协议 |
| core_host | Core Layer | 网络主机 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |
| [L4: 公共接口](../../L4_interfaces/public_interfaces.md) | 接口设计 |

---

**最后更新**：2026-01-13
