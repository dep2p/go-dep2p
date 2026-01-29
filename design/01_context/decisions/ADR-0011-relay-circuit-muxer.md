# ADR-0011: 中继电路多路复用架构

## 元数据

| 属性 | 值 |
|------|-----|
| **ID** | ADR-0011 |
| **标题** | 中继电路多路复用架构 (Relay Circuit Muxer Architecture) |
| **状态** | accepted |
| **决策日期** | 2026-01-27 |
| **更新日期** | 2026-01-27 |
| **决策者** | DeP2P 核心团队 |
| **关联 ADR** | [ADR-0003](ADR-0003-relay-first-connect.md), [ADR-0010](ADR-0010-relay-explicit-config.md) |
| **关联需求** | [REQ-NET-003](../requirements/functional/F3_network/REQ-NET-003.md) |

---

## 上下文

### 问题背景

在 v0.2.25 版本日志分析中发现，Relay 中继连接存在严重的架构缺陷：

1. **BUG-1: `MaxCircuitsPerPeer=0` 导致所有 CONNECT 请求被拒绝**
   - 配置 `Limits.MaxConcurrent` 默认值为 0（注释说"表示不限制"）
   - 但 `server.go` 检查逻辑 `srcCircuits >= maxCircuitsPerPeer` 在 `maxCircuitsPerPeer=0` 时恒为 true
   - 导致所有中继 CONNECT 请求被拒绝

2. **设计意图与实际实现的鸿沟**

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    旧设计讨论中遗漏的核心问题                                     │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  设计意图（relay.md）：                                                          │
│  ─────────────────────                                                          │
│  "Relay 建立的是透明双向隧道，具体协议协商在 stream 建立后才发生"               │
│                                                                                 │
│  实际实现（client.go）：                                                         │
│  ─────────────────────────                                                      │
│  func (c *RelayClient) Connect(...) (transportif.Conn, error) {                 │
│      stream, err := conn.OpenStream(ctx, ProtocolRelayHop)                     │
│      // ... STOP 握手 ...                                                       │
│      return &relayedConn{                                                       │
│          stream: stream,  // ⚠️ 单流！协议协商后流被关闭！                       │
│      }, nil                                                                     │
│  }                                                                              │
│                                                                                 │
│  问题：设计说"透明隧道"，但实现是"单流连接"                                      │
│       流关闭后，"隧道"就断了，无法再次使用                                       │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 核心认知

旧代码已经解决了流的半关闭问题（`CloseWrite()`/`CloseRead()`），但那是针对**普通流**的语义，**从未解决中继电路的多路复用问题**。

---

## 考虑的选项

### 选项 1: 单流连接模型（旧实现）❌ 不采用

每个中继连接使用单一流，流关闭即电路失效。

**优点**:
- 实现简单
- 资源占用少

**缺点**:
- 协议处理完成后流关闭导致电路失效
- 无法复用已建立的中继电路
- 每次通信需要重新建立中继电路
- 违背"透明隧道"设计意图

### 选项 2: 电路级 Muxer 架构 ✅ 采用

在 STOP 流上叠加 yamux Muxer，实现多路复用。

**优点**:
- 流关闭不影响电路
- 可以在同一电路上打开多个流
- 真正实现"透明隧道"语义
- 电路有独立的生命周期管理

**缺点**:
- 实现复杂度增加
- 需要管理 Muxer 状态

---

## 决策结果

选择 **选项 2: 电路级 Muxer 架构**。

### 核心决策

> **中继电路在 STOP 握手完成后，叠加 yamux Muxer 实现多路复用。流的生命周期独立于电路的生命周期。**

---

## 架构设计

### RelayCircuit 结构

