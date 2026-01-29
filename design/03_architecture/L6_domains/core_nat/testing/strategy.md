# core_nat 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| STUN | 85%+ | 外部地址获取 |
| UPnP | 80%+ | 端口映射 |
| HolePunch | 85%+ | 打洞逻辑 |
| Detector | 90%+ | NAT 检测 |

---

## 测试类型

### 单元测试

```
func TestSTUN_GetExternalAddr(t *testing.T) {
    // 使用公共 STUN 服务器
    client := NewSTUNClient("stun.l.google.com:19302")
    
    addr, err := client.GetExternalAddr()
    
    require.NoError(t, err)
    assert.NotNil(t, addr)
}

func TestDetector_DetectNATType(t *testing.T) {
    detector := NewDetector()
    
    natType, err := detector.Detect()
    
    require.NoError(t, err)
    assert.NotEqual(t, NATUnknown, natType)
}
```

### 打洞测试

```
func TestHolePunch_Success(t *testing.T) {
    // 模拟两个 NAT 后节点
    node1, node2 := setupNATNodes(t)
    
    // 尝试打洞
    err := node1.HolePunch(ctx, node2.ID(), node2.Addrs())
    
    // 验证直连
    require.NoError(t, err)
    assert.True(t, node1.IsDirectlyConnected(node2.ID()))
}
```

---

## Mock 策略

```
type MockNATService struct {
    mock.Mock
}

func (m *MockNATService) GetExternalAddr() (multiaddr.Multiaddr, error) {
    args := m.Called()
    return args.Get(0).(multiaddr.Multiaddr), args.Error(1)
}

func (m *MockNATService) Reachability() Reachability {
    args := m.Called()
    return args.Get(0).(Reachability)
}
```

---

## 网络模拟

```
// 使用 network namespace 模拟 NAT
func setupNATSimulation(t *testing.T) {
    // 创建虚拟网络
    // 配置 NAT 规则
    // 运行测试
}
```

---

**最后更新**：2026-01-11
