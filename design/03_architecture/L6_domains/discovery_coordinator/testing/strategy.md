# core_discovery 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| DHT | 85%+ | 核心发现 |
| mDNS | 80%+ | 局域网发现 |
| Rendezvous | 90%+ | 命名空间发现 |
| Composite | 85%+ | 组合逻辑 |

---

## 测试类型

### DHT 测试

```
func TestDHT_FindPeer(t *testing.T) {
    dht1, dht2 := setupDHTNetwork(t)
    
    // dht2 应该能找到 dht1
    peer, err := dht2.FindPeer(ctx, dht1.ID())
    
    require.NoError(t, err)
    assert.Equal(t, dht1.ID(), peer.ID)
}

func TestDHT_PutGetValue(t *testing.T) {
    dht := setupDHT(t)
    
    err := dht.PutValue(ctx, "/test/key", []byte("value"))
    require.NoError(t, err)
    
    value, err := dht.GetValue(ctx, "/test/key")
    require.NoError(t, err)
    assert.Equal(t, "value", string(value))
}
```

### Rendezvous 测试

```
func TestRendezvous_RegisterDiscover(t *testing.T) {
    rv1, rv2 := setupRendezvous(t)
    
    // rv1 注册
    err := rv1.Register(ctx, "/test/ns", time.Hour)
    require.NoError(t, err)
    
    // rv2 发现
    peers, err := rv2.Discover(ctx, "/test/ns", 10)
    require.NoError(t, err)
    
    assert.Contains(t, peerIDs(peers), rv1.ID())
}
```

---

## Mock 策略

```
type MockDiscovery struct {
    mock.Mock
}

func (m *MockDiscovery) FindPeers(ctx context.Context, ns string, opts ...Option) (<-chan PeerInfo, error) {
    args := m.Called(ctx, ns, opts)
    return args.Get(0).(<-chan PeerInfo), args.Error(1)
}

func (m *MockDiscovery) Advertise(ctx context.Context, ns string, opts ...Option) (time.Duration, error) {
    args := m.Called(ctx, ns, opts)
    return args.Get(0).(time.Duration), args.Error(1)
}
```

---

**最后更新**：2026-01-11
