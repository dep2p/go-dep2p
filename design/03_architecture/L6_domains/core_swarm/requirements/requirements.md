# Core Swarm 需求追溯

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 功能需求

| ID | 需求 | 优先级 | 状态 |
|----|------|--------|------|
| SWARM-001 | 管理到所有节点的连接池 | P0 | ✅ |
| SWARM-002 | 支持多地址监听 | P0 | ✅ |
| SWARM-003 | 智能拨号调度 | P0 | ✅ |
| SWARM-004 | 流多路复用 | P0 | ✅ |
| SWARM-005 | 连接/流事件通知 | P1 | ✅ |
| SWARM-006 | 并发拨号（Dial Many） | P1 | ✅ |
| SWARM-007 | 黑洞检测 | P2 | 📋 |

---

## 非功能需求

| ID | 需求 | 指标 |
|----|------|------|
| SWARM-NF-001 | 拨号超时 | ≤15s（远程），≤5s（本地） |
| SWARM-NF-002 | 最大连接数 | 可配置，默认 1000 |
| SWARM-NF-003 | 最大并发拨号 | 可配置，默认 100 |

---

## 追溯矩阵

| 需求 | 设计 | 代码 | 测试 |
|------|------|------|------|
| SWARM-001 | [overview.md](../design/overview.md) | `swarm.go` | `swarm_test.go` |
| SWARM-002 | [overview.md](../design/overview.md) | `swarm_listen.go` | `listen_test.go` |
| SWARM-003 | [overview.md](../design/overview.md) | `dial.go` | `dial_test.go` |

---

**最后更新**：2026-01-13
