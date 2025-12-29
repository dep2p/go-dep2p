package tls

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func TestNewCertificateManager(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)
	assert.NotNil(t, certMgr)
}

func TestGenerateCertificate(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)

	// 测试生成证书
	cert, err := certMgr.GenerateCertificate(ident.ID(), ident.PrivateKey())
	require.NoError(t, err)
	require.NotNil(t, cert)

	// 验证证书
	assert.NotEmpty(t, cert.Certificate)
	assert.NotNil(t, cert.PrivateKey)
	assert.NotNil(t, cert.Leaf)

	// 验证 NodeID 扩展
	foundNodeIDExt := false
	for _, ext := range cert.Leaf.Extensions {
		if ext.Id.Equal(nodeIDExtensionOID) {
			foundNodeIDExt = true
			assert.Equal(t, ident.ID().Bytes(), ext.Value)
			break
		}
	}
	assert.True(t, foundNodeIDExt, "证书中应包含 NodeID 扩展")
}

func TestGenerateCertificateFromIdentity(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)

	// 测试从 Identity 生成证书
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)
	require.NotNil(t, cert)

	// 验证证书
	assert.NotEmpty(t, cert.Certificate)
	assert.NotNil(t, cert.PrivateKey)
}

func TestGenerateCertificateFromIdentityNil(t *testing.T) {
	certMgr := NewCertificateManager(nil)

	// 没有 Identity 应该失败
	_, err := certMgr.GenerateCertificateFromIdentity()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "identity 未设置")
}

func TestExtractNodeID(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)

	// 生成证书
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)

	// 提取 NodeID（现在从公钥派生）
	nodeID, err := certMgr.ExtractNodeID(cert.Certificate[0])
	require.NoError(t, err)
	assert.Equal(t, ident.ID(), nodeID)
}

func TestExtractNodeIDFromCert(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)

	// 生成证书
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)

	// 从 x509.Certificate 提取 NodeID
	nodeID, err := ExtractNodeIDFromCert(cert.Leaf)
	require.NoError(t, err)
	assert.Equal(t, ident.ID(), nodeID)
}

func TestVerifyPeerCertificate(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)

	// 生成证书
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)

	// 验证证书 - 期望匹配的 NodeID
	err = certMgr.VerifyPeerCertificate(cert.Certificate, ident.ID())
	assert.NoError(t, err)

	// 验证证书 - 空 NodeID（不验证 NodeID）
	err = certMgr.VerifyPeerCertificate(cert.Certificate, types.EmptyNodeID)
	assert.NoError(t, err)

	// 验证证书 - 不匹配的 NodeID
	otherIdent, err := mgr.Create()
	require.NoError(t, err)
	err = certMgr.VerifyPeerCertificate(cert.Certificate, otherIdent.ID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NodeID 不匹配")

	// 验证空证书列表
	err = certMgr.VerifyPeerCertificate([][]byte{}, ident.ID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "对端未提供证书")
}

func TestDeriveNodeIDFromPublicKey(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)

	// 生成证书
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)

	// 序列化公钥
	pubKeyDER, err := x509.MarshalPKIXPublicKey(cert.Leaf.PublicKey)
	require.NoError(t, err)

	// 派生 NodeID
	nodeID, err := DeriveNodeIDFromPublicKey(pubKeyDER)
	require.NoError(t, err)
	assert.False(t, nodeID.IsEmpty())

	// 派生结果应该一致
	nodeID2, err := DeriveNodeIDFromPublicKey(pubKeyDER)
	require.NoError(t, err)
	assert.Equal(t, nodeID, nodeID2)
}

func TestGetNodeIDExtensionOID(t *testing.T) {
	oid := GetNodeIDExtensionOID()
	assert.Equal(t, []int{1, 3, 6, 1, 4, 1, 53594, 1, 1}, oid)
}

// TestGenerateCertificateWithECDSA 测试使用 ECDSA 密钥直接生成证书
func TestGenerateCertificateWithECDSA(t *testing.T) {
	// 直接生成 ECDSA 密钥对
	ecdsaPriv, ecdsaPub, err := identity.GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	// 创建一个模拟的 identity
	ident := identity.NewIdentity(ecdsaPriv)
	require.NotNil(t, ident)

	certMgr := NewCertificateManager(ident)

	// 生成证书
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)
	require.NotNil(t, cert)

	// 验证证书使用的是 ECDSA 密钥
	assert.NotEmpty(t, cert.Certificate)
	assert.NotNil(t, cert.PrivateKey)
	assert.NotNil(t, cert.Leaf)

	// 验证公钥算法是 ECDSA
	assert.Equal(t, x509.ECDSA, cert.Leaf.PublicKeyAlgorithm)

	// 验证 NodeID 从公钥派生
	nodeID, err := ExtractNodeIDFromCert(cert.Leaf)
	require.NoError(t, err)
	assert.Equal(t, ident.ID(), nodeID)

	// 验证 ECDSA 公钥类型
	assert.Equal(t, types.KeyTypeECDSAP256, ecdsaPub.Type())
}

