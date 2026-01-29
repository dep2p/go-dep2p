# core_realm 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/realm/
├── module.go           # Fx 模块
├── manager.go          # Realm 管理器
├── realm.go            # Realm 实现
├── auth.go             # 认证逻辑
├── member.go           # 成员缓存
├── keys.go             # 密钥派生
└── realm_test.go       # 测试
```

---

## 加入 Realm 流程

```
func (m *Manager) Join(ctx context.Context, name string, psk types.PSK) (Realm, error) {
    // 1. 如果已加入其他 Realm，先离开
    if m.current != nil {
        if err := m.Leave(ctx); err != nil {
            return nil, err
        }
    }
    
    // 2. 派生密钥
    realmID := deriveRealmID(psk)
    authKey := deriveAuthKey(psk)
    
    // 3. 创建 Realm 实例
    realm := &realmImpl{
        name:    name,
        id:      realmID,
        psk:     psk,
        authKey: authKey,
        members: newMemberCache(),
    }
    
    // 4. 注册到 Rendezvous
    ns := fmt.Sprintf("/dep2p/realm/%s", realmID.String())
    m.discovery.Advertise(ctx, ns, discovery.TTL(time.Hour))
    
    // 5. 发现现有成员
    go realm.discoverMembers(ctx)
    
    m.current = realm
    return realm, nil
}
```

---

## 成员认证

### 挑战-响应协议

```
func (a *Authenticator) Authenticate(ctx context.Context, peer types.NodeID) error {
    stream, err := a.host.NewStream(ctx, peer, authProtocol)
    if err != nil {
        return err
    }
    defer stream.Close()
    
    // 1. 发送认证请求
    req := &pb.AuthRequest{RealmID: a.realmID.Bytes()}
    writeMsg(stream, req)
    
    // 2. 接收挑战
    challenge := &pb.AuthChallenge{}
    readMsg(stream, challenge)
    
    // 3. 计算响应
    mac := hmac.New(sha256.New, a.authKey)
    mac.Write(a.localID.Bytes())
    mac.Write(a.realmID.Bytes())
    mac.Write(challenge.Nonce)
    response := mac.Sum(nil)
    
    // 4. 发送响应
    resp := &pb.AuthResponse{MAC: response}
    writeMsg(stream, resp)
    
    // 5. 接收结果
    result := &pb.AuthResult{}
    readMsg(stream, result)
    
    if !result.Success {
        return ErrAuthFailed
    }
    
    return nil
}
```

---

## 成员缓存

```
type MemberCache struct {
    mu      sync.RWMutex
    members map[types.NodeID]*Membership
}

func (c *MemberCache) Add(id types.NodeID) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    c.members[id] = &Membership{
        NodeID:   id,
        JoinTime: time.Now(),
        LastSeen: time.Now(),
    }
}

func (c *MemberCache) IsMember(id types.NodeID) bool {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    _, ok := c.members[id]
    return ok
}
```

---

## 错误处理

```
var (
    ErrNotJoined     = errors.New("realm: not joined any realm")
    ErrAlreadyJoined = errors.New("realm: already joined a realm")
    ErrAuthFailed    = errors.New("realm: authentication failed")
    ErrNotMember     = errors.New("realm: peer is not a member")
    ErrInvalidPSK    = errors.New("realm: invalid PSK")
)
```

---

**最后更新**：2026-01-11
