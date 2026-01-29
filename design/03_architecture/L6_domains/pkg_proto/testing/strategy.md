# pkg_proto 测试策略

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 测试目标

| 目标 | 指标 | 状态 |
|------|------|------|
| 覆盖率 | > 30% | ✅ ~33% |
| 往返测试 | 所有消息类型 | ✅ 完成 |
| 集成测试 | 与 pkg/types 兼容 | ✅ 完成 |

**说明**：Protobuf 生成代码的覆盖率目标为 30%（vs 手写代码的 80%），因为大部分是自动生成的 getter/setter。

---

## 测试分类

### 1. 往返测试（Round Trip）

**目标**：验证序列化/反序列化正确性

```go
func TestRoundTrip(t *testing.T) {
    original := &identify.Identify{
        ProtocolVersion: []byte("dep2p/1.0.0"),
        AgentVersion:    []byte("go-dep2p/1.0.0"),
    }
    
    // Marshal
    data, _ := proto.Marshal(original)
    
    // Unmarshal
    var decoded identify.Identify
    proto.Unmarshal(data, &decoded)
    
    // 验证
    if !proto.Equal(original, &decoded) {
        t.Error("Round trip failed")
    }
}
```

### 2. 字段测试

**目标**：验证各字段类型正确

```go
func TestMessage_Fields(t *testing.T) {
    msg := &messaging.Message{
        Id:        []byte("uuid"),
        From:      []byte("sender"),
        Type:      messaging.MessageType_DIRECT,
        Priority:  messaging.Priority_HIGH,
        Timestamp: 1234567890,
    }
    
    data, _ := proto.Marshal(msg)
    var decoded messaging.Message
    proto.Unmarshal(data, &decoded)
    
    // 验证每个字段
    assert.Equal(t, msg.Type, decoded.Type)
    assert.Equal(t, msg.Priority, decoded.Priority)
}
```

### 3. 枚举测试

**目标**：验证枚举值正确

```go
func TestEnums(t *testing.T) {
    tests := []struct {
        name string
        val  relay.Status
    }{
        {"OK", relay.Status_OK},
        {"REFUSED", relay.Status_RESERVATION_REFUSED},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            msg := &relay.HopMessage{Status: tt.val}
            // Marshal/Unmarshal
            data, _ := proto.Marshal(msg)
            var decoded relay.HopMessage
            proto.Unmarshal(data, &decoded)
            
            assert.Equal(t, tt.val, decoded.Status)
        })
    }
}
```

---

## 覆盖率策略

### Protobuf 生成代码的覆盖率特点

| 代码类型 | 覆盖率 | 说明 |
|---------|--------|------|
| 消息结构 | ~20% | getter/setter（自动生成） |
| 枚举类型 | ~40% | String(), Descriptor() |
| Marshal 逻辑 | ~80% | 序列化（protobuf 库） |
| Unmarshal 逻辑 | ~80% | 反序列化（protobuf 库） |

**平均覆盖率**：~30-35%

### 当前覆盖率

```
总体平均：~33%

详细分组：
  key:         51.6%  (最高)
  peer:        50.5%
  holepunch:   44.7%
  common:      44.0%
  identify:    42.2%
  gossipsub:   29.3%
  dht:         27.8%
  autonat:     24.5%
  relay:       23.8%
  realm:       23.0%
  rendezvous:  18.4%
  messaging:   17.0%  (最低)
```

---

## 测试用例设计

### 正常路径测试

```go
func TestMessage_Valid(t *testing.T) {
    tests := []proto.Message{
        &common.Timestamp{Seconds: 123, Nanos: 456},
        &key.PublicKey{Type: key.KeyType_Ed25519, Data: []byte("key")},
        &peer.PeerID{Id: []byte("peer")},
    }
    
    for _, msg := range tests {
        t.Run(reflect.TypeOf(msg).String(), func(t *testing.T) {
            data, err := proto.Marshal(msg)
            if err != nil {
                t.Fatalf("Marshal failed: %v", err)
            }
            
            decoded := proto.Clone(msg)
            proto.Reset(decoded)
            err = proto.Unmarshal(data, decoded)
            if err != nil {
                t.Fatalf("Unmarshal failed: %v", err)
            }
            
            if !proto.Equal(msg, decoded) {
                t.Error("Round trip failed")
            }
        })
    }
}
```

### 空消息测试

```go
func TestMessage_Empty(t *testing.T) {
    msg := &identify.Identify{}
    
    data, err := proto.Marshal(msg)
    if err != nil {
        t.Fatalf("Marshal empty failed: %v", err)
    }
    
    var decoded identify.Identify
    err = proto.Unmarshal(data, &decoded)
    if err != nil {
        t.Fatalf("Unmarshal empty failed: %v", err)
    }
}
```

### 大数据测试

```go
func TestMessage_LargeData(t *testing.T) {
    // 创建大消息（模拟真实场景）
    msg := &messaging.Message{
        Id:      make([]byte, 16),  // UUID
        Payload: make([]byte, 1024*1024),  // 1MB
    }
    
    data, err := proto.Marshal(msg)
    if err != nil {
        t.Fatalf("Marshal large data failed: %v", err)
    }
    
    t.Logf("Marshaled size: %d bytes", len(data))
}
```

---

## 测试数据管理

### 标准测试数据

```go
var (
    testPeerID = []byte("12D3KooWTest")
    testRealmID = []byte("realm-test-id-32-bytes-long")
    testMultiaddr = []byte("/ip4/127.0.0.1/tcp/4001")
    testTimestamp = uint64(1234567890)
)
```

### 测试辅助函数

```go
// 创建测试用的 PeerInfo
func newTestPeerInfo() *autonat.PeerInfo {
    return &autonat.PeerInfo{
        Id:    testPeerID,
        Addrs: [][]byte{testMultiaddr},
    }
}

// 验证消息相等
func assertProtoEqual(t *testing.T, expected, actual proto.Message) {
    if !proto.Equal(expected, actual) {
        t.Errorf("Messages not equal:\nexpected: %v\nactual: %v", 
                 expected, actual)
    }
}
```

---

## 性能测试

### 序列化性能

```go
func BenchmarkMarshal_Identify(b *testing.B) {
    msg := &identify.Identify{
        ProtocolVersion: []byte("dep2p/1.0.0"),
        AgentVersion:    []byte("go-dep2p/1.0.0"),
        Protocols:       []string{"/test/1.0.0"},
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = proto.Marshal(msg)
    }
}
```

### 反序列化性能

```go
func BenchmarkUnmarshal_Identify(b *testing.B) {
    msg := &identify.Identify{...}
    data, _ := proto.Marshal(msg)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        var decoded identify.Identify
        _ = proto.Unmarshal(data, &decoded)
    }
}
```

---

## 测试执行

### CI/CD 流程

```bash
# 单元测试
go test ./pkg/proto/...

# 带覆盖率
go test -cover ./pkg/proto/...

# 性能测试
go test -bench=. ./pkg/proto/...
```

### 本地测试

```bash
# 快速测试
go test ./pkg/proto/...

# 详细测试
go test -v ./pkg/proto/...

# 覆盖率报告
go test -coverprofile=coverage.out ./pkg/proto/...
go tool cover -html=coverage.out
```

---

## 相关文档

- [../coding/guidelines.md](../coding/guidelines.md) - 编码指南
- [../design/overview.md](../design/overview.md) - 设计概述

---

**最后更新**：2026-01-13
