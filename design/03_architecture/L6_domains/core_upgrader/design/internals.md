# core_upgrader 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/upgrader/
├── doc.go              # 包文档
├── module.go           # Fx 模块定义
├── config.go           # 配置
├── upgrader.go         # Upgrader 主实现
├── multistream.go      # multistream-select 协商
├── conn.go             # UpgradedConn 封装
├── errors.go           # 错误定义
├── testing.go          # 测试辅助
└── *_test.go           # 单元测试
```

---

## 关键实现

### Upgrader 结构

```go
// internal/core/upgrader/upgrader.go

type Upgrader struct {
    identity pkgif.Identity           // 本地身份（用于握手）
    
    securityTransports []pkgif.SecureTransport  // 安全传输列表（按优先级）
    streamMuxers       []pkgif.StreamMuxer      // 多路复用器列表（按优先级）
    
    resourceMgr pkgif.ResourceManager  // 资源管理器（可选）
}

// New 创建连接升级器
func New(id pkgif.Identity, cfg Config) (*Upgrader, error) {
    if id == nil {
        return nil, ErrNilIdentity
    }
    if len(cfg.SecurityTransports) == 0 {
        return nil, ErrNoSecurityTransport
    }
    if len(cfg.StreamMuxers) == 0 {
        return nil, ErrNoStreamMuxer
    }
    
    return &Upgrader{
        identity:           id,
        securityTransports: cfg.SecurityTransports,
        streamMuxers:       cfg.StreamMuxers,
        resourceMgr:        cfg.ResourceManager,
    }, nil
}
```

### Upgrade 流程

```go
func (u *Upgrader) Upgrade(
    ctx context.Context,
    conn net.Conn,
    dir pkgif.Direction,
    remotePeer types.PeerID,
) (pkgif.UpgradedConn, error) {
    // Outbound 必须提供 remotePeer
    if dir == pkgif.DirOutbound && remotePeer == "" {
        return nil, ErrNoPeerID
    }
    
    // 检测 QUIC 连接，跳过升级
    if isQUICConn(conn) {
        return wrapQUICConn(conn, u.resourceMgr, remotePeer)
    }
    
    isServer := dir == pkgif.DirInbound
    
    // 1. 申请连接资源
    var connScope pkgif.ConnManagementScope
    if u.resourceMgr != nil {
        connScope, _ = u.resourceMgr.OpenConnection(dir, true, endpoint)
    }
    
    // 2. 协商安全协议（multistream-select）
    secTransport, err := u.negotiateSecurity(ctx, conn, isServer)
    
    // 3. 安全握手
    var secConn pkgif.SecureConn
    if isServer {
        secConn, err = secTransport.SecureInbound(ctx, conn, remotePeer)
    } else {
        secConn, err = secTransport.SecureOutbound(ctx, conn, remotePeer)
    }
    
    // 4. 设置远程 PeerID
    if connScope != nil {
        connScope.SetPeer(secConn.RemotePeer())
    }
    
    // 5. 协商多路复用器（multistream-select）
    muxer, err := u.negotiateMuxer(ctx, secConn, isServer)
    
    // 6. 创建多路复用连接
    muxedConn, err := muxer.NewConn(secConn, isServer, peerScope)
    
    // 7. 封装为 UpgradedConn
    return newUpgradedConnWithScope(muxedConn, secConn, secTransport.ID(), muxer.ID(), connScope), nil
}
```

### multistream-select 协商

```go
// internal/core/upgrader/multistream.go

import mss "github.com/multiformats/go-multistream"

