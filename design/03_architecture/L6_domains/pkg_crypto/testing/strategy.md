# pkg_crypto 测试策略

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 测试目标

| 目标 | 指标 | 状态 |
|------|------|------|
| 覆盖率 | > 80% | ✅ 83.0% |
| 边界测试 | 所有边界条件 | ✅ 完成 |
| 性能基准 | 所有密钥类型 | ✅ 完成 |
| 确定性 | 支持固定随机源 | ✅ 完成 |

---

## 测试分类

### 1. 单元测试

**目标**：验证每个函数的正确性

| 组件 | 测试文件 | 覆盖内容 |
|------|---------|---------|
| Ed25519 | `ed25519_test.go` | 生成、签名、验证、序列化 |
| Secp256k1 | `secp256k1_test.go` | 生成、签名、验证、序列化 |
| ECDSA | `ecdsa_test.go` | 生成、签名、验证、序列化 |
| RSA | `rsa_test.go` | 生成、签名、验证、序列化 |
| 序列化 | `marshal_test.go` | 公钥、私钥、签名序列化 |
| PeerID | `peerid_test.go` | PeerID 派生和验证 |
| 密钥存储 | `keystore_test.go` | 存储、加密、检索 |

### 2. 集成测试

**目标**：验证模块间集成

```go
// 示例：端到端测试
func TestE2E_KeyGeneration_To_PeerID(t *testing.T) {
    // 1. 生成密钥
    priv, pub, err := GenerateKeyPair(KeyTypeEd25519)
    
    // 2. 派生 PeerID
    peerID, err := PeerIDFromPublicKey(pub)
    
    // 3. 验证 PeerID
    valid, err := VerifyPeerID(pub, peerID)
    assert.True(t, valid)
    
    // 4. 序列化/反序列化
    data, err := MarshalPrivateKey(priv)
    priv2, err := UnmarshalPrivateKeyBytes(data)
    
    // 5. 验证一致性
    assert.True(t, priv.Equals(priv2))
}
```

### 3. 基准测试

**目标**：性能验证和回归检测

```go
func BenchmarkEd25519_Generate(b *testing.B) {
    for i := 0; i < b.N; i++ {
        _, _, _ = GenerateKeyPair(KeyTypeEd25519)
    }
}

func BenchmarkEd25519_Sign(b *testing.B) {
    priv, _, _ := GenerateKeyPair(KeyTypeEd25519)
    data := make([]byte, 256)
    rand.Read(data)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = priv.Sign(data)
    }
}
```

---

## 测试用例设计

### 正常路径测试

```go
func TestEd25519_SignVerify(t *testing.T) {
    // 1. 生成密钥
    priv, pub, _ := GenerateEd25519Key(rand.Reader)
    
    // 2. 签名
    data := []byte("test message")
    sig, err := priv.Sign(data)
    assert.NoError(t, err)
    
    // 3. 验证
    valid, err := pub.Verify(data, sig)
    assert.NoError(t, err)
    assert.True(t, valid)
}
```

### 边界条件测试

```go
func TestEd25519_Verify_BadSignature(t *testing.T) {
    _, pub, _ := GenerateEd25519Key(rand.Reader)
    data := []byte("test message")
    
    tests := []struct {
        name string
        sig  []byte
        want bool
    }{
        {"empty signature", []byte{}, false},
        {"short signature", []byte{1, 2, 3}, false},
        {"wrong length", make([]byte, 63), false},
        {"all zeros", make([]byte, 64), false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            valid, _ := pub.Verify(data, tt.sig)
            assert.Equal(t, tt.want, valid)
        })
    }
}
```

### 错误路径测试

```go
func TestUnmarshalPublicKey_InvalidType(t *testing.T) {
    _, err := UnmarshalPublicKey(KeyType(99), []byte{1, 2, 3})
    assert.Error(t, err)
    assert.ErrorIs(t, err, ErrBadKeyType)
}
```

### 跨类型测试

```go
func TestCrossKeyType_Verify(t *testing.T) {
    // 用 Ed25519 签名，用 Secp256k1 验证应失败
    priv1, _, _ := GenerateKeyPair(KeyTypeEd25519)
    _, pub2, _ := GenerateKeyPair(KeyTypeSecp256k1)
    
    data := []byte("test data")
    sig, _ := priv1.Sign(data)
    
    valid, _ := pub2.Verify(data, sig)
    assert.False(t, valid)
}
```

---

## 确定性测试

### 固定随机源

```go
func TestDeterministicGeneration(t *testing.T) {
    seed := make([]byte, 64)
    for i := range seed {
        seed[i] = byte(i)
    }
    
    reader1 := bytes.NewReader(seed)
    reader2 := bytes.NewReader(seed)
    
    priv1, _, _ := GenerateKeyPairWithReader(KeyTypeEd25519, reader1)
    priv2, _, _ := GenerateKeyPairWithReader(KeyTypeEd25519, reader2)
    
    assert.True(t, priv1.Equals(priv2), "相同种子应生成相同密钥")
}
```

