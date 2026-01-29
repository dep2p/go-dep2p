# 测试策略 (Strategy)

> 测试策略、分层模型、质量门禁

---

## 目录结构

```
strategy/
├── README.md                  # 本文件
├── test_strategy.md           # 整体测试策略
├── test_pyramid.md            # 测试金字塔（四层架构）
├── test_matrix.md             # 测试矩阵
├── network_validation.md      # 【新】真实网络验证策略
├── test_path.md               # 测试路径与优先级
└── quality_gates.md           # 质量门禁
```

---

## 概述

本目录定义 DeP2P 的测试策略和质量保障机制。DeP2P 采用**四层测试架构**：

```
Level 4: 真实网络验证  ← NAT/Relay/跨网络（无法本地模拟）
Level 3: E2E 测试      ← 完整用户场景（本地多节点）
Level 2: 集成测试      ← 跨模块协作
Level 1: 单元测试      ← 单函数/方法
```

| 文档 | 说明 |
|------|------|
| [test_strategy.md](test_strategy.md) | 整体测试方法论和工具选型 |
| [test_pyramid.md](test_pyramid.md) | 测试金字塔（**四层架构**） |
| [test_matrix.md](test_matrix.md) | 模块 × 测试类型 覆盖矩阵 |
| [network_validation.md](network_validation.md) | **真实网络验证策略（新）** |
| [test_path.md](test_path.md) | 测试执行顺序、依赖关系、优先级 |
| [quality_gates.md](quality_gates.md) | PR 合并和发布的质量门禁 |

---

## 核心原则

| 原则 | 说明 |
|------|------|
| **测试优先** | 关键功能必须有测试覆盖 |
| **分层测试** | 遵循测试金字塔，单元测试为基础 |
| **自动化** | 所有测试必须可自动执行 |
| **快速反馈** | 单元测试应在秒级完成 |
| **可重复** | 测试结果必须可重复 |

---

## 测试类型

| 层级 | 类型 | 目标 | 执行时机 | 运行命令 |
|:----:|------|------|----------|----------|
| L1 | 单元测试 | 函数/方法正确性 | 每次提交 | `go test ./...` |
| L2 | 集成测试 | 模块间交互 | 每次 PR | `go test -tags=integration ./tests/integration/...` |
| L3 | E2E 测试 | 完整用户流程 | 每日/发布前 | `go test -tags=e2e ./tests/e2e/...` |
| L4 | **真实网络验证** | NAT/Relay/跨网络 | 发布前 | 手动 + AI 分析 |
| - | 基准测试 | 性能回归检测 | 每周/发布前 | `go test -bench=. ./...` |
| - | 混沌测试 | 故障恢复能力 | 发布前 | - |

### 真实网络验证场景

| 场景 ID | 场景名称 | 优先级 |
|---------|----------|:------:|
| S01 | 基础三节点通信 | P0 |
| S02 | NAT 穿透验证 | P0 |
| S03 | DHT 路由验证 | P0 |
| S04 | 中继通信验证 | P1 |
| S05 | 成员同步验证 | P1 |
| S06 | 网络切换恢复 | P1 |

**详细说明**：[network_validation.md](network_validation.md)

---

## 快速链接

- [测试策略详情](test_strategy.md)
- [测试金字塔（四层架构）](test_pyramid.md)
- [测试矩阵](test_matrix.md)
- [**真实网络验证策略**](network_validation.md)
- [测试路径与优先级](test_path.md)
- [质量门禁](quality_gates.md)

---

**最后更新**：2026-01-20
