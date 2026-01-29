# core_discovery 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/discovery/
├── module.go           # Fx 模块
├── service.go          # 组合服务
├── dht.go              # DHT
├── mdns.go             # mDNS
├── rendezvous.go       # Rendezvous
└── *_test.go           # 测试
```

---

## 发现使用规范

### 正确使用 FindPeers

```
func discoverPeers(ctx context.Context, svc discovery.Service, ns string) {
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()
    
    peers, err := svc.FindPeers(ctx, ns, discovery.Limit(50))
    if err != nil {
        return
    }
    
    for peer := range peers {
        // 处理发现的节点
        handlePeer(peer)
    }
}
```

### 公告管理

```
func advertiseLoop(ctx context.Context, svc discovery.Service, ns string) {
    for {
        ttl, err := svc.Advertise(ctx, ns, discovery.TTL(time.Hour))
        if err != nil {
            time.Sleep(time.Minute)
            continue
        }
        
        // 在 TTL 过期前重新公告
        select {
        case <-time.After(ttl * 7 / 10):
            continue
        case <-ctx.Done():
            return
        }
    }
}
```

---

## 命名空间规范

| 用途 | 命名空间格式 | 示例 |
|------|--------------|------|
| Realm 发现 | `/dep2p/realm/<realmID>` | `/dep2p/realm/abc123` |
| 应用发现 | `/dep2p/app/<name>` | `/dep2p/app/chat` |

---

## 并发模式

### 并行发现

```
func discoverAll(ctx context.Context, sources []discovery.Service, ns string) []PeerInfo {
    var wg sync.WaitGroup
    results := make(chan PeerInfo, 100)
    
    for _, src := range sources {
        wg.Add(1)
        go func(s discovery.Service) {
            defer wg.Done()
            peers, _ := s.FindPeers(ctx, ns)
            for p := range peers {
                results <- p
            }
        }(src)
    }
    
    go func() {
        wg.Wait()
        close(results)
    }()
    
    return dedup(collectResults(results))
}
```

---

**最后更新**：2026-01-11
