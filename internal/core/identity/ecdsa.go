// Package identity 提供身份管理模块的实现
//
// ECDSA 密钥支持：
// - P-256 曲线
// - P-384 曲线
// - PKCS8/X509 序列化
package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

// ECDSA 相关错误
var (
	// ErrInvalidECDSASignature ECDSA 签名无效
	ErrInvalidECDSASignature = errors.New("invalid ECDSA signature")
	ErrInvalidCurve          = errors.New("invalid elliptic curve")
	ErrInvalidKeyData        = errors.New("invalid key data")
)

// ============================================================================
//                              ECDSAPublicKey 实现
// ============================================================================

// ECDSAPublicKey ECDSA 公钥
type ECDSAPublicKey struct {
	key   *ecdsa.PublicKey
	curve elliptic.Curve
}

// 确保实现接口
var _ identityif.PublicKey = (*ECDSAPublicKey)(nil)

// NewECDSAPublicKey 从 ecdsa.PublicKey 创建公钥
func NewECDSAPublicKey(key *ecdsa.PublicKey) *ECDSAPublicKey {
	return &ECDSAPublicKey{
		key:   key,
		curve: key.Curve,
	}
}

// NewECDSAPublicKeyFromBytes 从字节创建公钥
func NewECDSAPublicKeyFromBytes(data []byte, curve elliptic.Curve) (*ECDSAPublicKey, error) {
	if curve == nil {
		// 尝试自动检测曲线
		switch len(data) {
		case 65: // P-256 uncompressed
			curve = elliptic.P256()
		case 97: // P-384 uncompressed
			curve = elliptic.P384()
		default:
			return nil, ErrInvalidKeyData
		}
	}

	x, y := elliptic.Unmarshal(curve, data)
	if x == nil {
		return nil, ErrInvalidKeyData
	}

	return &ECDSAPublicKey{
		key: &ecdsa.PublicKey{
			Curve: curve,
			X:     x,
			Y:     y,
		},
		curve: curve,
	}, nil
}

// Bytes 返回公钥字节
func (k *ECDSAPublicKey) Bytes() []byte {
	return elliptic.Marshal(k.key.Curve, k.key.X, k.key.Y)
}

// Type 返回密钥类型
func (k *ECDSAPublicKey) Type() types.KeyType {
	switch k.curve {
	case elliptic.P256():
		return types.KeyTypeECDSAP256
	case elliptic.P384():
		return types.KeyTypeECDSAP384
	default:
		return types.KeyTypeUnknown
	}
}

// Verify 验证签名
func (k *ECDSAPublicKey) Verify(data, sig []byte) (bool, error) {
	// 解析签名（ASN.1 DER 格式或 r||s 格式）
	r, s, err := parseECDSASignature(sig, k.curve)
	if err != nil {
		return false, err
	}

	// 根据曲线选择哈希算法
	// P-256 使用 SHA-256，P-384 使用 SHA-384
	var hash []byte
	switch k.curve {
	case elliptic.P384():
		h := sha512.Sum384(data)
		hash = h[:]
	default: // P-256 及其他
		h := sha256.Sum256(data)
		hash = h[:]
	}

	// 验证
	return ecdsa.Verify(k.key, hash, r, s), nil
}

// Equal 比较公钥是否相等
func (k *ECDSAPublicKey) Equal(other identityif.PublicKey) bool {
	if other == nil {
		return false
	}
	otherECDSA, ok := other.(*ECDSAPublicKey)
	if !ok {
		return false
	}
	return k.key.X.Cmp(otherECDSA.key.X) == 0 && k.key.Y.Cmp(otherECDSA.key.Y) == 0
}

// Raw 返回原始 ecdsa.PublicKey
func (k *ECDSAPublicKey) Raw() crypto.PublicKey {
	return k.key
}

// ============================================================================
//                              ECDSAPrivateKey 实现
// ============================================================================

// ECDSAPrivateKey ECDSA 私钥
type ECDSAPrivateKey struct {
	key       *ecdsa.PrivateKey
	publicKey *ECDSAPublicKey
}

// 确保实现接口
var _ identityif.PrivateKey = (*ECDSAPrivateKey)(nil)

// NewECDSAPrivateKey 从 ecdsa.PrivateKey 创建私钥
func NewECDSAPrivateKey(key *ecdsa.PrivateKey) *ECDSAPrivateKey {
	return &ECDSAPrivateKey{
		key:       key,
		publicKey: NewECDSAPublicKey(&key.PublicKey),
	}
}

// GenerateECDSAKeyPair 生成 ECDSA 密钥对
func GenerateECDSAKeyPair(curve elliptic.Curve) (*ECDSAPrivateKey, *ECDSAPublicKey, error) {
	if curve == nil {
		curve = elliptic.P256() // 默认 P-256
	}

	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	priv := NewECDSAPrivateKey(key)
	return priv, priv.publicKey, nil
}

