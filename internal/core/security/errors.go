// Package security 实现安全传输
package security

import "errors"

var (
	// ErrNoCertificate 对端未提供证书
	ErrNoCertificate = errors.New("security: no certificate provided")

	// ErrPeerIDMismatch 对端 PeerID 不匹配（INV-001）
	ErrPeerIDMismatch = errors.New("security: peer ID mismatch (INV-001 violation)")

	// ErrNoPublicKeyExtension 证书缺少公钥扩展
	ErrNoPublicKeyExtension = errors.New("security: no public key extension in certificate")

	// ErrInvalidPublicKey 公钥无效
	ErrInvalidPublicKey = errors.New("security: invalid public key")
)
