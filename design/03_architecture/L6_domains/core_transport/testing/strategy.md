# core_transport 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| Dial/Listen | 90%+ | 核心功能 |
| Connection | 90%+ | 连接管理 |
| Stream | 85%+ | 流操作 |
| AddrParser | 100% | 地址解析 |

---

## 测试类型

### 单元测试

```
func TestParseMultiaddr(t *testing.T) {
    tests := []struct {
        addr    string
        wantIP  string
        wantPort int
    }{
        {"/ip4/127.0.0.1/udp/4001/quic-v1", "127.0.0.1", 4001},
        {"/ip6/::1/udp/4001/quic-v1", "::1", 4001},
    }
    
    for _, tt := range tests {
        t.Run(tt.addr, func(t *testing.T) {
            ma, _ := multiaddr.NewMultiaddr(tt.addr)
            udpAddr, err := parseAddr(ma)
            
            require.NoError(t, err)
            assert.Equal(t, tt.wantIP, udpAddr.IP.String())
            assert.Equal(t, tt.wantPort, udpAddr.Port)
        })
    }
}
```

### 集成测试

```
func TestDialAndListen(t *testing.T) {
    // 创建两个传输实例
    t1, _ := NewTransport(identity1, security1)
    t2, _ := NewTransport(identity2, security2)
    
    // 启动监听
    listener, _ := t1.Listen(testAddr)
    defer listener.Close()
    
    // 连接
    conn, err := t2.Dial(ctx, listener.Addr(), identity1.ID())
    require.NoError(t, err)
    defer conn.Close()
    
    // 验证
    assert.Equal(t, identity1.ID(), conn.RemotePeer())
}
```

---

## 关键测试场景

| 场景 | 测试点 |
|------|--------|
| 正常连接 | Dial + Accept 成功 |
| 身份验证 | 验证 RemotePeer |
| 流操作 | 打开/接受/读写流 |
| 并发流 | 多流并发 |
| 错误处理 | 连接失败、超时 |
| 资源释放 | 关闭后资源释放 |

---

## Mock 策略

传输层通常被其他模块 Mock：

```
type MockTransport struct {
    mock.Mock
}

func (m *MockTransport) Dial(ctx context.Context, addr multiaddr.Multiaddr, peer types.NodeID) (Connection, error) {
    args := m.Called(ctx, addr, peer)
    return args.Get(0).(Connection), args.Error(1)
}

func (m *MockTransport) Listen(addr multiaddr.Multiaddr) (Listener, error) {
    args := m.Called(addr)
    return args.Get(0).(Listener), args.Error(1)
}
```

### MockConnection

```
type MockConnection struct {
    mock.Mock
}

func (m *MockConnection) OpenStream(ctx context.Context) (Stream, error) {
    args := m.Called(ctx)
    return args.Get(0).(Stream), args.Error(1)
}
```

---

## 性能测试

```
func BenchmarkDial(b *testing.B) {
    t1, t2 := setupTransports(b)
    listener, _ := t1.Listen(testAddr)
    defer listener.Close()
    
    go acceptLoop(listener)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        conn, _ := t2.Dial(ctx, listener.Addr(), t1.ID())
        conn.Close()
    }
}

func BenchmarkStream(b *testing.B) {
    conn := setupConnection(b)
    defer conn.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        stream, _ := conn.OpenStream(ctx)
        stream.Close()
    }
}
```

---

**最后更新**：2026-01-11
