# INV-003: 连接即成员不变量

**创建日期**: 2026-01-28  
**状态**: 活跃  
**最后更新**: 2026-01-29（BUG-34.1 修正）  
**关联 ADR**: [ADR-0012 断开检测架构](../ADR-0012-disconnect-detection.md)

---

## 不变量定义

```
成员可见性 = (有活跃连接 ∨ 由可信成员广播) ∧ 通过 PSK 认证
```

等价表述：

1. **信任链传递**：已认证成员广播的其他成员信息应被信任
2. **断开即离开**：连接断开时，成员状态必须同步更新为离线
3. **多重保护**：断开保护期 + 防误判机制防止竞态重新添加

> ⚠️ **BUG-34.1 修正**：原"有连接才能是成员"设计过于严格，阻止了正常的 PubSub 成员同步。
> 详见 [BUG-34.1 修复说明](#bug-341-修正2026-01-29)。

---

## 适用范围

| 场景 | 是否适用 | 说明 |
|------|---------|------|
| Realm 内成员管理 | ✓ 适用 | 核心应用场景 |
| 节点级连接 | ✓ 适用 | 无 Realm 时也遵循 |
| DHT/PubSub 消息来源 | ✓ 适用 | 接受来自可信成员的广播 |

---

## 保护期例外

### 重连宽限期

```
状态转换:
  CONNECTED ─(断开)─▶ DISCONNECTING ─(15s超时)─▶ DISCONNECTED
                           │
                           └─(重连成功)─▶ CONNECTED
```

**规则**:
- 断开后进入 DISCONNECTING 状态，持续 15s
- DISCONNECTING 期间成员仍被视为"在线"
- 15s 内重连成功，状态恢复为 CONNECTED
- 15s 超时，状态变为 DISCONNECTED，移除成员

**目的**: 容忍移动网络切换（4G/WiFi）导致的短暂断开

### 断开保护期

```
规则:
  检测到断开 → 记录 peerID 到 recentlyDisconnected 列表
  保护期内（30s）的 Add() 请求被静默忽略
  保护期超时后，自动清理记录
```

**目的**: 防止 PubSub/DHT 缓存的过期数据导致刚断开的成员被重新添加

---

## 违反检测

### 检测方法

```go
// 定期检查：离线成员超时清理
func (m *Manager) cleanupStaleMembers() {
    for peerID, member := range m.members {
        // 检查 LastSeen 是否超过阈值（默认 24 小时）
        if !member.Online && member.LastSeen.Before(threshold) {
            logger.Info("清理过期成员", "peerID", peerID[:8])
            m.Remove(context.Background(), peerID)
        }
    }
}
```

### 违反场景

| 场景 | 原因 | 修复 |
|------|------|------|
| 已断开成员被 PubSub 消息重新添加 | 保护期未生效 | 检查 recentlyDisconnected |
| 竞态条件 | 断开事件处理延迟 | 加锁 + 原子操作 |
| 恶意成员广播伪造信息 | PubSub 验证缺失 | PubSub 验证器只接受已知成员消息 |

---

## 实现要求

### MemberManager.Add()

```
伪代码:

  FUNCTION Add(memberInfo)
    // 1. 检查防误判机制（震荡检测、断开保护期）
    IF antiFalsePositive.ShouldRejectAdd(memberInfo.PeerID) THEN
      RETURN nil  // 静默忽略
    END
    
    // 2. 检查断开保护期（兼容旧逻辑）
    IF recentlyDisconnected[memberInfo.PeerID] EXISTS THEN
      IF time.Since(disconnectTime) < DisconnectProtection THEN
        RETURN nil  // 静默忽略
      END
    END
    
    // ★ BUG-34.1 修正：移除"必须有活跃连接"检查
    // 原因：成员同步通过 PubSub 广播传递，A 通过 B 收到 C 的信息
    //       A 与 C 可能没有直接连接，但 C 是合法成员
    // 保护机制：
    //   1. 断开保护期 - 已断开的成员在保护期内不会被重新添加
    //   2. 防误判机制 - 震荡检测 + 宽限期
    //   3. PubSub 验证 - 只接受来自已知成员的广播
    
    // 3. 正常添加
    ...
  END
```

### 断开事件处理

```
伪代码:

  FUNCTION onPeerDisconnected(peerID)
    // 1. 记录到保护列表
    recentlyDisconnected[peerID] = time.Now()
    
    // 2. 启动宽限期定时器
    IF gracePeriodEnabled THEN
      setStatus(peerID, StatusDisconnecting)
      startGraceTimer(peerID, ReconnectGracePeriod)
    ELSE
      // 3. 立即移除
      Remove(peerID)
    END
  END
```

---

## 测试用例

| 测试 | 描述 | 预期结果 |
|------|------|---------|
| TestAddWithoutConnection | 添加无连接的成员 | 被静默忽略 |
| TestAddInProtectionPeriod | 保护期内添加刚断开的成员 | 被静默忽略 |
| TestGracePeriodReconnect | 宽限期内重连 | 成员未被移除 |
| TestGracePeriodTimeout | 宽限期超时 | 成员被移除 |
| TestInvariantViolationRecovery | 模拟不变量违反 | 自动修复 |

---

## BUG-34.1 修正（2026-01-29）

### 问题发现

5 节点测试中发现部分节点只能看到 3 个成员（期望 5 个）：
- 节点 4FQtqGwE（完整 Bootstrap+Relay）: 看到 5 个成员 ✅
- 节点 F2pHenMb（缺少 Relay）: 看到 3 个成员 ❌

日志显示：
```log
BUG-34: 拒绝添加无活跃连接的成员 peerID=F2pHenMb
```

### 根本原因

原 BUG-34 设计的"必须有活跃连接"检查过于严格，阻止了正常的成员同步：

```
节点 A ←→ 节点 B ←→ 节点 C
   │         │
   │   PubSub 收到 C   
   │  "join: C 加入"   
   └─────────┬─────────
             ↓
        BUG-34 检查：
        A 与 C 有连接吗？
             ↓
           ❌ 无（NAT 限制）
             ↓
        拒绝添加 C ← 错误！
```

### 修正方案

**移除"必须有活跃连接"检查**，改为依赖以下保护机制：

| 机制 | 作用 |
|------|------|
| 断开保护期 | 已断开的成员在 30s 内不会被重新添加 |
| 防误判机制 | 震荡检测 + 宽限期处理快速断开重连 |
| PubSub 验证 | 只接受来自已知成员的广播消息 |

### 新设计理念

```
旧设计：成员可见性 = 有活跃连接 ∧ 通过认证
新设计：成员可见性 = (有活跃连接 ∨ 由可信成员广播) ∧ 通过认证

核心变化：信任链传递
  - 如果 B 是已认证的可信成员
  - B 通过 PubSub 广播 C 的加入消息
  - 则 A 信任 B 的广播，添加 C 到成员列表
  - 后续 A 尝试通过 B 或 Relay 连接 C
```

### 修改文件

| 文件 | 修改内容 |
|------|----------|
| `internal/realm/member/manager.go` | 移除 `Add()` 中的活跃连接检查 |

---

## 关联文档

| 类型 | 文档 |
|------|------|
| 架构决策 | [ADR-0012 断开检测架构](../ADR-0012-disconnect-detection.md) |
| 行为设计 | [断开检测流程](../../../03_architecture/L3_behavioral/disconnect_detection.md) |
| 需求 | [REQ-NET-007 快速断开检测](../../requirements/functional/F3_network/REQ-NET-007.md) |
| BUG 分析 | [BUG-34 分析与修复](../../../_discussions/20260128-member-connection-state-sync.md) |
| BUG 分析 | [BUG-34.1 测试分析](../../../_discussions/20260129-chat-test-bug-analysis.md) |
| 其他不变量 | [INV-001 身份优先](INV-001-identity-first.md) |
| 其他不变量 | [INV-002 Realm 成员资格](INV-002-realm-membership.md) |
