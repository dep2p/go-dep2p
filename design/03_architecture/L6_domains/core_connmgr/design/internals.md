# core_connmgr 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/connmgr/
├── module.go           # Fx 模块
├── manager.go          # 连接管理器
├── gater.go            # 连接门控
├── tags.go             # 标签存储
├── protect.go          # 保护存储
├── trim.go             # 回收逻辑
└── manager_test.go     # 测试
```

---

## 标签存储

```
type TagStore struct {
    mu   sync.RWMutex
    tags map[types.NodeID]map[string]int
}

func (s *TagStore) Set(peer types.NodeID, tag string, value int) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.tags[peer] == nil {
        s.tags[peer] = make(map[string]int)
    }
    s.tags[peer][tag] = value
}

func (s *TagStore) Sum(peer types.NodeID) int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    sum := 0
    for _, v := range s.tags[peer] {
        sum += v
    }
    return sum
}
```

---

## 保护存储

```
type ProtectStore struct {
    mu       sync.RWMutex
    protects map[types.NodeID]map[string]struct{}
}

func (s *ProtectStore) Protect(peer types.NodeID, tag string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.protects[peer] == nil {
        s.protects[peer] = make(map[string]struct{})
    }
    s.protects[peer][tag] = struct{}{}
}

func (s *ProtectStore) IsProtected(peer types.NodeID) bool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    return len(s.protects[peer]) > 0
}
```

---

## 回收算法

```
func (m *Manager) TrimOpenConns(ctx context.Context) {
    conns := m.host.Connections()
    
    if len(conns) <= m.lowWater {
        return
    }
    
    // 收集候选
    candidates := make([]peerScore, 0)
    for _, conn := range conns {
        peer := conn.RemotePeer()
        if m.protects.IsProtected(peer) {
            continue // 跳过受保护的
        }
        candidates = append(candidates, peerScore{
            peer:  peer,
            score: m.calculateScore(peer),
        })
    }
    
    // 按分数排序
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].score < candidates[j].score
    })
    
    // 关闭低分连接
    toClose := len(conns) - m.lowWater
    for i := 0; i < toClose && i < len(candidates); i++ {
        m.host.CloseConnection(candidates[i].peer)
    }
}
```

---

## 错误处理

```
var (
    ErrConnectionDenied = errors.New("connmgr: connection denied")
    ErrPeerBlocked      = errors.New("connmgr: peer blocked")
)
```

---

**最后更新**：2026-01-11