// TestGenerateCertificateWithEd25519 测试使用 Ed25519 密钥生成证书
func TestGenerateCertificateWithEd25519(t *testing.T) {
	// 使用默认 Ed25519 配置创建身份
	cfg := identityif.DefaultConfig()
	cfg.KeyType = types.KeyTypeEd25519
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)

	// 生成证书 - 现在 Ed25519 直接用于 TLS 证书（Go 1.13+）
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)
	require.NotNil(t, cert)

	// 验证证书
	assert.NotEmpty(t, cert.Certificate)
	assert.NotNil(t, cert.PrivateKey)
	assert.NotNil(t, cert.Leaf)

	// Ed25519 现在直接用于证书
	assert.Equal(t, x509.Ed25519, cert.Leaf.PublicKeyAlgorithm)

	// 验证 NodeID 从公钥派生
	nodeID, err := ExtractNodeIDFromCert(cert.Leaf)
	require.NoError(t, err)
	assert.Equal(t, ident.ID(), nodeID)
}

// ============================================================================
//                              身份绑定强化测试
// ============================================================================

// TestVerifyPeerCertificate_ForgedExtension 测试伪造扩展被拒绝
func TestVerifyPeerCertificate_ForgedExtension(t *testing.T) {
	// 创建两个不同的 identity
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)

	ident1, err := mgr.Create()
	require.NoError(t, err)

	ident2, err := mgr.Create()
	require.NoError(t, err)

	// 使用 ident1 的私钥生成证书，但嵌入 ident2 的 NodeID 扩展
	// 这模拟攻击者尝试伪造身份
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"DeP2P"},
			CommonName:   "Forged Node",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour * 24 * 365),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		// 嵌入伪造的 NodeID（ident2 的 ID，但用 ident1 的密钥签名）
		ExtraExtensions: []pkix.Extension{
			{
				Id:       nodeIDExtensionOID,
				Critical: false,
				Value:    ident2.ID().Bytes(), // 伪造的 NodeID
			},
		},
	}

	// 使用 ident1 的私钥签名证书
	privKey := ident1.PrivateKey().Raw().(ed25519.PrivateKey)
	pubKey := privKey.Public()

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pubKey, privKey)
	require.NoError(t, err)

	// 验证时应该失败，因为扩展值与公钥派生值不一致
	certMgr := NewCertificateManager(nil)
	err = certMgr.VerifyPeerCertificate([][]byte{certDER}, ident2.ID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NodeID 扩展与公钥派生不一致")
}

// TestVerifyPeerCertificate_SelfSignedIntegrity 测试自签名完整性检查
func TestVerifyPeerCertificate_SelfSignedIntegrity(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)

	// 生成有效证书
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)

	// 篡改证书内容（修改 DER 字节）
	tamperedCert := make([]byte, len(cert.Certificate[0]))
	copy(tamperedCert, cert.Certificate[0])
	// 修改中间某个字节
	if len(tamperedCert) > 100 {
		tamperedCert[100] ^= 0xFF
	}

	// 验证应该失败
	err = certMgr.VerifyPeerCertificate([][]byte{tamperedCert}, types.EmptyNodeID)
	assert.Error(t, err)
	// 错误可能是解析失败或签名验证失败
}

// TestDeriveNodeIDFromCertPublicKey_Ed25519 测试 Ed25519 公钥派生
func TestDeriveNodeIDFromCertPublicKey_Ed25519(t *testing.T) {
	cfg := identityif.DefaultConfig()
	cfg.KeyType = types.KeyTypeEd25519
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	certMgr := NewCertificateManager(ident)
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)

	// 从证书公钥派生 NodeID
	derivedID, err := DeriveNodeIDFromCertPublicKey(cert.Leaf)
	require.NoError(t, err)

	// 应该与 identity 的 NodeID 相等
	assert.Equal(t, ident.ID(), derivedID)
}

// TestDeriveNodeIDFromCertPublicKey_ECDSA 测试 ECDSA 公钥派生
func TestDeriveNodeIDFromCertPublicKey_ECDSA(t *testing.T) {
	ecdsaPriv, _, err := identity.GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	ident := identity.NewIdentity(ecdsaPriv)
	require.NotNil(t, ident)

	certMgr := NewCertificateManager(ident)
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)

	// 从证书公钥派生 NodeID
	derivedID, err := DeriveNodeIDFromCertPublicKey(cert.Leaf)
	require.NoError(t, err)

	// 应该与 identity 的 NodeID 相等
	assert.Equal(t, ident.ID(), derivedID)
}

// TestGenerateCertificate_NilPrivateKey 测试 nil 私钥
func TestGenerateCertificate_NilPrivateKey(t *testing.T) {
	certMgr := NewCertificateManager(nil)

	_, err := certMgr.GenerateCertificate(types.EmptyNodeID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "私钥不能为空")
}
