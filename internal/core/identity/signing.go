// Package identity 实现身份管理
//
// 本文件提供签名和验证的辅助函数，作为 pkg/interfaces 密钥接口的简单包装。
package identity

import (
	"errors"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 错误定义
// ============================================================================

var (
	// ErrSigningFailed 签名失败
	ErrSigningFailed = errors.New("signing failed")
	// ErrInvalidSignature 无效的签名
	ErrInvalidSignature = errors.New("invalid signature")
)

// ============================================================================
// 签名操作（简单包装）
// ============================================================================

// Sign 使用私钥签名数据
//
// 这是 pkgif.PrivateKey.Sign() 的简单包装，添加了 nil 检查和验证。
func Sign(privKey pkgif.PrivateKey, data []byte) ([]byte, error) {
	if privKey == nil {
		return nil, errors.New("nil private key")
	}

	sig, err := privKey.Sign(data)
	if err != nil {
		return nil, err
	}

	if len(sig) == 0 {
		return nil, ErrSigningFailed
	}

	return sig, nil
}

// Verify 使用公钥验证签名
//
// 这是 pkgif.PublicKey.Verify() 的简单包装，添加了 nil 检查和验证。
func Verify(pubKey pkgif.PublicKey, data, sig []byte) (bool, error) {
	if pubKey == nil {
		return false, errors.New("nil public key")
	}

	if len(sig) == 0 {
		return false, ErrInvalidSignature
	}

	return pubKey.Verify(data, sig)
}
