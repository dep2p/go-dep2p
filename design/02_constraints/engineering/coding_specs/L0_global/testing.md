# 测试规范

> 定义 DeP2P 项目的测试编写标准

---

## 核心原则

```
┌─────────────────────────────────────────┐
│              测试原则                    │
├─────────────────────────────────────────┤
│                                         │
│  1. 测试是代码的一部分                   │
│  2. 测试应该可重复                       │
│  3. 测试应该独立                         │
│  4. 测试应该快速                         │
│                                         │
└─────────────────────────────────────────┘
```

---

## 测试分类

### 分类定义

| 类型 | 范围 | 依赖 | 速度 |
|------|------|------|------|
| **单元测试** | 函数/方法 | 无外部 | 毫秒 |
| **集成测试** | 模块交互 | 本地服务 | 秒 |
| **端到端测试** | 完整流程 | 网络 | 分钟 |

### 测试金字塔

```
                    ┌───────┐
                    │  E2E  │        少量
                   ┌┴───────┴┐
                   │ 集成测试 │       适量
                  ┌┴─────────┴┐
                  │  单元测试  │      大量
                  └───────────┘
```

---

## 覆盖率要求

### 覆盖率目标

| 代码类型 | 目标覆盖率 | 说明 |
|----------|-----------|------|
| 核心逻辑 | ≥ 80% | 关键路径 |
| 工具函数 | ≥ 70% | 辅助功能 |
| 错误处理 | ≥ 60% | 异常分支 |

### 覆盖重点

```
覆盖优先级：

  高优先级：
    • 公开 API
    • 核心算法
    • 错误处理路径
    
  低优先级：
    • 简单 getter/setter
    • 日志语句
    • panic 分支
```

---

## 测试命名

### 命名规则

```
测试函数命名：

  格式：Test[被测对象]_[场景]_[预期结果]
  
  示例：
    TestConnect_ValidPeer_Success
    TestConnect_InvalidAddr_ReturnsError
    TestJoinRealm_AlreadyJoined_ReturnsErrAlreadyJoined
```

### 子测试命名

```
子测试命名：

  格式：场景描述
  
  示例：
    "valid peer address"
    "nil context"
    "timeout exceeded"
```

---

## 表驱动测试

### 结构模式

```
表驱动测试伪代码：

  tests := []struct{
    name     string
    input    InputType
    expected OutputType
    wantErr  bool
  }{
    {"valid input", validInput, expectedOutput, false},
    {"invalid input", invalidInput, nil, true},
  }
  
  FOR EACH test IN tests
    result, err := function(test.input)
    
    IF test.wantErr THEN
      ASSERT err != nil
    ELSE
      ASSERT result == test.expected
    END
  END
```

### 适用场景

| 场景 | 是否适用 | 原因 |
|------|----------|------|
| 多输入输出 | 适用 | 减少重复 |
| 复杂设置 | 不适用 | 每个用例差异大 |
| 边界测试 | 适用 | 覆盖边界值 |

---

## Mock 策略

### Mock 原则

```
┌─────────────────────────────────────────┐
│              Mock 原则                   │
├─────────────────────────────────────────┤
│                                         │
│  1. Mock 接口而非实现                    │
│  2. Mock 外部依赖                        │
│  3. 不 Mock 被测对象                     │
│  4. Mock 行为要明确                      │
│                                         │
└─────────────────────────────────────────┘
```

### P2P 特有 Mock

| 组件 | Mock 方式 | 用途 |
|------|----------|------|
| 网络连接 | 内存连接 | 测试协议逻辑 |
| 发现服务 | 固定返回 | 测试连接逻辑 |
| Relay | 模拟中继 | 测试中继路径 |

---

## 测试隔离

### 隔离级别

| 级别 | 网络 | 文件系统 | 适用 |
|------|------|----------|------|
| **L0** | 禁止 | 临时目录 | 单元测试 |
| **L1** | 本地 | 临时目录 | 集成测试 |
| **L2** | 受控 | 隔离目录 | E2E 测试 |

### 资源清理

```
资源清理伪代码：

  SETUP
    resource = create_test_resource()
  END
  
  CLEANUP (defer)
    resource.close()
    cleanup_temp_files()
  END
  
  TEST
    use(resource)
  END
```

---

## P2P 测试模式

### 节点测试

```
P2P 节点测试模式：

  SETUP
    nodeA = create_test_node()
    nodeB = create_test_node()
  END
  
  TEST "connection"
    nodeA.connect(nodeB.id)
    ASSERT nodeA.is_connected(nodeB.id)
  END
  
  CLEANUP
    nodeA.close()
    nodeB.close()
  END
```

### Realm 测试

```
Realm 测试模式：

  SETUP
    node = create_test_node()
    realm = create_test_realm()
  END
  
  TEST "join and leave"
    node.join(realm)
    ASSERT node.current_realm() == realm
    
    node.leave()
    ASSERT node.current_realm() == nil
  END
```

---

## 测试辅助

### 辅助函数

| 函数 | 用途 |
|------|------|
| createTestNode | 创建测试节点 |
| createTestRealm | 创建测试 Realm |
| waitForCondition | 等待异步条件 |
| assertEventually | 断言最终状态 |

### 超时处理

```
异步测试超时：

  TEST "async operation"
    result = await_with_timeout(
      operation(),
      timeout = 5s
    )
    
    ASSERT result == expected
  END
```

---

## 验证清单

| 检查项 | 说明 |
|--------|------|
| 测试通过 | 所有测试绿色 |
| 覆盖率达标 | 核心逻辑 ≥ 80% |
| 无竞态 | race detector 无告警 |
| 资源清理 | 无泄漏 |

---

## 相关文档

- [测试隔离](../../isolation/testing_isolation.md)
- [代码规范](../../standards/code_standards.md)

---

**最后更新**：2026-01-11
