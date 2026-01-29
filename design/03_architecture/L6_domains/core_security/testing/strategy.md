# core_security 测试策略

> 测试范围与方法

---

## 测试范围

| 组件 | 覆盖要求 | 说明 |
|------|----------|------|
| CertGenerator | 100% | 安全关键 |
| PeerVerifier | 100% | INV-001 核心 |
| TLSConfig | 90%+ | 配置正确性 |

---

## 测试类型

### 单元测试

```
func TestGenerateCert(t *testing.T) {
    id, _ := identity.NewService()
    
    cert, err := GenerateCert(id)
    
    require.NoError(t, err)
    assert.NotNil(t, cert.Certificate)
    assert.NotNil(t, cert.PrivateKey)
}

func TestExtractPublicKey(t *testing.T) {
    id, _ := identity.NewService()
    cert, _ := GenerateCert(id)
    x509Cert, _ := x509.ParseCertificate(cert.Certificate[0])
    
    pubKey, err := extractPublicKey(x509Cert)
    
    require.NoError(t, err)
    assert.Equal(t, id.PublicKey().Bytes(), []byte(pubKey))
}
```

### 验证测试

```
func TestVerifyPeer_Success(t *testing.T) {
    id, _ := identity.NewService()
    cert, _ := GenerateCert(id)
    
    err := verifyPeer(cert.Certificate, id.ID())
    
    assert.NoError(t, err)
}

func TestVerifyPeer_Mismatch(t *testing.T) {
    id1, _ := identity.NewService()
    id2, _ := identity.NewService()
    cert, _ := GenerateCert(id1)
    
    err := verifyPeer(cert.Certificate, id2.ID())
    
    assert.ErrorIs(t, err, ErrPeerMismatch)
}
```

---

## 关键测试场景

| 场景 | 测试点 |
|------|--------|
| 证书生成 | 包含正确公钥 |
| 身份验证成功 | NodeID 匹配 |
| 身份验证失败 | NodeID 不匹配 |
| 无证书 | 正确拒绝 |
| 无公钥扩展 | 正确拒绝 |
| TLS 版本 | 强制 TLS 1.3 |

---

## 集成测试

```
func TestSecureHandshake(t *testing.T) {
    // 创建两个安全传输
    sec1, _ := NewSecurityTransport(identity1)
    sec2, _ := NewSecurityTransport(identity2)
    
    // 建立连接
    serverConf := sec1.TLSServerConfig()
    clientConf := sec2.TLSClientConfig(identity1.ID())
    
    // TLS 握手
    serverConn, clientConn := tlsHandshake(t, serverConf, clientConf)
    
    // 验证
    assert.Equal(t, uint16(tls.VersionTLS13), 
        clientConn.ConnectionState().Version)
}
```

---

## Mock 策略

```
type MockSecurityTransport struct {
    mock.Mock
}

func (m *MockSecurityTransport) TLSServerConfig() *tls.Config {
    args := m.Called()
    return args.Get(0).(*tls.Config)
}

func (m *MockSecurityTransport) TLSClientConfig(peer types.NodeID) *tls.Config {
    args := m.Called(peer)
    return args.Get(0).(*tls.Config)
}
```

---

**最后更新**：2026-01-11
