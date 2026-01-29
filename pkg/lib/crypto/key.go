// Package crypto 提供 DeP2P 密码学工具
package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"io"
)

// ============================================================================
//                              密钥类型定义
// ============================================================================

// KeyType 密钥类型
//
// 值与 pkg/proto/key/key.proto 中的 KeyType 枚举对齐：
//   - KEY_TYPE_UNSPECIFIED = 0
//   - RSA = 1
//   - Ed25519 = 2
//   - Secp256k1 = 3
//   - ECDSA = 4
type KeyType int

const (
	// KeyTypeUnspecified 未指定密钥类型
	KeyTypeUnspecified KeyType = 0
	// KeyTypeRSA RSA 密钥
	KeyTypeRSA KeyType = 1
	// KeyTypeEd25519 Ed25519 密钥（默认推荐）
	KeyTypeEd25519 KeyType = 2
	// KeyTypeSecp256k1 Secp256k1 密钥（区块链兼容）
	KeyTypeSecp256k1 KeyType = 3
	// KeyTypeECDSA ECDSA 密钥
	KeyTypeECDSA KeyType = 4
)

// String 返回密钥类型名称
func (kt KeyType) String() string {
	switch kt {
	case KeyTypeUnspecified:
		return "Unspecified"
	case KeyTypeRSA:
		return "RSA"
	case KeyTypeEd25519:
		return "Ed25519"
	case KeyTypeSecp256k1:
		return "Secp256k1"
	case KeyTypeECDSA:
		return "ECDSA"
	default:
		return "Unknown"
	}
}

// KeyTypes 支持的密钥类型列表
var KeyTypes = []KeyType{
	KeyTypeRSA,
	KeyTypeEd25519,
	KeyTypeSecp256k1,
	KeyTypeECDSA,
}

// ============================================================================
//                              密钥接口定义
// ============================================================================

// Key 基础密钥接口
type Key interface {
	// Raw 返回原始密钥字节
	Raw() ([]byte, error)

	// Type 返回密钥类型
	Type() KeyType

	// Equals 比较两个密钥是否相等
	Equals(Key) bool
}

// PublicKey 公钥接口
type PublicKey interface {
	Key

	// Verify 使用此公钥验证签名
	//
	// 参数：
	//   - data: 原始数据
	//   - sig: 签名字节
	//
	// 返回：
	//   - bool: 签名是否有效
	//   - error: 验证过程中的错误
	Verify(data, sig []byte) (bool, error)
}

// PrivateKey 私钥接口
type PrivateKey interface {
	Key

	// Sign 使用此私钥签名数据
	//
	// 参数：
	//   - data: 要签名的数据
	//
	// 返回：
	//   - []byte: 签名字节
	//   - error: 签名过程中的错误
	Sign(data []byte) ([]byte, error)

	// GetPublic 返回对应的公钥
	GetPublic() PublicKey
}

// ============================================================================
//                              密钥工厂函数
// ============================================================================

// GenerateKeyPair 生成密钥对
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
func GenerateKeyPair(keyType KeyType) (PrivateKey, PublicKey, error) {
	return GenerateKeyPairWithReader(keyType, rand.Reader)
}

// GenerateKeyPairWithReader 使用指定的随机源生成密钥对
//
// 参数：
//   - keyType: 密钥类型
//   - reader: 随机源（用于测试时的确定性生成）
//
// 返回：
//   - PrivateKey: 私钥
//   - PublicKey: 公钥
//   - error: 生成错误
func GenerateKeyPairWithReader(keyType KeyType, reader io.Reader) (PrivateKey, PublicKey, error) {
	switch keyType {
	case KeyTypeEd25519:
		return GenerateEd25519Key(reader)
	case KeyTypeSecp256k1:
		return GenerateSecp256k1Key(reader)
	case KeyTypeECDSA:
		return GenerateECDSAKey(reader)
	case KeyTypeRSA:
		return GenerateRSAKey(2048, reader)
	default:
		return nil, nil, ErrBadKeyType
	}
}

// ============================================================================
//                              反序列化函数
// ============================================================================

// PubKeyUnmarshaller 公钥反序列化函数类型
type PubKeyUnmarshaller func(data []byte) (PublicKey, error)

// PrivKeyUnmarshaller 私钥反序列化函数类型
type PrivKeyUnmarshaller func(data []byte) (PrivateKey, error)

// PubKeyUnmarshallers 公钥反序列化函数映射
var PubKeyUnmarshallers = map[KeyType]PubKeyUnmarshaller{
	KeyTypeEd25519:   UnmarshalEd25519PublicKey,
	KeyTypeSecp256k1: UnmarshalSecp256k1PublicKey,
	KeyTypeECDSA:     UnmarshalECDSAPublicKey,
	KeyTypeRSA:       UnmarshalRSAPublicKey,
}

// PrivKeyUnmarshallers 私钥反序列化函数映射
var PrivKeyUnmarshallers = map[KeyType]PrivKeyUnmarshaller{
	KeyTypeEd25519:   UnmarshalEd25519PrivateKey,
	KeyTypeSecp256k1: UnmarshalSecp256k1PrivateKey,
	KeyTypeECDSA:     UnmarshalECDSAPrivateKey,
	KeyTypeRSA:       UnmarshalRSAPrivateKey,
}

// UnmarshalPublicKey 从字节反序列化公钥
//
// 参数：
//   - keyType: 密钥类型
//   - data: 原始密钥字节
//
// 返回：
//   - PublicKey: 公钥对象
//   - error: 反序列化错误
func UnmarshalPublicKey(keyType KeyType, data []byte) (PublicKey, error) {
	um, ok := PubKeyUnmarshallers[keyType]
	if !ok {
		return nil, ErrBadKeyType
	}
	return um(data)
}

// UnmarshalPrivateKey 从字节反序列化私钥
//
// 参数：
//   - keyType: 密钥类型
//   - data: 原始密钥字节
//
// 返回：
//   - PrivateKey: 私钥对象
//   - error: 反序列化错误
func UnmarshalPrivateKey(keyType KeyType, data []byte) (PrivateKey, error) {
	um, ok := PrivKeyUnmarshallers[keyType]
	if !ok {
		return nil, ErrBadKeyType
	}
	return um(data)
}

// ============================================================================
//                              辅助函数
// ============================================================================

// KeyEqual 使用常量时间比较两个密钥是否相等
//
// 这是一个安全的比较方法，可以防止时序攻击。
func KeyEqual(k1, k2 Key) bool {
	if k1.Type() != k2.Type() {
		return false
	}

	b1, err1 := k1.Raw()
	b2, err2 := k2.Raw()

	if err1 != nil || err2 != nil {
		return false
	}

	return subtle.ConstantTimeCompare(b1, b2) == 1
}

// ============================================================================
//                              随机数工具
// ============================================================================

// RandomBytes 生成指定长度的加密安全随机字节
//
// 使用系统的加密安全随机源 (crypto/rand)。
//
// 参数：
//   - n: 需要生成的字节数
//
// 返回：
//   - []byte: 随机字节切片
//   - error: 如果随机源不可用则返回错误
//
// 示例：
//
//	key, err := crypto.RandomBytes(32) // 生成 256 位随机密钥
func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, b)
	return b, err
}

// GenerateNonce 生成 32 字节（256 位）随机 nonce
//
// 适用于加密协议中的一次性随机数。
//
// 返回：
//   - []byte: 32 字节的随机 nonce
//   - error: 如果随机源不可用则返回错误
func GenerateNonce() ([]byte, error) {
	return RandomBytes(32)
}
