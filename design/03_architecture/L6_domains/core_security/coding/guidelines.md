# core_security 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/security/
├── module.go           # Fx 模块
├── transport.go        # 主接口实现
├── cert.go             # 证书生成
├── verify.go           # 身份验证
├── errors.go           # 错误定义
└── *_test.go           # 测试
```

---

## 安全编码规范

### 私钥处理

```
// 不要在日志中打印私钥
// BAD:
log.Printf("key: %v", privateKey)

// GOOD:
log.Printf("using identity: %s", nodeID.Pretty())
```

### 证书验证

```
// 始终验证对端身份
// 不要跳过验证！
config := &tls.Config{
    InsecureSkipVerify: true, // 必须，因为自签名
    VerifyPeerCertificate: verifyPeer, // 自定义验证
}
```

---

## 错误处理

### 安全相关错误

```
// 不要泄露过多信息
// BAD:
return fmt.Errorf("expected %s got %s", expected, actual)

// GOOD:
return ErrPeerMismatch  // 只告知验证失败
```

### 错误日志

```
// 只在 Debug 级别记录详细信息
if err := verifyPeer(cert, expected); err != nil {
    slog.Debug("peer verification failed",
        "expected", expected.Pretty(),
        "error", err,
    )
    return ErrPeerMismatch
}
```

---

## 并发模式

Security 模块的配置是不可变的：

```
type securityTransport struct {
    cert       *tls.Certificate  // 不可变
    serverConf *tls.Config       // 不可变
}

// TLSClientConfig 每次调用创建新配置（包含 expectedPeer）
func (s *securityTransport) TLSClientConfig(expectedPeer types.NodeID) *tls.Config {
    // 基于 baseConfig 克隆
    conf := s.baseClientConf.Clone()
    conf.VerifyPeerCertificate = makeVerifier(expectedPeer)
    return conf
}
```

---

## 测试规范

### 安全测试

```
func TestVerifyPeer_Mismatch(t *testing.T) {
    // 验证身份不匹配时返回错误
    cert := generateCert(identity1)
    wrongPeer := identity2.ID()
    
    err := verifyPeer(cert, wrongPeer)
    
    assert.ErrorIs(t, err, ErrPeerMismatch)
}

func TestTLS_ForwardSecrecy(t *testing.T) {
    // 验证使用 TLS 1.3
    conn := establishConnection(t)
    
    assert.Equal(t, uint16(tls.VersionTLS13), conn.ConnectionState().Version)
}
```

---

**最后更新**：2026-01-11
