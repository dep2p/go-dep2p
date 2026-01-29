// Package tls 实现 TLS 1.3 安全传输
package tls

import "errors"

// TLS 相关错误
var (
	// ErrNoCertificate 对端未提供证书
	ErrNoCertificate = errors.New("tls: no certificate provided")

	// ErrPeerIDMismatch 对端 PeerID 不匹配（INV-001）
	ErrPeerIDMismatch = errors.New("tls: peer ID mismatch (INV-001 violation)")

	// ErrNoPublicKeyExtension 证书缺少公钥扩展
	ErrNoPublicKeyExtension = errors.New("tls: no public key extension in certificate")

	// ErrInvalidPublicKey 公钥无效
	ErrInvalidPublicKey = errors.New("tls: invalid public key")
)
