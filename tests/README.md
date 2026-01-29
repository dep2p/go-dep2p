# DEP2P 测试目录

本目录存放 DEP2P 的 **Level 2（集成测试）** 和 **Level 3（E2E 测试）** 代码。

DEP2P 采用**四层测试架构**：

```
Level 4: 真实网络验证  ← NAT/Relay/跨网络（手动 + AI 分析）
Level 3: E2E 测试      ← 完整用户场景（本目录 tests/e2e/）
Level 2: 集成测试      ← 跨模块协作（本目录 tests/integration/）
Level 1: 单元测试      ← 单函数/方法（与源码同目录）
```

**参考文档**：
- [测试策略文档](../design/04_delivery/testing/strategy/)
- [真实网络验证策略](../design/04_delivery/testing/strategy/network_validation.md)

## 目录结构

```
tests/
├── README.md                    # 本文档
├── testutil/                    # 测试工具库
│   ├── node.go                  # TestNodeBuilder - 节点构建器
│   ├── realm.go                 # TestRealmBuilder - Realm 构建器
│   ├── wait.go                  # 等待/断言工具
│   └── fixtures.go              # 测试数据固件
│
├── integration/                 # 集成测试 (单进程多节点)
│   ├── README.md                # 集成测试说明
│   ├── core/                    # 核心层集成
│   │   ├── connection_test.go   # 连接建立/断开
│   │   ├── realm_auth_test.go   # Realm 认证
│   │   ├── member_test.go       # 成员管理
│   │   └── eventbus_test.go     # 事件总线
│   │
│   └── protocol/                # 协议层集成
│       ├── pubsub_test.go       # PubSub 真实测试
│       ├── streams_test.go      # Streams 真实测试
│       └── liveness_test.go     # Liveness 测试
│
└── e2e/                         # 端到端测试 (完整场景)
    ├── README.md                # E2E 测试说明
    ├── scenario/                # 用户场景
    │   ├── chat_test.go         # 聊天场景 (对标 chat-local)
    │   └── discovery_test.go    # 发现场景
    │
    └── resilience/              # 韧性测试
        ├── partition_test.go    # 网络分区 (占位)
        └── recovery_test.go     # 故障恢复 (占位)
```

## 测试分层策略

DEP2P 采用**四层测试策略**：

### Level 1: 单元测试（Unit Tests）

**位置**：与源代码同目录（`*_test.go`）

**范围**：测试单个函数、方法或类型

**运行**：
```bash
go test ./...                    # 所有包的单元测试
go test ./internal/realm         # 特定包的单元测试
go test -short ./...             # 跳过耗时测试
```

---

### Level 2: 组件集成测试（Integration Tests）

**位置**：`tests/integration/`

**范围**：测试多个组件协同工作，使用**真实组件**（非 Mock）

**特点**：
- 单进程内多节点
- 使用真实网络连接
- 验证组件间真实交互

**运行**：
```bash
# 运行所有集成测试
go test ./tests/integration/... -v -timeout 5m

# 运行特定测试
go test ./tests/integration/core/... -v
go test ./tests/integration/protocol/... -v

# 使用构建标签
go test -tags=integration ./tests/integration/...
```

---

### Level 3: 端到端测试（E2E Tests）

**位置**：`tests/e2e/`

**范围**：测试完整的用户场景，对标 `examples/chat-local`

**特点**：
- 完整用户流程
- 多节点真实交互（本地网络）
- 覆盖 chat-local 全部功能

**运行**：
```bash
# 运行所有 E2E 测试
go test ./tests/e2e/... -v -timeout 10m

# 运行特定场景
go test ./tests/e2e/scenario/... -v

# 使用构建标签
go test -tags=e2e ./tests/e2e/... -timeout 10m
```

---

### Level 4: 真实网络验证（Network Validation）

**位置**：手动执行 + AI 日志分析

**范围**：测试 NAT 穿透、中继通信、跨网络连接等**无法本地模拟**的场景

**特点**：
- 需要部署多机节点（云服务器 + WiFi + 4G）
- 通过结构化日志记录系统行为
- 使用 AI 分析日志验证检查点

**验证场景**：

| 场景 ID | 场景名称 | 验证重点 |
|---------|----------|----------|
| S01 | 基础三节点通信 | WiFi + 4G + 云连接 |
| S02 | NAT 穿透验证 | 打洞、中继回退 |
| S03 | DHT 路由验证 | 路由表、节点发现 |
| S04 | 中继通信验证 | 电路建立、延迟 |
| S05 | 成员同步验证 | 同步协议、去重 |
| S06 | 网络切换恢复 | 检测、重连、恢复 |

