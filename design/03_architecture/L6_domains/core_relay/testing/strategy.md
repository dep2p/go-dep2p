# core_relay 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| RelayService | 90%+ | ★ 统一 Relay 服务（三大职责 v2.0） |
| AddressBook | 90%+ | ★ 地址簿（第一层） |
| Signaling | 90%+ | ★ 信令通道（第二层） |
| Selector | 100% | 选择逻辑 |
| Limiter | 100% | 限制逻辑 |
| Reservation | 90%+ | ★ 预留管理 |

---

## 测试类型

### 单元测试

```pseudocode
// ★ 三大职责 v2.0 测试
fn TestRelayService_ThreeRoles(t: Test) {
    relay := NewRelayService(config)
    
    // 第一层：地址发现
    entry := MemberEntry { nodeID: "peer1", addrs: [...] }
    relay.RegisterAddress(entry)
    result := relay.QueryAddress("peer1")
    assert(result.found == true)
    
    // 第二层：信令通道
    msg := SignalingMsg { type: CONNECT_REQ, target: "peer2" }
    relay.SendSignaling("peer2", msg)
    // 验证消息转发
    
    // 第三层：数据转发
    conn := relay.DialViaRelay(ctx, "peer2")
    assert(conn != nil)
}

// 预留续期测试
fn TestReservation_TTLRenewal(t: Test) {
    reservation := NewReservation(config)
    
    // 初始 TTL
    assert(reservation.expiresAt > now())
    
    // 模拟时间流逝到续期窗口
    advanceTime(55 * minute)
    
    // 触发续期
    reservation.Renew()
    assert(reservation.expiresAt > now() + 55 * minute)
}

// 资源限制测试
fn TestLimiter_BandwidthExceeded(t: Test) {
    config := RelayConfig { maxBandwidth: 10240 }  // 10KB
    limiter := NewRelayLimiter(config)
    
    // 超过限制
    for i := 0; i < 100; i++ {
        limiter.Allow(Request { size: 1024 })
    }
    
    err := limiter.Allow(Request { size: 1024 })
    assert(err == ErrBandwidthExceeded)
}
```

### 集成测试

```pseudocode
// 完整连接流程测试
fn TestRelayConnection_FullFlow(t: Test) {
    relay, node1, node2 := setupRelayNetwork(t)
    
    // 1. 地址注册
    node1.RegisterToRelay(relay)
    node2.RegisterToRelay(relay)
    
    // 2. 地址查询
    entry := node1.QueryAddress(node2.ID())
    assert(entry.found == true)
    
    // 3. 尝试直连
    conn := node1.DialDirect(ctx, entry.addrs)
    if conn.is_err() {
        // 4. 通过信令通道协调打洞
        node1.HolePunchViaRelay(relay, node2.ID())
    }
    
    // 5. 如果打洞失败，使用 Relay 转发
    if conn.is_err() {
        conn = node1.DialViaRelay(ctx, relay.ID(), node2.ID())
    }
    
    assert(conn.is_ok())
    
    // ★ 验证打洞成功后 Relay 连接仍保留
    assert(node1.IsRelayConnected(relay) == true)
}
```

---

## Mock 策略

```pseudocode
struct MockRelayService {
    mock: Mock
}

fn GetActiveRelay() -> Option<NodeID> {
    args := self.mock.Called()
    return args.Get(0)
}

fn QueryAddress(target: NodeID) -> Result<MemberEntry, Error> {
    args := self.mock.Called(target)
    return args.Get(0), args.Error(1)
}

fn SendSignaling(target: NodeID, msg: SignalingMsg) -> Result<(), Error> {
    args := self.mock.Called(target, msg)
    return args.Error(0)
}

fn DialViaRelay(ctx: Context, target: NodeID) -> Result<Connection, Error> {
    args := self.mock.Called(ctx, target)
    return args.Get(0), args.Error(1)
}
```

---

**最后更新**：2026-01-23
