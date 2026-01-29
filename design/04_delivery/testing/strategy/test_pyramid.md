# 测试金字塔

> 测试分层模型和比例分配策略

---

## 元信息

| 字段 | 值 |
|------|-----|
| **状态** | approved |
| **Owner** | DeP2P Team |
| **创建日期** | 2026-01-11 |
| **更新日期** | 2026-01-20 |

---

## 1. 测试金字塔模型（四层架构）

P2P 系统的特殊性决定了传统三层测试金字塔无法覆盖所有场景。DeP2P 采用**四层测试架构**：

```
                        ┌───────────────┐
                        │ 真实网络验证   │  ← Level 4: 手动 + AI 分析
                       ─┼───────────────┼─     NAT/Relay/跨网络
                      ┌─┴───────────────┴─┐
                      │    E2E 测试        │  ← Level 3: 5-10%
                     ─┼───────────────────┼─     完整用户场景
                    ┌─┴───────────────────┴─┐
                    │      集成测试          │  ← Level 2: 15-25%
                   ─┼───────────────────────┼─     跨模块协作
                  ┌─┴───────────────────────┴─┐
                  │        单元测试            │  ← Level 1: 70-80%
                  └───────────────────────────┘     函数/方法级别
```

### 1.1 四层定义

| 层级 | 名称 | 说明 | 运行环境 |
|------|------|------|----------|
| **Level 4** | 真实网络验证 | NAT 穿透、中继通信、跨网络连接 | 多机/真实网络 |
| **Level 3** | E2E 测试 | 完整用户场景，多节点协作 | 单机/本地网络 |
| **Level 2** | 集成测试 | 跨模块组件协作 | 单机/本地 |
| **Level 1** | 单元测试 | 单个函数/方法 | 单机/隔离 |

### 1.2 比例分配

| 层级 | 比例 | 用例数量目标 | 执行时间目标 | 运行频率 |
|------|------|--------------|--------------|----------|
| 真实网络验证 | - | 关键场景 | 手动 | 发布前/问题排查 |
| E2E 测试 | 5-10% | 关键路径 | < 10 分钟 | 每日/发布前 |
| 集成测试 | 15-25% | 模块交互 | < 5 分钟 | 每次 PR |
| 单元测试 | 70-80% | 大量边界 | < 1 分钟 | 每次提交 |

### 1.3 为什么需要第四层？

传统测试金字塔假设本地环境可以模拟所有场景，但 P2P 系统存在以下无法本地模拟的场景：

| 场景 | 本地测试 | 真实网络验证 | 原因 |
|------|:--------:|:------------:|------|
| NAT 穿透 | ❌ | ✅ | 需要真实 NAT 设备 |
| 中继通信 | ⚠️ 部分 | ✅ | 需要跨网络节点 |
| 4G/WiFi 切换 | ❌ | ✅ | 需要真实网络切换 |
| 公网地址发现 | ❌ | ✅ | 需要真实 STUN 查询 |
| DHT 大规模路由 | ⚠️ 有限 | ✅ | 本地节点数有限 |

**详细说明**：参见 [network_validation.md](network_validation.md)

---

## 2. Level 1: 单元测试

### 2.1 定义

- **范围**：单个函数或方法
- **依赖**：完全隔离，使用 Mock
- **速度**：毫秒级
- **位置**：与源码同目录 (`*_test.go`)

### 2.2 特点

| 特点 | 说明 |
|------|------|
| 快速 | 单个测试 < 100ms |
| 隔离 | 不依赖外部资源 |
| 确定 | 结果可重复 |
| 细粒度 | 针对具体逻辑 |

### 2.3 覆盖范围

