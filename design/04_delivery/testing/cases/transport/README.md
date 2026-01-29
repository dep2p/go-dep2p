# 传输测试用例 (Transport)

> 传输模块的测试用例集

---

## 概述

本目录包含传输模块 (`internal/core/transport/`) 的测试用例，覆盖 TCP、QUIC、WebSocket 等传输协议的连接管理和数据传输。

---

## 用例清单

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| TST-TRANSPORT-0001 | TCP 监听 | 单元 | P0 | ✅ |
| TST-TRANSPORT-0002 | TCP 连接建立 | 集成 | P0 | ✅ |
| TST-TRANSPORT-0003 | TCP 数据传输 | 集成 | P0 | ✅ |
| TST-TRANSPORT-0004 | QUIC 监听 | 单元 | P0 | ✅ |
| TST-TRANSPORT-0005 | QUIC 连接建立 | 集成 | P0 | ✅ |
| TST-TRANSPORT-0006 | QUIC 数据传输 | 集成 | P0 | ✅ |
| TST-TRANSPORT-0007 | 连接超时处理 | 单元 | P1 | ✅ |
| TST-TRANSPORT-0008 | 连接关闭 | 集成 | P1 | ✅ |
| TST-TRANSPORT-0009 | 并发连接 | 集成 | P1 | ✅ |
| TST-TRANSPORT-0010 | 大数据传输 | 集成 | P1 | ✅ |
| TST-TRANSPORT-0011 | 连接复用 | 集成 | P1 | ✅ |
| TST-TRANSPORT-0012 | WebSocket 连接 | 集成 | P2 | ⏳ |

---

## 用例详情

### TST-TRANSPORT-0001: TCP 监听

| 字段 | 值 |
|------|-----|
| **ID** | TST-TRANSPORT-0001 |
| **类型** | 单元测试 |
| **优先级** | P0 |
| **关联需求** | REQ-TRANSPORT-0001 |
| **代码位置** | `internal/core/transport/tcp_test.go` |

**测试目标**：验证 TCP 监听功能

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 创建 TCP Transport | 成功 |
| 2 | 在随机端口监听 | 返回 Listener，无错误 |
| 3 | 获取监听地址 | 返回有效地址 |
| 4 | 关闭 Listener | 成功关闭 |

**测试代码**：

```go
func TestTCPTransport_Listen(t *testing.T) {
    transport := NewTCPTransport()
    
    listener, err := transport.Listen("127.0.0.1:0")
    require.NoError(t, err)
    defer listener.Close()
    
    addr := listener.Addr()
    assert.NotEmpty(t, addr.String())
}
```

---

### TST-TRANSPORT-0002: TCP 连接建立

| 字段 | 值 |
|------|-----|
| **ID** | TST-TRANSPORT-0002 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **关联需求** | REQ-TRANSPORT-0002 |
| **代码位置** | `tests/integration/transport_test.go` |

**测试目标**：验证 TCP 连接建立流程

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 启动 TCP 监听 | 成功 |
| 2 | 客户端连接到监听地址 | 返回连接，无错误 |
| 3 | 服务端接受连接 | 返回连接，无错误 |
| 4 | 验证连接状态 | 双方连接有效 |

**测试代码**：

```go
func TestTCPTransport_Connect(t *testing.T) {
    serverTransport := NewTCPTransport()
    clientTransport := NewTCPTransport()
    
    // 服务端监听
    listener, _ := serverTransport.Listen("127.0.0.1:0")
    defer listener.Close()
    
    // 异步接受连接
    var serverConn Conn
    go func() {
        serverConn, _ = listener.Accept()
    }()
    
    // 客户端连接
    clientConn, err := clientTransport.Dial(listener.Addr())
    require.NoError(t, err)
    defer clientConn.Close()
    
    // 等待服务端接受
    time.Sleep(100 * time.Millisecond)
    assert.NotNil(t, serverConn)
}
```

---

### TST-TRANSPORT-0003: TCP 数据传输

| 字段 | 值 |
|------|-----|
| **ID** | TST-TRANSPORT-0003 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **关联需求** | REQ-TRANSPORT-0003 |
| **代码位置** | `tests/integration/transport_test.go` |

**测试目标**：验证 TCP 连接上的数据传输

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 建立 TCP 连接 | 成功 |
| 2 | 客户端发送数据 | 成功 |
| 3 | 服务端接收数据 | 数据一致 |
| 4 | 服务端回复数据 | 成功 |
| 5 | 客户端接收回复 | 数据一致 |

---

### TST-TRANSPORT-0005: QUIC 连接建立

| 字段 | 值 |
|------|-----|
| **ID** | TST-TRANSPORT-0005 |
| **类型** | 集成测试 |
| **优先级** | P0 |
| **关联需求** | REQ-TRANSPORT-0004 |
| **代码位置** | `tests/integration/quic_test.go` |

**测试目标**：验证 QUIC 连接建立流程

**测试步骤**：

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | 启动 QUIC 监听 | 成功 |
| 2 | 客户端连接 | 返回连接，无错误 |
| 3 | 验证 0-RTT | 如支持，无额外往返 |
| 4 | 验证连接状态 | 双方连接有效 |

---

## 边界条件测试

| 场景 | 输入 | 预期行为 |
|------|------|----------|
| 无效地址连接 | "invalid:port" | 返回错误 |
| 连接已关闭服务 | 关闭的端口 | 返回连接拒绝错误 |
| 连接超时 | 无响应地址 | 超时后返回错误 |
| 重复关闭连接 | 已关闭连接 | 无 panic，返回错误 |

---

## 性能测试场景

| 场景 | 参数 | 目标指标 |
|------|------|----------|
| 连接建立延迟 | TCP | P99 ≤ 50ms |
| 连接建立延迟 | QUIC | P99 ≤ 100ms |
| 数据吞吐量 | 1MB 数据 | ≥ 100MB/s |
| 并发连接 | 1000 连接 | 成功率 ≥ 99% |

---

## 代码覆盖目标

| 文件 | 目标覆盖率 |
|------|-----------|
| `tcp.go` | ≥ 80% |
| `quic.go` | ≥ 80% |
| `conn.go` | ≥ 85% |
| `listener.go` | ≥ 80% |

---

**最后更新**：2026-01-11
