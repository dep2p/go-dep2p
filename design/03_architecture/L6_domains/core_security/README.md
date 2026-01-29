# Core Security 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 安全层（Core Layer）

---

## 模块概述

core_security 提供连接加密和身份验证能力，支持 TLS 1.3 和 Noise 协议。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/security/` |
| **Fx 模块** | `fx.Module("security")` |
| **状态** | ✅ 已实现 |
| **依赖** | core_identity |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        core_security 职责                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. TLS 1.3 传输（默认）                                                    │
│     • 生成自签名证书                                                        │
│     • 配置 TLS 1.3                                                         │
│     • 证书验证                                                              │
│                                                                             │
│  2. Noise 传输                                                              │
│     • Noise_XX 握手                                                         │
│     • 密钥交换                                                              │
│     • 前向保密                                                              │
│                                                                             │
│  3. 身份验证 (INV-001)                                                      │
│     • 从证书/握手提取公钥                                                   │
│     • 验证 PeerID 匹配                                                      │
│     • 拒绝身份不匹配的连接                                                  │
│                                                                             │
│  4. 加密通道                                                                │
│     • 端到端加密                                                            │
│     • 前向保密                                                              │
│     • 完整性保护                                                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 支持的安全传输

| 传输 | 协议 ID | 默认 | 说明 |
|------|---------|------|------|
| TLS 1.3 | `/tls/1.0.0` | ✅ | 推荐，标准协议 |
| Noise | `/noise` | | libp2p 兼容 |
| Insecure | `/insecure` | | 仅测试使用 |

---

## 公共接口

```go
// pkg/interfaces/security.go

// SecureTransport 安全传输接口
type SecureTransport interface {
    // SecureInbound 安全化入站连接
    SecureInbound(ctx context.Context, insecure net.Conn, p types.PeerID) (SecureConn, error)
    
    // SecureOutbound 安全化出站连接
    SecureOutbound(ctx context.Context, insecure net.Conn, p types.PeerID) (SecureConn, error)
    
    // ID 返回协议 ID
    ID() protocol.ID
}

// SecureConn 安全连接接口
type SecureConn interface {
    net.Conn
    LocalPeer() types.PeerID
    RemotePeer() types.PeerID
    RemotePublicKey() crypto.PubKey
}
```

---

## 目录结构

```
internal/core/security/
├── doc.go
├── module.go           # Fx 模块
├── security.go         # 安全层抽象
├── tls/                # TLS 实现
│   ├── transport.go
│   └── config.go
├── noise/              # Noise 实现
│   ├── transport.go
│   └── handshake.go
└── interfaces/
    └── security.go
```

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `Transports` | [TLS, Noise] | 安全传输优先级 |
| `TLS.MinVersion` | TLS 1.3 | 最低 TLS 版本 |
| `InsecureAllowed` | false | 是否允许不安全连接 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_identity](../core_identity/) | 身份管理 |
| [core_upgrader](../core_upgrader/) | 连接升级 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
