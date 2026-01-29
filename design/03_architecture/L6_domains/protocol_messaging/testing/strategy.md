# core_messaging 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| Messaging | 90%+ | 核心消息 |
| PubSub | 90%+ | 发布订阅 |
| Streams | 85%+ | 流管理 |
| Validator | 100% | 消息验证 |

---

## 测试类型

### 单元测试

```
func TestMessaging_Request(t *testing.T) {
    node1, node2 := setupNodes(t)
    joinSameRealm(t, node1, node2)
    
    // 注册处理器
    node2.Realm().Messaging().OnRequest("/test/echo", func(ctx context.Context, from types.NodeID, data []byte) ([]byte, error) {
        return data, nil
    })
    
    // 发送请求
    resp, err := node1.Realm().Messaging().Request(ctx, node2.ID(), "/test/echo", []byte("hello"))
    
    require.NoError(t, err)
    assert.Equal(t, "hello", string(resp))
}

func TestMessaging_Send(t *testing.T) {
    node1, node2 := setupNodes(t)
    joinSameRealm(t, node1, node2)
    
    received := make(chan []byte, 1)
    node2.Realm().Messaging().OnMessage("/test/notify", func(ctx context.Context, from types.NodeID, data []byte) {
        received <- data
    })
    
    err := node1.Realm().Messaging().Send(ctx, node2.ID(), "/test/notify", []byte("hello"))
    
    require.NoError(t, err)
    assert.Equal(t, "hello", string(<-received))
}
```

### PubSub 测试

```
func TestPubSub_PublishSubscribe(t *testing.T) {
    node1, node2 := setupNodes(t)
    joinSameRealm(t, node1, node2)
    
    // 订阅
    sub, _ := node2.Realm().PubSub().Subscribe("test-topic")
    
    // 发布
    node1.Realm().PubSub().Publish(ctx, "test-topic", []byte("hello"))
    
    // 接收
    msg, err := sub.Next(ctx)
    require.NoError(t, err)
    assert.Equal(t, "hello", string(msg.Data))
}
```

---

## Mock 策略

```
type MockMessaging struct {
    mock.Mock
}

func (m *MockMessaging) Send(ctx context.Context, peer types.NodeID, proto string, data []byte) error {
    args := m.Called(ctx, peer, proto, data)
    return args.Error(0)
}

func (m *MockMessaging) Request(ctx context.Context, peer types.NodeID, proto string, data []byte) ([]byte, error) {
    args := m.Called(ctx, peer, proto, data)
    return args.Get(0).([]byte), args.Error(1)
}
```

---

## 性能测试

```
func BenchmarkMessaging_Request(b *testing.B) {
    node1, node2 := setupNodes(b)
    setupEcho(node2)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        node1.Realm().Messaging().Request(ctx, node2.ID(), "/test/echo", data)
    }
}

func BenchmarkPubSub_Publish(b *testing.B) {
    node := setupNode(b)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        node.Realm().PubSub().Publish(ctx, "test", data)
    }
}
```

---

**最后更新**：2026-01-11
