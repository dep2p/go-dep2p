# Core Swarm 设计审查

> **日期**: 2026-01-13  
> **版本**: v1.0.0  
> **审查人**: AI Agent

---

## 一、设计概述

core_swarm 是 DeP2P 的连接群管理核心，负责：
- 管理到所有节点的连接池
- 智能拨号调度（并发拨号、地址排序）
- 监听管理（多地址监听、入站连接接受）
- 流多路复用
- 连接/流事件通知

---

## 二、与 go-libp2p 对比

### 2.1 核心结构对比

| 组件 | go-libp2p | DeP2P v1.0 | 说明 |
|------|-----------|------------|------|
| 主结构 | `Swarm` | `Swarm` | 相同 |
| 连接池 | `map[peer.ID][]Conn` | `map[string][]Connection` | 相同设计 |
| 拨号调度 | `dialWorker` | `dialWorker` | 采用 |
| 地址排序 | `DialRanker` | `rankAddrs` | 简化版 |
| 黑洞检测 | `BlackHoleDetector` | ⬜ v1.1+ | 暂不实现 |
| 资源管理 | `ResourceManager` | ⬜ v1.1+ | 暂不实现 |

### 2.2 拨号流程对比

**go-libp2p 拨号流程**:
```
DialPeer
  → dialSync (去重)
  → dialWorker (并发拨号)
    → dialRanker (地址排序)
    → blackHoleDetector (黑洞检测)
    → limiter (并发限制)
    → Transport.Dial
    → Upgrader.Upgrade
```

**DeP2P v1.0 拨号流程**:
```
DialPeer
  → 检查连接池（复用）
  → PeerStore.Addrs（获取地址）
  → rankAddrs（地址排序）
  → dialWorker（并发拨号）
    → Transport.Dial
    → Upgrader.Upgrade
```

**简化点**:
- 无 dialSync（暂不处理多个协程同时拨号同一节点的去重）
- 无黑洞检测（v1.1+ 实现）
- 无资源管理器（v1.1+ 实现）

### 2.3 监听流程对比

**go-libp2p**:
```
Listen
  → 按 ListenOrder 排序传输层
  → 依次创建 Listener
  → AcceptLoop
    → Accept
    → Upgrader.Upgrade
    → 添加到连接池
```

**DeP2P v1.0**:
```
Listen
  → 选择传输层
  → 创建 Listener
  → AcceptLoop
    → Accept
    → Upgrader.Upgrade
    → 添加到连接池
```

**相同点**: 核心流程一致

---

## 三、核心设计决策

### ADR-1: 连接池使用 map[string][]Connection

**决策**: 每个节点可以有多个连接

**理由**:
- 支持多地址连接（QUIC + TCP）
- 连接失败时可以快速切换
- 与 go-libp2p 设计一致

**实现**:
```go
type Swarm struct {
    mu    sync.RWMutex
    conns map[string][]Connection  // peerID -> []Connection
}

func (s *Swarm) ConnsToPeer(peer string) []Connection {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.conns[peer]
}
```

### ADR-2: 并发拨号使用 dialWorker

**决策**: 采用 go-libp2p 的 dialWorker 模式

**理由**:
- 并发尝试多个地址，第一个成功胜出
- 使用 context 控制超时和取消
- 避免重复拨号同一地址

**实现**:
```go
func (s *Swarm) dialWorker(ctx context.Context, peer string, addrs []string) (Connection, error) {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()
    
    results := make(chan dialResult, len(addrs))
    
    for _, addr := range addrs {
        go func(addr string) {
            conn, err := s.dialAddr(ctx, peer, addr)
            results <- dialResult{conn, err}
        }(addr)
    }
    
    // 等待第一个成功或全部失败
    var errs []error
    for i := 0; i < len(addrs); i++ {
        select {
        case res := <-results:
            if res.err == nil {
                return res.conn, nil  // 第一个成功
            }
            errs = append(errs, res.err)
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
    
    return nil, fmt.Errorf("all dials failed: %v", errs)
}
```

### ADR-3: 地址排序优先级

**决策**: 本地网络 > QUIC > TCP

**理由**:
- 本地网络延迟最低
- QUIC 性能优于 TCP（0-RTT、多路复用）
- TCP 作为兜底选项

**实现**:
```go
func rankAddrs(addrs []string) []string {
    var local, quic, tcp []string
    
    for _, addr := range addrs {
        if isPrivate(addr) {
            local = append(local, addr)
        } else if strings.Contains(addr, "/quic") {
            quic = append(quic, addr)
        } else if strings.Contains(addr, "/tcp") {
            tcp = append(tcp, addr)
        }
    }
    
    return append(append(local, quic...), tcp...)
}
```

### ADR-4: 事件通知使用观察者模式

**决策**: 异步通知，不阻塞主流程

**理由**:
- 通知器可能执行耗时操作
- 避免死锁
- 与 go-libp2p 一致

**实现**:
```go
func (s *Swarm) notifyConnected(conn Connection) {
    s.mu.RLock()
    notifiers := append([]SwarmNotifier(nil), s.notifiers...)
    s.mu.RUnlock()
    
    for _, n := range notifiers {
        go n.Connected(conn)  // 异步通知
    }
}
```

