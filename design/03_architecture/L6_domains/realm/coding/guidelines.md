# core_realm 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/realm/
├── module.go           # Fx 模块
├── manager.go          # 管理器
├── realm.go            # Realm 实现
├── auth.go             # 认证
├── member.go           # 成员缓存
├── keys.go             # 密钥派生
└── *_test.go           # 测试
```

---

## Realm 使用规范

### 正确的生命周期

```
// 加入 Realm
realm, err := node.JoinRealm(ctx, "my-realm", psk)
if err != nil {
    return err
}
defer realm.Leave(ctx)

// 使用 Realm 服务
realm.Messaging().Send(ctx, target, proto, data)
```

### 成员资格检查

```
// 在业务逻辑中检查成员资格 (INV-002)
func handleMessage(realm Realm, from types.NodeID, msg []byte) error {
    if !realm.IsMember(from) {
        return ErrNotMember
    }
    // 处理消息
}
```

---

## 并发模式

### 成员缓存并发安全

```
// MemberCache 使用读写锁
type MemberCache struct {
    mu sync.RWMutex
    members map[types.NodeID]*Membership
}

// 读操作使用 RLock
func (c *MemberCache) IsMember(id types.NodeID) bool {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.members[id] != nil
}

// 写操作使用 Lock
func (c *MemberCache) Add(id types.NodeID) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.members[id] = &Membership{}
}
```

---

## 安全规范

### PSK 处理

```
// PSK 不要打印到日志
// BAD:
log.Printf("joining realm with PSK: %x", psk)

// GOOD:
log.Printf("joining realm: %s", realmID.Pretty())
```

---

**最后更新**：2026-01-11
