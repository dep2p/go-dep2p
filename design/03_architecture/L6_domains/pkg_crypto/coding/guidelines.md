# pkg_crypto 编码指南

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 代码组织

### 文件命名规范

| 文件类型 | 命名规则 | 示例 |
|---------|---------|------|
| 密钥实现 | `<算法名>.go` | `ed25519.go` |
| 测试文件 | `<算法名>_test.go` | `ed25519_test.go` |
| 接口定义 | `key.go` | `key.go` |
| 工具函数 | `<功能>.go` | `marshal.go` |

### 导入顺序

```go
import (
    // 1. 标准库
    "crypto/ed25519"
    "crypto/rand"
    "io"
    
    // 2. 外部库
    "golang.org/x/crypto/argon2"
    
    // 3. 内部包
    "github.com/dep2p/go-dep2p/pkg/types"
)
```

---

## 接口实现规范

### 密钥结构定义

```go
// ✅ 好：使用指针字段，避免拷贝
type Ed25519PrivateKey struct {
    k ed25519.PrivateKey  // 切片类型，本身是引用
}

// ✅ 好：小对象使用值
type KeyType int

// ❌ 坏：大对象使用值
type Ed25519PrivateKey struct {
    k [64]byte  // 大数组，拷贝开销大
}
```

### Raw() 实现

```go
// ✅ 好：返回拷贝，防止外部修改
func (k *Ed25519PublicKey) Raw() ([]byte, error) {
    buf := make([]byte, len(k.k))
    copy(buf, k.k)
    return buf, nil
}

// ❌ 坏：返回内部引用
func (k *Ed25519PublicKey) Raw() ([]byte, error) {
    return k.k, nil  // 外部可以修改
}
```

### Equals() 实现

```go
// ✅ 好：使用常量时间比较
func (k *Ed25519PublicKey) Equals(other Key) bool {
    ek, ok := other.(*Ed25519PublicKey)
    if !ok {
        return KeyEqual(k, other)
    }
    return subtle.ConstantTimeCompare(k.k, ek.k) == 1
}

// ❌ 坏：可能泄露时序信息
func (k *Ed25519PublicKey) Equals(other Key) bool {
    ek, ok := other.(*Ed25519PublicKey)
    if !ok {
        return false
    }
    return bytes.Equal(k.k, ek.k)  // 非常量时间
}
```

---

## 安全编码规范

### 1. 敏感数据处理

```go
// ✅ 好：使用后清零
func processPrivateKey(key []byte) error {
    defer SecureZero(key)
    // ... 使用 key
    return nil
}

// ❌ 坏：敏感数据残留
func processPrivateKey(key []byte) error {
    // ... 使用 key
    return nil  // key 仍在内存中
}
```

### 2. 随机数生成

```go
// ✅ 好：使用加密安全随机源
func GenerateKeyPair(kt KeyType) (PrivateKey, PublicKey, error) {
    return GenerateKeyPairWithReader(kt, rand.Reader)
}

// ❌ 坏：使用非加密随机源
func GenerateKeyPair(kt KeyType) (PrivateKey, PublicKey, error) {
    r := mathRand.New(mathRand.NewSource(time.Now().Unix()))
    return generateKey(r)  // 不安全！
}
```

### 3. 错误处理

```go
// ✅ 好：不泄露敏感信息
func (ks *FSKeystore) Get(id string) (PrivateKey, error) {
    data, err := os.ReadFile(ks.keyPath(id))
    if os.IsNotExist(err) {
        return nil, ErrKeyNotFound  // 不暴露路径
    }
    // ...
}

// ❌ 坏：泄露路径信息
func (ks *FSKeystore) Get(id string) (PrivateKey, error) {
    path := ks.keyPath(id)
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read %s: %w", path, err)
    }
    // ...
}
```

---

## 性能优化规范

### 1. 避免不必要的分配

```go
// ✅ 好：直接使用
func Sign(key PrivateKey, data []byte) ([]byte, error) {
    return key.Sign(data)
}

// ❌ 坏：不必要的拷贝
func Sign(key PrivateKey, data []byte) ([]byte, error) {
    dataCopy := make([]byte, len(data))
    copy(dataCopy, data)
    return key.Sign(dataCopy)
}
```

### 2. 预分配切片

```go
// ✅ 好：预分配大小
func MarshalPublicKey(key PublicKey) ([]byte, error) {
    raw, _ := key.Raw()
    buf := make([]byte, 5+len(raw))  // 预分配
    // ... 填充 buf
    return buf, nil
}

// ❌ 坏：频繁扩容
func MarshalPublicKey(key PublicKey) ([]byte, error) {
    var buf []byte
    buf = append(buf, byte(key.Type()))
    // ... 多次 append 导致扩容
}
```

