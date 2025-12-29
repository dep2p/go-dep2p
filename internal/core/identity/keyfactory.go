// Package identity 提供身份管理模块的实现
package identity

import (
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              KeyFactory 实现
// ============================================================================

// KeyFactoryImpl 密钥工厂实现
//
// 实现 identityif.KeyFactory 接口，提供从字节创建公钥/私钥的能力。
type KeyFactoryImpl struct{}

// 确保实现接口
var _ identityif.KeyFactory = (*KeyFactoryImpl)(nil)

// NewKeyFactory 创建密钥工厂
func NewKeyFactory() *KeyFactoryImpl {
	return &KeyFactoryImpl{}
}

// PublicKeyFromBytes 从字节创建公钥
//
// 根据 keyType 自动选择对应的密钥实现。
// 支持：Ed25519、ECDSA P-256、ECDSA P-384
func (f *KeyFactoryImpl) PublicKeyFromBytes(keyBytes []byte, keyType types.KeyType) (identityif.PublicKey, error) {
	return PublicKeyFromBytes(keyBytes, keyType)
}

// PrivateKeyFromBytes 从字节创建私钥
//
// 根据 keyType 自动选择对应的密钥实现。
func (f *KeyFactoryImpl) PrivateKeyFromBytes(keyBytes []byte, keyType types.KeyType) (identityif.PrivateKey, error) {
	return PrivateKeyFromBytes(keyBytes, keyType)
}

