// Package quic 提供基于 QUIC 的传输层实现
package quic

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
	"time"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// nodeIDExtensionOID 是用于在证书扩展中存储 NodeID 的 OID
// 使用自定义 OID: 1.3.6.1.4.1.53594.1.1 (DeP2P Node ID)
// 注意：此扩展仅作为向后兼容和调试用途，NodeID 验证以公钥派生为准
var nodeIDExtensionOID = []int{1, 3, 6, 1, 4, 1, 53594, 1, 1}

// TLSConfig TLS 配置生成器
type TLSConfig struct {
	identity identityif.Identity
}

// NewTLSConfig 创建 TLS 配置生成器
func NewTLSConfig(identity identityif.Identity) *TLSConfig {
	return &TLSConfig{identity: identity}
}

// GenerateConfig 生成用于 QUIC 的 TLS 配置
//
// 证书直接使用 identity 私钥签名，NodeID 可从证书公钥派生。
// 这与 security/tls 模块保持一致，确保身份不可伪造。
func (t *TLSConfig) GenerateConfig() (*tls.Config, error) {
	if t.identity == nil {
		return nil, fmt.Errorf("identity is nil")
	}

	// 获取节点 ID
	nodeID := t.identity.ID()
	privateKey := t.identity.PrivateKey()

	if privateKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}

	// 直接使用 identity 私钥（与 security/tls 一致）
	var certPrivKey crypto.Signer
	var pubKey crypto.PublicKey

	raw := privateKey.Raw()
	switch key := raw.(type) {
	case *rsa.PrivateKey:
		certPrivKey = key
		pubKey = &key.PublicKey
	case *ecdsa.PrivateKey:
		certPrivKey = key
		pubKey = &key.PublicKey
	case ed25519.PrivateKey:
		// Ed25519 直接用于 TLS 证书（Go 1.13+ 支持）
		certPrivKey = key
		pubKey = key.Public()
	default:
		return nil, fmt.Errorf("不支持的密钥类型: %T", raw)
	}

	// 创建自签名证书模板
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"DeP2P"},
			CommonName:   "DeP2P Node " + hex.EncodeToString(nodeID[:8]) + "...",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour * 24 * 180), // 180 天有效期
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

	// 创建 TLS 证书
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  certPrivKey,
	}

	// 创建 TLS 配置
	//
	// 安全说明：
	// - InsecureSkipVerify=true 禁用标准 CA 验证，但在 P2P 场景中这是正确的做法
	// - 因为我们使用自签名证书，没有 CA 可以验证
	// - 安全性由 VerifyPeerCertificate 回调保证：
	//   1. 从证书公钥派生 NodeID（不可伪造）
	//   2. 若证书带有 NodeID 扩展，验证扩展值等于派生值
	//   3. 验证证书有效期
	// - 这种方式与 security/tls 的设计一致
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"dep2p-quic"},
		// 禁用标准 CA 验证（P2P 使用自签名证书）
		// 安全性由 VerifyPeerCertificate 保证
		InsecureSkipVerify: true,
		// 要求客户端提供证书（双向 TLS）
		ClientAuth: tls.RequireAnyClientCert,
		// 自定义验证函数：从公钥派生 NodeID，验证扩展一致性
		VerifyPeerCertificate: verifyPeerCertificate,
		// 强制 TLS 1.3 以获得最佳安全性
		MinVersion: tls.VersionTLS13,
	}, nil
}

// GenerateClientConfig 生成客户端 TLS 配置
func (t *TLSConfig) GenerateClientConfig() (*tls.Config, error) {
	config, err := t.GenerateConfig()
	if err != nil {
		return nil, err
	}
	// 客户端不需要 ClientAuth
	config.ClientAuth = tls.NoClientCert
	return config, nil
}

// verifyPeerCertificate 验证对端证书
//
// 验证逻辑（与 security/tls 一致）：
//  1. 从证书公钥派生 NodeID（不可伪造）
//  2. 若证书带有 NodeID 扩展，验证扩展值必须等于派生值
//  3. 验证证书有效期
func verifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("对端未提供证书")
	}

	// 解析证书
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("解析证书失败: %w", err)
	}

	// 1. 从证书公钥派生 NodeID（不可伪造）
	derivedID, err := deriveNodeIDFromCertPublicKey(cert)
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

	// 3. 验证证书有效期
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("证书尚未生效: NotBefore=%v", cert.NotBefore)
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("证书已过期: NotAfter=%v", cert.NotAfter)
	}

	return nil
}

// ExtractNodeID 从 TLS 连接状态中提取 NodeID
//
// 总是从证书公钥派生 NodeID，确保身份不可伪造。
func ExtractNodeID(tlsState tls.ConnectionState) (types.NodeID, error) {
	if len(tlsState.PeerCertificates) == 0 {
		return types.EmptyNodeID, fmt.Errorf("对端未提供 TLS 证书")
	}

	// 从公钥派生（唯一可信来源）
	return deriveNodeIDFromCertPublicKey(tlsState.PeerCertificates[0])
}

// deriveNodeIDFromCertPublicKey 从证书公钥派生 NodeID
//
// 派生规则与 identity 模块保持一致：
//   - Ed25519: SHA256(原始 32 字节公钥)
//   - ECDSA: SHA256(elliptic.Marshal(curve, X, Y))
//   - RSA: SHA256(x509.MarshalPKCS1PublicKey)
func deriveNodeIDFromCertPublicKey(cert *x509.Certificate) (types.NodeID, error) {
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

// deriveNodeIDFromPublicKey 从公钥 DER 字节派生 NodeID
//
// 注意：此函数使用 PKIX 格式的 DER 字节。
// 推荐使用 deriveNodeIDFromCertPublicKey 以保证一致性。
func deriveNodeIDFromPublicKey(pubKeyDER []byte) (types.NodeID, error) {
	hash := sha256.Sum256(pubKeyDER)
	return types.NodeIDFromBytes(hash[:])
}

// VerifyNodeID 验证 TLS 连接中的 NodeID 是否与期望的匹配
func VerifyNodeID(tlsState tls.ConnectionState, expectedID types.NodeID) error {
	nodeID, err := ExtractNodeID(tlsState)
	if err != nil {
		return err
	}

	if !expectedID.IsEmpty() && !nodeID.Equal(expectedID) {
		return fmt.Errorf("NodeID 验证失败: 期望 %s, 实际 %s",
			expectedID.String(), nodeID.String())
	}

	return nil
}
