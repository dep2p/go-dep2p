# core_muxer 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| OpenStream | 90%+ | 核心功能 |
| AcceptStream | 90%+ | 核心功能 |
| Stream I/O | 85%+ | 读写操作 |
| 关闭/重置 | 100% | 资源管理 |

---

## 测试类型

### 单元测试

```
func TestOpenStream(t *testing.T) {
    conn := setupMuxedConn(t)
    defer conn.Close()
    
    stream, err := conn.OpenStream(context.Background())
    
    require.NoError(t, err)
    assert.NotNil(t, stream)
    stream.Close()
}

func TestConcurrentStreams(t *testing.T) {
    conn := setupMuxedConn(t)
    defer conn.Close()
    
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            stream, err := conn.OpenStream(context.Background())
            require.NoError(t, err)
            stream.Close()
        }()
    }
    wg.Wait()
}
```

### 流读写测试

```
func TestStreamReadWrite(t *testing.T) {
    client, server := setupConnPair(t)
    defer client.Close()
    defer server.Close()
    
    // 打开流
    go func() {
        stream, _ := client.OpenStream(ctx)
        stream.Write([]byte("hello"))
        stream.Close()
    }()
    
    // 接受流
    stream, _ := server.AcceptStream()
    data, _ := io.ReadAll(stream)
    
    assert.Equal(t, "hello", string(data))
}
```

---

## 关键测试场景

| 场景 | 测试点 |
|------|--------|
| 正常流 | 打开/接受/读写/关闭 |
| 并发流 | 多流并发 |
| 流重置 | Reset 行为 |
| 连接关闭 | 所有流关闭 |
| 超时 | Deadline 行为 |

---

## Mock 策略

```
type MockMuxedConn struct {
    mock.Mock
}

func (m *MockMuxedConn) OpenStream(ctx context.Context) (Stream, error) {
    args := m.Called(ctx)
    return args.Get(0).(Stream), args.Error(1)
}

func (m *MockMuxedConn) AcceptStream() (Stream, error) {
    args := m.Called()
    return args.Get(0).(Stream), args.Error(1)
}
```

---

## 性能测试

```
func BenchmarkOpenStream(b *testing.B) {
    conn := setupMuxedConn(b)
    defer conn.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        stream, _ := conn.OpenStream(context.Background())
        stream.Close()
    }
}

func BenchmarkStreamWrite(b *testing.B) {
    stream := setupStream(b)
    defer stream.Close()
    
    data := make([]byte, 1024)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        stream.Write(data)
    }
}
```

---

**最后更新**：2026-01-11
