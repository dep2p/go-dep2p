# Core Upgrader 设计审查

> **版本**: v1.0.0  
> **日期**: 2026-01-13  
> **目标**: 审查 upgrader 设计，理解连接升级机制

---

## 一、模块定位

### 1.1 架构定位

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DeP2P 五层架构                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Level 3: Core Layer 高层                                                   │
│  ═════════════════════════                                                  │
│                                                                             │
│    Raw Connection (from Transport)                                          │
│         │                                                                   │
│         ▼                                                                   │
│    ┌─────────────────────────────────┐                                     │
│    │      core_upgrader              │  ◄── 本模块                          │
│    │  • Security Negotiation         │                                     │
│    │  • Muxer Negotiation            │                                     │
│    │  • Connection Upgrade           │                                     │
│    └─────────────────────────────────┘                                     │
│         │                                                                   │
│         ▼                                                                   │
│    Upgraded Connection (Secure + Muxed)                                     │
│         │                                                                   │
│         ▼                                                                   │
│    ┌─────────────────────────────────┐                                     │
│    │      core_swarm                 │                                     │
│    │  连接池管理                      │                                     │
│    └─────────────────────────────────┘                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**关键职责**:
- 将原始连接升级为安全、多路复用连接
- 协调 security 和 muxer 模块
- 使用 multistream-select 协议协商

---

## 二、Multistream-Select 协议

### 2.1 协议概述

Multistream-Select 是 libp2p 使用的协议协商机制。

**协议交互**:
```
Client                          Server
  │                               │
  │ ─── /multistream/1.0.0 ─────► │  1. 协议头
  │ ◄── /multistream/1.0.0 ────── │
  │                               │
  │ ─── /tls/1.0.0 ─────────────► │  2. 提议协议
  │ ◄── /tls/1.0.0 ──────────────│  3. 确认
  │                               │
  │ ═══ Protocol Data ═══════════ │  4. 协议数据
  │                               │
```

**关键特性**:
- 文本协议（newline-delimited）
- 客户端提议，服务器选择
- 支持协议列表
- 失败回退机制

### 2.2 go-multistream 库

**核心 API**:
```go
// 服务器端
muxer := mss.NewMultistreamMuxer[protocol.ID]()
muxer.AddHandler(protocolID, handler)
selected, handler, err := muxer.Negotiate(conn)

// 客户端
selected, err := mss.SelectOneOf([]protocol.ID{...}, conn)
```

**使用场景**:
1. **安全协议协商**: `/tls/1.0.0`, `/noise`
2. **多路复用协商**: `/yamux/1.0.0`, `/mplex/6.7.0`

---

## 三、安全协商流程

### 3.1 协商流程

```
Raw Connection
     │
     ▼
┌─────────────────────────────────────┐
│  Security Negotiation               │
│  (multistream-select)               │
│                                     │
│  Client proposals:                  │
│    1. /tls/1.0.0                   │
│    2. /noise                        │
│                                     │
│  Server selects: /tls/1.0.0         │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│  TLS Handshake                      │
│  • Client Hello                     │
│  • Server Hello                     │
│  • Certificate Exchange             │
│  • Finished                         │
│                                     │
│  验证: INV-001 (PeerID)             │
└─────────────────────────────────────┘
     │
     ▼
Secure Connection
```

### 3.2 支持的安全协议

| 协议 | Protocol ID | 状态 | 说明 |
|------|-------------|------|------|
| TLS 1.3 | `/tls/1.0.0` | ✅ 已实现 | core_security/tls |
| Noise XX | `/noise` | ⚠️ 基础实现 | core_security/noise |

### 3.3 核心代码（go-libp2p 参考）

