# core_transport 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/transport/
├── module.go           # TransportManager + Fx 模块定义
├── errors.go           # 错误定义
├── testing.go          # 测试辅助
├── quic/
│   ├── transport.go    # QUIC 传输实现
│   ├── listener.go     # QUIC 监听器
│   ├── conn.go         # QUIC 连接
│   ├── stream.go       # QUIC 流
│   ├── tls.go          # TLS 配置（从 Identity 生成）
│   ├── rebind.go       # 网络切换支持
│   └── *_test.go       # 单元测试
└── tcp/
    ├── transport.go    # TCP 传输实现
    ├── listener.go     # TCP 监听器
    ├── conn.go         # TCP 连接（需 Upgrader）
    └── *_test.go       # 单元测试
```

---

## QUIC 关键实现

### TLS 配置生成

QUIC 的 TLS 配置从 `Identity` 生成，确保对端验证：

```go
// internal/core/transport/quic/tls.go

func NewTLSConfig(identity pkgif.Identity) (*tls.Config, *tls.Config, error) {
    // 1. 从 Identity 获取私钥和公钥
    privKey := identity.PrivateKey()
    pubKey := identity.PublicKey()
    
    // 2. 生成自签名证书（嵌入 PeerID 用于验证）
    cert, err := generateCertificate(privKey, pubKey)
    
    // 3. 返回服务端和客户端 TLS 配置
    serverConf := &tls.Config{
        Certificates: []tls.Certificate{cert},
        NextProtos:   []string{"dep2p"},
        // ...
    }
    clientConf := &tls.Config{
        InsecureSkipVerify: true,  // 使用自定义验证
        VerifyConnection:   verifyPeerCertificate,
        NextProtos:         []string{"dep2p"},
        // ...
    }
    
    return serverConf, clientConf, nil
}
```

### QUIC 连接建立

```go
// internal/core/transport/quic/transport.go

func (t *Transport) Dial(ctx context.Context, raddr types.Multiaddr, p types.PeerID) (pkgif.Connection, error) {
    // 1. 解析地址
    udpAddr, err := t.parseAddr(raddr)
    if err != nil {
        return nil, err
    }
    
    // 2. 建立 QUIC 连接（使用客户端 TLS 配置）
    quicConn, err := quic.DialAddr(ctx, udpAddr.String(), t.clientTLS, t.config)
    if err != nil {
        return nil, fmt.Errorf("dial: %w", err)
    }
    
    // 3. 验证远程节点身份
    remotePeer := extractPeerID(quicConn)
    if p != "" && remotePeer != p {
        quicConn.CloseWithError(0, "peer mismatch")
        return nil, ErrPeerMismatch
    }
    
    return newConn(quicConn, t.localPeer, remotePeer), nil
}
```

### Rebind 实现

```go
// internal/core/transport/quic/rebind.go

// Rebind 重新绑定 QUIC socket
//
// 当网络接口变化时（如 4G→WiFi），UDP socket 需要重新绑定
// 到新的网络接口，否则连接将断开。
func (t *Transport) Rebind(ctx context.Context) error {
    // 遍历所有监听器，执行 rebind
    var lastErr error
    t.listeners.Range(func(key, value interface{}) bool {
        listener := value.(*Listener)
        if err := listener.Rebind(); err != nil {
            lastErr = err
        }
        return true
    })
    return lastErr
}
```

---

## TCP 关键实现

### TCP 连接升级

TCP 连接需要通过 Upgrader 进行安全握手和多路复用设置：

```go
// internal/core/transport/tcp/transport.go

