# F3: 网络层需求

> 定义 DeP2P 的节点发现、NAT 穿透、Relay 中继和网络弹性能力

---

## 需求列表

| ID | 标题 | 优先级 | 状态 | 来源 |
|----|------|--------|------|------|
| [REQ-NET-001](REQ-NET-001.md) | 节点发现 | P0 | draft | 竞品分析 |
| [REQ-NET-002](REQ-NET-002.md) | NAT 穿透 | P1 | draft | 竞品分析 |
| [REQ-NET-003](REQ-NET-003.md) | Relay 中继 | P0 | draft | 竞品分析 |
| [REQ-NET-004](REQ-NET-004.md) | 网络变化处理 | P1 | draft | iroh |
| [REQ-NET-005](REQ-NET-005.md) | **网络弹性与恢复** | **P0** | draft | **旧 dep2p** |
| [REQ-NET-006](REQ-NET-006.md) | **可达性验证** | **P1** | draft | **旧 dep2p** |
| [REQ-NET-007](REQ-NET-007.md) | **★ 快速断开检测** | **P0** | draft | **BUG-34** |

---

## 核心设计

- **多机制发现**：Bootstrap + DHT + Rendezvous + mDNS
- **智能穿透**：NAT 类型检测 + 打洞 + 降级 + 共享 Socket
- **统一 Relay**：单一 Relay 概念（DeP2P 特有）
- **网络变化处理**：主动检测 + 抖动容忍 + 自动重连
- **★ 网络弹性**：状态机监控 + 自动恢复 + Transport Rebind
- **★ 可达性优先**：dial-back 验证 + witness 协议 + 地址分级
- **★ 快速断开检测**：多层检测 + 见证人机制 + 重连宽限期 + 震荡抑制

---

## 关键竞品参考

| 竞品 | 特点 | DeP2P 借鉴 |
|------|------|------------|
| iroh | Home Relay、DNS 发现、network_change() | Relay 设计、网络变化 API |
| libp2p | DHT、AutoNAT | 发现和穿透机制 |
| torrent | Tracker、PEX | 多机制并行 |
| **旧 dep2p** | **IMPL-NETWORK-RESILIENCE 6 Phase** | **网络弹性完整方案** |

---

## 2026-01-18 新增需求说明

基于旧代码分析发现的重要能力：

### REQ-NET-005: 网络弹性与恢复

解决"长时间运行后断连无法恢复"的核心问题：
- NetworkMonitor 状态机 (Healthy → Degraded → Down → Recovering)
- RecoveryManager 自动恢复 (Rebind + 地址重发现 + 重连)
- Publish 语义修复（不再静默成功）

### REQ-NET-006: 可达性验证

实现"可达性优先"策略：
- dial-back 协议验证地址可达性
- witness 协议多证人验证
- Relay 地址立即发布，直连地址验证后发布

---

## 2026-01-28 新增需求说明

### REQ-NET-007: 快速断开检测

解决"成员状态与连接状态脱节"的核心问题（BUG-34）：
- 多层次检测架构（QUIC Keep-Alive + MemberLeave + Witness + Liveness）
- 检测延迟目标：直连 <10s，Relay <15s，非直连 <20s
- 见证人机制：最先检测到断开的节点广播 WitnessReport
- 重连宽限期：防止移动网络切换误判（默认 15s）
- 震荡抑制：防止网络抖动导致频繁状态变更（60s 内 >=3 次判定为震荡）
- 断开保护期：防止竞态条件重新添加已断开成员（默认 30s）

关联文档：
- [ADR-0012 断开检测架构](../../decisions/ADR-0012-disconnect-detection.md)
- [INV-003 连接即成员](../../decisions/invariants/INV-003-connection-membership.md)
- [断开检测行为设计](../../../../03_architecture/L3_behavioral/disconnect_detection.md)

---

**最后更新**：2026-01-28（新增 REQ-NET-007 快速断开检测）