```go
// github.com/libp2p/go-libp2p/p2p/net/upgrader/upgrader.go

func (u *upgrader) negotiateSecurity(ctx context.Context, conn net.Conn, isServer bool) (protocol.ID, error) {
    if isServer {
        // 服务器端使用 MultistreamMuxer
        selected, _, err := u.securityMuxer.Negotiate(conn)
        return selected, err
    } else {
        // 客户端使用 SelectOneOf
        selected, err := mss.SelectOneOf(u.securityIDs, conn)
        return selected, err
    }
}

func (u *upgrader) setupSecurity(ctx context.Context, conn net.Conn, p peer.ID, isServer bool) (sec.SecureConn, protocol.ID, error) {
    st, err := u.negotiateSecurity(ctx, conn, isServer)
    if err != nil {
        return nil, "", err
    }
    
    // 找到对应的 SecureTransport
    var transport sec.SecureTransport
    for _, s := range u.security {
        if s.ID() == st {
            transport = s
            break
        }
    }
    
    // 执行握手
    if isServer {
        sconn, err := transport.SecureInbound(ctx, conn, p)
        return sconn, st, err
    }
    sconn, err := transport.SecureOutbound(ctx, conn, p)
    return sconn, st, err
}
```

---

## 四、多路复用协商流程

### 4.1 协商流程

```
Secure Connection
     │
     ▼
┌─────────────────────────────────────┐
│  Muxer Negotiation                  │
│  (multistream-select)               │
│                                     │
│  Client proposals:                  │
│    1. /yamux/1.0.0                 │
│    2. /mplex/6.7.0                 │
│                                     │
│  Server selects: /yamux/1.0.0       │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│  Yamux Setup                        │
│  • Create session                   │
│  • Enable keepalive                 │
│  • Configure windows                │
└─────────────────────────────────────┘
     │
     ▼
Muxed Connection
```

### 4.2 支持的复用器

| 复用器 | Protocol ID | 状态 | 说明 |
|--------|-------------|------|------|
| yamux | `/yamux/1.0.0` | ✅ 已实现 | core_muxer |
| mplex | `/mplex/6.7.0` | ⬜ 未实现 | 可选 |

### 4.3 核心代码（go-libp2p 参考）

```go
func (u *upgrader) negotiateMuxer(nc net.Conn, isServer bool) (*StreamMuxer, error) {
    var proto protocol.ID
    
    if isServer {
        selected, _, err := u.muxerMuxer.Negotiate(nc)
        if err != nil {
            return nil, err
        }
        proto = selected
    } else {
        selected, err := mss.SelectOneOf(u.muxerIDs, nc)
        if err != nil {
            return nil, err
        }
        proto = selected
    }
    
    // 找到对应的 Muxer
    for i, m := range u.muxers {
        if m.ID == proto {
            return &u.muxers[i], nil
        }
    }
    return nil, fmt.Errorf("selected muxer not found")
}

func (u *upgrader) setupMuxer(ctx context.Context, conn net.Conn, isServer bool, scope network.PeerScope) (protocol.ID, network.MuxedConn, error) {
    muxer, err := u.negotiateMuxer(conn, isServer)
    if err != nil {
        return "", nil, err
    }
    
    // 创建多路复用连接
    smconn, err := muxer.Muxer.NewConn(conn, isServer, scope)
    return muxer.ID, smconn, err
}
```

---

## 五、QUIC 特殊处理

### 5.1 QUIC 优势

QUIC 协议自带：
- ✅ **内置加密** (TLS 1.3)
- ✅ **内置多路复用** (stream)
- ✅ **0-RTT 连接恢复**
- ✅ **连接迁移**

因此 QUIC 连接**不需要**额外的 Security 和 Muxer 协商。

### 5.2 特殊处理流程

```
QUIC Connection (已加密、已复用)
     │
     ▼
检测连接类型
     │
     ├─ QUIC? ────► 直接返回（跳过升级）
     │
     └─ TCP/Other ─► 正常升级流程
```

### 5.3 实现方式

```go
func (u *Upgrader) Upgrade(ctx context.Context, conn net.Conn, dir Direction, remotePeer types.PeerID) (UpgradedConn, error) {
    // 检测 QUIC 连接
    if isQUICConn(conn) {
        // QUIC 不需要升级，直接封装返回
        return wrapQUICConn(conn), nil
    }
    
    // 非 QUIC：正常升级流程
    // 1. Security negotiation
    // 2. Security handshake
    // 3. Muxer negotiation
    // 4. Muxer setup
    ...
}
```

---

## 六、与 go-libp2p 对比

### 6.1 相同点

