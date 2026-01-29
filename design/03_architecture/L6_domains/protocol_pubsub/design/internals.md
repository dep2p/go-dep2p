# Protocol PubSub 实现细节

> **组件**: P6-02 protocol_pubsub

---

## 关键修复

### 1. Heartbeat Goroutine清理

**问题**: goroutine泄漏

**修复**:
```go
type heartbeat struct {
    wg sync.WaitGroup  // 新增
}

func (hb *heartbeat) Start() {
    hb.wg.Add(1)
    go hb.run()
}

func (hb *heartbeat) Stop() {
    hb.cancel()
    hb.wg.Wait()  // 等待退出
}
```

### 2. Join() 锁优化

**修复**: double-check pattern

```go
func (gs *gossipSub) Join(name string) (*topic, error) {
    // 读锁检查
    gs.mu.RLock()
    _, exists := gs.topics[name]
    gs.mu.RUnlock()
    
    if exists {
        return nil, ErrTopicAlreadyJoined
    }
    
    // 写锁创建
    gs.mu.Lock()
    // double-check
    if _, exists := gs.topics[name]; exists {
        gs.mu.Unlock()
        return nil, ErrTopicAlreadyJoined
    }
    t := newTopic(name, nil, gs)
    gs.topics[name] = t
    gs.mu.Unlock()
    
    return t, nil
}
```

### 3. Stop() 死锁修复 ⭐

**问题**: 持有锁时调用topic.Close() → Leave()

**修复**:
```go
func (gs *gossipSub) Stop() error {
    gs.mu.Lock()
    
    // 复制topics
    topics := make([]*topic, 0, len(gs.topics))
    for _, t := range gs.topics {
        topics = append(topics, t)
    }
    gs.mu.Unlock()  // 释放锁
    
    // 不持有锁时关闭
    for _, topic := range topics {
        topic.Close()
    }
    
    return nil
}
```

---

## 测试模式

添加 `WithDisableHeartbeat(true)` 用于测试,避免goroutine干扰。

---

**最后更新**: 2026-01-14
