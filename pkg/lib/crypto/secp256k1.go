package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"math/big"
)

// Secp256k1 密钥常量
const (
	// Secp256k1PrivateKeySize Secp256k1 私钥大小（32 字节）
	Secp256k1PrivateKeySize = 32
	// Secp256k1PublicKeySize Secp256k1 压缩公钥大小（33 字节）
	Secp256k1PublicKeySize = 33
	// Secp256k1UncompressedPublicKeySize Secp256k1 未压缩公钥大小（65 字节）
	Secp256k1UncompressedPublicKeySize = 65
	// Secp256k1SignatureSize Secp256k1 签名大小（64 字节）
	Secp256k1SignatureSize = 64
)

// secp256k1 曲线参数
var (
	secp256k1P, _  = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	secp256k1N, _  = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	secp256k1B     = big.NewInt(7)
	secp256k1Gx, _ = new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	secp256k1Gy, _ = new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
)

// ============================================================================
//                              Secp256k1PublicKey
// ============================================================================

// Secp256k1PublicKey Secp256k1 公钥实现
type Secp256k1PublicKey struct {
	X, Y *big.Int
}

// Raw 返回压缩格式的公钥字节（33 字节）
func (k *Secp256k1PublicKey) Raw() ([]byte, error) {
	return secp256k1CompressPublicKey(k.X, k.Y), nil
}

// Type 返回密钥类型
func (k *Secp256k1PublicKey) Type() KeyType {
	return KeyTypeSecp256k1
}

// Equals 比较两个公钥是否相等
func (k *Secp256k1PublicKey) Equals(other Key) bool {
	sk, ok := other.(*Secp256k1PublicKey)
	if !ok {
		return KeyEqual(k, other)
	}
	return k.X.Cmp(sk.X) == 0 && k.Y.Cmp(sk.Y) == 0
}

// Verify 使用此公钥验证签名
//
// 签名格式为 64 字节：R (32 字节) + S (32 字节)
func (k *Secp256k1PublicKey) Verify(data, sig []byte) (bool, error) {
	if len(sig) != Secp256k1SignatureSize {
		return false, nil
	}

	// 计算数据哈希
	hash := sha256.Sum256(data)
	z := new(big.Int).SetBytes(hash[:])

	// 解析签名
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	// 验证 r, s 在有效范围内
	if r.Sign() <= 0 || r.Cmp(secp256k1N) >= 0 {
		return false, nil
	}
	if s.Sign() <= 0 || s.Cmp(secp256k1N) >= 0 {
		return false, nil
	}

	// ECDSA 验证
	// w = s^(-1) mod n
	w := new(big.Int).ModInverse(s, secp256k1N)
	if w == nil {
		return false, nil
	}

	// u1 = z * w mod n
	u1 := new(big.Int).Mul(z, w)
	u1.Mod(u1, secp256k1N)

	// u2 = r * w mod n
	u2 := new(big.Int).Mul(r, w)
	u2.Mod(u2, secp256k1N)

	// (x1, y1) = u1 * G + u2 * PublicKey
	x1, y1 := secp256k1ScalarBaseMult(u1.Bytes())
	x2, y2 := secp256k1ScalarMult(k.X, k.Y, u2.Bytes())
	x, _ := secp256k1AddPoints(x1, y1, x2, y2)

	if x == nil {
		return false, nil
	}

	// v = x mod n
	v := new(big.Int).Mod(x, secp256k1N)

	return v.Cmp(r) == 0, nil
}

// ============================================================================
//                              Secp256k1PrivateKey
// ============================================================================

// Secp256k1PrivateKey Secp256k1 私钥实现
type Secp256k1PrivateKey struct {
	D    *big.Int
	X, Y *big.Int // 公钥坐标
}

// Raw 返回原始私钥字节（32 字节）
func (k *Secp256k1PrivateKey) Raw() ([]byte, error) {
	return secp256k1PaddedBytes(k.D, Secp256k1PrivateKeySize), nil
}

// Type 返回密钥类型
func (k *Secp256k1PrivateKey) Type() KeyType {
	return KeyTypeSecp256k1
}

// Equals 比较两个私钥是否相等
func (k *Secp256k1PrivateKey) Equals(other Key) bool {
	sk, ok := other.(*Secp256k1PrivateKey)
	if !ok {
		return KeyEqual(k, other)
	}

	b1 := secp256k1PaddedBytes(k.D, Secp256k1PrivateKeySize)
	b2 := secp256k1PaddedBytes(sk.D, Secp256k1PrivateKeySize)
	return subtle.ConstantTimeCompare(b1, b2) == 1
}

// GetPublic 返回对应的公钥
func (k *Secp256k1PrivateKey) GetPublic() PublicKey {
	return &Secp256k1PublicKey{X: k.X, Y: k.Y}
}

