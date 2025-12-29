# Security 安全层模块

## 概述

**层级**: Tier 2  
**职责**: 提供 TLS 1.3 安全层，实现通信加密和身份认证。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [安全协议规范](../../../docs/01-design/protocols/transport/02-security.md) | TLS 1.3 设计 |
| [身份协议规范](../../../docs/01-design/protocols/foundation/01-identity.md) | 身份认证 |

## 能力清单

### 核心能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| TLS 1.3 握手 | ✅ 已实现 | 1-RTT 握手 |
| 自签名证书 | ✅ 已实现 | 基于节点公钥生成 |
| 身份验证 | ✅ 已实现 | 验证对方 NodeID |
| 加密通信 | ✅ 已实现 | AES-GCM / ChaCha20-Poly1305 |
| 前向安全 | ✅ 已实现 | ECDHE 密钥交换 |

### 0-RTT 能力

| 能力 | 状态 | 说明 |
|------|------|------|
| Session 缓存 | ✅ 已实现 | transport/quic SessionStore |
| 0-RTT 重连 | ✅ 已实现 | QUIC session resumption |
| 重放保护 | ✅ 已实现 | QUIC 内置重放保护 |

## 依赖关系

### 接口依赖

```
pkg/types/              → NodeID
pkg/interfaces/core/    → Connection
pkg/interfaces/security/ → SecureTransport, SecureConn
pkg/interfaces/identity/ → Identity
```

### 模块依赖

```
identity → 节点身份（用于生成证书）
transport → 底层连接
```

## 目录结构

```
security/
├── README.md            # 本文件
├── module.go            # fx 模块定义
└── tls/                 # TLS 实现
    ├── README.md        # TLS 子模块说明
    ├── transport.go     # TLS 安全传输
    ├── config.go        # TLS 配置
    └── cert.go          # 证书生成
```

## 公共接口

实现 `pkg/interfaces/security/` 中的接口：

```go
// SecureTransport 安全传输接口
type SecureTransport interface {
    // SecureInbound 安全升级入站连接
    SecureInbound(ctx context.Context, conn net.Conn) (SecureConn, error)
    
    // SecureOutbound 安全升级出站连接
    SecureOutbound(ctx context.Context, conn net.Conn, remoteID types.NodeID) (SecureConn, error)
}

// SecureConn 安全连接接口
type SecureConn interface {
    net.Conn
    
    // LocalPeer 返回本地节点 ID
    LocalPeer() types.NodeID
    
    // RemotePeer 返回远程节点 ID
    RemotePeer() types.NodeID
    
    // RemotePublicKey 返回远程公钥
    RemotePublicKey() crypto.PublicKey
    
    // ConnectionState 返回 TLS 状态
    ConnectionState() tls.ConnectionState
}
```

## 关键算法

### 自签名证书生成 (来自设计文档)

```go
func GenerateCertificate(identity Identity) (*tls.Certificate, error) {
    // 1. 创建证书模板
    template := &x509.Certificate{
        SerialNumber: big.NewInt(time.Now().UnixNano()),
        Subject: pkix.Name{
            CommonName: identity.ID().String(),
        },
        NotBefore: time.Now(),
        NotAfter:  time.Now().Add(365 * 24 * time.Hour),
        
        KeyUsage:              x509.KeyUsageDigitalSignature,
        ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
        BasicConstraintsValid: true,
    }
    
    // 2. 添加 NodeID 到 SAN 扩展
    template.ExtraExtensions = []pkix.Extension{
        {
            Id:       oidNodeID,
            Critical: true,
            Value:    identity.ID().Bytes(),
        },
    }
    
    // 3. 使用节点私钥签名
    certDER, err := x509.CreateCertificate(
        rand.Reader,
        template,
        template,
        identity.PublicKey(),
        identity.PrivateKey(),
    )
    
    return &tls.Certificate{
        Certificate: [][]byte{certDER},
        PrivateKey:  identity.PrivateKey(),
    }, nil
}
```

### 身份验证流程

```go
func (s *SecureTransport) verifyPeer(conn *tls.Conn, expectedID types.NodeID) error {
    state := conn.ConnectionState()
    if len(state.PeerCertificates) == 0 {
        return ErrNoPeerCertificate
    }
    
    cert := state.PeerCertificates[0]
    
    // 1. 从证书扩展提取 NodeID
    var peerID types.NodeID
    for _, ext := range cert.Extensions {
        if ext.Id.Equal(oidNodeID) {
            copy(peerID[:], ext.Value)
            break
        }
    }
    
    // 2. 验证 NodeID 是否匹配
    if expectedID != types.EmptyNodeID && peerID != expectedID {
        return ErrPeerIDMismatch
    }
    
    // 3. 验证 NodeID 与公钥的派生关系
    expectedPeerID := DeriveNodeID(cert.PublicKey)
    if peerID != expectedPeerID {
        return ErrInvalidPeerID
    }
    
    return nil
}
```

## TLS 配置

```go
type TLSConfig struct {
    // 最低 TLS 版本 (固定 1.3)
    MinVersion uint16  // tls.VersionTLS13
    
    // 密码套件 (TLS 1.3 自动选择)
    // - TLS_AES_128_GCM_SHA256
    // - TLS_AES_256_GCM_SHA384
    // - TLS_CHACHA20_POLY1305_SHA256
    
    // ALPN 协议
    NextProtos []string  // ["dep2p"]
    
    // 证书配置
    Certificates []tls.Certificate
    
    // 客户端认证
    ClientAuth tls.ClientAuthType  // RequireAnyClientCert
}
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Identity identityif.Identity `name:"identity"`
    Config   *securityif.Config  `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    SecureTransport securityif.SecureTransport `name:"secure_transport"`
}

func Module() fx.Option {
    return fx.Module("security",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 安全特性 (来自设计文档)

| 特性 | 说明 |
|------|------|
| 机密性 | AES-GCM / ChaCha20-Poly1305 加密 |
| 完整性 | AEAD 认证加密 |
| 身份认证 | 基于 NodeID 的双向认证 |
| 前向安全 | ECDHE 密钥交换 |
| 重放保护 | TLS 1.3 内置 |

## 相关文档

- [安全协议规范](../../../docs/01-design/protocols/transport/02-security.md)
- [身份协议规范](../../../docs/01-design/protocols/foundation/01-identity.md)
- [pkg/interfaces/security](../../../pkg/interfaces/security/)