func (t *Transport) Dial(ctx context.Context, raddr types.Multiaddr, p types.PeerID) (pkgif.Connection, error) {
    // 1. 解析地址
    tcpAddr, err := t.parseAddr(raddr)
    if err != nil {
        return nil, err
    }
    
    // 2. 建立原始 TCP 连接
    rawConn, err := net.DialTimeout("tcp", tcpAddr.String(), t.dialTimeout)
    if err != nil {
        return nil, fmt.Errorf("dial: %w", err)
    }
    
    // 3. 使用 Upgrader 升级连接
    if t.upgrader != nil {
        upgradedConn, err := t.upgrader.Upgrade(ctx, rawConn, pkgif.DirOutbound, p)
        if err != nil {
            rawConn.Close()
            return nil, fmt.Errorf("upgrade: %w", err)
        }
        return wrapUpgradedConn(upgradedConn, t.localPeer), nil
    }
    
    // 无 Upgrader 时返回原始连接（仅用于测试）
    return wrapRawConn(rawConn, t.localPeer, p), nil
}
```

### TCP 升级流程详解

```
TCP 连接升级流程：
═══════════════════════════════════════════════════════════════════

  ┌──────────────┐
  │   net.Conn   │   原始 TCP 连接
  └──────┬───────┘
         │
         ▼
  ┌──────────────────────────────────────────────────────────────┐
  │                    Upgrader.Upgrade()                        │
  │                                                              │
  │  1. 申请连接资源（ResourceManager）                          │
  │        │                                                     │
  │        ▼                                                     │
  │  2. multistream-select 协商安全协议                          │
  │     客户端: 提议 ["/tls/1.0.0", "/noise"]                   │
  │     服务端: 选择最优协议                                     │
  │        │                                                     │
  │        ▼                                                     │
  │  3. 安全握手                                                 │
  │     TLS: 标准 TLS 1.3 握手                                   │
  │     Noise: IK 模式握手                                       │
  │        │                                                     │
  │        ▼                                                     │
  │  4. 设置远程 PeerID（ResourceManager）                       │
  │        │                                                     │
  │        ▼                                                     │
  │  5. multistream-select 协商多路复用器                        │
  │     客户端: 提议 ["/yamux/1.0.0"]                            │
  │     服务端: 选择最优协议                                     │
  │        │                                                     │
  │        ▼                                                     │
  │  6. yamux 多路复用设置                                       │
  │                                                              │
  └──────────────────────────────────────────────────────────────┘
         │
         ▼
  ┌──────────────┐
  │ UpgradedConn │   安全 + 多路复用
  └──────────────┘
```

---

## 地址处理

### Multiaddr 解析

```go
// QUIC 地址格式: /ip4/127.0.0.1/udp/4001/quic-v1
// TCP 地址格式:  /ip4/127.0.0.1/tcp/4001

func parseAddr(addr types.Multiaddr) (net.Addr, error) {
    // 提取 IP
    ip, err := addr.ValueForProtocol(multiaddr.P_IP4)
    if err != nil {
        ip, err = addr.ValueForProtocol(multiaddr.P_IP6)
    }
    if err != nil {
        return nil, fmt.Errorf("no IP in address: %w", err)
    }
    
    // 提取端口（根据协议类型）
    port, err := addr.ValueForProtocol(multiaddr.P_UDP)  // QUIC
    if err != nil {
        port, err = addr.ValueForProtocol(multiaddr.P_TCP)  // TCP
    }
    
    return buildNetAddr(ip, port), nil
}
```

### 支持的协议

| 传输 | 协议模式 | 示例 |
|------|---------|------|
| QUIC | /ip4/.../udp/.../quic-v1 | /ip4/127.0.0.1/udp/4001/quic-v1 |
| QUIC | /ip6/.../udp/.../quic-v1 | /ip6/::1/udp/4001/quic-v1 |
| TCP | /ip4/.../tcp/... | /ip4/127.0.0.1/tcp/4001 |
| TCP | /ip6/.../tcp/... | /ip6/::1/tcp/4001 |

---

## 状态管理

| 状态 | 管理方式 | 说明 |
|------|----------|------|
| listeners | sync.Map | 活跃监听器（按地址索引） |
| connections | 外部管理 | 由 Host/Swarm 管理 |
| config | 不可变 | 创建时固定 |
| identity | 不可变 | 用于 TLS 证书生成 |

---

## 错误处理

```go
// internal/core/transport/errors.go

var (
    ErrPeerMismatch     = errors.New("transport: peer ID mismatch")
    ErrInvalidAddr      = errors.New("transport: invalid multiaddr")
    ErrListenerClosed   = errors.New("transport: listener closed")
    ErrConnectionReset  = errors.New("transport: connection reset")
    ErrUnsupportedProto = errors.New("transport: unsupported protocol")
)

// internal/core/transport/quic/errors.go

var (
    ErrNilIdentity      = errors.New("quic: identity is nil")
    ErrTLSConfigFailed  = errors.New("quic: TLS config generation failed")
    ErrRebindFailed     = errors.New("quic: rebind failed")
)

// internal/core/transport/tcp/errors.go

var (
    ErrNilUpgrader      = errors.New("tcp: upgrader is nil")
    ErrUpgradeFailed    = errors.New("tcp: connection upgrade failed")
)
```

---

**最后更新**：2026-01-25
