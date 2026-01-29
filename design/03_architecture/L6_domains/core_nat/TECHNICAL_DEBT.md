# Core NAT 技术债追踪

> **日期**: 2026-01-13  
> **版本**: v1.0.0  
> **状态**: 追踪中

---

## 概述

本文档追踪 core_nat 模块中因依赖其他未完成组件而暂时无法实现的功能。

---

## TD-001: Hole Punching 完整实现

### 基本信息

| 属性 | 值 |
|------|-----|
| **状态** | BLOCKED - 等待 core_relay |
| **优先级** | P1 |
| **预估工作量** | 3-5 天 |
| **影响范围** | `internal/core/nat/holepunch/puncher.go` |
| **标记位置** | `puncher.go:47-50` |

### 依赖组件

**必需依赖**：
- ✅ core_swarm - 连接管理（已完成）
- ⬜ core_relay - 中继服务（**未实现，阻塞中**）
- ⬜ core_protocol - 协议注册（部分完成，需注册 holepunch 协议）

### 当前实现

**已完成部分**：
```go
// holepunch/puncher.go
type HolePuncher struct {
    mu     sync.RWMutex
    active map[string]struct{}  // ✅ 活跃打洞去重
}

func (h *HolePuncher) MarkActive(peerID string)    // ✅ 完成
func (h *HolePuncher) ClearActive(peerID string)   // ✅ 完成
func (h *HolePuncher) IsActive(peerID string) bool // ✅ 完成
func (h *HolePuncher) ActiveCount() int            // ✅ 完成
```

**待完成部分**：
```go
func (h *HolePuncher) DirectConnect(ctx context.Context, peerID string, addrs []string) error {
    // ⬜ 1. 通过中继建立协商流（需 core_relay）
    // ⬜ 2. 交换观察地址
    // ⬜ 3. SYNC 同步消息
    // ⬜ 4. 同时发送 UDP 包
    // ⬜ 5. 等待连接建立
    
    return ErrHolePunchFailed  // 当前仅返回失败
}
```

### 完整实现方案

参考 go-libp2p `p2p/protocol/holepunch/holepuncher.go:DirectConnect`：

**步骤 1：通过中继协商**
```go
// 需要 core_relay 提供中继连接
relayConn := swarm.GetRelayConnection(peerID)
if relayConn == nil {
    return ErrNoRelayConnection
}

// 打开协议流
stream, err := relayConn.NewStream(ctx)
stream.SetProtocol(HolePunchProtocol)
```

**步骤 2：交换地址**
```go
// 发送我们的观察地址
myAddrs := swarm.LocalAddrs()
msg := &HolePunchMsg{
    Type:          MsgTypeConnect,
    ObservedAddrs: myAddrs,
}
sendMsg(stream, msg)

// 接收对方的观察地址
peerMsg := recvMsg(stream)
peerAddrs := peerMsg.ObservedAddrs
```

**步骤 3：同步**
```go
// 发送 SYNC 消息
syncMsg := &HolePunchMsg{
    Type:  MsgTypeSync,
    Nonce: randomNonce(),
}
sendMsg(stream, syncMsg)

// 等待对方 SYNC
peerSync := recvMsg(stream)
```

**步骤 4：UDP 打洞**
```go
// 同时向所有对方地址发送 UDP 包
for _, addr := range peerAddrs {
    go sendUDPPunch(addr, syncMsg.Nonce)
}
```

**步骤 5：等待连接**
```go
// 监听 swarm 的新连接事件
select {
case conn := <-waitForConnection(peerID):
    return nil  // 打洞成功
case <-time.After(10 * time.Second):
    return ErrHolePunchTimeout
}
```

### 解除阻塞条件

- [ ] **core_relay 实现完成**
  - 中继连接建立
  - 中继流创建
  - 协议路由
  
- [ ] **core_protocol 扩展**
  - 注册 `/dep2p/sys/holepunch/1.0.0` 协议
  - 协议处理器注册

- [ ] **core_swarm 扩展**（可能需要）
  - 连接事件通知
  - 观察地址管理

### 测试计划

**单元测试**（当前已有）：
- ✅ 活跃打洞管理
- ✅ 去重机制
- ✅ 并发安全

**集成测试**（待补充）：
- ⬜ 两节点打洞测试（需测试环境）
- ⬜ 对称 NAT 测试
- ⬜ 超时和重试
- ⬜ 中继回退

### 参考资料

- go-libp2p: `p2p/protocol/holepunch/holepuncher.go`
- 设计文档: `design/03_architecture/L6_domains/core_nat/design/internals.md`
- DCUtR 规范: https://github.com/libp2p/specs/tree/master/relay

