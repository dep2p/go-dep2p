# Core Swarm 编码指南

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 代码组织

```
internal/core/swarm/
├── doc.go              # 包文档
├── module.go           # Fx 模块
├── swarm.go            # Swarm 主结构
├── conn.go             # Connection 实现
├── stream.go           # Stream 实现
├── dial.go             # 拨号逻辑
├── dial_ranker.go      # 地址排序
├── interfaces/         # 内部接口
│   ├── swarm.go
│   └── notifier.go
└── metrics.go          # 指标
```

---

## 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 接口 | 名词 | `Swarm`, `Connection`, `Stream` |
| 方法 | 动词 | `DialPeer`, `NewStream`, `Close` |
| 事件 | Evt 前缀 | `EvtConnected`, `EvtDisconnected` |

---

## 并发安全

```go
// ✅ 正确：读锁
func (s *Swarm) ConnsToPeer(peer types.PeerID) []Connection {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.conns[peer]
}

// ✅ 正确：写锁
func (s *Swarm) addConn(conn Connection) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.conns[conn.RemotePeer()] = append(s.conns[conn.RemotePeer()], conn)
}
```

---

## 错误处理

```go
var (
    ErrSwarmClosed   = errors.New("swarm closed")
    ErrDialTimeout   = errors.New("dial timed out")
    ErrAddrFiltered  = errors.New("address filtered")
    ErrNoAddresses   = errors.New("no addresses")
    ErrGaterRejected = errors.New("gater rejected connection")
)
```

---

**最后更新**：2026-01-13
