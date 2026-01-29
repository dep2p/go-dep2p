# 测试隔离策略

> 定义 DeP2P 测试的隔离级别和实施方式

---

## 隔离级别

### 级别定义

```
┌─────────────────────────────────────────────────────────────┐
│                    测试隔离级别                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  L0 完全隔离（Full）                                        │
│  ──────────────────                                         │
│  • 无外部依赖                                               │
│  • 无网络访问                                               │
│  • 使用内存/临时文件                                        │
│  • 适用：单元测试                                           │
│                                                             │
│  L1 网络隔离（Network）                                     │
│  ──────────────────────                                     │
│  • 禁止外网访问                                             │
│  • 允许本地网络（localhost）                                │
│  • 使用临时目录                                             │
│  • 适用：集成测试                                           │
│                                                             │
│  L2 部分隔离（Partial）                                     │
│  ──────────────────────                                     │
│  • 受控的外部访问                                           │
│  • 隔离的测试环境                                           │
│  • 适用：E2E 测试                                           │
│                                                             │
│  L3 无隔离（None）                                          │
│  ─────────────────                                          │
│  • 真实环境                                                 │
│  • 适用：压力测试、混沌测试                                 │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 网络隔离

### L0/L1 网络禁止

```
网络隔离实施：

  测试运行时：
    • 不发起外网请求
    • 使用 Mock 替代外部服务
    • 使用内存网络替代真实网络
```

### P2P 网络 Mock

```
P2P Mock 方式：

  内存传输：
    • 使用内存通道模拟网络
    • 无真实 UDP/TCP 连接
    • 可模拟延迟和丢包
    
  本地多节点：
    • 多个节点在同一进程
    • 使用 localhost 通信
    • 无外网访问
```

### 测试构建标签

| 标签 | 隔离级别 | 使用场景 |
|------|----------|----------|
| (无) | L0 | 单元测试 |
| integration | L1 | 集成测试 |
| e2e | L2 | 端到端测试 |
| stress | L3 | 压力测试 |

---

## Mock 策略

### 需要 Mock 的组件

| 组件 | Mock 方式 | 隔离级别 |
|------|----------|----------|
| 网络连接 | 内存连接 | L0, L1 |
| 发现服务 | 固定返回 | L0, L1 |
| Relay | 模拟中继 | L0, L1 |
| 时间 | 可控时钟 | L0 |
| 随机数 | 固定种子 | L0 |

### Mock 接口设计

```
Mock 设计原则：

  1. 依赖接口而非实现
  2. Mock 实现接口
  3. 通过依赖注入使用
  
示例接口：
  
  Transport 接口
    Dial(addr) -> Connection
    Listen(addr) -> Listener
    
  MockTransport 实现
    使用内存通道
    可注入错误
    可控制延迟
```

---

## Realm 测试隔离

### 隔离规则

```
┌─────────────────────────────────────────────────────────────┐
│                  Realm 测试隔离规则                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  规则 1：每个测试用例使用独立的 Realm                        │
│  规则 2：Realm ID 包含测试名称，避免冲突                     │
│  规则 3：测试结束后清理 Realm 状态                           │
│  规则 4：不同测试的 Realm 完全隔离                           │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Realm 测试辅助

```
Realm 测试辅助伪代码：

  FUNCTION create_test_realm(test_name)
    realm_id = format("test-{}-{}", test_name, random_suffix())
    psk = generate_test_psk()
    
    RETURN TestRealm{
      id: realm_id,
      psk: psk,
      cleanup: FUNCTION() { cleanup_realm(realm_id) }
    }
  END
  
  FUNCTION with_test_realm(test, realm_count)
    realms = []
    
    FOR i = 1 TO realm_count
      realm = create_test_realm(test.name)
      realms.append(realm)
    END
    
    DEFER
      FOR EACH realm IN realms
        realm.cleanup()
      END
    END
    
    test.run(realms)
  END
```

---

## 节点测试隔离

### 测试节点创建

```
测试节点创建伪代码：

  FUNCTION create_test_node(options)
    // 使用测试专用配置
    config = TestNodeConfig{
      transport: InMemoryTransport,
      discovery: MockDiscovery,
      relay: MockRelay,
      storage: TempStorage,
    }
    
    // 覆盖自定义选项
    apply_options(config, options)
    
    node = create_node(config)
    
    RETURN TestNode{
      node: node,
      cleanup: FUNCTION() {
        node.close()
        cleanup_temp_storage(config.storage)
      }
    }
  END
```

### 多节点测试

```
多节点测试伪代码：

  FUNCTION with_test_nodes(test, count)
    nodes = []
    network = create_isolated_network()
    
    FOR i = 1 TO count
      node = create_test_node(network = network)
      nodes.append(node)
    END
    
    // 互相发现
    FOR EACH node IN nodes
      FOR EACH other IN nodes
        IF node != other THEN
          node.address_book.add(other.id, other.addr)
        END
      END
    END
    
    DEFER cleanup_all(nodes, network)
    
    test.run(nodes)
  END
```

---

## 确定性测试

### 时间控制

```
时间控制伪代码：

  // 使用可控时钟
  clock = MockClock{current: fixed_time}
  
  // 注入到被测组件
  component = create_component(clock = clock)
  
  // 测试中推进时间
  clock.advance(duration)
  
  // 验证超时行为
  ASSERT component.state == TIMEOUT
```

### 随机数控制

```
随机数控制：

  // 使用固定种子
  rand = create_random(seed = 12345)
  
  // 注入到被测组件
  component = create_component(random = rand)
  
  // 结果可重复
```

---

## 资源清理

### 清理检查清单

| 资源 | 清理方式 | 时机 |
|------|----------|------|
| 测试节点 | close() | defer |
| 测试 Realm | 删除状态 | defer |
| 临时文件 | 删除目录 | defer |
| 定时器 | stop() | defer |
| Goroutine | 等待退出 | defer |

### 泄漏检测

```
资源泄漏检测：

  1. Goroutine 泄漏检测
  2. 文件描述符检测
  3. 内存增长检测
```

---

## 验证清单

| 检查项 | 说明 |
|--------|------|
| 无外网请求 | L0/L1 测试无外网 |
| 使用 Mock | 外部依赖已 Mock |
| 资源清理 | 测试后无残留 |
| 确定性 | 结果可重复 |

---

## 相关文档

- [测试规范](../coding_specs/L0_global/testing.md)
- [网络边界约束](network_boundary.md)

---

**最后更新**：2026-01-11
