// Package tls 实现 TLS 1.3 安全传输
package tls

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// oidDep2pPublicKey 定义 DeP2P 公钥扩展的 OID
var oidDep2pPublicKey = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1}

// GenerateCert 生成自签名 TLS 证书
func GenerateCert(id pkgif.Identity) (*tls.Certificate, error) {
	if id == nil {
		return nil, fmt.Errorf("identity is nil")
	}

	// 获取原始私钥字节
	privKeyBytes, err := id.PrivateKey().Raw()
	if err != nil {
		return nil, fmt.Errorf("get private key bytes: %w", err)
	}

	// 获取原始公钥字节
	pubKeyBytes, err := id.PublicKey().Raw()
	if err != nil {
		return nil, fmt.Errorf("get public key bytes: %w", err)
	}

	// 转换为 ed25519 密钥（用于签名）
	privKey := ed25519.PrivateKey(privKeyBytes)
	pubKey := ed25519.PublicKey(pubKeyBytes)

	// 生成随机序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}

	// 创建证书模板
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: id.PeerID(), // CN = PeerID
		},
		NotBefore: time.Now().Add(-1 * time.Hour),       // 提前 1 小时（防时钟偏移）
		NotAfter:  time.Now().Add(365 * 24 * time.Hour), // 1 年有效期
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth, // TLS 服务器
			x509.ExtKeyUsageClientAuth, // TLS 客户端
		},
		BasicConstraintsValid: true,

		// ⭐ 核心：嵌入 Ed25519 公钥到证书扩展
		ExtraExtensions: []pkix.Extension{{
			Id:       oidDep2pPublicKey,
			Critical: false,
			Value:    pubKeyBytes, // 32 bytes
		}},
	}

	// 自签名证书
	certDER, err := x509.CreateCertificate(
		rand.Reader,
		template, // 证书模板
		template, // 父证书（自签名）
		pubKey,   // 公钥
		privKey,  // 私钥（签名）
	)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	// 返回 TLS 证书
	return &tls.Certificate{
		Certificate: [][]byte{certDER}, // DER 编码的证书
		PrivateKey:  privKey,           // ed25519.PrivateKey 实现 crypto.Signer
	}, nil
}

// ExtractPeerIDFromCert 从 TLS 证书中提取 PeerID
func ExtractPeerIDFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", fmt.Errorf("certificate is nil")
	}

	// 方法 1: 从 CommonName 获取 PeerID
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName, nil
	}

	// 方法 2: 从扩展中提取公钥，派生 PeerID
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidDep2pPublicKey) {
			// ext.Value 包含 32 字节的 Ed25519 公钥
			pubKeyBytes := ext.Value
			if len(pubKeyBytes) != 32 {
				return "", fmt.Errorf("invalid public key length: %d", len(pubKeyBytes))
			}

			// 从公钥派生 PeerID
			// 注意：这里需要使用与 identity 相同的派生方法
			peerID, err := derivePeerIDFromPublicKey(pubKeyBytes)
			if err != nil {
				return "", fmt.Errorf("derive peer id: %w", err)
			}

			return peerID, nil
		}
	}

	return "", fmt.Errorf("no peer id found in certificate")
}

// derivePeerIDFromPublicKey 从公钥字节派生 PeerID
//
// 使用标准 libp2p PeerID 派生算法：
//   1. 将公钥序列化为 protobuf 格式
//   2. SHA256 哈希
//   3. Base58 编码
//
// 参数：
//   - pubKeyBytes: Ed25519 公钥字节（32 字节）
//
// 返回：
//   - string: 派生的 PeerID
//   - error: 派生失败时的错误
func derivePeerIDFromPublicKey(pubKeyBytes []byte) (string, error) {
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid ed25519 public key length: %d, expected %d",
			len(pubKeyBytes), ed25519.PublicKeySize)
	}
	
	// 使用 pkg/crypto 创建 Ed25519 公钥并派生 PeerID
	cryptoPubKey, err := crypto.UnmarshalEd25519PublicKey(pubKeyBytes)
	if err != nil {
		return "", fmt.Errorf("unmarshal ed25519 public key: %w", err)
	}
	
	// 使用标准 PeerID 派生算法
	peerID, err := crypto.PeerIDFromPublicKey(cryptoPubKey)
	if err != nil {
		return "", fmt.Errorf("derive peer id: %w", err)
	}
	
	return string(peerID), nil
}
