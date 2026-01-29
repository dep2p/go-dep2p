# C2-03 core_security 设计审查报告

> **审查日期**: 2026-01-13  
> **审查人**: AI Agent  
> **审查依据**: requirements.md, overview.md, internals.md, go-libp2p TLS 实现

---

## 一、需求理解

### 1.1 功能需求

| 需求 | 说明 | 优先级 | 验收标准 |
|------|------|--------|----------|
| FR-SEC-001 | TLS 1.3 握手 | P0 | 成功建立加密通道 |
| FR-SEC-002 | 自签名证书生成 | P0 | 证书包含节点公钥 |
| FR-SEC-003 | 身份验证 (INV-001) | P0 | RemotePeer == ExpectedPeer |
| FR-SEC-004 | 公钥提取 | P0 | 正确提取 Ed25519 公钥 |

**核心**: INV-001 身份第一性不变量 - 必须验证 PeerID 匹配

### 1.2 非功能需求

- **NFR-SEC-001**: TLS 1.3，前向保密
- **NFR-SEC-002**: 握手 < 50ms (本地)

---

## 二、架构理解

### 2.1 模块依赖

```
core_security
    ↓
core_identity (获取私钥/公钥)
    ↓
crypto/tls, crypto/x509 (标准库)
```

**被依赖**: core_transport (QUIC/TCP 使用 TLS 配置)

### 2.2 核心组件

1. **CertGenerator** (`tls/cert.go`)
   - 生成自签名 TLS 证书
   - 嵌入 Ed25519 公钥到证书扩展
   - OID: `1.3.6.1.4.1.99999.1` (自定义)

2. **PeerVerifier** (`tls/verify.go`)
   - 从证书提取公钥
   - 派生 PeerID
   - 验证匹配 (INV-001)

3. **Transport** (`tls/transport.go`)
   - SecureInbound: TLS 服务器握手
   - SecureOutbound: TLS 客户端握手
   - 配置 VerifyPeerCertificate

4. **SecureConn** (`tls/conn.go`)
   - 封装 *tls.Conn
   - 实现 pkg/interfaces/security.SecureConn

---

## 三、TLS 握手流程

### 3.1 证书生成流程

```
1. core_identity 提供 Ed25519 PrivateKey
2. 创建 x509.Certificate 模板
   - CN = PeerID (string)
   - ExtKeyUsage = ServerAuth + ClientAuth
   - ExtraExtensions = dep2p-public-key (32 bytes)
3. x509.CreateCertificate 自签名
4. 返回 tls.Certificate{Certificate, PrivateKey}
```

**关键**: 公钥嵌入证书扩展，不依赖标准 SubjectPublicKeyInfo

### 3.2 握手验证流程

```
Client                        Server
  |                             |
  |-- ClientHello ------------->|
  |                             | 1. 生成证书
  |<-- ServerCertificate -------|    (含公钥扩展)
  | 2. 提取公钥                  |
  | 3. 派生 PeerID               |
  | 4. 验证 == ExpectedPeer      |
  |-- ClientCertificate -------->| 5. 服务器验证客户端
  |-- ClientFinished ----------->|
  |<-- ServerFinished -----------|
  |                             |
  [加密通道建立]
```

**INV-001 验证点**: 
- 客户端: VerifyPeerCertificate (服务器证书)
- 服务器: VerifyPeerCertificate (客户端证书)

---

## 四、go-libp2p 参考分析

### 4.1 证书扩展 OID

**libp2p 使用**:
```go
var extensionPrefix = []int{1, 3, 6, 1, 4, 1, 53594} // libp2p
```

**DeP2P 使用**:
```go
var oidDep2pPublicKey = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1}
```

**说明**: 自定义 OID 不冲突，99999 为临时企业号

### 4.2 公钥签名 (libp2p 特性)

libp2p 使用 `signedKey` 结构：
- 嵌入公钥 + 签名
- 签名证明: "我拥有此公钥"

**DeP2P 简化**:
- 直接嵌入 Ed25519 公钥 (32 bytes)
- 证书本身已签名 (自签名)
- 无需额外签名字段

### 4.3 握手流程对比

| 步骤 | libp2p | DeP2P |
|------|--------|-------|
| 证书生成 | keyToCertificate | GenerateCert |
| 公钥嵌入 | 证书扩展 | 证书扩展 |
| 验证 | VerifyPeerCertificate | VerifyPeerCertificate |
| PeerID 派生 | peer.IDFromPublicKey | identity.PeerIDFromPublicKey |

**兼容性**: 流程一致，OID 不同

---

## 五、关键技术点

### 5.1 Ed25519 密钥处理

**core_identity 提供**:
```go
type Identity struct {
    peerID  string
    privKey pkgif.PrivateKey  // Ed25519
    pubKey  pkgif.PublicKey   // Ed25519
}
```

**证书要求**:
- x509.CreateCertificate 需要 crypto.Signer
- Ed25519 实现 crypto.Signer ✅
- 公钥: ed25519.PublicKey (32 bytes)

### 5.2 公钥扩展提取