```
internal/
├── core/                        # 核心层
│   ├── identity/
│   │   ├── identity.go
│   │   └── identity_test.go    ← 单元测试
│   ├── transport/
│   │   ├── transport.go
│   │   └── transport_test.go   ← 单元测试
│   ├── security/
│   ├── relay/
│   └── ...
├── realm/                       # Realm 层
│   ├── realm.go
│   ├── realm_test.go           ← 单元测试
│   ├── auth/
│   │   └── auth_test.go        ← 认证单元测试
│   ├── member/
│   │   └── member_test.go      ← 成员单元测试
│   ├── routing/
│   │   └── routing_test.go     ← 路由单元测试
│   └── gateway/
│       └── gateway_test.go     ← 网关单元测试
├── protocol/                    # Protocol 层
│   ├── messaging/
│   │   └── service_test.go     ← 消息单元测试
│   ├── pubsub/
│   │   └── topic_test.go       ← 发布订阅单元测试
│   ├── streams/
│   │   └── stream_test.go      ← 流单元测试
│   └── liveness/
│       └── ping_test.go        ← 活性单元测试
├── discovery/                   # Discovery 层
│   ├── dht/
│   ├── mdns/
│   ├── bootstrap/
│   └── ...
└── util/
    └── ...
```

### 2.4 示例

```go
// internal/core/identity/nodeid_test.go
func TestNodeID_FromPublicKey(t *testing.T) {
    tests := []struct {
        name    string
        pubKey  []byte
        wantLen int
    }{
        {"valid key", validPubKey, 32},
        {"empty key", nil, 0},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            id, err := NodeIDFromPublicKey(tt.pubKey)
            // 断言
        })
    }
}
```

---

## 3. Level 2: 集成测试

### 3.1 定义

- **范围**：多个模块协作
- **依赖**：真实的内部依赖，Mock 外部依赖
- **速度**：秒级
- **位置**：`tests/integration/`（需要 `-tags=integration`）

### 3.2 特点

| 特点 | 说明 |
|------|------|
| 模块交互 | 验证模块间协作 |
| 部分真实 | 使用真实内部组件 |
| 有限范围 | 不涉及完整系统 |

### 3.3 覆盖范围

```
tests/
├── integration/
│   ├── identity_test.go        ← 身份模块集成
│   ├── transport_test.go       ← 传输模块集成
│   ├── relay_test.go           ← 中继模块集成
│   └── realm_test.go           ← Realm 模块集成
└── ...
```

### 3.4 示例

```go
// tests/integration/transport_test.go
//go:build integration

func TestTransport_Connection(t *testing.T) {
    // 创建真实的 Identity
    identity := identity.NewIdentity()
    
    // 创建真实的 Transport
    transport := transport.NewTCPTransport(identity)
    
    // 测试连接建立
    listener, _ := transport.Listen("127.0.0.1:0")
    defer listener.Close()
    
    conn, err := transport.Dial(listener.Addr())
    require.NoError(t, err)
    defer conn.Close()
    
    // 验证连接
    assert.NotNil(t, conn)
}
```

### 3.5 运行方式

```bash
# 运行集成测试
go test -tags=integration ./tests/integration/...
```

---

## 4. Level 3: E2E 测试

### 4.1 定义

- **范围**：完整系统流程
- **依赖**：真实的多节点环境（本地网络）
- **速度**：分钟级
- **位置**：`tests/e2e/`（需要 `-tags=e2e`）

### 4.2 特点

| 特点 | 说明 |
|------|------|
| 端到端 | 模拟真实用户场景 |
| 多节点 | 涉及多个节点交互 |
| 完整流程 | 覆盖完整业务流程 |

### 4.3 覆盖范围

```
tests/
├── e2e/
│   ├── connection_test.go      ← 连接建立流程
│   ├── discovery_test.go       ← 节点发现流程
│   ├── relay_test.go           ← 中继流程
│   ├── realm_test.go           ← Realm 流程
│   └── messaging_test.go       ← 消息传递流程
└── ...
```

### 4.4 示例

```go
// tests/e2e/connection_test.go
//go:build e2e

func TestE2E_DirectConnection(t *testing.T) {
    // 启动节点 A
    nodeA, _ := dep2p.NewNode(dep2p.DefaultConfig())
    defer nodeA.Close()
    
    // 启动节点 B
    nodeB, _ := dep2p.NewNode(dep2p.DefaultConfig())
    defer nodeB.Close()
    
    // 节点 B 连接到节点 A
    err := nodeB.Connect(context.Background(), nodeA.Addrs()[0])
    require.NoError(t, err)
    
    // 验证连接建立
    assert.True(t, nodeB.IsConnected(nodeA.ID()))
}
```

