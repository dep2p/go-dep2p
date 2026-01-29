# Realm 测试用例

> Realm 模块的测试用例集

---

## 概述

本目录包含 Realm 模块 (`internal/realm/`) 的测试用例，覆盖 Realm 创建、PSK 认证、成员管理、路由、网关等核心功能。

---

## 代码位置

```
internal/realm/
├── realm.go              # Realm 核心实现
├── realm_test.go         # Realm 单元测试
├── manager.go            # Manager 实现
├── manager_test.go       # Manager 单元测试
├── integration_test.go   # Realm 层集成测试
├── auth/                 # 认证子模块
│   ├── auth.go
│   └── auth_test.go
├── member/               # 成员管理子模块
│   ├── member.go
│   ├── member_test.go
│   └── integration_test.go
├── routing/              # 路由子模块
│   ├── router.go
│   ├── routing_test.go
│   └── integration_test.go
└── gateway/              # 网关子模块
    ├── gateway.go
    ├── gateway_test.go
    └── integration_test.go
```

---

## 用例清单

### Realm 核心

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-REALM-0001 | Realm 创建 | 单元 | P0 | ✅ |
| TST-REALM-0002 | RealmID 生成 | 单元 | P0 | ✅ |
| TST-REALM-0003 | Manager 生命周期 | 单元 | P0 | ✅ |
| TST-REALM-0004 | Join/Leave 流程 | 集成 | P0 | ✅ |
| TST-REALM-0005 | 服务门面访问 | 单元 | P0 | ✅ |
| TST-REALM-0006 | 统计信息 | 单元 | P2 | ✅ |

### Auth 子模块

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-REALM-AUTH-0001 | PSK 生成 | 单元 | P0 | ✅ |
| TST-REALM-AUTH-0002 | PSK 认证成功 | 集成 | P0 | ✅ |
| TST-REALM-AUTH-0003 | PSK 认证失败 | 集成 | P0 | ✅ |
| TST-REALM-AUTH-0004 | 认证管理器 | 单元 | P1 | ✅ |
| TST-REALM-AUTH-0005 | Challenge/Response | 单元 | P1 | ✅ |

### Member 子模块

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-REALM-MEMBER-0001 | 成员加入 | 集成 | P0 | ✅ |
| TST-REALM-MEMBER-0002 | 成员离开 | 集成 | P0 | ✅ |
| TST-REALM-MEMBER-0003 | 成员列表 | 单元 | P0 | ✅ |
| TST-REALM-MEMBER-0004 | 成员发现 | 集成 | P1 | ✅ |
| TST-REALM-MEMBER-0005 | 心跳检测 | 单元 | P1 | ✅ |
| TST-REALM-MEMBER-0006 | 成员缓存 | 单元 | P2 | ✅ |
| TST-REALM-MEMBER-0007 | 成员同步 | 集成 | P2 | ✅ |

### Routing 子模块

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-REALM-ROUTING-0001 | 路由表初始化 | 单元 | P0 | ✅ |
| TST-REALM-ROUTING-0002 | 路由查找 | 单元 | P0 | ✅ |
| TST-REALM-ROUTING-0003 | 路由更新 | 单元 | P0 | ✅ |
| TST-REALM-ROUTING-0004 | 负载均衡 | 单元 | P1 | ✅ |
| TST-REALM-ROUTING-0005 | 延迟探测 | 单元 | P2 | ✅ |

### Gateway 子模块

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-REALM-GATEWAY-0001 | 网关初始化 | 单元 | P0 | ✅ |
| TST-REALM-GATEWAY-0002 | 中继请求 | 集成 | P0 | ✅ |
| TST-REALM-GATEWAY-0003 | 带宽限制 | 单元 | P1 | ✅ |
| TST-REALM-GATEWAY-0004 | 连接池 | 单元 | P1 | ✅ |
| TST-REALM-GATEWAY-0005 | 指标统计 | 单元 | P2 | ✅ |

---

## 用例详情

### TST-REALM-0001: Realm 创建

| 字段 | 值 |
|------|-----|
| **ID** | TST-REALM-0001 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **关联需求** | REQ-REALM-0001 |
| **代码位置** | `internal/realm/realm_test.go` |

**测试目标**：验证 Realm 实例创建功能

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 创建 realmImpl 实例 | 实例非空 |
| 2 | 验证 ID | 非空字符串 |
| 3 | 验证 Name | 与设置一致 |
| 4 | 验证 PSK | 非空字节数组 |

**测试代码**：

```go
// internal/realm/realm_test.go
func TestRealm_ID(t *testing.T) {
    realm := &realmImpl{
        id: "test-realm",
    }
    assert.Equal(t, "test-realm", realm.ID())
}
```

---

### TST-REALM-AUTH-0002: PSK 认证成功

| 字段 | 值 |
|------|-----|
| **ID** | TST-REALM-AUTH-0002 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **关联需求** | REQ-REALM-0002 |
| **代码位置** | `internal/realm/auth/auth_test.go` |

**测试目标**：验证持有正确 PSK 的节点可以通过认证

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 创建 Realm 并获取 PSK | 成功 |
| 2 | 使用正确 PSK 进行认证 | 返回 true |
| 3 | 生成认证证明 | 非空 proof |

---

### TST-REALM-MEMBER-0001: 成员加入

| 字段 | 值 |
|------|-----|
| **ID** | TST-REALM-MEMBER-0001 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **关联需求** | REQ-REALM-0003 |
| **代码位置** | `internal/realm/member/integration_test.go` |

**测试目标**：验证成员加入 Realm 流程

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | Manager.Join() | 返回 Realm 实例 |
| 2 | 验证成员列表 | 包含该成员 |
| 3 | 验证 IsMember() | 返回 true |

---

## 安全测试场景

| 场景 | 测试内容 | 预期行为 |
|------|----------|----------|
| PSK 暴力破解 | 多次错误尝试 | 速率限制 |
| 重放攻击 | 重放认证消息 | 拒绝重复 nonce |
| 中间人攻击 | 篡改认证消息 | 签名验证失败 |
| 权限提升 | 非成员访问服务 | 拒绝访问 |

---

## 性能测试场景

| 场景 | 参数 | 目标指标 |
|------|------|----------|
| PSK 验证延迟 | 单次 | ≤ 5ms |
| 成员加入延迟 | 单节点 | ≤ 100ms |
| 成员发现延迟 | 100 成员 | ≤ 500ms |
| 消息广播延迟 | 100 成员 | P99 ≤ 200ms |

---

## 代码覆盖目标

| 文件 | 目标覆盖率 | 当前状态 |
|------|-----------|----------|
| `realm.go` | ≥ 85% | ✅ |
| `manager.go` | ≥ 80% | ✅ |
| `auth/auth.go` | ≥ 90% | ✅ |
| `member/member.go` | ≥ 80% | ✅ |
| `routing/router.go` | ≥ 80% | ✅ |
| `gateway/gateway.go` | ≥ 80% | ✅ |

---

**最后更新**：2026-01-15