---

## TD-002: AutoNAT 服务端实现

### 基本信息

| 属性 | 值 |
|------|-----|
| **状态** | DEFERRED - v1.1 规划 |
| **优先级** | P2 |
| **预估工作量** | 2-3 天 |
| **影响范围** | `internal/core/nat/autonat.go` |
| **标记位置** | `autonat.go:138-142` |

### 依赖组件

**必需依赖**：
- ✅ core_swarm - 连接管理
- ✅ core_protocol - 协议注册

**可选依赖**：
- ⬜ 限流器 - 防止滥用探测

### 当前实现

**已完成部分**：
```go
// AutoNAT 客户端（完整实现）
func (a *AutoNAT) runProbeLoop(ctx context.Context)  // ✅ 探测循环
func (a *AutoNAT) recordSuccess()                    // ✅ 记录成功
func (a *AutoNAT) recordFailure()                    // ✅ 记录失败
```

**待完成部分**：
```go
// AutoNAT 服务端
type AutoNATServer struct {
    // ⬜ 处理 DIAL 请求
    // ⬜ 拨回客户端地址
    // ⬜ 返回 DIAL_RESPONSE
    // ⬜ 限流保护
}
```

### 完整实现方案

参考 go-libp2p `p2p/host/autonat/svc.go`：

```go
func (s *AutoNATServer) handleStream(stream Stream) {
    // 1. 读取 DIAL 请求
    var req AutoNATMessage
    readMsg(stream, &req)
    
    // 2. 限流检查
    if !s.rateLimiter.Allow(req.PeerID) {
        sendResponse(stream, E_DIAL_REFUSED, "rate limit")
        return
    }
    
    // 3. 地址验证
    if isPrivateAddr(req.Addrs) {
        sendResponse(stream, E_DIAL_REFUSED, "private address")
        return
    }
    
    // 4. 拨号测试
    err := s.dialBack(req.PeerID, req.Addrs)
    
    // 5. 返回结果
    if err == nil {
        sendResponse(stream, OK, "")
    } else {
        sendResponse(stream, E_DIAL_ERROR, err.Error())
    }
}
```

### 解除阻塞条件

- [x] core_swarm 实现完成
- [x] core_protocol 实现完成
- [ ] 限流器实现（可选）
- [ ] v1.1 开发周期

---

## TD-003: 复杂 NAT 穿透策略

### 基本信息

| 属性 | 值 |
|------|-----|
| **状态** | BLOCKED - 依赖 TD-001 |
| **优先级** | P3 |
| **预估工作量** | 5-7 天 |
| **影响范围** | `internal/core/nat/holepunch/` |

### 依赖

- ⬜ TD-001 完成（Hole Punching 基础实现）

### 待实现内容

1. **对称 NAT 穿透**
   - 多端口同时打洞
   - 端口预测
   - 生日悖论攻击

2. **Backoff 策略**
   - 指数退避
   - 黑洞检测
   - 自适应超时

3. **打洞成功率优化**
   - 地址排序优化
   - 并发打洞策略
   - 统计和监控

---

## 技术债管理

### 优先级定义

- **P0**: 阻塞核心功能，必须立即解决
- **P1**: 重要功能，下个版本解决
- **P2**: 增强功能，规划中
- **P3**: 优化功能，长期规划

### 解除阻塞路径

```
TD-002 (AutoNAT 服务端)
  ↓ 无依赖，v1.1 即可实现
  
TD-001 (Hole Punching)
  ↓ 等待 core_relay
  ↓
TD-003 (复杂穿透策略)
  ↓ 依赖 TD-001
```

### 版本规划

**v1.0 (当前)**:
- ✅ AutoNAT 客户端
- ✅ STUN 完整实现
- ✅ UPnP 完整实现
- ✅ NAT-PMP 完整实现
- ⬜ Hole Punching (TD-001)

**v1.1 (规划)**:
- ⬜ TD-001: Hole Punching 完整实现
- ⬜ TD-002: AutoNAT 服务端

**v1.2+ (长期)**:
- ⬜ TD-003: 复杂 NAT 穿透策略

---

## 代码标记规范

所有技术债在代码中使用以下格式标记：

```go
// TODO(TD-XXX): 简短描述
// 详细说明见 TECHNICAL_DEBT.md
```

示例：
```go
// TODO(TD-001): 需要 core_relay 实现完整打洞
// 当前仅提供框架，完整实现见 TECHNICAL_DEBT.md TD-001
```

---

**最后更新**: 2026-01-13  
**维护者**: DeP2P Team
