# core_security 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/security/
├── module.go           # Fx 模块定义
├── transport.go        # 主实现
├── cert.go             # 证书生成
├── verify.go           # 身份验证
└── transport_test.go   # 测试
```

---

## 证书生成

### 证书结构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          DeP2P 自签名证书                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Subject:                                                                   │
│  • CN = NodeID (Base58)                                                    │
│                                                                             │
│  Extensions:                                                                │
│  • dep2p-public-key = Ed25519 公钥 (32 bytes)                              │
│                                                                             │
│  签名算法: Ed25519                                                          │
│  有效期: 1 年（自动更新）                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 证书生成实现

```
func GenerateCert(id identity.Service) (*tls.Certificate, error) {
    // 创建证书模板
    template := &x509.Certificate{
        SerialNumber: big.NewInt(1),
        Subject: pkix.Name{
            CommonName: id.ID().String(),
        },
        NotBefore: time.Now(),
        NotAfter:  time.Now().Add(365 * 24 * time.Hour),
        KeyUsage:  x509.KeyUsageDigitalSignature,
        ExtKeyUsage: []x509.ExtKeyUsage{
            x509.ExtKeyUsageClientAuth,
            x509.ExtKeyUsageServerAuth,
        },
        ExtraExtensions: []pkix.Extension{{
            Id:    oidDep2pPublicKey,
            Value: id.PublicKey().Bytes(),
        }},
    }
    
    // 自签名
    certDER, err := x509.CreateCertificate(
        rand.Reader, template, template,
        id.PublicKey(), id.PrivateKey(),
    )
    if err != nil {
        return nil, err
    }
    
    return &tls.Certificate{
        Certificate: [][]byte{certDER},
        PrivateKey:  id.PrivateKey(),
    }, nil
}
```

---

## 身份验证

### INV-001 实现

```
func verifyExpectedPeer(rawCerts [][]byte, expected types.NodeID) error {
    if len(rawCerts) == 0 {
        return ErrNoCertificate
    }
    
    cert, err := x509.ParseCertificate(rawCerts[0])
    if err != nil {
        return fmt.Errorf("parse cert: %w", err)
    }
    
    // 提取公钥
    pubKey, err := extractPublicKey(cert)
    if err != nil {
        return err
    }
    
    // 计算 NodeID
    actual := deriveNodeID(pubKey)
    
    // 验证匹配
    if !actual.Equals(expected) {
        return fmt.Errorf("%w: expected %s, got %s",
            ErrPeerMismatch, expected.Pretty(), actual.Pretty())
    }
    
    return nil
}
```

### 公钥提取

```
var oidDep2pPublicKey = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1}

func extractPublicKey(cert *x509.Certificate) (ed25519.PublicKey, error) {
    for _, ext := range cert.Extensions {
        if ext.Id.Equal(oidDep2pPublicKey) {
            if len(ext.Value) != ed25519.PublicKeySize {
                return nil, ErrInvalidPublicKey
            }
            return ed25519.PublicKey(ext.Value), nil
        }
    }
    return nil, ErrNoPublicKeyExtension
}
```

---

## 错误处理

```
var (
    ErrNoCertificate       = errors.New("security: no certificate provided")
    ErrPeerMismatch        = errors.New("security: peer ID mismatch")
    ErrInvalidPublicKey    = errors.New("security: invalid public key")
    ErrNoPublicKeyExtension = errors.New("security: no public key extension")
)
```

---

## 安全考虑

### 密码套件

```
// 仅允许 TLS 1.3 的安全套件
// TLS 1.3 自动使用安全套件，无需显式配置
MinVersion: tls.VersionTLS13
```

### 前向保密

```
TLS 1.3 强制使用 ECDHE，提供前向保密：
• 每次握手生成临时密钥
• 历史会话密钥泄露不影响其他会话
```

---

**最后更新**：2026-01-11
