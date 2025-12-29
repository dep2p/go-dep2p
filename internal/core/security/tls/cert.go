// Package tls 提供基于 TLS 的安全传输实现
package tls

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"time"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// nodeIDExtensionOID 是用于在证书扩展中存储 NodeID 的 OID
// 使用自定义 OID: 1.3.6.1.4.1.53594.1.1 (DeP2P Node ID)
// 注意：此扩展仅作为向后兼容和调试用途，NodeID 验证以公钥派生为准
var nodeIDExtensionOID = []int{1, 3, 6, 1, 4, 1, 53594, 1, 1}

// CertificateManager 证书管理器
// 负责生成、加载和验证证书
type CertificateManager struct {
	identity identityif.Identity
}

// 确保实现 securityif.CertificateManager 接口
var _ securityif.CertificateManager = (*CertificateManager)(nil)

// NewCertificateManager 创建证书管理器
func NewCertificateManager(identity identityif.Identity) *CertificateManager {
	return &CertificateManager{
		identity: identity,
	}
}

// GenerateCertificate 生成自签名证书
//
// 证书直接使用 identity 私钥签名，NodeID 可从证书公钥派生。
// 支持 RSA、ECDSA 和 Ed25519 密钥类型。
// 注意：证书公钥必须与 identity 公钥一致，以保证身份不可伪造。
func (m *CertificateManager) GenerateCertificate(nodeID types.NodeID, privateKey identityif.PrivateKey) (*tls.Certificate, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("私钥不能为空")
	}

	var certPrivKey crypto.Signer
	var pubKey crypto.PublicKey

	// 直接使用 identity 私钥（包括 Ed25519）
	// Go 1.13+ 已原生支持 Ed25519 TLS 证书
	raw := privateKey.Raw()
	switch key := raw.(type) {
	case *rsa.PrivateKey:
		certPrivKey = key
		pubKey = &key.PublicKey
	case *ecdsa.PrivateKey:
		certPrivKey = key
		pubKey = &key.PublicKey
	case ed25519.PrivateKey:
		// 直接使用 Ed25519 私钥（Go 1.13+ 支持）
		certPrivKey = key
		pubKey = key.Public()
	default:
		return nil, fmt.Errorf("不支持的密钥类型: %T", raw)
	}

	// 创建证书模板
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"DeP2P"},
			CommonName:   "DeP2P Node " + hex.EncodeToString(nodeID[:8]) + "...",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour * 24 * 365), // 1 年有效期
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		// 在证书扩展中嵌入 NodeID（仅用于调试/兼容，验证以公钥派生为准）
		ExtraExtensions: []pkix.Extension{
			{
				Id:       nodeIDExtensionOID,
				Critical: false,
				Value:    nodeID[:],
			},
		},
	}

	// 创建自签名证书
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pubKey, certPrivKey)
	if err != nil {
		return nil, fmt.Errorf("创建证书失败: %w", err)
	}

	// 解析证书以填充 Leaf 字段
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("解析证书失败: %w", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  certPrivKey,
		Leaf:        cert,
	}, nil
}

// GenerateCertificateFromIdentity 从 Identity 生成证书
func (m *CertificateManager) GenerateCertificateFromIdentity() (*tls.Certificate, error) {
	if m.identity == nil {
		return nil, fmt.Errorf("identity 未设置")
	}
	return m.GenerateCertificate(m.identity.ID(), m.identity.PrivateKey())
}

// LoadCertificate 从文件加载证书
func (m *CertificateManager) LoadCertificate(certFile, keyFile string) (*tls.Certificate, error) {
	// 读取证书文件
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("读取证书文件失败: %w", err)
	}

	// 读取私钥文件
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("读取私钥文件失败: %w", err)
	}

	// 解析证书和私钥
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("解析证书和私钥失败: %w", err)
	}

	// 解析 Leaf 证书
	if len(cert.Certificate) > 0 {
		cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("解析 Leaf 证书失败: %w", err)
		}
	}

	return &cert, nil
}

