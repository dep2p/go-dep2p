// Package identity 提供身份管理模块的实现
package identity

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"errors"

	cryptoif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 错误定义
var (
	// ErrInvalidKeySize 无效的密钥大小
	ErrInvalidKeySize = errors.New("invalid key size")
	// ErrInvalidKeyType 无效的密钥类型
	ErrInvalidKeyType = errors.New("invalid key type")
	// ErrSignatureFailed 签名失败
	ErrSignatureFailed = errors.New("signature failed")
)

// ============================================================================
//                              Ed25519PublicKey
// ============================================================================

// Ed25519PublicKey Ed25519 公钥实现
type Ed25519PublicKey struct {
	key ed25519.PublicKey
}

// 确保实现接口
var _ cryptoif.PublicKey = (*Ed25519PublicKey)(nil)

// NewEd25519PublicKey 从字节创建 Ed25519 公钥
func NewEd25519PublicKey(keyBytes []byte) (*Ed25519PublicKey, error) {
	if len(keyBytes) != ed25519.PublicKeySize {
		return nil, ErrInvalidKeySize
	}
	return &Ed25519PublicKey{key: ed25519.PublicKey(keyBytes)}, nil
}

// Bytes 返回公钥的字节表示
func (k *Ed25519PublicKey) Bytes() []byte {
	return k.key
}

// Equal 比较两个公钥是否相等
func (k *Ed25519PublicKey) Equal(other cryptoif.PublicKey) bool {
	otherEd, ok := other.(*Ed25519PublicKey)
	if !ok {
		return false
	}
	return k.key.Equal(otherEd.key)
}

// Verify 使用公钥验证签名
func (k *Ed25519PublicKey) Verify(data, signature []byte) (bool, error) {
	if len(signature) != ed25519.SignatureSize {
		return false, nil
	}
	return ed25519.Verify(k.key, data, signature), nil
}

// Type 返回密钥类型
func (k *Ed25519PublicKey) Type() types.KeyType {
	return types.KeyTypeEd25519
}

// Raw 返回底层密钥
func (k *Ed25519PublicKey) Raw() crypto.PublicKey {
	return k.key
}

// ============================================================================
//                              Ed25519PrivateKey
// ============================================================================

// Ed25519PrivateKey Ed25519 私钥实现
type Ed25519PrivateKey struct {
	key ed25519.PrivateKey
}

// 确保实现接口
var _ cryptoif.PrivateKey = (*Ed25519PrivateKey)(nil)

// NewEd25519PrivateKey 从字节创建 Ed25519 私钥
func NewEd25519PrivateKey(keyBytes []byte) (*Ed25519PrivateKey, error) {
	if len(keyBytes) != ed25519.PrivateKeySize {
		return nil, ErrInvalidKeySize
	}
	return &Ed25519PrivateKey{key: ed25519.PrivateKey(keyBytes)}, nil
}

// PublicKey 返回对应的公钥
func (k *Ed25519PrivateKey) PublicKey() cryptoif.PublicKey {
	pub := k.key.Public().(ed25519.PublicKey)
	return &Ed25519PublicKey{key: pub}
}

// Sign 使用私钥签名数据
func (k *Ed25519PrivateKey) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(k.key, data), nil
}

// Bytes 返回私钥的字节表示
func (k *Ed25519PrivateKey) Bytes() []byte {
	return k.key
}

// Type 返回密钥类型
func (k *Ed25519PrivateKey) Type() types.KeyType {
	return types.KeyTypeEd25519
}

// Raw 返回底层密钥
func (k *Ed25519PrivateKey) Raw() crypto.PrivateKey {
	return k.key
}

// ============================================================================
//                              密钥对生成
// ============================================================================

// GenerateEd25519KeyPair 生成 Ed25519 密钥对
func GenerateEd25519KeyPair() (cryptoif.PrivateKey, cryptoif.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return &Ed25519PrivateKey{key: priv}, &Ed25519PublicKey{key: pub}, nil
}

// ============================================================================
//                              Ed25519KeyGenerator
// ============================================================================

// Ed25519KeyGenerator Ed25519 密钥生成器
//
// 注意：KeyGenerator public 接口已删除（v1.1 清理）。
// 此实现保留供测试使用。
type Ed25519KeyGenerator struct{}

// NewEd25519KeyGenerator 创建 Ed25519 密钥生成器
func NewEd25519KeyGenerator() *Ed25519KeyGenerator {
	return &Ed25519KeyGenerator{}
}

// Generate 生成新的密钥对
func (g *Ed25519KeyGenerator) Generate() (cryptoif.PrivateKey, cryptoif.PublicKey, error) {
	return GenerateEd25519KeyPair()
}

// Type 返回生成的密钥类型
func (g *Ed25519KeyGenerator) Type() types.KeyType {
	return types.KeyTypeEd25519
}

