# core_connmgr 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| TagStore | 100% | 核心数据结构 |
| ProtectStore | 100% | 核心数据结构 |
| Trim | 90%+ | 回收逻辑 |
| Gater | 85%+ | 门控逻辑 |

---

## 测试类型

### 单元测试

```
func TestTagPeer(t *testing.T) {
    mgr := NewManager(100, 200)
    
    mgr.TagPeer(testPeer, "score", 10)
    
    assert.Equal(t, 10, mgr.tags.Sum(testPeer))
}

func TestProtect(t *testing.T) {
    mgr := NewManager(100, 200)
    
    mgr.Protect(testPeer, "important")
    
    assert.True(t, mgr.IsProtected(testPeer, "important"))
}

func TestUnprotect(t *testing.T) {
    mgr := NewManager(100, 200)
    mgr.Protect(testPeer, "a")
    mgr.Protect(testPeer, "b")
    
    hasMore := mgr.Unprotect(testPeer, "a")
    
    assert.True(t, hasMore)
    assert.True(t, mgr.IsProtected(testPeer, "b"))
}
```

### 回收测试

```
func TestTrim(t *testing.T) {
    mgr := NewManager(2, 4)
    host := newMockHost(5) // 5 个连接
    mgr.SetHost(host)
    
    mgr.TrimOpenConns(context.Background())
    
    assert.LessOrEqual(t, host.ConnCount(), 2)
}

func TestTrim_ProtectedNotClosed(t *testing.T) {
    mgr := NewManager(2, 4)
    host := newMockHost(5)
    mgr.SetHost(host)
    
    mgr.Protect(host.Peers()[0], "keep")
    
    mgr.TrimOpenConns(context.Background())
    
    assert.True(t, host.IsConnected(host.Peers()[0]))
}
```

---

## Mock 策略

```
type MockManager struct {
    mock.Mock
}

func (m *MockManager) Protect(peer types.NodeID, tag string) {
    m.Called(peer, tag)
}

func (m *MockManager) IsProtected(peer types.NodeID, tag string) bool {
    args := m.Called(peer, tag)
    return args.Bool(0)
}
```

---

**最后更新**：2026-01-11
