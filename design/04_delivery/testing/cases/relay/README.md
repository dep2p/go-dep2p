# 中继测试用例 (Relay)

> 中继模块的测试用例集

---

## 概述

本目录包含中继模块 (`internal/core/relay/`) 的测试用例，覆盖 Relay 的注册、转发、故障恢复等功能。

---

## 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-RELAY-0001 | Relay 注册 | 集成 | P0 | ✅ |
| TST-RELAY-0002 | Relay 发现 | 集成 | P0 | ✅ |
| TST-RELAY-0003 | Relay 注册（Realm 场景） | 集成 | P0 | ✅ |
| TST-RELAY-0004 | Relay 发现（Realm 场景） | 集成 | P0 | ✅ |
| TST-RELAY-0005 | 中继数据转发 | 集成 | P0 | ✅ |
| TST-RELAY-0006 | 中继连接建立 | 集成 | P0 | ✅ |
| TST-RELAY-0007 | 中继断开重连 | 集成 | P1 | ✅ |
| TST-RELAY-0008 | 中继选择策略 | 单元 | P1 | ✅ |
| TST-RELAY-0009 | 带宽限制 | 集成 | P2 | ✅ |
| TST-RELAY-0010 | 多中继备份 | 集成 | P1 | ✅ |
| TST-RELAY-0011 | 中继健康检查 | 集成 | P1 | ✅ |
| TST-RELAY-0012 | 中继负载均衡 | 集成 | P2 | ✅ |

---

## 用例详情

### TST-RELAY-0001: Relay 注册

| 字段 | 值 |
|------|-----|
| **ID** | TST-RELAY-0001 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **关联需求** | REQ-RELAY-0001 |
| **代码位置** | `tests/integration/relay_test.go` |

**测试目标**：验证节点向 Relay 注册

**前置条件**：
- Relay 服务运行中
- 节点已初始化身份

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 创建节点 | 成功 |
| 2 | 连接到 Relay | 连接建立成功 |
| 3 | 发送注册请求 | 返回成功响应 |
| 4 | 验证注册状态 | 节点状态为已注册 |

**测试代码**：

```go
func TestRelay_Register(t *testing.T) {
    // 启动测试 Relay
    relay := startTestRelay(t)
    defer relay.Close()
    
    // 创建节点
    node := createTestNode(t)
    defer node.Close()
    
    // 注册到 Relay
    err := node.RegisterRelay(relay.Addr())
    require.NoError(t, err)
    
    // 验证注册状态
    assert.True(t, node.IsRelayRegistered())
}
```

---

### TST-RELAY-0005: 中继数据转发

| 字段 | 值 |
|------|-----|
| **ID** | TST-RELAY-0005 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **关联需求** | REQ-RELAY-0003 |
| **代码位置** | `tests/integration/relay_test.go` |

**测试目标**：验证通过 Relay 转发数据

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 启动 Relay | 成功 |
| 2 | 节点 A 和 B 注册到 Relay | 成功 |
| 3 | A 通过 Relay 向 B 发送数据 | 成功 |
| 4 | B 接收数据 | 数据与发送一致 |
| 5 | B 通过 Relay 回复 A | 成功 |
| 6 | A 接收回复 | 数据与发送一致 |

**测试代码**：

```go
func TestRelay_DataForward(t *testing.T) {
    relay := startTestRelay(t)
    defer relay.Close()
    
    nodeA := createTestNode(t)
    nodeB := createTestNode(t)
    defer nodeA.Close()
    defer nodeB.Close()
    
    // 双方注册到 Relay
    nodeA.RegisterRelay(relay.Addr())
    nodeB.RegisterRelay(relay.Addr())
    
    // A 通过 Relay 连接 B
    stream, err := nodeA.OpenRelayStream(nodeB.ID())
    require.NoError(t, err)
    
    // 发送数据
    testData := []byte("hello via relay")
    _, err = stream.Write(testData)
    require.NoError(t, err)
    
    // B 接收数据
    received := make([]byte, len(testData))
    _, err = stream.Read(received)
    require.NoError(t, err)
    assert.Equal(t, testData, received)
}
```

---

### TST-RELAY-0007: 中继断开重连

| 字段 | 值 |
|------|-----|
| **ID** | TST-RELAY-0007 |
| **类型** | 集成测试 |
| **优先级** | P1 |
| **关联需求** | REQ-RELAY-0004 |
| **代码位置** | `tests/integration/relay_test.go` |

**测试目标**：验证 Relay 断开后的自动重连

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 节点注册到 Relay | 成功 |
| 2 | 断开 Relay 连接 | 连接断开 |
| 3 | 等待重连超时 | 触发重连 |
| 4 | Relay 恢复可用 | 自动重连成功 |
| 5 | 验证注册状态 | 恢复注册 |

---

## Relay 架构测试

### Relay 场景对比

| 场景 | Relay（基础设施） | Relay（业务方） |
|------|:------------:|:-----------:|
| 控制面通信 | ✅ | - |
| 数据面通信 | - | ✅ |
| 全局发现 | ✅ | - |
| Realm 内通信 | - | ✅ |

### 测试矩阵

| 场景 | 单元 | 集成 | E2E |
|------|:----:|:----:|:---:|
| Relay 注册 | - | ✅ | ✅ |
| Relay 注册（Realm 场景） | - | ✅ | ✅ |
| 数据转发 | - | ✅ | ✅ |
| 故障恢复 | - | ✅ | ✅ |
| 负载均衡 | ✅ | ✅ | - |

---

## 故障场景测试

| 故障类型 | 测试内容 | 预期恢复时间 |
|----------|----------|--------------|
| Relay 宕机 | 切换到备用 Relay | ≤ 30s |
| 网络分区 | 使用本地缓存路由 | - |
| 高延迟 | 降级处理 | - |
| 过载 | 拒绝新连接 | - |

---

## 性能测试场景

| 场景 | 参数 | 目标指标 |
|------|------|----------|
| 中继延迟 | 单次转发 | P99 ≤ 20ms |
| 吞吐量 | 大数据转发 | ≥ 50MB/s |
| 并发转发 | 1000 流 | 成功率 ≥ 99% |
| 长时间运行 | 24 小时 | 无内存泄漏 |

---

## 代码覆盖目标

| 文件 | 目标覆盖率 |
|------|-----------|
| `relay.go` | ≥ 85% |
| `system_relay.go` | ≥ 85% |
| `realm_relay.go` | ≥ 85% |
| `selector.go` | ≥ 80% |

---

**最后更新**：2026-01-11
