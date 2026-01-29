# core_identity 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/identity/
├── module.go           # Fx 模块定义
├── identity.go         # 主 Identity 实现
├── key.go              # 密钥生成与适配器
├── peerid.go           # PeerID 派生（包装 pkg/lib/crypto）
├── signing.go          # 签名与验证
├── errors.go           # 错误定义
├── device.go           # 设备相关
└── *_test.go           # 单元测试
```

---

## 关键算法

### Ed25519 密钥生成

```go
// internal/core/identity/key.go
// 包装 pkg/lib/crypto.GenerateEd25519Key

func GenerateEd25519Key() (pkgif.PrivateKey, pkgif.PublicKey, error) {
    priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
    if err != nil {
        return nil, nil, fmt.Errorf("generate key: %w", err)
    }
    // 适配器转换：crypto.PrivateKey → pkgif.PrivateKey
    return adaptPrivateKey(priv), adaptPublicKey(pub), nil
}
```

### PeerID 派生

> **NodeID = PeerID（类型别名）**，见 `pkg/types/ids.go`

```go
// internal/core/identity/peerid.go
// 包装 pkg/lib/crypto.PeerIDFromPublicKey

func PeerIDFromPublicKey(pubKey pkgif.PublicKey) (string, error) {
    // 1. 获取公钥原始字节
    raw, err := pubKey.Raw()
    if err != nil {
        return "", err
    }
    // 2. 计算 SHA-256 哈希
    hash := sha256.Sum256(raw)
    // 3. Base58 编码返回字符串
    return Base58Encode(hash[:]), nil
}
```

### 签名与验证

```go
// internal/core/identity/signing.go

func Sign(privKey pkgif.PrivateKey, data []byte) ([]byte, error) {
    return privKey.Sign(data)
}

func Verify(pubKey pkgif.PublicKey, data, sig []byte) (bool, error) {
    return pubKey.Verify(data, sig)
}

// Identity 方法实现
func (i *Identity) Sign(data []byte) ([]byte, error) {
    return Sign(i.privKey, data)
}

func (i *Identity) Verify(data, sig []byte) (bool, error) {
    return Verify(i.pubKey, data, sig)
}
```

---

## 数据结构

### PeerID / NodeID

> **NodeID 是 PeerID 的类型别名**，统一用于路由和身份标识。

```go
// pkg/types/ids.go

type PeerID string        // Base58(SHA256(pubKey))
type NodeID = PeerID      // 类型别名

func (id PeerID) String() string {
    return string(id)
}

func (id PeerID) ShortString() string {
    s := string(id)
    if len(s) <= 14 {
        return s
    }
    return s[:8] + "..." + s[len(s)-3:]  // 前8+后3
}

func (id PeerID) Equal(other PeerID) bool {
    return id == other
}

// DHT 路由相关
func (id PeerID) Hash() [32]byte          // SHA256(id) 用于 XOR 距离
func (id PeerID) XOR(other PeerID) [32]byte
func (id PeerID) CommonPrefixLen(other PeerID) int
```

### Identity 结构

```go
// internal/core/identity/identity.go

type Identity struct {
    peerID  string              // Base58 编码的 PeerID
    privKey pkgif.PrivateKey    // 私钥接口
    pubKey  pkgif.PublicKey     // 公钥接口
}

// 确保实现接口
var _ pkgif.Identity = (*Identity)(nil)
```

---

## 状态管理

Identity 模块无复杂状态，主要是不可变数据：

| 数据 | 可变性 | 说明 |
|------|--------|------|
| privKey | 不可变 | 创建后固定（pkgif.PrivateKey） |
| pubKey | 不可变 | 从私钥派生（pkgif.PublicKey） |
| peerID | 不可变 | 从公钥派生（Base58 字符串） |

---

## 错误处理

```go
// internal/core/identity/errors.go

var (
    ErrNilPrivateKey    = errors.New("private key is nil")
    ErrNilPublicKey     = errors.New("public key is nil")
    ErrKeyPairMismatch  = errors.New("key pair mismatch")
    ErrInvalidKeyLength = errors.New("invalid key length")
    ErrSignatureFailed  = errors.New("signature verification failed")
    ErrUnsupportedKey   = errors.New("unsupported key type")
)
```

---

## 安全考虑

### 私钥保护

```go
// 私钥通过接口访问，不直接暴露内部实现
// pkgif.PrivateKey 接口不包含 String() 方法，避免意外打印到日志

// Identity 结构中私钥字段不参与序列化
type Identity struct {
    peerID  string
    privKey pkgif.PrivateKey  // 不可序列化
    pubKey  pkgif.PublicKey
}
```

### 密钥持久化

```go
// internal/core/identity/key.go

// PEM 格式存储私钥
func MarshalPrivateKeyPEM(privKey pkgif.PrivateKey) ([]byte, error)
func UnmarshalPrivateKeyPEM(data []byte) (pkgif.PrivateKey, error)

// 从文件加载身份
func loadIdentityFromFile(path string) (*Identity, error)
```

---

**最后更新**：2026-01-25