---

## 测试数据管理

### 测试向量

```go
// 使用已知测试向量验证算法实现
func TestEd25519_KnownVector(t *testing.T) {
    // RFC 8032 测试向量
    seed := hexDecode("9d61b19deffd5a60ba844af492ec2cc4...")
    message := hexDecode("...")
    expectedSig := hexDecode("...")
    
    priv := ed25519.NewKeyFromSeed(seed)
    sig := ed25519.Sign(priv, message)
    
    assert.Equal(t, expectedSig, sig)
}
```

### 模糊测试

```go
func FuzzUnmarshalPublicKey(f *testing.F) {
    // 添加种子语料库
    f.Add(byte(KeyTypeEd25519), []byte{})
    f.Add(byte(KeyTypeSecp256k1), make([]byte, 33))
    
    f.Fuzz(func(t *testing.T, kt byte, data []byte) {
        // 不应 panic
        _, _ = UnmarshalPublicKey(KeyType(kt), data)
    })
}
```

---

## 安全测试

### 常量时间验证

```go
func TestKeyEqual_ConstantTime(t *testing.T) {
    priv1, _, _ := GenerateKeyPair(KeyTypeEd25519)
    priv2, _, _ := GenerateKeyPair(KeyTypeEd25519)
    
    // 测试多次，时间应稳定
    timings := make([]time.Duration, 1000)
    for i := 0; i < 1000; i++ {
        start := time.Now()
        KeyEqual(priv1, priv2)
        timings[i] = time.Since(start)
    }
    
    // 标准差应较小
    stdDev := calculateStdDev(timings)
    assert.Less(t, stdDev, time.Microsecond*10)
}
```

### 密钥清零验证

```go
func TestSecureZero(t *testing.T) {
    data := []byte{1, 2, 3, 4, 5}
    SecureZero(data)
    
    for i, b := range data {
        if b != 0 {
            t.Errorf("data[%d] = %d, want 0", i, b)
        }
    }
}
```

---

## Mock 和 Stub

### Keystore Mock

```go
type MockKeystore struct {
    keys map[string]PrivateKey
}

func (m *MockKeystore) Get(id string) (PrivateKey, error) {
    key, ok := m.keys[id]
    if !ok {
        return nil, ErrKeyNotFound
    }
    return key, nil
}

func TestWithMockKeystore(t *testing.T) {
    mock := &MockKeystore{
        keys: make(map[string]PrivateKey),
    }
    
    priv, _, _ := GenerateKeyPair(KeyTypeEd25519)
    mock.keys["test"] = priv
    
    got, err := mock.Get("test")
    assert.NoError(t, err)
    assert.True(t, priv.Equals(got))
}
```

---

## 覆盖率报告

### 生成覆盖率

```bash
# 运行测试并生成覆盖率
go test -cover -coverprofile=coverage.out ./pkg/crypto

# 查看覆盖率详情
go tool cover -func=coverage.out

# 生成 HTML 报告
go tool cover -html=coverage.out -o coverage.html
```

### 当前覆盖率

```
coverage: 83.0% of statements

详细分组：
  key.go          89.2%
  ed25519.go      95.1%
  secp256k1.go    87.3%
  ecdsa.go        84.2%
  rsa.go          76.5%
  marshal.go      91.0%
  peerid.go       92.3%
  keystore.go     79.8%
  signature.go    88.5%
```

---

## 测试执行策略

### CI/CD 流程

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Go
        uses: actions/setup-go@v3
        with:
          go-version: '1.24'
      
      - name: Run tests
        run: go test -v -race -cover ./pkg/crypto/...
      
      - name: Run benchmarks
        run: go test -bench=. -benchmem ./pkg/crypto/...
      
      - name: Check coverage
        run: |
          go test -coverprofile=coverage.out ./pkg/crypto/...
          go tool cover -func=coverage.out | grep total
```

### 本地测试

```bash
# 快速测试
make test-crypto

# 详细测试
go test -v ./pkg/crypto/...

# 带竞态检测
go test -race ./pkg/crypto/...

# 基准测试
go test -bench=. -benchmem ./pkg/crypto/...

# 覆盖率测试
go test -cover ./pkg/crypto/...
```

---

## 测试维护

### 定期审查

- [ ] 每月审查测试覆盖率
- [ ] 新增功能必须有测试
- [ ] 修复 bug 必须添加回归测试
- [ ] 基准测试结果趋势分析

### 测试质量指标

| 指标 | 目标 | 当前 |
|------|------|------|
| 覆盖率 | > 80% | 83.0% |
| 测试数量 | > 100 | 125 |
| 平均测试时间 | < 5s | 3.2s |
| Flaky 测试数 | 0 | 0 |

---

## 相关文档

- [../coding/guidelines.md](../coding/guidelines.md) - 编码指南
- [../design/overview.md](../design/overview.md) - 设计概述

---

**最后更新**：2026-01-13
