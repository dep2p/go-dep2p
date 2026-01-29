# pkg_crypto 内部实现细节

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## Secp256k1 纯 Go 实现

### 为什么选择纯 Go 实现

| 方案 | 优点 | 缺点 | 选择 |
|------|------|------|------|
| cgo + libsecp256k1 | 性能最优 | 编译复杂、不可移植 | ❌ |
| btcd/btcec | 成熟稳定 | 外部依赖 | ❌ |
| **纯 Go 实现** | **无依赖、可移植** | **性能稍慢** | ✅ |

### 椭圆曲线参数

```go
// secp256k1 曲线参数
// y² = x³ + 7 (mod P)

P  = FFFFFFFF FFFFFFFF FFFFFFFF FFFFFFFF 
     FFFFFFFF FFFFFFFF FFFFFFFE FFFFFC2F

N  = FFFFFFFF FFFFFFFF FFFFFFFF FFFFFFFE 
     BAAEDCE6 AF48A03B BFD25E8C D0364141

Gx = 79BE667E F9DCBBAC 55A06295 CE870B07 
     029BFCDB 2DCE28D9 59F2815B 16F81798

Gy = 483ADA77 26A3C465 5DA4FBFC 0E1108A8 
     FD17B448 A6855419 9C47D08F FB10D4B8

B  = 7
```

### 点运算实现

#### 点加法（Point Addition）

```
给定两个点 P1=(x1, y1) 和 P2=(x2, y2)
计算 P3 = P1 + P2 = (x3, y3)

λ = (y2 - y1) / (x2 - x1) mod P
x3 = λ² - x1 - x2 mod P
y3 = λ(x1 - x3) - y1 mod P
```

```go
func secp256k1AddPoints(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
    // 特殊情况处理
    if x1 == nil || x2 == nil {
        // ...
    }
    
    // 相同点使用点倍
    if x1.Cmp(x2) == 0 && y1.Cmp(y2) == 0 {
        return secp256k1DoublePoint(x1, y1)
    }
    
    // 互为逆元
    if x1.Cmp(x2) == 0 {
        return nil, nil
    }
    
    // 计算 λ
    dy := new(big.Int).Sub(y2, y1)
    dx := new(big.Int).Sub(x2, x1)
    dxInv := new(big.Int).ModInverse(dx, secp256k1P)
    lambda := new(big.Int).Mul(dy, dxInv)
    lambda.Mod(lambda, secp256k1P)
    
    // 计算 x3, y3
    x3 := new(big.Int).Mul(lambda, lambda)
    x3.Sub(x3, x1)
    x3.Sub(x3, x2)
    x3.Mod(x3, secp256k1P)
    
    y3 := new(big.Int).Sub(x1, x3)
    y3.Mul(y3, lambda)
    y3.Sub(y3, y1)
    y3.Mod(y3, secp256k1P)
    
    return x3, y3
}
```

#### 点倍运算（Point Doubling）

```
给定点 P=(x, y)
计算 2P = (x3, y3)

λ = 3x² / 2y mod P
x3 = λ² - 2x mod P
y3 = λ(x - x3) - y mod P
```

```go
func secp256k1DoublePoint(x, y *big.Int) (*big.Int, *big.Int) {
    if y.Sign() == 0 {
        return nil, nil
    }
    
    // λ = 3x² / 2y
    x2 := new(big.Int).Mul(x, x)
    x2.Mul(x2, big.NewInt(3))
    y2 := new(big.Int).Mul(y, big.NewInt(2))
    y2Inv := new(big.Int).ModInverse(y2, secp256k1P)
    lambda := new(big.Int).Mul(x2, y2Inv)
    lambda.Mod(lambda, secp256k1P)
    
    // x3 = λ² - 2x
    x3 := new(big.Int).Mul(lambda, lambda)
    x3.Sub(x3, x)
    x3.Sub(x3, x)
    x3.Mod(x3, secp256k1P)
    
    // y3 = λ(x - x3) - y
    y3 := new(big.Int).Sub(x, x3)
    y3.Mul(y3, lambda)
    y3.Sub(y3, y)
    y3.Mod(y3, secp256k1P)
    
    return x3, y3
}
```

#### 标量乘法（Scalar Multiplication）

使用**双加算法**（Double-and-Add）：

