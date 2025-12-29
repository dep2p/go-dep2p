# Noise 安全协议模块

## 概述

Noise 模块实现了基于 Noise Protocol Framework 的安全传输层，作为 TLS 的轻量级替代方案。

## 架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Noise 安全传输                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐        │
│  │  Transport   │────►│  Handshaker  │────►│  SecureConn  │        │
│  └──────────────┘     └──────────────┘     └──────────────┘        │
│        │                    │                     │                 │
│        │                    │                     │                 │
│        ▼                    ▼                     ▼                 │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐        │
│  │  SecureIn/   │     │   Noise XX   │     │  加密读写    │        │
│  │  OutBound    │     │   握手协议   │     │  CipherState │        │
│  └──────────────┘     └──────────────┘     └──────────────┘        │
│                                                                      │
│  握手流程 (Noise XX):                                               │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  发起者                         响应者                       │   │
│  │    │                               │                         │   │
│  │    │──────── e ────────────────────►                         │   │
│  │    │                               │                         │   │
│  │    │◄──────── e, ee, s, es ────────│                         │   │
│  │    │                               │                         │   │
│  │    │──────── s, se ────────────────►                         │   │
│  │    │                               │                         │   │
│  │    │◄─────── 加密通信 ─────────────►                         │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## 文件结构

```
internal/core/security/noise/
├── README.md           # 本文档
├── handshake.go        # Noise XX 握手协议实现
├── conn.go             # 加密连接封装
├── transport.go        # SecureTransport 实现
└── noise_test.go       # 单元测试
```

## 核心组件

### Handshaker

处理 Noise 握手协议，支持以下模式：

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| XX | 双向认证 | 双方都不知道对方身份（默认） |
| IK | 发起者已知响应者 | 服务端公钥已知 |
| NK | 仅验证响应者 | 匿名客户端 |

### SecureConn

加密连接，提供：
- 透明加密/解密
- 分片处理大消息
- 实现 `securityif.SecureConn` 接口

### Transport

安全传输层，实现 `securityif.SecureTransport` 接口：

```go
type Transport interface {
    SecureInbound(ctx context.Context, conn transport.Conn) (SecureConn, error)
    SecureOutbound(ctx context.Context, conn transport.Conn, remotePeer types.NodeID) (SecureConn, error)
    Protocol() string
}
```

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `HandshakePattern` | "XX" | 握手模式 |
| `DHCurve` | "25519" | DH 曲线（Curve25519） |
| `CipherSuite` | "ChaChaPoly" | 加密套件 |
| `HashFunction` | "SHA256" | 哈希函数 |
| `MaxMessageSize` | 65535 | 最大消息大小 |
| `HandshakeTimeout` | 10s | 握手超时 |

## 使用示例

### 配置 Noise 协议

```go
import (
    securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
)

// 配置使用 Noise
config := securityif.DefaultConfig()
config.Protocol = "noise"
config.NoiseConfig = &securityif.NoiseConfig{
    HandshakePattern: "XX",
    DHCurve:          "25519",
    CipherSuite:      "ChaChaPoly",
    HashFunction:     "SHA256",
}
```

### 直接使用 Noise Transport

```go
import (
    "github.com/dep2p/go-dep2p/internal/core/security/noise"
)

// 创建 Noise 传输
transport, err := noise.NewTransport(identity, config.NoiseConfig, logger)
if err != nil {
    return err
}

// 作为客户端
secureConn, err := transport.SecureOutbound(ctx, rawConn, remotePeerID)

// 作为服务端
secureConn, err := transport.SecureInbound(ctx, rawConn)
```

## Noise vs TLS

| 特性 | Noise | TLS 1.3 |
|------|-------|---------|
| 握手消息数 | 3 | 4+ |
| 证书需求 | 无 | 需要 X.509 |
| 实现复杂度 | 低 | 高 |
| 代码大小 | ~1KB | ~50KB+ |
| CPU 开销 | 低 | 中 |
| 适用场景 | P2P、IoT | Web、企业 |

## 安全考虑

1. **密钥派生**：使用 HKDF 从握手派生会话密钥
2. **重放保护**：每个方向使用独立的计数器
3. **前向保密**：使用临时 DH 密钥
4. **身份隐藏**：XX 模式下身份在加密后发送

## 相关文档

- [安全协议设计](../../../docs/01-design/protocols/transport/02-security.md)
- [TLS 实现](../tls/README.md)
- [Noise Protocol Specification](https://noiseprotocol.org/noise.html)

