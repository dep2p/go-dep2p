# core_relay 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/relay/
├── module.go           # Fx 模块
├── doc.go              # 模块文档
├── manager.go          # 中继管理器
├── service.go          # ★ 统一 Relay 服务
├── client.go           # Relay 客户端
├── server.go           # Relay 服务端
├── addressbook.go      # ★ 地址簿（第一层）
├── signaling.go        # ★ 信令通道（第二层）
├── selector.go         # 选择器
├── limiter.go          # 限制器
├── reservation.go      # ★ 预留管理
└── *_test.go           # 测试
```

---

## 中继使用规范

### ★ 统一 Relay 使用

```pseudocode
// 连接优先级：直连 → 打洞 → Relay（INV-003）
fn dial(peer: NodeID) -> Result<Connection, Error> {
    // 1. 尝试直连
    if conn := dialDirect(peer); conn.is_ok() {
        return conn
    }
    
    // 2. 尝试打洞（需要信令通道）
    if relay.IsConnected() {
        entry := relay.QueryAddress(peer)
        if entry.is_ok() && canHolePunch(self.natType, entry.natType) {
            if conn := dialHolePunch(peer, relay); conn.is_ok() {
                // ★ 打洞成功后保留 Relay 连接
                return conn
            }
        }
    }
    
    // 3. 使用 Relay 转发
    return relay.DialViaRelay(peer)
}
```

### 资源释放

```pseudocode
// 确保释放中继资源
fn useRelay(relay: Relay) -> Result<(), Error> {
    circuit := relay.Open(target)?
    defer circuit.Close()  // 确保关闭
    
    // 使用 circuit
    return Ok(())
}
```

### ★ 预留管理

```pseudocode
// 预留续期
fn manageReservation(relay: RelayClient) {
    ticker := NewTicker(config.renewalInterval)
    
    for _ in ticker {
        if shouldRenew(relay.reservation) {
            err := relay.RenewReservation()
            if err != nil && relay.reservation.failureCount >= config.maxRenewalFailures {
                // 续期失败过多，重新建立连接
                relay.Reconnect()
            }
        }
    }
}
```

---

## 错误处理

### 故障转移（指数退避）

```pseudocode
fn dialWithRetry(target: NodeID) -> Result<Connection, Error> {
    retryConfig := RetryConfig {
        initialDelay: 1s,
        multiplier: 2,
        maxDelay: 30s,
        maxRetries: 5,
        jitterFactor: 0.2,
    }
    
    lastErr := nil
    
    for i := 0; i < retryConfig.maxRetries; i++ {
        conn := dialViaRelay(target)
        if conn.is_ok() {
            return conn
        }
        lastErr = conn.err()
        
        // 指数退避 + 抖动
        delay := min(
            retryConfig.initialDelay * pow(retryConfig.multiplier, i),
            retryConfig.maxDelay
        )
        delay = delay * (1 + random(-retryConfig.jitterFactor, retryConfig.jitterFactor))
        sleep(delay)
    }
    
    return Err(fmt.Errorf("all retries failed: %w", lastErr))
}
```

---

## 并发模式

### 健康检查并发

```pseudocode
fn checkAllRelays(relays: []RelayInfo) {
    wg := WaitGroup{}
    
    for relay in relays {
        wg.Add(1)
        spawn fn(r: RelayInfo) {
            defer wg.Done()
            checkHealth(r)
            
            // ★ 检查是否需要续期
            if shouldRenew(r.reservation) {
                renewReservation(r)
            }
        }(relay)
    }
    
    wg.Wait()
}
```

---

## ★ 三大职责 v2.0 使用示例

```pseudocode
// 完整的 Relay 使用流程
fn connectToPeer(peer: NodeID) -> Result<Connection, Error> {
    relay := getConfiguredRelay()
    
    // 第一层：地址发现
    entry := relay.QueryAddress(peer)
    if entry.is_ok() {
        // 尝试直连
        if conn := dialDirect(entry.addrs); conn.is_ok() {
            return conn
        }
        
        // 第二层：信令通道（打洞协调）
        if canHolePunch(self.natType, entry.natType) {
            // 通过 Relay 交换地址
            relay.SendSignaling(peer, SignalingMsg { type: CONNECT_REQ })
            if conn := performHolePunch(peer); conn.is_ok() {
                // ★ 打洞成功后保留 Relay 连接
                return conn
            }
        }
    }
    
    // 第三层：数据保底
    return relay.DialViaRelay(peer)
}
```

---

**最后更新**：2026-01-23
