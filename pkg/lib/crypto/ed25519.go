package crypto

import (
	"crypto/ed25519"
	"crypto/subtle"
	"fmt"
	"io"
)

// Ed25519 密钥常量
const (
	// Ed25519PrivateKeySize Ed25519 私钥大小（64 字节）
	Ed25519PrivateKeySize = ed25519.PrivateKeySize
	// Ed25519PublicKeySize Ed25519 公钥大小（32 字节）
	Ed25519PublicKeySize = ed25519.PublicKeySize
	// Ed25519SignatureSize Ed25519 签名大小（64 字节）
	Ed25519SignatureSize = ed25519.SignatureSize
	// Ed25519SeedSize Ed25519 种子大小（32 字节）
	Ed25519SeedSize = ed25519.SeedSize
)

// ============================================================================
//                              Ed25519PublicKey
// ============================================================================

// Ed25519PublicKey Ed25519 公钥实现
type Ed25519PublicKey struct {
	k ed25519.PublicKey
}

// Raw 返回原始公钥字节
func (k *Ed25519PublicKey) Raw() ([]byte, error) {
	buf := make([]byte, len(k.k))
	copy(buf, k.k)
	return buf, nil
}

// Type 返回密钥类型
func (k *Ed25519PublicKey) Type() KeyType {
	return KeyTypeEd25519
}

// Equals 比较两个公钥是否相等
//
// 使用常量时间比较以防止时序攻击。
func (k *Ed25519PublicKey) Equals(other Key) bool {
	ek, ok := other.(*Ed25519PublicKey)
	if !ok {
		return KeyEqual(k, other)
	}
	return subtle.ConstantTimeCompare(k.k, ek.k) == 1
}

// Verify 使用此公钥验证签名
func (k *Ed25519PublicKey) Verify(data, sig []byte) (bool, error) {
	if len(sig) != Ed25519SignatureSize {
		return false, nil
	}
	return ed25519.Verify(k.k, data, sig), nil
}

// ============================================================================
//                              Ed25519PrivateKey
// ============================================================================

// Ed25519PrivateKey Ed25519 私钥实现
type Ed25519PrivateKey struct {
	k ed25519.PrivateKey
}

// Raw 返回原始私钥字节
//
// Ed25519 私钥为 64 字节，包含 32 字节私钥种子和 32 字节公钥。
func (k *Ed25519PrivateKey) Raw() ([]byte, error) {
	buf := make([]byte, len(k.k))
	copy(buf, k.k)
	return buf, nil
}

// Seed 返回私钥种子（32 字节）
func (k *Ed25519PrivateKey) Seed() []byte {
	return k.k.Seed()
}

// Type 返回密钥类型
func (k *Ed25519PrivateKey) Type() KeyType {
	return KeyTypeEd25519
}

// Equals 比较两个私钥是否相等
//
// 使用常量时间比较以防止时序攻击。
func (k *Ed25519PrivateKey) Equals(other Key) bool {
	ek, ok := other.(*Ed25519PrivateKey)
	if !ok {
		return KeyEqual(k, other)
	}
	return subtle.ConstantTimeCompare(k.k, ek.k) == 1
}

// GetPublic 返回对应的公钥
func (k *Ed25519PrivateKey) GetPublic() PublicKey {
	// ed25519.PrivateKey.Public() 总是返回 ed25519.PublicKey
	pub := k.k.Public().(ed25519.PublicKey) //nolint:errcheck // 类型断言安全
	return &Ed25519PublicKey{k: pub}
}

// PublicKey 返回对应的公钥（兼容 pkg/interfaces.PrivateKey 接口）
func (k *Ed25519PrivateKey) PublicKey() PublicKey {
	return k.GetPublic()
}

// Sign 使用此私钥签名数据
func (k *Ed25519PrivateKey) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(k.k, data), nil
}

// ============================================================================
//                              工厂函数
// ============================================================================

// GenerateEd25519Key 生成新的 Ed25519 密钥对
//
// 参数：
//   - src: 随机源
//
// 返回：
//   - PrivateKey: 私钥
//   - PublicKey: 公钥
//   - error: 生成错误
func GenerateEd25519Key(src io.Reader) (PrivateKey, PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(src)
	if err != nil {
		return nil, nil, err
	}
	return &Ed25519PrivateKey{k: priv}, &Ed25519PublicKey{k: pub}, nil
}

// UnmarshalEd25519PublicKey 从字节反序列化 Ed25519 公钥
//
// 参数：
//   - data: 原始公钥字节（32 字节）
//
// 返回：
//   - PublicKey: 公钥对象
//   - error: 反序列化错误
func UnmarshalEd25519PublicKey(data []byte) (PublicKey, error) {
	if len(data) != Ed25519PublicKeySize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidKeySize, Ed25519PublicKeySize, len(data))
	}

	k := make([]byte, Ed25519PublicKeySize)
	copy(k, data)
	return &Ed25519PublicKey{k: k}, nil
}

// UnmarshalEd25519PrivateKey 从字节反序列化 Ed25519 私钥
//
// 支持两种格式：
//   - 64 字节：完整私钥（私钥种子 + 公钥）
//   - 32 字节：仅私钥种子
//   - 96 字节：带冗余公钥的格式（兼容 libp2p）
//
// 参数：
//   - data: 原始私钥字节
//
// 返回：
//   - PrivateKey: 私钥对象
//   - error: 反序列化错误
func UnmarshalEd25519PrivateKey(data []byte) (PrivateKey, error) {
	switch len(data) {
	case Ed25519PrivateKeySize + Ed25519PublicKeySize:
		// 96 字节格式：64 字节私钥 + 32 字节冗余公钥
		// 验证冗余公钥是否匹配
		redundantPk := data[Ed25519PrivateKeySize:]
		pk := data[Ed25519PrivateKeySize-Ed25519PublicKeySize : Ed25519PrivateKeySize]
		if subtle.ConstantTimeCompare(pk, redundantPk) == 0 {
			return nil, fmt.Errorf("%w: redundant public key mismatch", ErrInvalidPrivateKey)
		}
		// 只保留 64 字节
		k := make([]byte, Ed25519PrivateKeySize)
		copy(k, data[:Ed25519PrivateKeySize])
		return &Ed25519PrivateKey{k: k}, nil

	case Ed25519PrivateKeySize:
		// 64 字节格式：完整私钥
		k := make([]byte, Ed25519PrivateKeySize)
		copy(k, data)
		return &Ed25519PrivateKey{k: k}, nil

	case Ed25519SeedSize:
		// 32 字节格式：仅种子，需要派生完整私钥
		return &Ed25519PrivateKey{k: ed25519.NewKeyFromSeed(data)}, nil

	default:
		return nil, fmt.Errorf("%w: expected %d, %d or %d bytes, got %d",
			ErrInvalidKeySize, Ed25519SeedSize, Ed25519PrivateKeySize, Ed25519PrivateKeySize+Ed25519PublicKeySize, len(data))
	}
}
