# Core Swarm 内部设计

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 内部结构

```go
type Swarm struct {
    mu sync.RWMutex
    
    // 身份
    localPeer   types.PeerID
    privKey     crypto.PrivKey
    
    // 连接池
    conns       map[types.PeerID][]Connection
    
    // 传输
    transports  []transport.Transport
    
    // 升级器
    upgrader    upgrader.Upgrader
    
    // 监听器
    listeners   []Listener
    
    // 通知器
    notifiers   []Notifiee
    
    // 资源管理
    rcmgr       ResourceManager
    
    // 连接门控
    gater       ConnectionGater
    
    // 拨号配置
    dialTimeout      time.Duration
    dialTimeoutLocal time.Duration
    dialRanker       DialRanker
    
    // 状态
    closed      bool
}
```

---

## 拨号调度器

```go
// DialRanker 对地址进行排序
type DialRanker interface {
    Rank(addrs []types.Multiaddr) []types.Multiaddr
}

// DefaultDialRanker 默认排序策略
// 优先级：本地 > QUIC > TCP > WebSocket > Relay
func DefaultDialRanker(addrs []types.Multiaddr) []types.Multiaddr {
    // 1. 分类
    // 2. 排序
    // 3. 合并
}
```

---

## 并发拨号

```go
// dialWorker 执行并发拨号
func (s *Swarm) dialWorker(ctx context.Context, peer types.PeerID, addrs []types.Multiaddr) (Connection, error) {
    // 限制并发数
    sem := make(chan struct{}, MaxParallelDials)
    
    // 结果通道
    results := make(chan dialResult)
    
    // 并发拨号
    for _, addr := range addrs {
        go func(addr types.Multiaddr) {
            sem <- struct{}{}
            defer func() { <-sem }()
            
            conn, err := s.dial(ctx, peer, addr)
            results <- dialResult{conn, err}
        }(addr)
    }
    
    // 等待第一个成功
    // ...
}
```

---

## 连接状态

```go
type ConnectionStat struct {
    Direction    Direction      // Inbound/Outbound
    Opened       time.Time      // 连接建立时间
    Transient    bool           // 临时连接
    Limited      bool           // 受限连接 (Relay)
    NumStreams   int            // 当前流数
}
```

---

**最后更新**：2026-01-13
