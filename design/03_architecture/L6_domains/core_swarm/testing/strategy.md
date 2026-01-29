# Core Swarm 测试策略

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 测试层次

| 层次 | 范围 | 工具 |
|------|------|------|
| 单元测试 | 单个函数 | `testing` |
| 集成测试 | 组件交互 | `testify` |
| 端到端测试 | 完整流程 | 多节点模拟 |

---

## 单元测试

```go
func TestSwarm_DialPeer(t *testing.T) {
    // 准备
    swarm := newTestSwarm(t)
    
    // 执行
    conn, err := swarm.DialPeer(ctx, peerID)
    
    // 验证
    require.NoError(t, err)
    require.NotNil(t, conn)
    require.Equal(t, peerID, conn.RemotePeer())
}
```

---

## 集成测试

```go
func TestSwarm_Connect(t *testing.T) {
    // 创建两个 Swarm
    s1, s2 := createSwarmPair(t)
    
    // s1 连接 s2
    conn, err := s1.DialPeer(ctx, s2.LocalPeer())
    require.NoError(t, err)
    
    // 验证双向连接
    require.Len(t, s2.ConnsToPeer(s1.LocalPeer()), 1)
}
```

---

## 覆盖率要求

| 组件 | 最低覆盖率 |
|------|-----------|
| swarm.go | 80% |
| dial.go | 80% |
| conn.go | 70% |
| stream.go | 70% |

---

**最后更新**：2026-01-13
