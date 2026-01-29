# DeP2P 设计文档索引 (SUMMARY)

> 全目录索引，便于快速导航

---

## 入口

- [README](README.md) - 设计文档入口

---

## 01 背景与决策 (Context)

- [README](01_context/README.md)
- [需求文档](01_context/requirements/README.md)
  - 功能需求
  - 非功能需求
- [参考研究](01_context/references/README.md)
  - iroh 分析
  - libp2p 分析
- [架构决策](01_context/decisions/README.md)
  - ADR-0001 身份优先
  - ADR-0002 Realm 隔离
  - ADR-0003 Relay 优先连接
  - ADR-0004 控制面/数据面分离
  - ADR-0005 Relay 部署模型

---

## 02 约束与规范 (Constraints)

- [README](02_constraints/README.md)
- [协议规范](02_constraints/protocol/README.md)
  - L0 编码规范
  - L1 身份规范
  - L2 传输层协议
  - L3 网络层协议
  - L4 应用层协议
- [工程标准](02_constraints/engineering/README.md)
  - 编码规范
  - 工程标准

---

## 03 架构设计 (Architecture)

- [README](03_architecture/README.md)
- [L1 系统概览](03_architecture/L1_overview/README.md)
  - 系统定位
  - 系统边界
  - 术语表
  - 核心概念
  - 系统不变量
- [L2 结构设计](03_architecture/L2_structural/README.md)
  - 分层模型
  - 依赖规则
  - 模块划分
  - C4 可视化
- [L3 行为设计](03_architecture/L3_behavioral/README.md)
  - 连接建立流程
  - Realm 加入流程
  - 节点发现流程
  - Relay 中继流程
  - 消息传递流程
- [L4 接口契约](03_architecture/L4_interfaces/README.md)
  - 公共接口设计
  - 内部接口设计
  - 组件-接口映射
  - Fx + Lifecycle 模式
- [L5 领域模型](03_architecture/L5_models/README.md)
  - 身份领域
  - 连接领域
  - Realm 领域
  - Relay 领域
- [L6 模块设计](03_architecture/L6_domains/README.md)
  - 模块清单与实现状态
  - 代码规范
  - core_identity
  - core_transport
  - core_security
  - core_muxer
  - core_connmgr
  - core_relay
  - core_discovery
  - core_nat
  - core_realm
  - core_messaging

---

## 04 交付保障 (Delivery)

- [README](04_delivery/README.md)
- [测试](04_delivery/testing/README.md)
  - 测试策略
  - 测试用例
  - 性能基准
  - 混沌测试
- [发布](04_delivery/release/README.md)
  - 版本策略
  - 发布流程
  - 发布产物
- [安全](04_delivery/security/README.md)
  - 威胁模型
  - 安全要求
  - 安全审计

---

## 05 治理 (Governance)

- [README](05_governance/README.md)
- 改进提案
- 版本治理
- 社区治理

---

## 06 开发指南 (Guides)

- [README](06_guides/README.md)
- 快速开始
- 开发指南
- 架构指南
- 运维指南

---

## 模板库 (Templates)

- [README](templates/README.md)
- [ADR 模板](templates/adr_template.md)
- [需求模板](templates/req_template.md)
- [测试模板](templates/tst_template.md)

---

## 工作区

- [讨论记录](_discussions/README.md)
- [归档](_archive/README.md)

---

**最后更新**：2026-01-11