// PublicKey 返回对应的公钥（兼容 pkg/interfaces.PrivateKey 接口）
func (k *Secp256k1PrivateKey) PublicKey() PublicKey {
	return k.GetPublic()
}

// Sign 使用此私钥签名数据
//
// 返回 64 字节签名：R (32 字节) + S (32 字节)
func (k *Secp256k1PrivateKey) Sign(data []byte) ([]byte, error) {
	hash := sha256.Sum256(data)
	z := new(big.Int).SetBytes(hash[:])

	// 生成随机 k
	for {
		kBytes := make([]byte, 32)
		if _, err := rand.Read(kBytes); err != nil {
			return nil, err
		}
		kInt := new(big.Int).SetBytes(kBytes)

		// 确保 k 在有效范围内
		if kInt.Sign() <= 0 || kInt.Cmp(secp256k1N) >= 0 {
			continue
		}

		// R = k * G
		rx, _ := secp256k1ScalarBaseMult(kInt.Bytes())
		if rx == nil {
			continue
		}

		// r = rx mod n
		r := new(big.Int).Mod(rx, secp256k1N)
		if r.Sign() == 0 {
			continue
		}

		// s = k^(-1) * (z + r * d) mod n
		kInv := new(big.Int).ModInverse(kInt, secp256k1N)
		if kInv == nil {
			continue
		}

		s := new(big.Int).Mul(r, k.D)
		s.Add(s, z)
		s.Mul(s, kInv)
		s.Mod(s, secp256k1N)

		if s.Sign() == 0 {
			continue
		}

		// 规范化 s（确保 s <= n/2）
		halfN := new(big.Int).Div(secp256k1N, big.NewInt(2))
		if s.Cmp(halfN) > 0 {
			s.Sub(secp256k1N, s)
		}

		// 返回固定长度的 R || S
		sig := make([]byte, Secp256k1SignatureSize)
		copy(sig[:32], secp256k1PaddedBytes(r, 32))
		copy(sig[32:], secp256k1PaddedBytes(s, 32))
		return sig, nil
	}
}

// ============================================================================
//                              工厂函数
// ============================================================================

// GenerateSecp256k1Key 生成新的 Secp256k1 密钥对
func GenerateSecp256k1Key(src io.Reader) (PrivateKey, PublicKey, error) {
	// 生成私钥
	for {
		dBytes := make([]byte, 32)
		if _, err := io.ReadFull(src, dBytes); err != nil {
			return nil, nil, err
		}

		d := new(big.Int).SetBytes(dBytes)

		// 确保私钥在有效范围内 [1, n-1]
		if d.Sign() <= 0 || d.Cmp(secp256k1N) >= 0 {
			continue
		}

		// 计算公钥 P = d * G
		x, y := secp256k1ScalarBaseMult(dBytes)
		if x == nil {
			continue
		}

		priv := &Secp256k1PrivateKey{D: d, X: x, Y: y}
		pub := &Secp256k1PublicKey{X: x, Y: y}
		return priv, pub, nil
	}
}

// UnmarshalSecp256k1PublicKey 从字节反序列化 Secp256k1 公钥
//
// 支持压缩格式（33 字节）和未压缩格式（65 字节）
func UnmarshalSecp256k1PublicKey(data []byte) (PublicKey, error) {
	switch len(data) {
	case Secp256k1PublicKeySize:
		// 压缩格式
		x, y := secp256k1DecompressPublicKey(data)
		if x == nil || y == nil {
			return nil, ErrInvalidPublicKey
		}
		return &Secp256k1PublicKey{X: x, Y: y}, nil

	case Secp256k1UncompressedPublicKeySize:
		// 未压缩格式
		if data[0] != 0x04 {
			return nil, ErrInvalidPublicKey
		}
		x := new(big.Int).SetBytes(data[1:33])
		y := new(big.Int).SetBytes(data[33:65])
		return &Secp256k1PublicKey{X: x, Y: y}, nil

	default:
		return nil, fmt.Errorf("%w: expected %d or %d bytes, got %d",
			ErrInvalidKeySize, Secp256k1PublicKeySize, Secp256k1UncompressedPublicKeySize, len(data))
	}
}

// UnmarshalSecp256k1PrivateKey 从字节反序列化 Secp256k1 私钥
func UnmarshalSecp256k1PrivateKey(data []byte) (PrivateKey, error) {
	if len(data) != Secp256k1PrivateKeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d",
			ErrInvalidKeySize, Secp256k1PrivateKeySize, len(data))
	}

	d := new(big.Int).SetBytes(data)

	// 验证私钥在有效范围内
	if d.Sign() <= 0 || d.Cmp(secp256k1N) >= 0 {
		return nil, ErrInvalidPrivateKey
	}

	// 计算公钥
	x, y := secp256k1ScalarBaseMult(data)
	if x == nil {
		return nil, ErrInvalidPrivateKey
	}

	return &Secp256k1PrivateKey{D: d, X: x, Y: y}, nil
}

