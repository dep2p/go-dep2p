// Package identity 实现身份管理
package identity

import "errors"

// ============================================================================
// 错误定义
// ============================================================================

var (
	// ErrNilPrivateKey 私钥为 nil
	ErrNilPrivateKey = errors.New("private key is nil")

	// ErrNilPublicKey 公钥为 nil
	ErrNilPublicKey = errors.New("public key is nil")

	// ErrKeyPairMismatch 密钥对不匹配
	ErrKeyPairMismatch = errors.New("key pair mismatch")

	// ErrFailedToGenerateKey 密钥生成失败
	ErrFailedToGenerateKey = errors.New("failed to generate key")

	// ErrFailedToDerivePeerID PeerID 派生失败
	ErrFailedToDerivePeerID = errors.New("failed to derive peer id")
)