**详细说明**：[真实网络验证策略](../design/04_delivery/testing/strategy/network_validation.md)

---

## 测试工具库 (testutil)

### TestNodeBuilder

简化节点创建：

```go
node := testutil.NewTestNode(t).
    WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
    WithPreset("minimal").
    Start()
```

### TestRealmBuilder

简化 Realm 创建：

```go
realm := testutil.NewTestRealm(t, node).
    WithPSK("test-secret").
    Join()
```

### 断言工具

```go
// 等待条件满足
testutil.Eventually(t, 10*time.Second, func() bool {
    return node.ConnectionCount() > 0
}, "应该建立连接")

// 等待成员发现
testutil.WaitForMembers(t, realm, 3, 30*time.Second)

// 等待 PubSub 消息
msg := testutil.WaitForMessage(t, sub, 10*time.Second)
```

---

## 测试覆盖矩阵

| 功能 | 单元测试 | 集成测试 | E2E 测试 | chat-local 对应 |
|-----|---------|---------|---------|----------------|
| **节点启动** | ✅ | ✅ | ✅ | `dep2p.Start()` |
| **连接建立** | ✅ | ✅ | ✅ | mDNS 自动连接 |
| **Realm 加入** | ✅ | ✅ | ✅ | `JoinRealm()` |
| **PSK 认证** | ✅ | ✅ | ✅ | 自动 |
| **成员发现** | ✅ | ✅ | ✅ | `Members()` |
| **PubSub Join** | ✅ | ✅ | ✅ | `PubSub().Join()` |
| **PubSub Subscribe** | ✅ | ✅ | ✅ | `topic.Subscribe()` |
| **PubSub Publish** | ✅ | ✅ | ✅ | `topic.Publish()` |
| **PubSub Receive** | - | ✅ | ✅ | `sub.Next()` |
| **Streams Register** | ✅ | ✅ | ✅ | `RegisterHandler()` |
| **Streams Open** | ✅ | ✅ | ✅ | `Streams().Open()` |
| **Streams Read/Write** | - | ✅ | ✅ | `stream.Read/Write()` |
| **EventBus 订阅** | ✅ | ✅ | ✅ | `Subscribe()` |
| **连接事件** | - | ✅ | ✅ | `EvtPeerConnected` |
| **断开事件** | - | ✅ | ✅ | `EvtPeerDisconnected` |

---

## 快速开始

### 运行所有测试

```bash
# 单元测试 (< 1 分钟)
go test -short ./...

# 集成测试 (约 2-5 分钟)
go test ./tests/integration/... -v -timeout 5m

# E2E 测试 (约 5-10 分钟)
go test ./tests/e2e/... -v -timeout 10m

# 完整测试
go test ./... -timeout 15m
```

### 运行特定测试

```bash
# 连接测试
go test ./tests/integration/core/connection_test.go -v

# PubSub 测试
go test ./tests/integration/protocol/pubsub_test.go -v

# 聊天场景
go test ./tests/e2e/scenario/chat_test.go -v -tags=e2e
```

---

## 测试最佳实践

### 1. 使用 testutil 工具

```go
// ✅ 推荐
node := testutil.NewTestNode(t).Start()
realm := testutil.NewTestRealm(t, node).WithPSK("secret").Join()

// ❌ 不推荐 (直接调用 API)
node, err := dep2p.Start(ctx, ...)
```

### 2. 设置合理超时

```go
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()
```

### 3. 使用 Eventually 等待异步操作

```go
testutil.Eventually(t, 10*time.Second, func() bool {
    return len(realm.Members()) >= 2
}, "等待成员发现")
```

### 4. 清理资源

testutil 工具会自动注册清理函数，无需手动清理：

```go
node := testutil.NewTestNode(t).Start()  // 自动注册 t.Cleanup
// 测试结束时自动调用 node.Close()
```

---

## 参考

### 内部文档

- [测试策略文档](../design/04_delivery/testing/strategy/)
- [测试金字塔（四层架构）](../design/04_delivery/testing/strategy/test_pyramid.md)
- [真实网络验证策略](../design/04_delivery/testing/strategy/network_validation.md)
- [测试矩阵](../design/04_delivery/testing/strategy/test_matrix.md)
- [分布式验证框架设计](../design/_discussions/20260120-distributed-validation-framework.md)
- [BUG 跟踪](../design/_discussions/20260119-bug-tracking-chat-example.md)

### 外部资源

- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test)
- [go-libp2p test-plans](https://github.com/libp2p/go-libp2p/tree/master/test-plans)