| 特性 | go-libp2p | DeP2P |
|------|-----------|-------|
| 协议协商 | multistream-select | ✅ 相同 |
| 安全握手 | TLS 1.3 | ✅ 相同 |
| 多路复用 | yamux | ✅ 相同 |
| QUIC 处理 | 跳过升级 | ✅ 相同 |
| 双向升级 | Inbound/Outbound | ✅ 相同 |

### 6.2 简化点

| 特性 | go-libp2p | DeP2P | 理由 |
|------|-----------|-------|------|
| PSK (Private Network) | ✅ 支持 | ⬜ 暂不支持 | 简化设计 |
| Connection Gater | ✅ 支持 | ⬜ 待实现 | 后续添加 |
| Early Muxer Negotiation | ✅ 支持 | ⬜ 暂不支持 | TLS 1.3 优化 |
| 多种 Muxer | yamux, mplex | yamux only | 简化实现 |

### 6.3 差异点

| 项目 | go-libp2p | DeP2P |
|------|-----------|-------|
| Listener 升级 | `UpgradeListener()` | ⬜ 暂不实现 |
| 资源管理集成 | 完整集成 | ⚠️ 基础集成 |
| 指标收集 | Prometheus | ⬜ 待添加 |

---

## 七、实现要点

### 7.1 关键接口

```go
// Upgrader 核心接口
type Upgrader interface {
    Upgrade(ctx context.Context, conn net.Conn, dir Direction, 
            remotePeer types.PeerID) (UpgradedConn, error)
}

// UpgradedConn 升级后的连接
type UpgradedConn interface {
    MuxedConn
    LocalPeer() types.PeerID
    RemotePeer() types.PeerID
    Security() types.ProtocolID
    Muxer() string
}
```

### 7.2 超时处理

```go
const (
    defaultNegotiateTimeout = 60 * time.Second
    defaultHandshakeTimeout = 30 * time.Second
)

// 协商时设置超时
conn.SetDeadline(time.Now().Add(defaultNegotiateTimeout))
```

### 7.3 错误处理

关键错误场景：
- ❌ 协商超时
- ❌ 不支持的协议
- ❌ 握手失败
- ❌ PeerID 不匹配（INV-001）
- ❌ 连接关闭

---

## 八、依赖模块状态

| 依赖模块 | 状态 | 说明 |
|---------|------|------|
| core_security | ✅ 完成 | TLS 1.3 + Noise 基础实现 |
| core_muxer | ✅ 完成 | yamux 实现，84.1% 覆盖 |
| core_identity | ✅ 完成 | Ed25519 密钥，PeerID |
| core_resourcemgr | ✅ 完成 | 资源管理 |
| core_transport | ✅ 完成 | QUIC 传输 |

**依赖就绪**: ✅ 所有依赖模块已完成

---

## 九、实施风险

| 风险 | 等级 | 缓解措施 |
|------|------|----------|
| multistream-select 库集成复杂 | 低 | 参考 go-libp2p 实现 |
| 协议协商失败处理 | 中 | 充分测试错误路径 |
| QUIC 检测逻辑 | 低 | 类型断言 |
| 性能开销 | 低 | 协商超时优化 |

---

## 十、总结

### 10.1 核心流程

```
Raw Connection
     ↓
Security Negotiation (multistream-select)
     ↓
Security Handshake (TLS/Noise)
     ↓
Muxer Negotiation (multistream-select)
     ↓
Muxer Setup (yamux)
     ↓
Upgraded Connection
```

### 10.2 关键点

1. ✅ **Multistream-Select**: 使用 go-multistream 库
2. ✅ **双向升级**: Inbound 和 Outbound 不同逻辑
3. ✅ **依赖就绪**: security + muxer 已完成
4. ✅ **参考实现**: go-libp2p/p2p/net/upgrader
5. ⚠️ **QUIC 特殊处理**: 跳过升级流程

### 10.3 下一步

- ✅ 设计审查完成
- ➡️ 接下来：定义 Upgrader 接口

---

**审查完成日期**: 2026-01-13  
**审查结论**: ✅ 设计清晰，依赖就绪，可以开始实施
