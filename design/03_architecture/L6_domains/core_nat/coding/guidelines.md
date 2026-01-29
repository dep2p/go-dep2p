# core_nat 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/nat/
├── module.go           # Fx 模块
├── service.go          # NAT 服务
├── detector.go         # NAT 检测
├── upnp.go             # UPnP
├── stun.go             # STUN
├── puncher.go          # 打洞
└── *_test.go           # 测试
```

---

## NAT 穿透策略

### 优先级

```
func (s *Service) establishConnection(peer types.NodeID) error {
    // 1. 尝试直连
    if err := s.directConnect(peer); err == nil {
        return nil
    }
    
    // 2. 尝试打洞
    if err := s.holePunch(peer); err == nil {
        return nil
    }
    
    // 3. 使用中继
    return s.relayConnect(peer)
}
```

### 超时设置

```
const (
    stunTimeout      = 5 * time.Second
    holePunchTimeout = 10 * time.Second
    upnpTimeout      = 5 * time.Second
)
```

---

## 地址管理

### 观察地址

```
// 收集观察到的地址用于打洞
func (s *Service) getObservedAddrs() []multiaddr.Multiaddr {
    addrs := make([]multiaddr.Multiaddr, 0)
    
    // 本地地址
    addrs = append(addrs, s.host.Addrs()...)
    
    // STUN 获取的外部地址
    if ext, err := s.stun.GetExternalAddr(); err == nil {
        addrs = append(addrs, ext)
    }
    
    return addrs
}
```

---

## 并发模式

### 并发打洞

```
func (p *Puncher) punchAll(addrs []multiaddr.Multiaddr) {
    var wg sync.WaitGroup
    
    for _, addr := range addrs {
        wg.Add(1)
        go func(a multiaddr.Multiaddr) {
            defer wg.Done()
            p.sendPunch(a)
        }(addr)
    }
    
    wg.Wait()
}
```

---

**最后更新**：2026-01-11
