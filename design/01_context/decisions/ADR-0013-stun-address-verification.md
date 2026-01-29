# ADR-0013: STUN 地址验证策略

**状态**: 已采纳  
**日期**: 2026-01-29  
**决策者**: 架构组  

---

## 背景

在 P2P 网络中，节点需要发现并发布自己的公网地址，以便其他节点能够连接。传统方案要求：

1. STUN 探测获取外部地址（候选地址）
2. 通过 dial-back 验证地址可达性（需要其他节点协助）
3. 验证通过后才能发布到 DHT

### 问题：冷启动"鸡和蛋"困境

```
STUN 发现地址 → 存入 candidateAddrs → 需要 dial-back 验证
                                            ↓
                                    需要其他节点协助
                                            ↓
                                  冷启动阶段无协助节点
                                            ↓
                                  ❌ 永远无法发布地址！
```

实测日志显示：
```
09:15:07 直连地址候选已上报 ... candidates=1 verified=0  ← STUN 发现成功
09:15:08 已保存直连地址存储 ... candidates=1 verified=0  ← 但未验证
...（整个测试过程）...
09:25:26 跳过 PeerRecord 发布 ... directCount=0 relayCount=0  ← 无法发布
```

---

## 决策

**STUN 协议本身就是第三方验证机制，STUN 发现的地址直接标记为已验证。**

### 核心认知

1. **STUN 是验证机制**：STUN 服务器（如 Google STUN）是公正的第三方，返回的地址是经过协议验证的外部地址
2. **dial-back 是过度设计**：要求 dial-back 验证导致冷启动问题，且 STUN 本身已提供验证
3. **可选增强**：dial-back 验证作为可选增强，提升地址优先级

### 地址优先级体系

| 优先级 | 类型 | 验证方式 | 可发布 |
|--------|------|---------|--------|
| 150 | ConfiguredAdvertise | 用户配置 | ✅ |
| 100 | VerifiedDirect | dial-back 验证 | ✅ |
| **75** | **STUNDiscovered** | **STUN 协议验证** | **✅ 新增** |
| 50 | RelayGuarantee | Relay 连接 | ✅ |
| 10 | LocalListen | 无验证 | ⚠️ 仅局域网 |
| 0 | Unverified | 待验证 | ❌ |

### 代码变更

1. **`pkg/interfaces/reachability.go`**：新增 `PrioritySTUNDiscovered = 75`
2. **`internal/core/nat/service.go`**：STUN 地址调用 `OnDirectAddressVerified`（而非 `OnDirectAddressCandidate`）

```go
// 旧实现（v1.1）
coordinator.OnDirectAddressCandidate(maddr, "stun", PriorityUnverified)  // ❌ 无法发布

// 新实现（v1.2）
coordinator.OnDirectAddressVerified(maddr, "stun", PrioritySTUNDiscovered)  // ✅ 可发布
```

---

## 影响

### 正面影响

1. **解决冷启动问题**：节点无需依赖其他节点即可发布地址
2. **简化架构**：移除不必要的验证依赖
3. **提升用户体验**：地址发布更快，连接建立更顺畅

### 兼容性

- **向后兼容**：旧版本仍可连接新版本节点
- **TrustSTUNAddresses 配置**：保留此选项用于升级 STUN 地址优先级到 100

---

## 替代方案

### 方案 A：保持 dial-back 必需（已否决）

- 优点：更严格的验证
- 缺点：冷启动问题无法解决

### 方案 B：AdvertisedAddrs 回退到候选地址（已否决）

- 优点：不改变验证逻辑
- 缺点：增加复杂度，可能发布不可达地址

### 方案 C：STUN 默认为已验证（已采纳）

- 优点：简单直接，符合 STUN 协议本意
- 缺点：无

---

## 相关文档

- [NAT 穿透规范](../../02_constraints/protocol/L3_network/nat.md)
- [发现流程](../../03_architecture/L3_behavioral/discovery_flow.md)
- [Bug 分析报告](../../_discussions/20260129-chat-test-bug-analysis.md)

---

**最后更新**：2026-01-29
