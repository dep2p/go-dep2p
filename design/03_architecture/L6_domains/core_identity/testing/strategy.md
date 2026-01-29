# core_identity 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| KeyGenerator | 100% | 核心安全组件 |
| NodeIDDerivation | 100% | 核心功能 |
| Sign/Verify | 100% | 核心功能 |
| Service | 90%+ | 主服务 |

---

## 测试类型

### 单元测试

```
// service_test.go

func TestGenerateKeyPair(t *testing.T) {
    priv, pub, err := GenerateKeyPair()
    
    require.NoError(t, err)
    assert.Len(t, priv, ed25519.PrivateKeySize)
    assert.Len(t, pub, ed25519.PublicKeySize)
}

func TestDeriveNodeID(t *testing.T) {
    _, pub, _ := GenerateKeyPair()
    
    nodeID := DeriveNodeID(pub)
    
    assert.Len(t, nodeID, 32)
}

func TestDeriveNodeID_Deterministic(t *testing.T) {
    _, pub, _ := GenerateKeyPair()
    
    nodeID1 := DeriveNodeID(pub)
    nodeID2 := DeriveNodeID(pub)
    
    assert.True(t, nodeID1.Equals(nodeID2))
}
```

### 签名测试

```
func TestSignAndVerify(t *testing.T) {
    svc, _ := NewService()
    data := []byte("test message")
    
    sig, err := svc.Sign(data)
    require.NoError(t, err)
    
    valid, err := svc.Verify(svc.ID(), data, sig)
    require.NoError(t, err)
    assert.True(t, valid)
}

func TestVerify_InvalidSignature(t *testing.T) {
    svc, _ := NewService()
    data := []byte("test message")
    invalidSig := make([]byte, 64)
    
    valid, _ := svc.Verify(svc.ID(), data, invalidSig)
    
    assert.False(t, valid)
}
```

---

## 关键测试场景

| 场景 | 测试点 |
|------|--------|
| 密钥生成 | 长度正确、随机性 |
| NodeID 派生 | 确定性、唯一性 |
| 签名 | 正确签名 |
| 验证 | 正确验证、拒绝无效 |
| 序列化 | 导入导出一致性 |

---

## Mock 策略

Identity 模块通常作为依赖被 Mock：

```
// mock/identity.go

type MockIdentityService struct {
    mock.Mock
}

func (m *MockIdentityService) ID() types.NodeID {
    args := m.Called()
    return args.Get(0).(types.NodeID)
}

func (m *MockIdentityService) Sign(data []byte) ([]byte, error) {
    args := m.Called(data)
    return args.Get(0).([]byte), args.Error(1)
}
```

### 使用示例

```
func TestSomeService(t *testing.T) {
    mockIdentity := new(MockIdentityService)
    mockIdentity.On("ID").Return(testNodeID)
    mockIdentity.On("Sign", mock.Anything).Return(testSig, nil)
    
    svc := NewSomeService(mockIdentity)
    // ...
}
```

---

## 基准测试

```
func BenchmarkSign(b *testing.B) {
    svc, _ := NewService()
    data := []byte("benchmark data")
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        svc.Sign(data)
    }
}

func BenchmarkDeriveNodeID(b *testing.B) {
    _, pub, _ := GenerateKeyPair()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        DeriveNodeID(pub)
    }
}
```

---

**最后更新**：2026-01-11