// negotiateSecurity 协商安全协议
func (u *Upgrader) negotiateSecurity(ctx context.Context, conn net.Conn, isServer bool) (pkgif.SecureTransport, error) {
    // 设置协商超时
    deadline := time.Now().Add(defaultNegotiateTimeout)
    conn.SetDeadline(deadline)
    defer conn.SetDeadline(time.Time{})
    
    var selectedProto string
    
    if isServer {
        // 服务器端：从客户端提议中选择
        muxer := mss.NewMultistreamMuxer[string]()
        for _, st := range u.securityTransports {
            muxer.AddHandler(string(st.ID()), nil)
        }
        selectedProto, _, _ = muxer.Negotiate(conn)
    } else {
        // 客户端：提议协议列表
        protocols := make([]string, len(u.securityTransports))
        for i, st := range u.securityTransports {
            protocols[i] = string(st.ID())
        }
        selectedProto, _ = mss.SelectOneOf(protocols, conn)
    }
    
    // 找到对应的 SecureTransport
    for _, st := range u.securityTransports {
        if string(st.ID()) == selectedProto {
            return st, nil
        }
    }
    return nil, fmt.Errorf("negotiated protocol %s not found", selectedProto)
}

// negotiateMuxer 协商多路复用器（类似逻辑）
func (u *Upgrader) negotiateMuxer(ctx context.Context, conn net.Conn, isServer bool) (pkgif.StreamMuxer, error)
```

---

## 数据结构

### upgradedConn

```go
// internal/core/upgrader/conn.go

type upgradedConn struct {
    pkgif.MuxedConn                    // 嵌入多路复用连接
    
    secConn       pkgif.SecureConn     // 安全连接（用于访问 PeerID）
    securityProto types.ProtocolID     // 使用的安全协议（如 "/tls/1.0.0"）
    muxerID       string               // 使用的多路复用器（如 "/yamux/1.0.0"）
    connScope     pkgif.ConnManagementScope  // 资源范围
}

func (c *upgradedConn) LocalPeer() types.PeerID   { return c.secConn.LocalPeer() }
func (c *upgradedConn) RemotePeer() types.PeerID  { return c.secConn.RemotePeer() }
func (c *upgradedConn) Security() types.ProtocolID { return c.securityProto }
func (c *upgradedConn) Muxer() string              { return c.muxerID }

func (c *upgradedConn) Close() error {
    err := c.MuxedConn.Close()
    if c.connScope != nil {
        c.connScope.Done()
    }
    return err
}
```

### QUIC 连接封装

```go
// QUIC 连接自带 TLS 1.3 和流多路复用，跳过升级流程

type quicUpgradedConn struct {
    pkgif.Connection
}

func (c *quicUpgradedConn) Security() types.ProtocolID {
    return types.ProtocolID("/quic/tls/1.3")
}

func (c *quicUpgradedConn) Muxer() string {
    return "/quic/muxer/1.0"
}
```

---

## 状态管理

| 状态 | 管理方式 | 说明 |
|------|----------|------|
| identity | 不可变 | 创建时固定 |
| securityTransports | 不可变 | 按优先级排序的安全传输列表 |
| streamMuxers | 不可变 | 按优先级排序的多路复用器列表 |
| resourceMgr | 可选 | 资源管理器引用 |

---

## 错误处理

```go
// internal/core/upgrader/errors.go

var (
    ErrNilIdentity         = errors.New("upgrader: identity is nil")
    ErrNoSecurityTransport = errors.New("upgrader: no security transport configured")
    ErrNoStreamMuxer       = errors.New("upgrader: no stream muxer configured")
    ErrNoPeerID            = errors.New("upgrader: peer ID required for outbound")
    ErrSecurityNegotiation = errors.New("upgrader: security negotiation failed")
    ErrMuxerNegotiation    = errors.New("upgrader: muxer negotiation failed")
    ErrHandshakeFailed     = errors.New("upgrader: handshake failed")
    ErrPeerMismatch        = errors.New("upgrader: peer ID mismatch")
)
```

---

## 超时配置

```go
const (
    defaultNegotiateTimeout = 60 * time.Second  // 协议协商超时
    defaultHandshakeTimeout = 30 * time.Second  // 握手超时
)
```

---

**最后更新**：2026-01-25