```
┌───────────────────────────────────────────────────────────────────────────┐
│                     RelayCircuit（新设计）                                 │
│                                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  Control Channel (控制通道)                                          │  │
│  │  ─────────────────────────                                           │  │
│  │  • STOP 握手完成后保持                                               │  │
│  │  • KeepAlive 心跳                                                    │  │
│  │  • 电路状态同步                                                      │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  Data Channel (数据通道) - 通过 Muxer 实现                           │  │
│  │  ──────────────────────────────────────────                          │  │
│  │                                                                      │  │
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐                       │  │
│  │  │  Stream 1  │ │  Stream 2  │ │  Stream N  │  ← 独立生命周期        │  │
│  │  │  (chat)    │ │  (pubsub)  │ │  (dht)     │                       │  │
│  │  └────────────┘ └────────────┘ └────────────┘                       │  │
│  │                                                                      │  │
│  │  • 每个流可以独立 CloseWrite/CloseRead                               │  │
│  │  • 流关闭不影响电路                                                  │  │
│  │  • 可以随时 OpenStream() 创建新流                                    │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  Circuit State Machine (状态机)                                      │  │
│  │  ─────────────────────────────                                       │  │
│  │                                                                      │  │
│  │   ┌────────┐   STOP OK   ┌────────┐   心跳超时   ┌────────┐         │  │
│  │   │Creating│ ─────────→ │ Active │ ─────────→  │ Stale  │         │  │
│  │   └────────┘             └────────┘             └────────┘         │  │
│  │                               │                      │              │  │
│  │                               │ 配额耗尽/主动关闭     │ 重连成功     │  │
│  │                               ▼                      │              │  │
│  │                          ┌────────┐                  │              │  │
│  │                          │ Closed │ ←────────────────┘              │  │
│  │                          └────────┘   重连失败                       │  │
│  │                                                                      │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
```

### 代码结构

```go
// RelayCircuit 中继电路
type RelayCircuit struct {
    // 控制通道
    controlStream pkgif.Stream  // STOP 握手后保持
    
    // 数据通道 Muxer（关键改动！）
    muxer         muxerif.MuxedConn  // yamux over relay stream
    
    // 电路元信息
    relayPeer     types.PeerID
    remotePeer    types.PeerID
    localPeer     types.PeerID
    
    // 状态
    state         CircuitState
    stateMu       sync.RWMutex
    
    // 配额
    bytesUsed     int64
    maxBytes      int64
    deadline      time.Time
    
    // 心跳
    lastActivity  time.Time
    keepAliveTick *time.Ticker
}

// NewStream 创建新的逻辑流（关键方法！）
func (c *RelayCircuit) NewStream(ctx context.Context) (pkgif.Stream, error) {
    c.stateMu.RLock()
    if c.state != CircuitStateActive {
        c.stateMu.RUnlock()
        return nil, ErrCircuitNotActive
    }
    c.stateMu.RUnlock()
    
    // 通过 muxer 创建新流
    return c.muxer.OpenStream(ctx)
}

// OpenStream 实现 Connection 接口
func (c *RelayCircuit) OpenStream(ctx context.Context, proto string) (pkgif.Stream, error) {
    stream, err := c.NewStream(ctx)
    if err != nil {
        return nil, err
    }
    // 设置协议
    stream.SetProtocol(proto)
    return stream, nil
}
```

### 创建电路流程

```go
// Client 端创建电路
func (c *Client) Connect(ctx context.Context, target types.PeerID) (*RelayCircuit, error) {
    // ... STOP 握手 ...
    
    // 创建流到 net.Conn 的适配器
    netConn := newStreamNetConn(stream)
    
    // 创建 yamux muxer
    transport := muxer.NewTransport()
    muxedConn, err := transport.NewConn(netConn, false /* isClient */, nil)
    if err != nil {
        stream.Close()
        return nil, err
    }
    
    circuit := NewRelayCircuit(stream, muxedConn, localPeer, target, relayPeer)
    return circuit, nil
}
```

---

## 流与电路的语义

### 关键不变量

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    流与电路的正确语义                                             │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  1. 流的 CloseWrite() 只影响该流，不影响电路                                     │
│     stream.CloseWrite()  →  对端 Read() 返回 EOF                               │
│     电路状态不变，其他流不受影响                                                 │
│                                                                                 │
│  2. 流的 Close() 只影响该流，不影响电路                                          │
│     stream.Close()  →  该流完全关闭                                             │
│     电路状态不变，可以继续 OpenStream()                                          │
│                                                                                 │
│  3. 电路的 Close() 关闭所有流                                                    │
│     circuit.Close()  →  所有流收到 EOF/Reset                                   │
│     Swarm 中该连接被移除                                                        │
│                                                                                 │
│  4. 电路只有在以下情况才关闭：                                                   │
│     • 显式调用 circuit.Close()                                                  │
│     • 配额耗尽（MaxBytes/MaxDuration）                                          │
│     • 心跳超时（连续 N 次无响应）                                               │
│     • Relay Server 主动关闭                                                     │
│     • 底层网络故障                                                              │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 错误 vs 正确做法

