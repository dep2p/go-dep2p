# Protocol Messaging 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 消息传递（Protocol Layer）

---

## 模块概述

protocol_messaging 提供点对点消息传递能力，是 DeP2P 应用协议的核心组件。

| 属性 | 值 |
|------|-----|
| **架构层** | Protocol Layer |
| **代码位置** | `internal/protocol/messaging/` |
| **协议前缀** | `/dep2p/app/<realmID>/messaging/*` |
| **状态** | ✅ 已实现 |
| **依赖** | core_host, realm |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    protocol_messaging 职责                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 点对点消息 (Messaging)                                                   │
│     • 请求-响应模式                                                         │
│     • 单向通知模式                                                          │
│     • 消息处理器注册                                                        │
│                                                                             │
│  2. 消息编解码                                                               │
│     • 消息封装与解析                                                        │
│     • 协议版本处理                                                          │
│                                                                             │
│  3. Realm 隔离                                                               │
│     • 消息路由到指定 Realm                                                  │
│     • 成员资格验证                                                          │
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
| [L3: 消息流程](../../L3_behavioral/messaging_flow.md) | 行为设计 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