// ============================================================================
//                              椭圆曲线运算（纯 Go 实现）
// ============================================================================

// secp256k1PaddedBytes 返回固定长度的字节切片
func secp256k1PaddedBytes(n *big.Int, length int) []byte {
	b := n.Bytes()
	if len(b) >= length {
		return b[len(b)-length:]
	}
	padded := make([]byte, length)
	copy(padded[length-len(b):], b)
	return padded
}

// secp256k1CompressPublicKey 压缩公钥
func secp256k1CompressPublicKey(x, y *big.Int) []byte {
	compressed := make([]byte, Secp256k1PublicKeySize)
	if y.Bit(0) == 0 {
		compressed[0] = 0x02
	} else {
		compressed[0] = 0x03
	}
	copy(compressed[1:], secp256k1PaddedBytes(x, 32))
	return compressed
}

// secp256k1DecompressPublicKey 解压公钥
func secp256k1DecompressPublicKey(data []byte) (*big.Int, *big.Int) {
	if len(data) != Secp256k1PublicKeySize {
		return nil, nil
	}

	prefix := data[0]
	if prefix != 0x02 && prefix != 0x03 {
		return nil, nil
	}

	x := new(big.Int).SetBytes(data[1:])

	// y² = x³ + 7 (mod P)
	x3 := new(big.Int).Mul(x, x)
	x3.Mul(x3, x)
	x3.Mod(x3, secp256k1P)

	y2 := new(big.Int).Add(x3, secp256k1B)
	y2.Mod(y2, secp256k1P)

	// y = sqrt(y²) mod P
	// secp256k1 的 P ≡ 3 (mod 4)
	exp := new(big.Int).Add(secp256k1P, big.NewInt(1))
	exp.Div(exp, big.NewInt(4))
	y := new(big.Int).Exp(y2, exp, secp256k1P)

	// 验证
	check := new(big.Int).Mul(y, y)
	check.Mod(check, secp256k1P)
	if check.Cmp(y2) != 0 {
		return nil, nil
	}

	// 选择正确的 Y
	if (prefix == 0x02) != (y.Bit(0) == 0) {
		y.Sub(secp256k1P, y)
	}

	return x, y
}

// secp256k1ScalarBaseMult 计算 k * G
func secp256k1ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	return secp256k1ScalarMult(secp256k1Gx, secp256k1Gy, k)
}

// secp256k1ScalarMult 计算 k * P
func secp256k1ScalarMult(px, py *big.Int, k []byte) (*big.Int, *big.Int) {
	kInt := new(big.Int).SetBytes(k)
	if kInt.Sign() == 0 {
		return nil, nil
	}

	// 双加算法
	var rx, ry *big.Int
	tx, ty := new(big.Int).Set(px), new(big.Int).Set(py)

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

// secp256k1AddPoints 点加
func secp256k1AddPoints(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	if x1 == nil || x2 == nil {
		if x1 == nil {
			return x2, y2
		}
		return x1, y1
	}

	// 相同点使用点倍
	if x1.Cmp(x2) == 0 && y1.Cmp(y2) == 0 {
		return secp256k1DoublePoint(x1, y1)
	}

	// 互为逆元
	if x1.Cmp(x2) == 0 {
		return nil, nil
	}

	// λ = (y2 - y1) / (x2 - x1)
	dy := new(big.Int).Sub(y2, y1)
	dx := new(big.Int).Sub(x2, x1)
	dxInv := new(big.Int).ModInverse(dx, secp256k1P)
	if dxInv == nil {
		return nil, nil
	}
	lambda := new(big.Int).Mul(dy, dxInv)
	lambda.Mod(lambda, secp256k1P)

	// x3 = λ² - x1 - x2
	x3 := new(big.Int).Mul(lambda, lambda)
	x3.Sub(x3, x1)
	x3.Sub(x3, x2)
	x3.Mod(x3, secp256k1P)

	// y3 = λ(x1 - x3) - y1
	y3 := new(big.Int).Sub(x1, x3)
	y3.Mul(y3, lambda)
	y3.Sub(y3, y1)
	y3.Mod(y3, secp256k1P)

	return x3, y3
}

// secp256k1DoublePoint 点倍
func secp256k1DoublePoint(x, y *big.Int) (*big.Int, *big.Int) {
	if y.Sign() == 0 {
		return nil, nil
	}

	// λ = 3x² / 2y
	x2 := new(big.Int).Mul(x, x)
	x2.Mul(x2, big.NewInt(3))
	y2 := new(big.Int).Mul(y, big.NewInt(2))
	y2Inv := new(big.Int).ModInverse(y2, secp256k1P)
	if y2Inv == nil {
		return nil, nil
	}
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