| 场景 | 错误做法（旧实现）| 正确做法（新架构）|
|------|------------------|------------------|
| 协议处理完成 | stream.Close() → 电路失效 | stream.Close() → 仅该流关闭，电路仍活跃 |
| 需要新通信 | 重新建立中继电路 | 调用 circuit.OpenStream() |
| 连接检查 | 检查单流状态 | 检查电路状态 |

---

## Swarm 集成

```go
// Swarm 需要正确识别 RelayCircuit
type Swarm struct {
    // 直连
    conns    map[types.NodeID][]pkgif.Connection
    
    // 中继电路
    circuits map[types.NodeID][]*RelayCircuit
}

// ConnsToPeer 返回到指定节点的所有连接（包括直连和中继）
func (s *Swarm) ConnsToPeer(peerID types.NodeID) []pkgif.Connection {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    var result []pkgif.Connection
    
    // 直连
    result = append(result, s.conns[peerID]...)
    
    // 中继电路（实现 Connection 接口）
    for _, circuit := range s.circuits[peerID] {
        if circuit.State() == CircuitStateActive {
            result = append(result, circuit)
        }
    }
    
    return result
}
```

---

## 生命周期管理

### 与 NodeLifecycle 集成

```
┌────────────────────────────────────────────────────────────────────┐
│  CircuitHealthChecker                                               │
│                                                                     │
│  间隔: 30s                                                          │
│  任务:                                                              │
│  1. 检查所有电路的 lastActivity                                     │
│  2. 发送 KeepAlive 到 Stale 电路                                    │
│  3. 关闭无响应的电路                                                 │
│  4. 如果 Active 电路 < 阈值，触发 RelayManager 重建                  │
└────────────────────────────────────────────────────────────────────┘
```

### 优雅关闭

```
node.Close() → RelayCircuit.GracefulClose() →
  • 停止 KeepAlive
  • 等待活跃流完成（带超时）
  • 关闭 muxer
  • 关闭控制流
```

---

## 后果

### 正面后果

1. **真正实现"透明隧道"语义**
   - 流关闭不影响电路
   - 可以复用已建立的中继电路

2. **资源效率提升**
   - 减少重复建立中继电路的开销
   - 单个电路支持多个并发流

3. **与直连语义一致**
   - RelayCircuit 实现 Connection 接口
   - Swarm 可以统一管理

4. **明确的生命周期**
   - 电路有独立的状态机
   - 支持心跳和健康检查

### 负面后果

1. **实现复杂度增加**
   - 需要管理 Muxer 状态
   - 需要处理 Muxer 错误

2. **内存开销**
   - 每个电路需要维护 Muxer 状态
   - 需要缓冲区

### 缓解措施

| 负面后果 | 缓解措施 |
|----------|----------|
| 实现复杂度 | 复用 yamux 成熟实现 |
| 内存开销 | 设置合理的缓冲区限制 |

---

## 修复清单

### Phase 1: 核心架构（必须）

- [x] 1.1 修复 MaxCircuitsPerPeer=0 导致 CONNECT 全部失败
- [x] 1.2 定义 RelayCircuit 结构（控制通道 + Muxer）
- [x] 1.3 修改 Client.Connect 返回 RelayCircuit
- [ ] 1.4 修改 Server 端 HandleStopConnect
- [ ] 1.5 修改 Swarm 管理 RelayCircuit

### Phase 2: 生命周期管理

- [ ] 2.1 电路心跳（KeepAlive 协议）
- [ ] 2.2 配额管理（流量计数、超限关闭）
- [ ] 2.3 集成 NodeLifecycle

### Phase 3: 可选优化

- [ ] 3.1 打洞后电路作为热备
- [ ] 3.2 电路池/连接池
- [ ] 3.3 统计和监控

---

## 相关文档

| 类型 | 链接 |
|------|------|
| **概念澄清** | [NAT/Relay 概念澄清 §19](../../_discussions/20260123-nat-relay-concept-clarification.md) |
| **生命周期** | [节点生命周期横切面 §11](../../_discussions/20260125-node-lifecycle-cross-cutting.md) |
| **需求** | [REQ-NET-003](../requirements/functional/F3_network/REQ-NET-003.md): Relay 中继 |
| **ADR** | [ADR-0003](ADR-0003-relay-first-connect.md): 惰性中继策略 |
| **ADR** | [ADR-0010](ADR-0010-relay-explicit-config.md): Relay 明确配置 |

---

## 变更历史

| 日期 | 版本 | 变更说明 |
|------|------|----------|
| 2026-01-27 | 1.0 | 初始版本：定义中继电路多路复用架构 |
