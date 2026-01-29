# core_realm 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| Manager | 90%+ | 核心管理 |
| Authenticator | 100% | 安全关键 |
| MemberCache | 100% | 数据结构 |
| KeyDeriver | 100% | 密码学 |

---

## 测试类型

### 单元测试

```
func TestDeriveRealmID(t *testing.T) {
    psk := types.PSK(make([]byte, 32))
    rand.Read(psk)
    
    id1 := deriveRealmID(psk)
    id2 := deriveRealmID(psk)
    
    assert.True(t, id1.Equals(id2))
    assert.Len(t, id1, 32)
}

func TestMemberCache(t *testing.T) {
    cache := newMemberCache()
    nodeID := randomNodeID()
    
    assert.False(t, cache.IsMember(nodeID))
    
    cache.Add(nodeID)
    
    assert.True(t, cache.IsMember(nodeID))
}
```

### 认证测试

```
func TestAuthenticate_Success(t *testing.T) {
    psk := randomPSK()
    auth1, auth2 := setupAuthenticators(t, psk)
    
    err := auth1.Authenticate(ctx, auth2.ID())
    
    assert.NoError(t, err)
}

func TestAuthenticate_WrongPSK(t *testing.T) {
    auth1 := setupAuthenticator(t, randomPSK())
    auth2 := setupAuthenticator(t, randomPSK())  // 不同 PSK
    
    err := auth1.Authenticate(ctx, auth2.ID())
    
    assert.ErrorIs(t, err, ErrAuthFailed)
}
```

---

## 集成测试

```
func TestJoinRealm(t *testing.T) {
    node1, node2 := setupNodes(t)
    psk := randomPSK()
    
    // 两个节点加入同一 Realm
    realm1, _ := node1.JoinRealm(ctx, "test", psk)
    realm2, _ := node2.JoinRealm(ctx, "test", psk)
    
    // 等待发现
    time.Sleep(time.Second)
    
    // 应该互相发现
    assert.True(t, realm1.IsMember(node2.ID()))
    assert.True(t, realm2.IsMember(node1.ID()))
}
```

---

## Mock 策略

```
type MockRealm struct {
    mock.Mock
}

func (m *MockRealm) IsMember(id types.NodeID) bool {
    args := m.Called(id)
    return args.Bool(0)
}

func (m *MockRealm) Messaging() messaging.Service {
    args := m.Called()
    return args.Get(0).(messaging.Service)
}
```

---

**最后更新**：2026-01-11