### ADR-5: v1.0 简化设计

**决策**: 暂不实现黑洞检测、资源管理、dialSync

**理由**:
- 降低初期复杂度
- 专注核心功能
- 后续版本可扩展

**v1.0 范围**:
- ✅ 连接池管理
- ✅ 拨号调度（并发拨号、地址排序）
- ✅ 监听管理
- ✅ 流管理
- ✅ 事件通知

**v1.1+ 计划**:
- ⬜ 黑洞检测
- ⬜ 资源管理器集成
- ⬜ dialSync 去重
- ⬜ 拨号退避（Backoff）

---

## 四、核心接口设计

### 4.1 Swarm 接口

```go
type Swarm interface {
    // 身份
    LocalPeer() string
    
    // 连接管理
    Peers() []string
    Conns() []Connection
    ConnsToPeer(peer string) []Connection
    Connectedness(peer string) Connectedness
    
    // 拨号
    DialPeer(ctx context.Context, peer string) (Connection, error)
    
    // 流
    NewStream(ctx context.Context, peer string) (Stream, error)
    
    // 通知
    Notify(notifier SwarmNotifier)
    
    // 生命周期
    Close() error
}
```

### 4.2 内部结构

```go
type Swarm struct {
    mu          sync.RWMutex
    localPeer   string
    
    // 连接池
    conns       map[string][]Connection
    
    // 传输和升级
    transports  map[string]Transport
    upgrader    Upgrader
    
    // 监听
    listeners   []Listener
    
    // 通知
    notifiers   []SwarmNotifier
    
    // 依赖
    peerstore   Peerstore
    connmgr     ConnManager
    eventbus    EventBus
    
    // 配置
    config      *Config
    
    // 状态
    closed      atomic.Bool
}
```

---

## 五、依赖关系

```
Swarm 依赖：
  ├── Transport（传输层）
  │   ├── QUIC Transport
  │   └── TCP Transport
  ├── Upgrader（连接升级）
  │   ├── Security (TLS/Noise)
  │   └── Muxer (Yamux)
  ├── Peerstore（节点存储）
  ├── ConnMgr（连接管理器）
  └── EventBus（事件总线）
```

---

## 六、实现要点

### 6.1 并发安全

- 所有公共方法使用 `sync.RWMutex` 保护
- 读操作使用 `RLock`，写操作使用 `Lock`
- 事件通知不持有锁

### 6.2 资源清理

- `Close()` 关闭所有连接和监听器
- 使用 `atomic.Bool` 标记关闭状态
- 关闭后拒绝新操作

### 6.3 错误处理

- 定义 sentinel errors
- 拨号失败收集所有错误
- 超时使用 context

### 6.4 测试要点

- 拨号-接受完整流程
- 并发拨号
- 连接复用
- 事件通知
- 资源清理

---

## 七、go-libp2p 代码参考

### 7.1 关键文件

| 文件 | 行数 | 用途 |
|------|------|------|
| `swarm.go` | ~1000 | 主结构和基础方法 |
| `swarm_dial.go` | ~740 | 拨号逻辑 |
| `swarm_listen.go` | ~200 | 监听逻辑 |
| `dial_worker.go` | ~500 | 并发拨号工作器 |
| `dial_ranker.go` | ~200 | 地址排序 |
| `swarm_conn.go` | ~300 | 连接封装 |
| `swarm_stream.go` | ~200 | 流封装 |

**总计**: ~3100 行

### 7.2 DeP2P v1.0 预估

| 组件 | 预估行数 |
|------|----------|
| `swarm.go` | ~300 |
| `dial.go` | ~250 |
| `listen.go` | ~150 |
| `conn.go` | ~150 |
| `stream.go` | ~100 |
| `notifier.go` | ~50 |
| `config.go` | ~40 |
| `errors.go` | ~30 |
| `module.go` | ~80 |
| `doc.go` | ~150 |
| `testing.go` | ~100 |

**总计**: ~1400 行（约为 go-libp2p 的 45%）

---

## 八、实现步骤

1. ✅ 设计审查（本文档）
2. 接口验证
3. 测试先行
4. Swarm 主结构
5. 拨号实现
6. 监听实现
7. 连接封装
8. 流封装
9. 通知器
10. 配置和错误
11. Fx 模块
12. 测试通过
13. 文档清理

---

## 九、风险和缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 并发拨号死锁 | 高 | 使用 context 超时、充分测试 |
| 连接泄漏 | 高 | 完善资源清理、集成测试 |
| 地址排序不优 | 中 | 参考 go-libp2p、可配置 |
| 测试覆盖不足 | 中 | 集成测试、并发测试 |

---

## 十、总结

**设计优势**:
- ✅ 清晰的分层架构
- ✅ 参考成熟的 go-libp2p 设计
- ✅ 简化的 v1.0 范围
- ✅ 良好的扩展性

**实现重点**:
- 并发安全
- 资源管理
- 错误处理
- 完善测试

**后续演进**:
- v1.0: 核心功能
- v1.1: 黑洞检测、资源管理
- v2.0: 性能优化、指标收集

---

**审查完成日期**: 2026-01-13  
**下一步**: 接口验证
