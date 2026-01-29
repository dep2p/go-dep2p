# security - 安全传输层

> **版本**: v1.2.0  
> **状态**: ✅ TLS 可用，Noise 基础实现  
> **覆盖率**: ~33%  
> **最后更新**: 2026-01-13

---

## 快速开始

```go
import (
    "github.com/dep2p/go-dep2p/internal/core/identity"
    "github.com/dep2p/go-dep2p/internal/core/security/tls"
)

// 创建身份
id, _ := identity.Generate()

// 创建 TLS 传输
transport, _ := tls.New(id)

// 服务器端握手
secureConn, _ := transport.SecureInbound(ctx, conn, remotePeer)

// 客户端握手
secureConn, _ := transport.SecureOutbound(ctx, conn, remotePeer)

// 使用安全连接
remotePubKey := secureConn.RemotePublicKey()
```

---

## 核心特性

### 1. TLS 1.3 加密

**功能**:
- ✅ TLS 1.3 强制
- ✅ 前向保密 (ECDHE)
- ✅ 双向认证
- ✅ 自签名证书

### 2. INV-001 身份验证 ⭐

**验证流程**:
1. 从证书提取 Ed25519 公钥（32 bytes）
2. 派生 PeerID = Hash(PublicKey)
3. 验证 PeerID == ExpectedPeer
4. 不匹配则拒绝连接

**测试验证**:
```
✅ PeerID 匹配验证通过
✅ PeerID 不匹配被正确拒绝
```

### 3. 证书生成

**证书结构**:
- CN = PeerID
- ExtKeyUsage = ServerAuth + ClientAuth
- Extensions = dep2p-public-key (OID: 1.3.6.1.4.1.99999.1)
- 有效期: 1 年

**测试验证**:
```
✅ 证书生成成功
✅ 找到公钥扩展: 32 bytes
```

---

## 测试结果（真实测试）

### TLS 传输测试

```
✅ TestGenerateCert              - 证书生成
✅ TestCertificateExtension      - 公钥扩展
✅ TestExtractPublicKey          - 公钥提取
✅ TestVerifyPeerCertificate_Match    - INV-001 匹配
✅ TestVerifyPeerCertificate_Mismatch - INV-001 拒绝
✅ TestTLSHandshake              - 端到端握手 ⭐
```

**测试输出**（真实）:
```
Server PeerID: zQmYU7Dgu6xt86bjgxqbUyjgjZsApuqt2aniX3hR9RjUTS8
Client PeerID: zQmcpzm5rucM3chyjqVJ68TPAEasmy41hMfbGHyC9MV4tPh
✅ Client 握手成功
✅ Server 握手成功
✅ TLS 握手完整验证通过
```

---

## 文件结构

```
internal/core/security/
├── doc.go (54行)
├── module.go (37行)
├── errors.go (16行)
├── testing.go (17行)
└── tls/ (~450行)
    ├── transport.go (177行)
    ├── cert.go (91行)
    ├── verify.go (112行)
    ├── conn.go (77行)
    └── errors.go (19行)
```

**代码总量**: ~750 行

---

## Fx 模块集成

```go
import "github.com/dep2p/go-dep2p/internal/core/security"

app := fx.New(
    identity.Module(),
    security.Module(),
    fx.Invoke(func(st pkgif.SecureTransport) {
        // 使用安全传输
    }),
)
```

---

## 依赖关系

```
security
    ↓
identity (密钥和 PeerID)
    ↓
crypto/tls, crypto/x509 (标准库)
```

---

## 支持协议

### TLS 1.3 ✅ 完全可用
- 自签名证书
- INV-001 身份验证
- 前向保密

### Noise 协议 ⚠️ 基础实现
- 框架代码已就绪
- 完整握手待实现
- 计划后续版本完成

## 性能指标

**基准测试** (`go test -bench=.`):
- 证书生成: ~XXX ns/op
- PeerID 验证: ~XXX ns/op  
- TLS 握手: ~XXX ms/op

## 已知限制

1. **覆盖率**: ~33%（核心功能已覆盖）
2. **Noise**: 基础实现，完整功能开发中
3. **多协议选择**: 协议协商待完成

---

## 相关文档

| 文档 | 路径 |
|------|------|
| **设计文档** | design/03_architecture/L6_domains/core_security/ |
| **接口定义** | pkg/interfaces/security.go |
| **约束检查** | CONSTRAINTS_CHECK.md (A- 级) |

---

**维护者**: DeP2P Team  
**最后更新**: 2026-01-13
