# Protocol 层测试用例

> Protocol 层模块的测试用例集

---

## 概述

本目录包含 Protocol 层 (`internal/protocol/`) 的测试用例，涵盖 Messaging、PubSub、Streams、Liveness 四个核心协议服务。

---

## 代码位置

```
internal/protocol/
├── messaging/                # 消息传递
│   ├── service.go
│   ├── service_test.go
│   ├── handler.go
│   ├── handler_test.go
│   ├── codec.go
│   ├── codec_test.go
│   ├── concurrent_test.go
│   ├── benchmark_test.go
│   └── integration_test.go
├── pubsub/                   # 发布订阅
│   ├── service.go
│   ├── service_test.go
│   ├── topic.go
│   ├── topic_test.go
│   ├── subscription.go
│   ├── subscription_test.go
│   ├── mesh.go
│   ├── mesh_test.go
│   ├── validator.go
│   ├── validator_test.go
│   ├── concurrent_test.go
│   ├── benchmark_test.go
│   └── integration_test.go
├── streams/                  # 双向流
│   ├── service.go
│   ├── service_test.go
│   ├── stream.go
│   ├── stream_test.go
│   ├── concurrent_test.go
│   ├── benchmark_test.go
│   └── integration_test.go
└── liveness/                 # 活性检测
    ├── service.go
    ├── service_test.go
    ├── ping_test.go
    ├── status.go
    ├── status_test.go
    ├── concurrent_test.go
    ├── benchmark_test.go
    └── integration_test.go
```

---

## Messaging 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-MESSAGING-0001 | Service 初始化 | 单元 | P0 | ✅ |
| TST-MESSAGING-0002 | Request/Response | 单元 | P0 | ✅ |
| TST-MESSAGING-0003 | Handler 注册 | 单元 | P0 | ✅ |
| TST-MESSAGING-0004 | 超时处理 | 单元 | P0 | ✅ |
| TST-MESSAGING-0005 | 错误处理 | 单元 | P0 | ✅ |
| TST-MESSAGING-0006 | 编解码 | 单元 | P1 | ✅ |
| TST-MESSAGING-0007 | 协议协商 | 单元 | P1 | ✅ |
| TST-MESSAGING-0008 | 并发请求 | 并发 | P1 | ✅ |
| TST-MESSAGING-0009 | 性能基准 | 性能 | P2 | ✅ |
| TST-MESSAGING-0010 | 集成测试 | 集成 | P1 | ✅ |

### 用例详情

#### TST-MESSAGING-0002: Request/Response

| 字段 | 值 |
|------|-----|
| **ID** | TST-MESSAGING-0002 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **代码位置** | `internal/protocol/messaging/service_test.go` |

**测试目标**：验证 Request/Response 模式的消息传递

**测试代码**：

```go
func TestService_Request(t *testing.T) {
    svc := NewService(...)
    
    // 注册 Handler
    svc.RegisterHandler("test-protocol", handler)
    
    // 发送请求
    resp, err := svc.Request(ctx, peerID, "test-protocol", request)
    require.NoError(t, err)
    assert.Equal(t, expected, resp)
}
```

---

## PubSub 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-PUBSUB-0001 | Service 初始化 | 单元 | P0 | ✅ |
| TST-PUBSUB-0002 | Join Topic | 单元 | P0 | ✅ |
| TST-PUBSUB-0003 | Leave Topic | 单元 | P0 | ✅ |
| TST-PUBSUB-0004 | Subscribe | 单元 | P0 | ✅ |
| TST-PUBSUB-0005 | Publish | 单元 | P0 | ✅ |
| TST-PUBSUB-0006 | 消息验证 | 单元 | P1 | ✅ |
| TST-PUBSUB-0007 | Mesh 管理 | 单元 | P1 | ✅ |
| TST-PUBSUB-0008 | 消息去重 | 单元 | P1 | ✅ |
| TST-PUBSUB-0009 | 并发订阅 | 并发 | P1 | ✅ |
| TST-PUBSUB-0010 | 性能基准 | 性能 | P2 | ✅ |
| TST-PUBSUB-0011 | 集成测试 | 集成 | P1 | ✅ |