### 4.5 运行方式

```bash
# 运行 E2E 测试
go test -tags=e2e -timeout=10m ./tests/e2e/...
```

---

## 5. Level 4: 真实网络验证

### 5.1 定义

- **范围**：真实网络环境下的 P2P 通信
- **依赖**：多机部署、不同网络环境（WiFi/4G/公网）
- **速度**：手动执行，分钟到小时级
- **分析方式**：结构化日志 + AI 驱动分析

### 5.2 特点

| 特点 | 说明 |
|------|------|
| 多机部署 | 需要云服务器 + 本地设备 |
| 真实 NAT | 测试穿透能力 |
| 跨网络 | WiFi/4G/公网混合 |
| 日志驱动 | 通过日志分析验证 |
| AI 辅助 | 使用 AI 分析复杂日志 |

### 5.3 验证场景

| 场景 ID | 场景名称 | 节点配置 | 验证重点 |
|---------|----------|----------|----------|
| S01 | 基础三节点通信 | WiFi + 4G + 云 | 连接、消息投递 |
| S02 | NAT 穿透验证 | 严格NAT + 对称NAT | 打洞、中继回退 |
| S03 | DHT 路由验证 | 3+ 节点 | 路由表、节点发现 |
| S04 | 中继通信验证 | NAT后节点 + 中继 | 预留、电路、延迟 |
| S05 | 成员同步验证 | 动态加入/离开 | 同步协议、去重 |
| S06 | 网络切换恢复 | WiFi↔4G 切换 | 检测、重连、恢复 |

### 5.4 验证流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          真实网络验证流程                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 部署节点                                                                │
│     ├── 云服务器 (公网可达，--serve 模式)                                    │
│     ├── WiFi 节点 (NAT 后)                                                  │
│     └── 4G 节点 (NAT 后)                                                    │
│                         ↓                                                   │
│  2. 执行验证场景                                                            │
│     ├── 启动节点，正常使用 Chat 示例                                         │
│     ├── 执行测试操作（群聊、私聊、连接等）                                    │
│     └── 收集日志文件                                                        │
│                         ↓                                                   │
│  3. AI 分析日志                                                             │
│     ├── 提交日志 + 场景信息                                                 │
│     ├── AI 分析检查点是否通过                                               │
│     └── 生成验证报告                                                        │
│                         ↓                                                   │
│  4. 问题修复与回归                                                          │
│     ├── 根据报告修复代码                                                    │
│     ├── 本地测试通过                                                        │
│     └── 重新执行真实网络验证                                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.5 相关文档

- **详细设计**：`design/_discussions/20260120-distributed-validation-framework.md`
- **验证场景**：[network_validation.md](network_validation.md)
- **BUG 跟踪**：`design/_discussions/20260119-bug-tracking-chat-example.md`

---

## 6. 测试场景分配

### 6.1 按模块分配

**Core 层**

| 模块 | 单元测试 | 集成测试 | E2E 测试 |
|------|----------|----------|----------|
| Identity | 20+ | 5+ | 2+ |
| Transport | 30+ | 10+ | 3+ |
| Security | 25+ | 5+ | 2+ |
| Relay | 20+ | 10+ | 5+ |
| ConnMgr | 15+ | 5+ | - |

**Realm 层**

| 模块 | 单元测试 | 集成测试 | E2E 测试 |
|------|----------|----------|----------|
| Realm Core | 15+ | 5+ | 3+ |
| Auth | 10+ | 5+ | 2+ |
| Member | 10+ | 5+ | 2+ |
| Routing | 10+ | 5+ | - |
| Gateway | 10+ | 5+ | 2+ |

**Protocol 层**

| 模块 | 单元测试 | 集成测试 | E2E 测试 |
|------|----------|----------|----------|
| Messaging | 15+ | 5+ | 3+ |
| PubSub | 20+ | 5+ | 3+ |
| Streams | 10+ | 5+ | 2+ |
| Liveness | 10+ | 3+ | - |

**Discovery 层**

