// Package identity 实现身份管理
package identity

import (
	"crypto/rand"
	"encoding/pem"
	"errors"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 错误定义
// ============================================================================

var (
	// ErrInvalidKeyType 无效的密钥类型
	ErrInvalidKeyType = errors.New("invalid key type")
	// ErrInvalidKey 无效的密钥
	ErrInvalidKey = errors.New("invalid key")
	// ErrInvalidPEM 无效的 PEM 格式
	ErrInvalidPEM = errors.New("invalid PEM format")
)

// ============================================================================
// 密钥生成（使用 pkg/crypto）
// ============================================================================

// GenerateEd25519Key 生成 Ed25519 密钥对
//
// 此函数是 pkg/crypto.GenerateEd25519Key 的包装，返回兼容 pkg/interfaces 的类型。
func GenerateEd25519Key() (pkgif.PrivateKey, pkgif.PublicKey, error) {
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	// 包装为兼容类型
	return &privateKeyAdapter{priv}, &publicKeyAdapter{pub}, nil
}

// ============================================================================
// 适配器：桥接 crypto 和 interfaces 的接口差异
// ============================================================================

// privateKeyAdapter 适配 crypto.PrivateKey 到 pkgif.PrivateKey
type privateKeyAdapter struct {
	crypto.PrivateKey
}

// Type 实现 pkgif.PrivateKey 接口（转换 KeyType）
func (a *privateKeyAdapter) Type() pkgif.KeyType {
	return pkgif.KeyType(a.PrivateKey.Type())
}

// Equals 实现 pkgif.PrivateKey 接口
func (a *privateKeyAdapter) Equals(other pkgif.PrivateKey) bool {
	if other == nil {
		return false
	}
	// 通过比较原始字节
	myRaw, err1 := a.Raw()
	otherRaw, err2 := other.Raw()
	if err1 != nil || err2 != nil {
		return false
	}
	return string(myRaw) == string(otherRaw)
}

// PublicKey 实现 pkgif.PrivateKey 接口
func (a *privateKeyAdapter) PublicKey() pkgif.PublicKey {
	return &publicKeyAdapter{a.GetPublic()}
}

// publicKeyAdapter 适配 crypto.PublicKey 到 pkgif.PublicKey
type publicKeyAdapter struct {
	crypto.PublicKey
}

// Type 实现 pkgif.PublicKey 接口（转换 KeyType）
func (a *publicKeyAdapter) Type() pkgif.KeyType {
	return pkgif.KeyType(a.PublicKey.Type())
}

// Equals 实现 pkgif.PublicKey 接口
func (a *publicKeyAdapter) Equals(other pkgif.PublicKey) bool {
	if other == nil {
		return false
	}
	// 通过比较原始字节
	myRaw, err1 := a.Raw()
	otherRaw, err2 := other.Raw()
	if err1 != nil || err2 != nil {
		return false
	}
	return string(myRaw) == string(otherRaw)
}

// ============================================================================
// PEM 格式支持
// ============================================================================

const (
	// PEM block types
	pemTypeEd25519Private = "ED25519 PRIVATE KEY"
)

// MarshalPrivateKeyPEM 将私钥编码为 PEM 格式
//
// 目前仅支持 Ed25519 密钥。
func MarshalPrivateKeyPEM(key pkgif.PrivateKey) ([]byte, error) {
	if key.Type() != pkgif.KeyTypeEd25519 {
		return nil, ErrInvalidKeyType
	}

	raw, err := key.Raw()
	if err != nil {
		return nil, err
	}

	block := &pem.Block{
		Type:  pemTypeEd25519Private,
		Bytes: raw,
	}

	return pem.EncodeToMemory(block), nil
}

// UnmarshalPrivateKeyPEM 从 PEM 格式解析私钥
//
// 使用 pkg/crypto 的反序列化函数。
func UnmarshalPrivateKeyPEM(data []byte) (pkgif.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrInvalidPEM
	}

	if block.Type != pemTypeEd25519Private {
		return nil, ErrInvalidKeyType
	}

	// 使用 pkg/crypto 反序列化
	priv, err := crypto.UnmarshalEd25519PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return &privateKeyAdapter{priv}, nil
}

// ============================================================================
// 密钥序列化辅助函数（使用 pkg/crypto）
// ============================================================================

// PrivateKeyFromBytes 从字节创建私钥
func PrivateKeyFromBytes(data []byte, keyType pkgif.KeyType) (pkgif.PrivateKey, error) {
	var priv crypto.PrivateKey
	var err error

	switch keyType {
	case pkgif.KeyTypeEd25519:
		priv, err = crypto.UnmarshalEd25519PrivateKey(data)
	case pkgif.KeyTypeRSA:
		priv, err = crypto.UnmarshalRSAPrivateKey(data)
	case pkgif.KeyTypeSecp256k1:
		priv, err = crypto.UnmarshalSecp256k1PrivateKey(data)
	case pkgif.KeyTypeECDSA:
		priv, err = crypto.UnmarshalECDSAPrivateKey(data)
	default:
		return nil, ErrInvalidKeyType
	}

	if err != nil {
		return nil, err
	}
	return &privateKeyAdapter{priv}, nil
}

// PublicKeyFromBytes 从字节创建公钥
func PublicKeyFromBytes(data []byte, keyType pkgif.KeyType) (pkgif.PublicKey, error) {
	var pub crypto.PublicKey
	var err error

	switch keyType {
	case pkgif.KeyTypeEd25519:
		pub, err = crypto.UnmarshalEd25519PublicKey(data)
	case pkgif.KeyTypeRSA:
		pub, err = crypto.UnmarshalRSAPublicKey(data)
	case pkgif.KeyTypeSecp256k1:
		pub, err = crypto.UnmarshalSecp256k1PublicKey(data)
	case pkgif.KeyTypeECDSA:
		pub, err = crypto.UnmarshalECDSAPublicKey(data)
	default:
		return nil, ErrInvalidKeyType
	}

	if err != nil {
		return nil, err
	}
	return &publicKeyAdapter{pub}, nil
}
