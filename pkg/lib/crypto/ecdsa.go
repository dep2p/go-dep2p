package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"fmt"
	"io"
	"math/big"
)

// ECDSA 密钥常量（使用 P-256 曲线）
const (
	// ECDSAPrivateKeySize ECDSA 私钥大小（32 字节）
	ECDSAPrivateKeySize = 32
	// ECDSAPublicKeySize ECDSA 压缩公钥大小（33 字节）
	ECDSAPublicKeySize = 33
	// ECDSAUncompressedPublicKeySize ECDSA 未压缩公钥大小（65 字节）
	ECDSAUncompressedPublicKeySize = 65
)

// ============================================================================
//                              ECDSAPublicKey
// ============================================================================

// ECDSAPublicKey ECDSA 公钥实现（P-256 曲线）
type ECDSAPublicKey struct {
	k *ecdsa.PublicKey
}

// Raw 返回压缩格式的公钥字节（33 字节）
func (k *ECDSAPublicKey) Raw() ([]byte, error) {
	return compressECDSAPublicKey(k.k), nil
}

// Type 返回密钥类型
func (k *ECDSAPublicKey) Type() KeyType {
	return KeyTypeECDSA
}

// Equals 比较两个公钥是否相等
func (k *ECDSAPublicKey) Equals(other Key) bool {
	ek, ok := other.(*ECDSAPublicKey)
	if !ok {
		return KeyEqual(k, other)
	}
	return k.k.X.Cmp(ek.k.X) == 0 && k.k.Y.Cmp(ek.k.Y) == 0
}

// Verify 使用此公钥验证签名
//
// 签名格式为 64 字节：R (32 字节) + S (32 字节)
func (k *ECDSAPublicKey) Verify(data, sig []byte) (bool, error) {
	if len(sig) != 64 {
		return false, nil
	}

	// 计算数据哈希
	hash := sha256.Sum256(data)

	// 解析签名
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	return ecdsa.Verify(k.k, hash[:], r, s), nil
}

// ============================================================================
//                              ECDSAPrivateKey
// ============================================================================

// ECDSAPrivateKey ECDSA 私钥实现（P-256 曲线）
type ECDSAPrivateKey struct {
	k *ecdsa.PrivateKey
}

// Raw 返回原始私钥字节（32 字节）
func (k *ECDSAPrivateKey) Raw() ([]byte, error) {
	return ecdsaPaddedBytes(k.k.D, ECDSAPrivateKeySize), nil
}

// Type 返回密钥类型
func (k *ECDSAPrivateKey) Type() KeyType {
	return KeyTypeECDSA
}

// Equals 比较两个私钥是否相等
func (k *ECDSAPrivateKey) Equals(other Key) bool {
	ek, ok := other.(*ECDSAPrivateKey)
	if !ok {
		return KeyEqual(k, other)
	}

	b1 := ecdsaPaddedBytes(k.k.D, ECDSAPrivateKeySize)
	b2 := ecdsaPaddedBytes(ek.k.D, ECDSAPrivateKeySize)
	return subtle.ConstantTimeCompare(b1, b2) == 1
}

// GetPublic 返回对应的公钥
func (k *ECDSAPrivateKey) GetPublic() PublicKey {
	return &ECDSAPublicKey{k: &k.k.PublicKey}
}

// PublicKey 返回对应的公钥（兼容 pkg/interfaces.PrivateKey 接口）
func (k *ECDSAPrivateKey) PublicKey() PublicKey {
	return k.GetPublic()
}

// Sign 使用此私钥签名数据
//
// 返回 64 字节签名：R (32 字节) + S (32 字节)
func (k *ECDSAPrivateKey) Sign(data []byte) ([]byte, error) {
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, k.k, hash[:])
	if err != nil {
		return nil, err
	}

	// 返回固定长度的 R || S
	sig := make([]byte, 64)
	copy(sig[:32], ecdsaPaddedBytes(r, 32))
	copy(sig[32:], ecdsaPaddedBytes(s, 32))
	return sig, nil
}

// ============================================================================
//                              工厂函数
// ============================================================================

// GenerateECDSAKey 生成新的 ECDSA 密钥对（P-256 曲线）
func GenerateECDSAKey(src io.Reader) (PrivateKey, PublicKey, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), src)
	if err != nil {
		return nil, nil, err
	}
	return &ECDSAPrivateKey{k: priv}, &ECDSAPublicKey{k: &priv.PublicKey}, nil
}