### 3. 复用对象

```go
// ✅ 好：使用 sync.Pool
var bigIntPool = sync.Pool{
    New: func() interface{} {
        return new(big.Int)
    },
}

func calculate() *big.Int {
    x := bigIntPool.Get().(*big.Int)
    defer bigIntPool.Put(x)
    // ... 使用 x
}
```

---

## 注释规范

### GoDoc 注释

```go
// ✅ 好：完整的 GoDoc
// GenerateKeyPair 生成指定类型的密钥对
//
// 使用系统默认的加密安全随机源。
//
// 参数：
//   - keyType: 密钥类型
//
// 返回：
//   - PrivateKey: 私钥
//   - PublicKey: 公钥
//   - error: 生成错误
//
// 支持的密钥类型：
//   - KeyTypeEd25519: Ed25519 密钥（推荐）
//   - KeyTypeSecp256k1: Secp256k1 密钥
//   - KeyTypeECDSA: ECDSA 密钥
//   - KeyTypeRSA: RSA 密钥
func GenerateKeyPair(keyType KeyType) (PrivateKey, PublicKey, error) {
    // ...
}

// ❌ 坏：注释不完整
// 生成密钥对
func GenerateKeyPair(keyType KeyType) (PrivateKey, PublicKey, error) {
    // ...
}
```

### 内部注释

```go
// ✅ 好：解释"为什么"
// 使用常量时间比较防止时序攻击
return subtle.ConstantTimeCompare(k.k, ek.k) == 1

// ❌ 坏：重复代码
// 比较两个字节切片
return subtle.ConstantTimeCompare(k.k, ek.k) == 1
```

---

## 测试编码规范

### 测试命名

```go
// ✅ 好：清晰的测试名称
func TestEd25519_Generate(t *testing.T) { ... }
func TestEd25519_SignVerify(t *testing.T) { ... }
func TestEd25519_UnmarshalPrivateKey_InvalidSize(t *testing.T) { ... }

// ❌ 坏：模糊的名称
func TestKey(t *testing.T) { ... }
func Test1(t *testing.T) { ... }
```

### 测试结构

```go
// ✅ 好：使用 t.Run 分组
func TestEd25519_UnmarshalPrivateKey(t *testing.T) {
    t.Run("64 bytes", func(t *testing.T) {
        // ...
    })
    
    t.Run("32 bytes seed", func(t *testing.T) {
        // ...
    })
    
    t.Run("96 bytes with redundant pubkey", func(t *testing.T) {
        // ...
    })
}
```

### 错误消息

```go
// ✅ 好：提供上下文
if got != want {
    t.Errorf("GenerateKeyPair() type = %v, want %v", got, want)
}

// ❌ 坏：没有上下文
if got != want {
    t.Error("wrong type")
}
```

---

## 错误处理规范

### 预定义错误

```go
// ✅ 好：使用预定义错误
var (
    ErrBadKeyType = errors.New("invalid or unsupported key type")
    ErrInvalidKeySize = errors.New("invalid key size")
)

func UnmarshalPublicKey(kt KeyType, data []byte) (PublicKey, error) {
    if kt == KeyTypeUnspecified {
        return nil, ErrBadKeyType
    }
    // ...
}
```

### 错误包装

```go
// ✅ 好：提供上下文，不泄露敏感信息
func UnmarshalEd25519PublicKey(data []byte) (PublicKey, error) {
    if len(data) != Ed25519PublicKeySize {
        return nil, fmt.Errorf("%w: expected %d bytes, got %d",
            ErrInvalidKeySize, Ed25519PublicKeySize, len(data))
    }
    // ...
}

// ❌ 坏：泄露过多信息
func loadKey(path string, password []byte) (PrivateKey, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read key from %s with password %s: %w",
            path, string(password), err)  // 泄露密码！
    }
    // ...
}
```

---

## 代码审查清单

### 安全性

- [ ] 使用常量时间比较
- [ ] 敏感数据使用后清零
- [ ] 使用加密安全随机源
- [ ] 错误不泄露敏感信息
- [ ] 文件权限正确（0600）

### 性能

- [ ] 避免不必要的内存分配
- [ ] 预分配切片大小
- [ ] 避免不必要的拷贝
- [ ] 考虑使用对象池

### 可维护性

- [ ] GoDoc 注释完整
- [ ] 代码结构清晰
- [ ] 错误处理完善
- [ ] 测试覆盖充分
- [ ] 变量命名清晰

---

## 相关文档

- [../design/overview.md](../design/overview.md) - 设计概述
- [../design/internals.md](../design/internals.md) - 内部实现
- [../testing/strategy.md](../testing/strategy.md) - 测试策略

---

**最后更新**：2026-01-13
