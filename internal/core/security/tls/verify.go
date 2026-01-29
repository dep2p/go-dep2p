// Package tls 实现 TLS 1.3 安全传输
package tls

import (
	"crypto/x509"
	"fmt"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// VerifyPeerCertificate 验证对端证书（实现 INV-001）
func VerifyPeerCertificate(rawCerts [][]byte, expectedPeer types.PeerID) error {
	if len(rawCerts) == 0 {
		return ErrNoCertificate
	}

	// 提取公钥扩展
	pubKey, err := extractPublicKey(rawCerts)
	if err != nil {
		return err
	}

	// 派生 PeerID
	actualPeer, err := identity.PeerIDFromPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("derive peer ID: %w", err)
	}

	// 验证匹配 ⭐ INV-001
	// 允许入站握手在未知对端时通过验证（expectedPeer 为空）
	if expectedPeer != "" && actualPeer != string(expectedPeer) {
		return fmt.Errorf("%w: expected %s, got %s",
			ErrPeerIDMismatch,
			expectedPeer,
			actualPeer,
		)
	}

	return nil
}

// extractPublicKey 从证书扩展提取 Ed25519 公钥
func extractPublicKey(rawCerts [][]byte) (pkgif.PublicKey, error) {
	if len(rawCerts) == 0 {
		return nil, ErrNoCertificate
	}

	// 解析证书
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	// 查找公钥扩展
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidDep2pPublicKey) {
			// 验证公钥长度（Ed25519 = 32 bytes）
			if len(ext.Value) != 32 {
				return nil, fmt.Errorf("%w: expected 32 bytes, got %d",
					ErrInvalidPublicKey, len(ext.Value))
			}

			// 从字节创建公钥
			pubKey, err := identity.PublicKeyFromBytes(ext.Value, pkgif.KeyTypeEd25519)
			if err != nil {
				return nil, fmt.Errorf("unmarshal public key: %w", err)
			}

			return pubKey, nil
		}
	}

	// 未找到公钥扩展
	return nil, ErrNoPublicKeyExtension
}

// extractRemotePublicKeyFromConn 从 TLS 连接状态提取远程公钥（辅助函数）
func extractRemotePublicKeyFromConn(rawCerts [][]byte) (pkgif.PublicKey, error) {
	return extractPublicKey(rawCerts)
}