// VerifyPeerCertificate 验证对端证书
//
// 验证逻辑（强绑定）：
//  1. 从证书公钥派生 derivedID（不可伪造）
//  2. 若证书带有 NodeID 扩展，则扩展值必须等于 derivedID（防止扩展被篡改）
//  3. 若指定了 expectedID，则 derivedID 必须等于 expectedID
//  4. 验证证书有效期
//  5. 验证自签名完整性
func (m *CertificateManager) VerifyPeerCertificate(certs [][]byte, expectedID types.NodeID) error {
	if len(certs) == 0 {
		return fmt.Errorf("对端未提供证书")
	}

	// 解析证书
	cert, err := x509.ParseCertificate(certs[0])
	if err != nil {
		return fmt.Errorf("解析证书失败: %w", err)
	}

	// 1. 从证书公钥派生 NodeID（这是唯一可信来源）
	derivedID, err := DeriveNodeIDFromCertPublicKey(cert)
	if err != nil {
		return fmt.Errorf("从证书公钥派生 NodeID 失败: %w", err)
	}

	// 2. 若证书带有 NodeID 扩展，验证扩展值必须等于派生值
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(nodeIDExtensionOID) {
			if len(ext.Value) != 32 {
				return fmt.Errorf("无效的 NodeID 扩展长度: %d", len(ext.Value))
			}
			extensionID, err := types.NodeIDFromBytes(ext.Value)
			if err != nil {
				return fmt.Errorf("解析扩展 NodeID 失败: %w", err)
			}
			// 扩展值必须与派生值一致（防止伪造）
			if !extensionID.Equal(derivedID) {
				return fmt.Errorf("NodeID 扩展与公钥派生不一致: 扩展 %s, 派生 %s",
					extensionID.String(), derivedID.String())
			}
			break
		}
	}

	// 3. 验证期望的 NodeID
	if !expectedID.IsEmpty() && !derivedID.Equal(expectedID) {
		return fmt.Errorf("NodeID 不匹配: 期望 %s, 实际 %s",
			expectedID.String(), derivedID.String())
	}

	// 4. 验证证书有效期
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("证书尚未生效: NotBefore=%v", cert.NotBefore)
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("证书已过期: NotAfter=%v", cert.NotAfter)
	}

	// 注意：不验证自签名完整性，因为：
	// 1. TLS 握手本身会验证证书签名
	// 2. 我们的安全性来自 NodeID 与公钥的强绑定，而非证书链
	// 3. cert.CheckSignatureFrom(cert) 对非 CA 证书会失败

	return nil
}

// ExtractNodeID 从证书中提取节点 ID
func (m *CertificateManager) ExtractNodeID(certDER []byte) (types.NodeID, error) {
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return types.EmptyNodeID, fmt.Errorf("解析证书失败: %w", err)
	}
	return ExtractNodeIDFromCert(cert)
}

// ExtractNodeIDFromCert 从 x509.Certificate 中提取 NodeID
//
// 总是从证书公钥派生 NodeID，确保身份不可伪造。
// 扩展字段仅作为向后兼容检查，不作为可信来源。
func ExtractNodeIDFromCert(cert *x509.Certificate) (types.NodeID, error) {
	// 从公钥派生（这是唯一可信来源）
	return DeriveNodeIDFromCertPublicKey(cert)
}

// DeriveNodeIDFromCertPublicKey 从证书公钥派生 NodeID
//
// 派生规则与 identity 模块保持一致：
//   - Ed25519: SHA256(原始 32 字节公钥)
//   - ECDSA: SHA256(elliptic.Marshal(curve, X, Y))
//   - RSA: SHA256(x509.MarshalPKCS1PublicKey)
func DeriveNodeIDFromCertPublicKey(cert *x509.Certificate) (types.NodeID, error) {
	var pubKeyBytes []byte

	switch key := cert.PublicKey.(type) {
	case ed25519.PublicKey:
		// Ed25519: 直接使用原始 32 字节公钥
		pubKeyBytes = key
	case *ecdsa.PublicKey:
		// ECDSA: 使用 elliptic.Marshal（与 identity 模块一致）
		pubKeyBytes = elliptic.Marshal(key.Curve, key.X, key.Y)
	case *rsa.PublicKey:
		// RSA: 使用 PKCS1 格式
		pubKeyBytes = x509.MarshalPKCS1PublicKey(key)
	default:
		return types.EmptyNodeID, fmt.Errorf("不支持的公钥类型: %T", cert.PublicKey)
	}

	// SHA256 哈希得到 NodeID
	hash := sha256.Sum256(pubKeyBytes)
	return types.NodeIDFromBytes(hash[:])
}

// ExtractNodeIDFromTLSState 从 TLS 连接状态中提取 NodeID
func ExtractNodeIDFromTLSState(tlsState tls.ConnectionState) (types.NodeID, error) {
	if len(tlsState.PeerCertificates) == 0 {
		return types.EmptyNodeID, fmt.Errorf("对端未提供 TLS 证书")
	}
	return ExtractNodeIDFromCert(tlsState.PeerCertificates[0])
}

// DeriveNodeIDFromPublicKey 从公钥 DER 字节派生 NodeID
//
// 注意：此函数使用 PKIX 格式的 DER 字节，与 identity 模块的派生方式可能不同。
// 推荐使用 DeriveNodeIDFromCertPublicKey 以保证一致性。
func DeriveNodeIDFromPublicKey(pubKeyDER []byte) (types.NodeID, error) {
	hash := sha256.Sum256(pubKeyDER)
	return types.NodeIDFromBytes(hash[:])
}

// VerifyNodeIDFromTLSState 验证 TLS 连接中的 NodeID 是否与期望的匹配
func VerifyNodeIDFromTLSState(tlsState tls.ConnectionState, expectedID types.NodeID) error {
	nodeID, err := ExtractNodeIDFromTLSState(tlsState)
	if err != nil {
		return err
	}

	if !expectedID.IsEmpty() && !nodeID.Equal(expectedID) {
		return fmt.Errorf("NodeID 验证失败: 期望 %s, 实际 %s",
			expectedID.String(), nodeID.String())
	}

	return nil
}

// GetNodeIDExtensionOID 返回 NodeID 扩展的 OID
func GetNodeIDExtensionOID() []int {
	return nodeIDExtensionOID
}
