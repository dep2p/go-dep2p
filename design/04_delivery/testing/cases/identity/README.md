# 身份测试用例 (Identity)

> 身份模块的测试用例集

---

## 概述

本目录包含身份模块 (`internal/core/identity/`) 的测试用例，覆盖密钥管理、NodeID 派生、签名验证等核心功能。

---

## 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-IDENTITY-0001 | 密钥对生成 | 单元 | P0 | ✅ |
| TST-IDENTITY-0002 | PeerID 派生 | 单元 | P0 | ✅ |
| TST-IDENTITY-0003 | 签名生成 | 单元 | P0 | ✅ |
| TST-IDENTITY-0004 | 签名验证 | 单元 | P0 | ✅ |
| TST-IDENTITY-0005 | 密钥导出 | 单元 | P1 | ✅ |
| TST-IDENTITY-0006 | 密钥导入 | 单元 | P1 | ✅ |
| TST-IDENTITY-0007 | 密钥持久化 | 集成 | P1 | ✅ |
| TST-IDENTITY-0008 | 无效密钥处理 | 单元 | P1 | ✅ |
| TST-IDENTITY-0009 | 并发密钥生成 | 单元 | P2 | ✅ |
| TST-IDENTITY-0010 | 模块集成 | 集成 | P2 | ✅ |

---

## 用例详情

### TST-IDENTITY-0001: 密钥对生成

| 字段 | 值 |
|------|-----|
| **ID** | TST-IDENTITY-0001 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **关联需求** | REQ-IDENTITY-0001 |
| **代码位置** | `internal/core/identity/key_test.go` |

**测试目标**：验证 Ed25519 密钥对生成功能

**前置条件**：无

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 调用 GenerateKeyPair() | 返回密钥对，无错误 |
| 2 | 验证私钥长度 | 长度为 64 字节 |
| 3 | 验证公钥长度 | 长度为 32 字节 |
| 4 | 验证公私钥匹配 | 派生公钥与返回公钥一致 |

**测试代码**：

```go
func TestGenerateKeyPair(t *testing.T) {
    priv, pub, err := GenerateKeyPair()
    require.NoError(t, err)
    assert.Len(t, priv, 64)
    assert.Len(t, pub, 32)
    
    // 验证公私钥匹配
    derivedPub := priv.Public().(ed25519.PublicKey)
    assert.Equal(t, pub, derivedPub)
}
```

---

### TST-IDENTITY-0002: NodeID 派生

| 字段 | 值 |
|------|-----|
| **ID** | TST-IDENTITY-0002 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **关联需求** | REQ-IDENTITY-0002 |
| **代码位置** | `internal/core/identity/nodeid_test.go` |

**测试目标**：验证从公钥派生 NodeID 的正确性

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 生成密钥对 | 成功 |
| 2 | 从公钥派生 NodeID | 返回 NodeID，无错误 |
| 3 | 验证 NodeID 长度 | 32 字节 |
| 4 | 验证确定性 | 相同公钥产生相同 NodeID |

**测试代码**：

```go
func TestNodeIDFromPublicKey(t *testing.T) {
    _, pub, _ := GenerateKeyPair()
    
    id1, err := NodeIDFromPublicKey(pub)
    require.NoError(t, err)
    assert.Len(t, id1, 32)
    
    // 验证确定性
    id2, _ := NodeIDFromPublicKey(pub)
    assert.Equal(t, id1, id2)
}
```

---

### TST-IDENTITY-0003: 签名生成

| 字段 | 值 |
|------|-----|
| **ID** | TST-IDENTITY-0003 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **关联需求** | REQ-IDENTITY-0003 |
| **代码位置** | `internal/core/identity/sign_test.go` |

**测试目标**：验证消息签名功能

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 生成密钥对 | 成功 |
| 2 | 签名消息 | 返回签名，无错误 |
| 3 | 验证签名长度 | 64 字节 |
| 4 | 验证签名确定性 | 相同输入产生相同签名 |

---

### TST-IDENTITY-0004: 签名验证

| 字段 | 值 |
|------|-----|
| **ID** | TST-IDENTITY-0004 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **关联需求** | REQ-IDENTITY-0003 |
| **代码位置** | `internal/core/identity/sign_test.go` |

**测试目标**：验证签名验证功能

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 生成密钥对并签名 | 成功 |
| 2 | 验证有效签名 | 返回 true |
| 3 | 验证篡改消息 | 返回 false |
| 4 | 验证错误公钥 | 返回 false |
| 5 | 验证空签名 | 返回错误 |

---

## 边界条件测试

| 场景 | 输入 | 预期行为 |
|------|------|----------|
| 空公钥 | nil | 返回错误 |
| 无效长度公钥 | 31 字节 | 返回错误 |
| 空消息签名 | 空字节 | 成功签名 |
| 大消息签名 | 10MB | 成功签名 |

---

## 并发测试

| 场景 | 并发数 | 验证点 |
|------|--------|--------|
| 并发密钥生成 | 100 | 无竞态，各密钥唯一 |
| 并发签名 | 100 | 无竞态，签名正确 |

---

## 代码覆盖目标

| 文件 | 目标覆盖率 |
|------|-----------|
| `key.go` | ≥ 85% |
| `nodeid.go` | ≥ 90% |
| `sign.go` | ≥ 85% |

---

**最后更新**：2026-01-11