// UnmarshalECDSAPublicKey 从字节反序列化 ECDSA 公钥
//
// 支持压缩格式（33 字节）和未压缩格式（65 字节）
func UnmarshalECDSAPublicKey(data []byte) (PublicKey, error) {
	switch len(data) {
	case ECDSAPublicKeySize:
		// 压缩格式
		x, y := decompressECDSAPublicKey(data)
		if x == nil || y == nil {
			return nil, ErrInvalidPublicKey
		}
		return &ECDSAPublicKey{k: &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}}, nil

	case ECDSAUncompressedPublicKeySize:
		// 未压缩格式
		if data[0] != 0x04 {
			return nil, ErrInvalidPublicKey
		}
		x := new(big.Int).SetBytes(data[1:33])
		y := new(big.Int).SetBytes(data[33:65])
		return &ECDSAPublicKey{k: &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}}, nil

	default:
		return nil, fmt.Errorf("%w: expected %d or %d bytes, got %d",
			ErrInvalidKeySize, ECDSAPublicKeySize, ECDSAUncompressedPublicKeySize, len(data))
	}
}

// UnmarshalECDSAPrivateKey 从字节反序列化 ECDSA 私钥
//
// 支持原始格式（32 字节）和 PKCS#8/SEC1 格式
func UnmarshalECDSAPrivateKey(data []byte) (PrivateKey, error) {
	// 尝试 PKCS#8 格式
	if key, err := x509.ParsePKCS8PrivateKey(data); err == nil {
		if ecdsaKey, ok := key.(*ecdsa.PrivateKey); ok {
			return &ECDSAPrivateKey{k: ecdsaKey}, nil
		}
	}

	// 尝试 SEC1 格式
	if key, err := x509.ParseECPrivateKey(data); err == nil {
		return &ECDSAPrivateKey{k: key}, nil
	}

	// 尝试原始格式（32 字节）
	if len(data) == ECDSAPrivateKeySize {
		d := new(big.Int).SetBytes(data)
		// 计算公钥
		x, y := elliptic.P256().ScalarBaseMult(data)
		priv := &ecdsa.PrivateKey{
			D: d,
			PublicKey: ecdsa.PublicKey{
				Curve: elliptic.P256(),
				X:     x,
				Y:     y,
			},
		}
		return &ECDSAPrivateKey{k: priv}, nil
	}

	return nil, ErrInvalidPrivateKey
}

// ============================================================================
//                              辅助函数
// ============================================================================

// ecdsaPaddedBytes 返回固定长度的字节切片
func ecdsaPaddedBytes(n *big.Int, length int) []byte {
	b := n.Bytes()
	if len(b) >= length {
		return b[len(b)-length:]
	}
	padded := make([]byte, length)
	copy(padded[length-len(b):], b)
	return padded
}

// compressECDSAPublicKey 压缩公钥
func compressECDSAPublicKey(pub *ecdsa.PublicKey) []byte {
	compressed := make([]byte, ECDSAPublicKeySize)
	// 前缀：0x02 表示 Y 为偶数，0x03 表示 Y 为奇数
	if pub.Y.Bit(0) == 0 {
		compressed[0] = 0x02
	} else {
		compressed[0] = 0x03
	}
	copy(compressed[1:], ecdsaPaddedBytes(pub.X, 32))
	return compressed
}

// decompressECDSAPublicKey 解压公钥（P-256 曲线）
func decompressECDSAPublicKey(data []byte) (*big.Int, *big.Int) {
	if len(data) != ECDSAPublicKeySize {
		return nil, nil
	}

	prefix := data[0]
	if prefix != 0x02 && prefix != 0x03 {
		return nil, nil
	}

	curve := elliptic.P256()
	x := new(big.Int).SetBytes(data[1:])

	// P-256 曲线方程：y² = x³ - 3x + b (mod P)
	p := curve.Params().P
	b := curve.Params().B

	// x³
	x3 := new(big.Int).Mul(x, x)
	x3.Mul(x3, x)
	x3.Mod(x3, p)

	// -3x
	threeX := new(big.Int).Mul(x, big.NewInt(3))
	threeX.Mod(threeX, p)

	// x³ - 3x + b
	y2 := new(big.Int).Sub(x3, threeX)
	y2.Add(y2, b)
	y2.Mod(y2, p)

	// 计算平方根
	// 对于 P ≡ 3 (mod 4)：y = y²^((P+1)/4) mod P
	exp := new(big.Int).Add(p, big.NewInt(1))
	exp.Div(exp, big.NewInt(4))
	y := new(big.Int).Exp(y2, exp, p)

	// 验证 y
	check := new(big.Int).Mul(y, y)
	check.Mod(check, p)
	if check.Cmp(y2) != 0 {
		return nil, nil
	}

	// 根据前缀选择正确的 Y
	if (prefix == 0x02) != (y.Bit(0) == 0) {
		y.Sub(p, y)
	}

	return x, y
}