```
计算 k * P

算法：
  R = 无穷远点
  对 k 的每一位（从高到低）：
    R = 2 * R       (点倍)
    如果当前位为 1：
      R = R + P     (点加)
  返回 R
```

```go
func secp256k1ScalarMult(px, py *big.Int, k []byte) (*big.Int, *big.Int) {
    kInt := new(big.Int).SetBytes(k)
    if kInt.Sign() == 0 {
        return nil, nil
    }
    
    var rx, ry *big.Int
    tx, ty := new(big.Int).Set(px), new(big.Int).Set(py)
    
    // 双加算法
    for i := kInt.BitLen() - 1; i >= 0; i-- {
        if rx != nil {
            rx, ry = secp256k1DoublePoint(rx, ry)
        }
        if kInt.Bit(i) == 1 {
            if rx == nil {
                rx, ry = new(big.Int).Set(tx), new(big.Int).Set(ty)
            } else {
                rx, ry = secp256k1AddPoints(rx, ry, tx, ty)
            }
        }
    }
    
    return rx, ry
}
```

### 公钥压缩/解压

#### 压缩格式

```
压缩公钥：33 字节
┌──────────┬─────────────────────────────────┐
│ Prefix   │           X 坐标                │
│ (1 byte) │         (32 bytes)              │
└──────────┴─────────────────────────────────┘

Prefix:
  0x02 - Y 为偶数
  0x03 - Y 为奇数
```

#### 解压算法

```
给定 X，计算 Y：

1. 计算 y² = x³ + 7 (mod P)
2. 计算 y = sqrt(y²) mod P
   因为 P ≡ 3 (mod 4)，使用：
   y = y²^((P+1)/4) mod P
3. 根据前缀选择正确的 Y
```

```go
func secp256k1DecompressPublicKey(data []byte) (*big.Int, *big.Int) {
    prefix := data[0]
    x := new(big.Int).SetBytes(data[1:])
    
    // y² = x³ + 7
    x3 := new(big.Int).Mul(x, x)
    x3.Mul(x3, x)
    x3.Mod(x3, secp256k1P)
    
    y2 := new(big.Int).Add(x3, secp256k1B)
    y2.Mod(y2, secp256k1P)
    
    // y = y²^((P+1)/4) mod P
    exp := new(big.Int).Add(secp256k1P, big.NewInt(1))
    exp.Div(exp, big.NewInt(4))
    y := new(big.Int).Exp(y2, exp, secp256k1P)
    
    // 验证
    check := new(big.Int).Mul(y, y)
    check.Mod(check, secp256k1P)
    if check.Cmp(y2) != 0 {
        return nil, nil
    }
    
    // 根据前缀选择 Y
    if (prefix == 0x02) != (y.Bit(0) == 0) {
        y.Sub(secp256k1P, y)
    }
    
    return x, y
}
```

---

## Ed25519 实现细节

### 密钥格式

```
私钥（64 字节）：
┌─────────────────┬─────────────────┐
│  Seed (32字节)   │  公钥 (32字节)   │
└─────────────────┴─────────────────┘

种子（32 字节）：
├─ 仅种子格式，需派生完整私钥
```

### 多格式支持

```go
func UnmarshalEd25519PrivateKey(data []byte) (PrivateKey, error) {
    switch len(data) {
    case 96:  // 64字节私钥 + 32字节冗余公钥（libp2p格式）
        // 验证冗余公钥
        redundantPk := data[64:]
        pk := data[32:64]
        if subtle.ConstantTimeCompare(pk, redundantPk) == 0 {
            return nil, ErrInvalidPrivateKey
        }
        return &Ed25519PrivateKey{k: data[:64]}, nil
        
    case 64:  // 完整私钥
        return &Ed25519PrivateKey{k: data}, nil
        
    case 32:  // 仅种子
        return &Ed25519PrivateKey{
            k: ed25519.NewKeyFromSeed(data),
        }, nil
        
    default:
        return nil, ErrInvalidKeySize
    }
}
```

---

## 密钥存储实现

### FSKeystore 实现

