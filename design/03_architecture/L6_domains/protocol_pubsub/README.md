# Protocol PubSub 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 发布订阅（Protocol Layer）

---

## 模块概述

protocol_pubsub 提供 GossipSub 发布订阅能力，实现一对多消息广播。

| 属性 | 值 |
|------|-----|
| **架构层** | Protocol Layer |
| **代码位置** | `internal/protocol/pubsub/` |
| **协议前缀** | `/dep2p/app/<realmID>/pubsub/*` |
| **状态** | ✅ 已实现 |
| **依赖** | core_host, realm |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    protocol_pubsub 职责                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 发布订阅 (PubSub)                                                        │
│     • GossipSub 协议实现                                                    │
│     • 主题管理（创建/订阅/取消订阅）                                         │
│     • 消息发布与接收                                                        │
│                                                                             │
│  2. Mesh 管理                                                                │
│     • Mesh 节点选择                                                         │
│     • 心跳与维护                                                            │
│     • GRAFT/PRUNE 操作                                                      │
│                                                                             │
│  3. 消息验证                                                                 │
│     • 签名验证                                                              │
│     • Realm 成员验证                                                        │
│     • 重复消息过滤                                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 目录导航

| 文档 | 说明 |
|------|------|
| [requirements/requirements.md](requirements/requirements.md) | 需求追溯 |
| [design/overview.md](design/overview.md) | 整体设计 |
| [design/internals.md](design/internals.md) | 内部设计 |
| [coding/guidelines.md](coding/guidelines.md) | 编码指南 |
| [testing/strategy.md](testing/strategy.md) | 测试策略 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [L3: 消息流程](../../L3_behavioral/messaging_flow.md) | PubSub 流程 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
