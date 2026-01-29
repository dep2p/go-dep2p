// Package crypto 提供 DeP2P 密码学工具
package crypto

import "errors"

// ============================================================================
//                              错误定义
// ============================================================================

// 密钥相关错误
var (
	// ErrBadKeyType 不支持的密钥类型
	ErrBadKeyType = errors.New("invalid or unsupported key type")

	// ErrNilPrivateKey 私钥为空
	ErrNilPrivateKey = errors.New("nil private key")

	// ErrNilPublicKey 公钥为空
	ErrNilPublicKey = errors.New("nil public key")

	// ErrInvalidKeySize 密钥大小无效
	ErrInvalidKeySize = errors.New("invalid key size")

	// ErrInvalidPublicKey 公钥无效
	ErrInvalidPublicKey = errors.New("invalid public key")

	// ErrInvalidPrivateKey 私钥无效
	ErrInvalidPrivateKey = errors.New("invalid private key")
)

// 签名相关错误
var (
	// ErrNilSignature 签名为空
	ErrNilSignature = errors.New("nil signature")

	// ErrInvalidSignature 签名无效
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrSignatureTypeMismatch 签名类型不匹配
	ErrSignatureTypeMismatch = errors.New("signature type mismatch")

	// ErrSignatureTooShort 签名太短
	ErrSignatureTooShort = errors.New("signature too short")
)

// 序列化相关错误
var (
	// ErrMarshalFailed 序列化失败
	ErrMarshalFailed = errors.New("marshal failed")

	// ErrUnmarshalFailed 反序列化失败
	ErrUnmarshalFailed = errors.New("unmarshal failed")
)

// 密钥存储相关错误
var (
	// ErrKeyNotFound 密钥未找到
	ErrKeyNotFound = errors.New("key not found")

	// ErrKeyExists 密钥已存在
	ErrKeyExists = errors.New("key already exists")

	// ErrInvalidPassword 密码无效
	ErrInvalidPassword = errors.New("invalid password")

	// ErrDecryptionFailed 解密失败
	ErrDecryptionFailed = errors.New("decryption failed")

	// ErrEncryptionFailed 加密失败
	ErrEncryptionFailed = errors.New("encryption failed")

	// ErrInvalidKeyFile 密钥文件格式无效
	ErrInvalidKeyFile = errors.New("invalid key file format")
)