// GenerateECDSAP256KeyPair 生成 P-256 密钥对
func GenerateECDSAP256KeyPair() (*ECDSAPrivateKey, *ECDSAPublicKey, error) {
	return GenerateECDSAKeyPair(elliptic.P256())
}

// GenerateECDSAP384KeyPair 生成 P-384 密钥对
func GenerateECDSAP384KeyPair() (*ECDSAPrivateKey, *ECDSAPublicKey, error) {
	return GenerateECDSAKeyPair(elliptic.P384())
}

// Bytes 返回私钥字节（PKCS8 编码）
func (k *ECDSAPrivateKey) Bytes() []byte {
	data, err := x509.MarshalPKCS8PrivateKey(k.key)
	if err != nil {
		return nil
	}
	return data
}

// Type 返回密钥类型
func (k *ECDSAPrivateKey) Type() types.KeyType {
	return k.publicKey.Type()
}

// Sign 签名数据
func (k *ECDSAPrivateKey) Sign(data []byte) ([]byte, error) {
	// 根据曲线选择哈希算法
	// P-256 使用 SHA-256，P-384 使用 SHA-384
	var hash []byte
	switch k.key.Curve {
	case elliptic.P384():
		h := sha512.Sum384(data)
		hash = h[:]
	default: // P-256 及其他
		h := sha256.Sum256(data)
		hash = h[:]
	}

	// 签名
	r, s, err := ecdsa.Sign(rand.Reader, k.key, hash)
	if err != nil {
		return nil, err
	}

	// 编码签名（r||s 格式）
	return encodeECDSASignature(r, s, k.key.Curve), nil
}

// PublicKey 返回对应的公钥
func (k *ECDSAPrivateKey) PublicKey() identityif.PublicKey {
	return k.publicKey
}

// Equal 比较私钥是否相等
func (k *ECDSAPrivateKey) Equal(other identityif.PrivateKey) bool {
	if other == nil {
		return false
	}
	otherECDSA, ok := other.(*ECDSAPrivateKey)
	if !ok {
		return false
	}
	return k.key.D.Cmp(otherECDSA.key.D) == 0
}

// Raw 返回原始 ecdsa.PrivateKey
func (k *ECDSAPrivateKey) Raw() crypto.PrivateKey {
	return k.key
}

// ============================================================================
//                              序列化方法
// ============================================================================

// ECDSAPrivateKeyToPEM 将私钥导出为 PEM 格式
func ECDSAPrivateKeyToPEM(key *ECDSAPrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key.key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}), nil
}

// ECDSAPrivateKeyFromPEM 从 PEM 格式导入私钥
func ECDSAPrivateKeyFromPEM(data []byte) (*ECDSAPrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// 尝试 EC 私钥格式
		ecKey, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return NewECDSAPrivateKey(ecKey), nil
	}

	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an ECDSA private key")
	}

	return NewECDSAPrivateKey(ecdsaKey), nil
}

// ECDSAPublicKeyToPEM 将公钥导出为 PEM 格式
func ECDSAPublicKeyToPEM(key *ECDSAPublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key.key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}), nil
}

// ECDSAPublicKeyFromPEM 从 PEM 格式导入公钥
func ECDSAPublicKeyFromPEM(data []byte) (*ECDSAPublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	ecdsaKey, ok := key.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("not an ECDSA public key")
	}

	return NewECDSAPublicKey(ecdsaKey), nil
}

// ============================================================================
//                              辅助函数
// ============================================================================

// parseECDSASignature 解析 ECDSA 签名
func parseECDSASignature(sig []byte, curve elliptic.Curve) (*big.Int, *big.Int, error) {
	byteLen := (curve.Params().BitSize + 7) / 8

	// 检查是否是 r||s 格式
	if len(sig) == 2*byteLen {
		r := new(big.Int).SetBytes(sig[:byteLen])
		s := new(big.Int).SetBytes(sig[byteLen:])
		return r, s, nil
	}

	return nil, nil, fmt.Errorf("%w: invalid signature length %d, expected %d", ErrInvalidECDSASignature, len(sig), 2*byteLen)
}

// encodeECDSASignature 编码 ECDSA 签名为 r||s 格式
func encodeECDSASignature(r, s *big.Int, curve elliptic.Curve) []byte {
	byteLen := (curve.Params().BitSize + 7) / 8

	rBytes := r.Bytes()
	sBytes := s.Bytes()

	// 补零到固定长度
	sig := make([]byte, 2*byteLen)
	copy(sig[byteLen-len(rBytes):byteLen], rBytes)
	copy(sig[2*byteLen-len(sBytes):], sBytes)

	return sig
}