```go
for _, ext := range cert.Extensions {
    if ext.Id.Equal(oidDep2pPublicKey) {
        if len(ext.Value) != ed25519.PublicKeySize { // 32
            return nil, ErrInvalidPublicKey
        }
        return ed25519.PublicKey(ext.Value), nil
    }
}
```

### 5.3 INV-001 验证

```go
// 1. 提取公钥
pubKey := extractPublicKey(cert)

// 2. 派生 PeerID
actualPeer := identity.PeerIDFromPublicKey(pubKey)

// 3. 验证匹配 ⭐
if actualPeer != expectedPeer {
    return ErrPeerIDMismatch // 拒绝连接
}
```

---

## 六、设计决策

### 6.1 TLS 1.3 强制

**理由**:
- TLS 1.3 强制 ECDHE (前向保密)
- 简化密码套件配置
- 0-RTT 支持 (后续优化)

**配置**:
```go
MinVersion: tls.VersionTLS13
```

### 6.2 自定义验证

**理由**:
- 标准 TLS 验证依赖 CA
- DeP2P 自签名证书，无 CA
- 需要基于 PeerID 验证

**配置**:
```go
InsecureSkipVerify: true  // 跳过标准验证
VerifyPeerCertificate: customVerify  // 自定义验证
```

### 6.3 证书有效期

**设计**:
- 有效期: 1 年
- 自动轮换: 未实现 (Phase 2)

**理由**:
- P2P 网络节点长期运行
- 证书轮换复杂度高

---

## 七、实现要点

### 7.1 接口重构

**当前问题** (`pkg/interfaces/security.go`):
```go
// ❌ 错误
SecureInbound(ctx context.Context, conn net.Conn, peerID string) (SecureConn, error)
LocalPeer() string
LocalPublicKey() []byte
```

**重构为**:
```go
// ✅ 正确
SecureInbound(ctx context.Context, conn net.Conn, peerID types.PeerID) (SecureConn, error)
LocalPeer() types.PeerID
LocalPublicKey() types.PublicKey
```

### 7.2 测试策略

**必须真实测试**:
1. TLS 握手 (loopback)
2. 证书生成 (从 core_identity)
3. 公钥提取 (验证 32 bytes)
4. INV-001 验证 (匹配/不匹配)

**禁止**:
- `t.Skip()` ❌
- Mock TLS 连接 ❌
- 硬编码 PeerID ❌

### 7.3 错误处理

```go
var (
    ErrNoCertificate        = errors.New("security: no certificate")
    ErrPeerIDMismatch       = errors.New("security: peer ID mismatch (INV-001)")
    ErrNoPublicKeyExtension = errors.New("security: no public key extension")
    ErrInvalidPublicKey     = errors.New("security: invalid public key")
)
```

---

## 八、风险评估

| 风险 | 等级 | 缓解措施 |
|------|------|----------|
| 证书扩展兼容性 | 低 | 自定义 OID，无冲突 |
| Ed25519 转换 | 低 | 标准库支持 |
| TLS 1.3 握手 | 中 | 参考 go-libp2p，充分测试 |
| INV-001 验证逻辑 | 高 | 严格测试，覆盖边界情况 |

**高风险点**: INV-001 验证必须 100% 正确，否则破坏身份安全

---

## 九、实施计划摘要

### 9.1 核心文件 (8 个)

1. `tls/cert.go` - 证书生成
2. `tls/verify.go` - INV-001 验证
3. `tls/transport.go` - TLS 握手
4. `tls/conn.go` - 安全连接
5. `tls/config.go` - TLS 配置
6. `errors.go` - 错误定义
7. `module.go` - Fx 集成
8. `testing.go` - 测试辅助

### 9.2 测试文件 (8 个)

- `transport_test.go`
- `integration_test.go`
- `module_test.go`
- `tls/transport_test.go`
- `tls/cert_test.go`
- `tls/verify_test.go`
- `errors_test.go`

### 9.3 预期成果

- **代码量**: ~1200 行
- **覆盖率**: > 70%
- **真实测试**: 8 个 (无 Skip)
- **认证等级**: A (真实可用)

---

## 十、总结

### 10.1 关键理解

✅ **TLS 1.3 握手**: 使用自签名证书，嵌入 Ed25519 公钥  
✅ **INV-001 验证**: 从证书提取公钥 → 派生 PeerID → 验证匹配  
✅ **前向保密**: TLS 1.3 强制 ECDHE  
✅ **标准兼容**: 参考 go-libp2p，保持兼容性

### 10.2 设计符合度

| 文档 | 符合度 | 说明 |
|------|--------|------|
| requirements.md | 100% | 4 个功能需求明确 |
| overview.md | 100% | 架构清晰 |
| internals.md | 100% | 实现路径明确 |
| go-libp2p | 90% | 流程相同，OID 不同 |

### 10.3 准入 Step 2

✅ **准许进入 Step 2**: 接口重构

---

**审查完成日期**: 2026-01-13  
**审查人签名**: AI Agent  
**审查结论**: ✅ **通过** (设计清晰，可实施)