| 模块 | 单元测试 | 集成测试 | E2E 测试 |
|------|----------|----------|----------|
| DHT | 15+ | 5+ | 3+ |
| mDNS | 10+ | 5+ | 2+ |
| Bootstrap | 10+ | 3+ | 2+ |
| DNS | 5+ | 3+ | - |
| Rendezvous | 10+ | 5+ | 2+ |

### 6.2 按场景分配

| 场景类型 | 单元测试 | 集成测试 | E2E 测试 | 真实网络验证 |
|----------|----------|----------|----------|--------------|
| 正常路径 | ✓ | ✓ | ✓ | - |
| 边界条件 | ✓ | - | - | - |
| 错误处理 | ✓ | ✓ | - | - |
| 并发场景 | ✓ | ✓ | - | - |
| 性能场景 | Benchmark | - | - | - |
| 故障恢复 | - | ✓ | ✓ | ✓ |
| NAT 穿透 | - | - | - | ✓ |
| 中继通信 | - | ⚠️ 模拟 | ⚠️ 模拟 | ✓ |
| 跨网络连接 | - | - | - | ✓ |

---

## 7. 执行策略

### 7.1 开发阶段

```
开发者本地 → 单元测试（频繁执行）
                ↓
            保持快速反馈
```

### 7.2 PR 阶段

```
PR 提交 → 单元测试 + 集成测试
              ↓
          CI 自动执行
              ↓
          通过后可合并
```

### 7.3 发布阶段

```
发布前 → 单元测试 + 集成测试 + E2E 测试
              ↓
          完整回归测试
              ↓
          通过后可发布
```

### 7.4 重大版本/网络功能变更

```
真实网络验证 → 部署多节点
                  ↓
              执行验证场景
                  ↓
              日志 + AI 分析
                  ↓
              验证报告通过
                  ↓
              确认可发布
```

---

## 8. CI 配置示例

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  unit:
    name: Unit Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test -race -coverprofile=coverage.out ./...
      - uses: codecov/codecov-action@v4

  integration:
    name: Integration Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test -tags=integration -race -timeout=5m ./tests/integration/...

  e2e:
    name: E2E Tests
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go test -tags=e2e -timeout=10m ./tests/e2e/...
```

---

## 9. 反模式

### 9.1 避免的模式

| 反模式 | 问题 | 解决方案 |
|--------|------|----------|
| 冰淇淋锥 | E2E 过多，反馈慢 | 增加单元测试 |
| 测试重复 | 多层测试相同逻辑 | 明确每层职责 |
| Flaky 测试 | 测试不稳定 | 隔离依赖，固定数据 |
| 慢测试 | 单元测试太慢 | 使用 Mock，并行执行 |
| 忽略网络测试 | 本地测试通过但生产失败 | 执行真实网络验证 |

### 9.2 平衡考虑

- **不是所有代码都需要 E2E**：关键路径优先
- **不是所有测试都需要 Mock**：简单依赖可直接使用
- **覆盖率不是唯一指标**：测试质量同样重要
- **P2P 系统必须真实网络验证**：NAT/Relay 等场景无法本地模拟

---

## 10. 快速参考

### 10.1 测试命令速查

```bash
# Level 1: 单元测试
go test ./...                              # 所有包
go test -short ./...                       # 快速模式

# Level 2: 集成测试
go test -tags=integration ./tests/integration/... -timeout 5m

# Level 3: E2E 测试
go test -tags=e2e ./tests/e2e/... -timeout 10m

# Level 4: 真实网络验证
# 参见 network_validation.md
```

### 10.2 相关文档

| 文档 | 说明 |
|------|------|
| [test_strategy.md](test_strategy.md) | 测试策略（方法论、工具） |
| [test_matrix.md](test_matrix.md) | 测试覆盖矩阵 |
| [network_validation.md](network_validation.md) | 真实网络验证详细设计 |
| [quality_gates.md](quality_gates.md) | 质量门禁 |

---

## 变更历史

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|----------|
| v1.0 | 2026-01-11 | DeP2P Team | 初始版本 |
| v1.1 | 2026-01-15 | DeP2P Team | 更新代码路径，添加 Realm/Protocol/Discovery 层详情 |
| v2.0 | 2026-01-20 | DeP2P Team | **重大更新**：添加第四层"真实网络验证"，更新章节结构 |
