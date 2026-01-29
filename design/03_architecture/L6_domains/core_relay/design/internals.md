# core_relay 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/relay/
├── module.go           # Fx 模块
├── doc.go              # 模块文档
├── manager.go          # 中继管理器
├── service.go          # ★ 统一 Relay 服务（替代 system.go + realm.go）
├── client.go           # Relay 客户端
├── server.go           # Relay 服务端
├── addressbook.go      # ★ 地址簿（第一层作用）
├── signaling.go        # ★ 信令通道（第二层作用）
├── selector.go         # 中继选择
├── limiter.go          # 资源限制（统一）
├── reservation.go      # ★ Relay 预留管理（TTL/续期）
└── relay_test.go       # 测试
```

---

## 中继选择算法

```pseudocode
// ★ 多 Relay 选择原则：发布哪个 Relay 就用哪个
fn SelectRelay(target: NodeID) -> Option<RelayInfo> {
    // 1. 查询 target 发布的 Relay 地址
    entry := addressBook.Query(target)
    if entry.relayAddr != nil {
        // 使用 target 发布的 Relay
        return Some(getRelayInfo(entry.relayAddr))
    }
    
    // 2. 如果没有发布，使用本地配置的 Relay
    if self.relay.IsConfigured() {
        return Some(self.relay.info)
    }
    
    return None
}

// 多 Relay 场景下的选择（如果配置了多个备选 Relay）
fn SelectBest(relays: []RelayInfo, target: NodeID) -> RelayInfo {
    best := relays[0]
    bestScore := 0
    
    for relay in relays {
        score := calculateScore(relay, target)
        if score > bestScore {
            bestScore = score
            best = relay
        }
    }
    
    return best
}

fn calculateScore(relay: RelayInfo, target: NodeID) -> int {
    score := 100
    
    // 延迟评分（通常高于直连，但不必然"高"）
    if relay.latency < 50ms {
        score += 30
    } else if relay.latency < 100ms {
        score += 20
    }
    
    // 容量评分
    score += int(relay.capacity * 20)
    
    // 可靠性
    score += int(relay.reliability * 20)
    
    return score
}
```

---

## 资源限制

### ★ 统一 Relay 限制器

**设计原则：默认不限制，由提供者决定**

```pseudocode
// RelayConfig - 统一配置（默认不限制）
struct RelayConfig {
    maxBandwidth: int64           // 0 = 不限制（默认）
    maxBandwidthPerConn: int64    // 0 = 不限制
    maxConnections: int           // 默认 1024
    maxConnectionsPerPeer: int    // 0 = 不限制
    idleTimeout: Duration         // 0 = 不超时
    
    // ★ 预留配置
    reservationTTL: Duration      // 默认 1h
    renewalInterval: Duration     // 默认 30min
    renewalWindow: Duration       // 续期窗口 5min
    maxRenewalFailures: int       // 最大续期失败次数 3
}

struct RelayLimiter {
    config: RelayConfig
    reservations: Map<NodeID, Reservation>
}

fn Allow(req: Request) -> Result<(), Error> {
    // 如果配置为 0（默认），不做任何限制
    if self.config.maxBandwidth > 0 && req.bandwidth > self.config.maxBandwidth {
        return Err(ErrBandwidthExceeded)
    }
    
    // 检查预留
    if !self.hasValidReservation(req.nodeID) {
        return Err(ErrNoReservation)
    }
    
    // 让物理带宽成为自然上限
    return Ok(())
}

fn hasValidReservation(nodeID: NodeID) -> bool {
    reservation := self.reservations.get(nodeID)
    if reservation == nil {
        return false
    }
    return reservation.expiresAt > now()
}
```

**为什么默认不限制**：
- 中继的职责是【尽可能转发】，不是【限制流量】
- 让提供者的物理带宽成为自然上限
- 业务限制是业务层面的事，不是中继层面的事

---

## 故障转移

```pseudocode
fn DialWithFailover(target: NodeID) -> Result<Connection, Error> {
    // ★ 地址更新失败重试策略（指数退避）
    retryConfig := RetryConfig {
        initialDelay: 1s,
        multiplier: 2,
        maxDelay: 30s,
        maxRetries: 5,
        jitterFactor: 0.2,    // ±20%
    }
    
    relays := GetAvailableRelays()
    
    for i := 0; i < retryConfig.maxRetries; i++ {
        relay := selector.SelectBest(relays, target)
        
        conn := DialViaRelay(ctx, relay.id, target)
        if conn.is_ok() {
            return conn
        }
        
        markFailed(relay.id)
        relays = remove(relays, relay)
        
        // 指数退避
        delay := min(
            retryConfig.initialDelay * pow(retryConfig.multiplier, i),
            retryConfig.maxDelay
        )
        // 添加抖动
        delay = delay * (1 + random(-retryConfig.jitterFactor, retryConfig.jitterFactor))
        sleep(delay)
    }
    
    return Err(ErrAllRelaysFailed)
}
```

---

## 健康检查

```pseudocode
fn healthCheckLoop() {
    ticker := NewTicker(30s)
    
    for _ in ticker {
        for relay in relays.All() {
            spawn checkHealth(relay)
        }
    }
}

fn checkHealth(relay: RelayInfo) {
    ctx := WithTimeout(Background(), 10s)
    
    start := now()
    err := ping(ctx, relay.id)
    latency := now() - start
    
    if err != nil {
        markUnavailable(relay.id)
        
        // ★ 检查是否需要续期
        if shouldRenew(relay) {
            renewReservation(relay)
        }
    } else {
        updateLatency(relay.id, latency)
    }
}

// ★ Relay 预留续期
fn renewReservation(relay: RelayInfo) -> Result<(), Error> {
    reservation := reservations.get(relay.id)
    if reservation == nil {
        return Err(ErrNoReservation)
    }
    
    // 检查是否在续期窗口内
    timeToExpiry := reservation.expiresAt - now()
    if timeToExpiry > config.renewalWindow {
        return Ok(())  // 还不需要续期
    }
    
    // 发送续期请求
    err := sendRenewal(relay)
    if err != nil {
        reservation.failureCount++
        if reservation.failureCount >= config.maxRenewalFailures {
            // 续期失败次数过多，标记为不可用
            markUnavailable(relay.id)
        }
        return err
    }
    
    // 更新过期时间
    reservation.expiresAt = now() + config.reservationTTL
    reservation.failureCount = 0
    return Ok(())
}
```

---

## 错误处理

```pseudocode
// Relay 错误定义
const (
    ErrNoRelayAvailable   = "relay: no relay available"
    ErrBandwidthExceeded  = "relay: bandwidth exceeded"
    ErrDurationExceeded   = "relay: duration exceeded"
    ErrNoReservation      = "relay: no valid reservation"
    ErrReservationExpired = "relay: reservation expired"
    ErrRenewalFailed      = "relay: renewal failed"
    ErrSignalingFailed    = "relay: signaling failed"
    ErrAddressNotFound    = "relay: address not found in address book"
)
```

---

**最后更新**：2026-01-23
