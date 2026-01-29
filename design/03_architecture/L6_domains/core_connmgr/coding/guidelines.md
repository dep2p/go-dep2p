# core_connmgr 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/connmgr/
├── module.go           # Fx 模块
├── manager.go          # 管理器
├── gater.go            # 门控
├── tags.go             # 标签
├── protect.go          # 保护
├── trim.go             # 回收
└── *_test.go           # 测试
```

---

## 保护使用规范

### 正确使用保护

```
// 保护关键连接
func (s *Service) handleImportantPeer(peer types.NodeID) {
    // 保护
    s.connMgr.Protect(peer, "important-peer")
    defer s.connMgr.Unprotect(peer, "important-peer")
    
    // 业务逻辑
}
```

### 常用保护标签

| 标签 | 用途 |
|------|------|
| `dht-routing` | DHT 路由表节点 |
| `realm-member` | Realm 成员 |
| `relay-active` | 活跃中继 |
| `bootstrap` | 引导节点 |

---

## 并发模式

### 标签操作

```
// 标签操作是并发安全的
go s.connMgr.TagPeer(peer1, "score", 10)
go s.connMgr.TagPeer(peer2, "score", 20)
```

### 回收操作

```
// 回收在后台异步执行
go s.connMgr.TrimOpenConns(ctx)
```

---

## 门控实现

```
type myGater struct {
    blacklist map[types.NodeID]struct{}
}

func (g *myGater) InterceptPeerDial(peer types.NodeID) bool {
    _, blocked := g.blacklist[peer]
    return !blocked
}

func (g *myGater) InterceptAccept(conn net.Conn) bool {
    // 可以基于 IP 过滤
    return true
}
```

---

**最后更新**：2026-01-11