```go
type FSKeystore struct {
    dir      string
    password []byte  // 可选加密密码
}

// 密钥文件路径
func (ks *FSKeystore) keyPath(id string) string {
    return filepath.Join(ks.dir, id+".key")
}

// 编码密钥
func (ks *FSKeystore) encodeKey(key PrivateKey) ([]byte, error) {
    var buf bytes.Buffer
    
    // 写入魔数
    buf.WriteString("DEP2P-KEY")
    
    // 写入版本
    buf.WriteByte(1)
    
    // 写入密钥类型
    buf.WriteByte(byte(key.Type()))
    
    if len(ks.password) > 0 {
        // 加密存储
        buf.WriteByte(1)
        encrypted, _ := encryptData(raw, ks.password)
        buf.Write(encrypted)
    } else {
        // 明文存储
        buf.WriteByte(0)
        buf.Write(raw)
    }
    
    return buf.Bytes(), nil
}
```

### 加密实现

```go
func encryptData(plaintext, password []byte) ([]byte, error) {
    // 1. 生成随机盐
    salt := make([]byte, 16)
    rand.Read(salt)
    
    // 2. 派生密钥（Argon2id）
    key := argon2.IDKey(
        password, salt,
        1,          // 时间成本
        64*1024,    // 内存成本 (64 MB)
        4,          // 并行度
        32,         // 密钥长度
    )
    
    // 3. 创建 AES-GCM
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    
    // 4. 生成随机 nonce
    nonce := make([]byte, 12)
    rand.Read(nonce)
    
    // 5. 加密
    ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
    
    // 6. 组装结果：salt || nonce || ciphertext
    result := make([]byte, 16+12+len(ciphertext))
    copy(result[0:16], salt)
    copy(result[16:28], nonce)
    copy(result[28:], ciphertext)
    
    return result, nil
}
```

---

## 序列化实现

### 公钥序列化

```go
func MarshalPublicKey(key PublicKey) ([]byte, error) {
    raw, _ := key.Raw()
    
    // 分配缓冲区：1字节类型 + 4字节长度 + 数据
    buf := make([]byte, 5+len(raw))
    
    // 写入类型
    buf[0] = byte(key.Type())
    
    // 写入长度（大端序）
    binary.BigEndian.PutUint32(buf[1:5], uint32(len(raw)))
    
    // 写入数据
    copy(buf[5:], raw)
    
    return buf, nil
}
```

### 反序列化

```go
func UnmarshalPublicKeyBytes(data []byte) (PublicKey, error) {
    if len(data) < 5 {
        return nil, ErrUnmarshalFailed
    }
    
    // 读取类型
    keyType := KeyType(data[0])
    
    // 读取长度
    length := binary.BigEndian.Uint32(data[1:5])
    
    // 验证数据长度
    if len(data) < 5+int(length) {
        return nil, ErrUnmarshalFailed
    }
    
    // 读取密钥数据
    keyData := data[5 : 5+length]
    
    return UnmarshalPublicKey(keyType, keyData)
}
```

---

## PeerID 派生实现

```go
func PeerIDFromPublicKey(pub PublicKey) (types.PeerID, error) {
    // 1. 序列化公钥
    data, err := MarshalPublicKey(pub)
    if err != nil {
        return types.EmptyPeerID, err
    }
    
    // 2. SHA256 哈希
    hash := sha256.Sum256(data)
    
    // 3. Base58 编码
    encoded := types.Base58Encode(hash[:])
    
    return types.PeerID(encoded), nil
}
```

---

## 性能优化技巧

### 1. 内存池复用

```go
// 避免频繁分配大 buffer
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 4096)
    },
}
```

### 2. 预计算优化

```go
// 预计算曲线参数
var (
    secp256k1P, _  = new(big.Int).SetString("FFF...F2F", 16)
    secp256k1N, _  = new(big.Int).SetString("FFF...141", 16)
    secp256k1Gx, _ = new(big.Int).SetString("79B...798", 16)
    secp256k1Gy, _ = new(big.Int).SetString("483...4B8", 16)
)
```

### 3. 避免不必要的拷贝

```go
// 好：直接使用
func (k *Ed25519PublicKey) Verify(data, sig []byte) (bool, error) {
    return ed25519.Verify(k.k, data, sig), nil
}

// 坏：不必要的拷贝
func (k *Ed25519PublicKey) Verify(data, sig []byte) (bool, error) {
    dataCopy := make([]byte, len(data))
    copy(dataCopy, data)
    return ed25519.Verify(k.k, dataCopy, sig), nil
}
```

---

## 相关文档

- [overview.md](overview.md) - 设计概述
- [../coding/guidelines.md](../coding/guidelines.md) - 编码指南

---

**最后更新**：2026-01-13