### 用例详情

#### TST-PUBSUB-0002: Join Topic

| 字段 | 值 |
|------|-----|
| **ID** | TST-PUBSUB-0002 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **代码位置** | `internal/protocol/pubsub/topic_test.go` |

**测试目标**：验证加入 Topic 功能

**测试代码**：

```go
func TestPubSub_Join(t *testing.T) {
    svc := NewService(...)
    
    topic, err := svc.Join("test-topic")
    require.NoError(t, err)
    assert.NotNil(t, topic)
    assert.Equal(t, "test-topic", topic.Name())
}
```

---

## Streams 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-STREAMS-0001 | Service 初始化 | 单元 | P0 | ✅ |
| TST-STREAMS-0002 | OpenStream | 单元 | P0 | ✅ |
| TST-STREAMS-0003 | AcceptStream | 单元 | P0 | ✅ |
| TST-STREAMS-0004 | 流读写 | 单元 | P0 | ✅ |
| TST-STREAMS-0005 | 流关闭 | 单元 | P0 | ✅ |
| TST-STREAMS-0006 | 并发流 | 并发 | P1 | ✅ |
| TST-STREAMS-0007 | 性能基准 | 性能 | P2 | ✅ |
| TST-STREAMS-0008 | 集成测试 | 集成 | P1 | ✅ |

### 用例详情

#### TST-STREAMS-0002: OpenStream

| 字段 | 值 |
|------|-----|
| **ID** | TST-STREAMS-0002 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **代码位置** | `internal/protocol/streams/service_test.go` |

**测试目标**：验证打开双向流功能

**测试代码**：

```go
func TestStreams_OpenStream(t *testing.T) {
    svc := NewService(...)
    
    stream, err := svc.OpenStream(ctx, peerID, "test-protocol")
    require.NoError(t, err)
    defer stream.Close()
    
    assert.NotNil(t, stream)
    assert.Equal(t, peerID, stream.RemotePeer())
}
```

---

## Liveness 模块

### 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-LIVENESS-0001 | Service 初始化 | 单元 | P0 | ✅ |
| TST-LIVENESS-0002 | Ping | 单元 | P0 | ✅ |
| TST-LIVENESS-0003 | Health Check | 单元 | P0 | ✅ |
| TST-LIVENESS-0004 | Status 状态 | 单元 | P0 | ✅ |
| TST-LIVENESS-0005 | Watch 订阅 | 单元 | P1 | ✅ |
| TST-LIVENESS-0006 | 并发 Ping | 并发 | P1 | ✅ |
| TST-LIVENESS-0007 | 性能基准 | 性能 | P2 | ✅ |
| TST-LIVENESS-0008 | 集成测试 | 集成 | P1 | ✅ |

### 用例详情

#### TST-LIVENESS-0002: Ping

| 字段 | 值 |
|------|-----|
| **ID** | TST-LIVENESS-0002 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **代码位置** | `internal/protocol/liveness/ping_test.go` |

**测试目标**：验证 Ping/Pong 功能

**测试代码**：

```go
func TestLiveness_Ping(t *testing.T) {
    svc := NewService(...)
    
    latency, err := svc.Ping(ctx, peerID)
    require.NoError(t, err)
    assert.Greater(t, latency, time.Duration(0))
}
```

---

## 性能测试指标

| 模块 | 指标 | 目标 |
|------|------|------|
| Messaging | 单次 RTT | ≤ 10ms |
| PubSub | 发布延迟 | ≤ 5ms |
| Streams | 吞吐量 | ≥ 100MB/s |
| Liveness | Ping 延迟 | ≤ 2ms |

---

## 代码覆盖目标

| 文件 | 目标覆盖率 | 当前状态 |
|------|-----------|----------|
| `messaging/service.go` | ≥ 85% | ✅ |
| `pubsub/service.go` | ≥ 85% | ✅ |
| `pubsub/topic.go` | ≥ 80% | ✅ |
| `streams/service.go` | ≥ 80% | ✅ |
| `liveness/service.go` | ≥ 80% | ✅ |

---

**最后更新**：2026-01-15
