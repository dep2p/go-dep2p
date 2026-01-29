package tls

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// extractPublicKey 测试
// ============================================================================

// TestExtractPublicKey 测试公钥提取
func TestExtractPublicKey(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	// 生成证书
	cert, err := GenerateCert(id)
	require.NoError(t, err)

	// 解析证书
	rawCerts := cert.Certificate

	// 提取公钥
	pubKey, err := extractPublicKey(rawCerts)
	require.NoError(t, err)
	require.NotNil(t, pubKey)

	// 验证公钥
	expectedPubKey := id.PublicKey()
	assert.True(t, pubKey.Equals(expectedPubKey), "提取的公钥应匹配")

	t.Log("✅ 公钥提取成功")
}

// TestExtractPublicKey_EmptyCerts 测试空证书
func TestExtractPublicKey_EmptyCerts(t *testing.T) {
	_, err := extractPublicKey([][]byte{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoCertificate)
}

// TestExtractPublicKey_NilCerts 测试 nil 证书
func TestExtractPublicKey_NilCerts(t *testing.T) {
	_, err := extractPublicKey(nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoCertificate)
}

// TestExtractPublicKey_InvalidCertData 测试无效证书数据
func TestExtractPublicKey_InvalidCertData(t *testing.T) {
	_, err := extractPublicKey([][]byte{[]byte("not a valid cert")})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse certificate")
}

// TestExtractPublicKey_NoPublicKeyExtension 测试没有公钥扩展的证书
func TestExtractPublicKey_NoPublicKeyExtension(t *testing.T) {
	// 创建一个没有公钥扩展的证书
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),
	}

	// 使用 crypto/ed25519 生成密钥对
	_, edPrivKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// 创建证书（没有 dep2p 公钥扩展）
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, edPrivKey.Public(), edPrivKey)
	require.NoError(t, err)

	// 尝试提取公钥
	_, err = extractPublicKey([][]byte{certDER})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoPublicKeyExtension)
}

// ============================================================================
// VerifyPeerCertificate 测试
// ============================================================================

// TestVerifyPeerCertificate_Match 测试 PeerID 匹配验证
func TestVerifyPeerCertificate_Match(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	cert, err := GenerateCert(id)
	require.NoError(t, err)

	expectedPeer := types.PeerID(id.PeerID())

	// 验证应通过
	err = VerifyPeerCertificate(cert.Certificate, expectedPeer)
	assert.NoError(t, err, "匹配的 PeerID 应通过验证")

	t.Log("✅ PeerID 匹配验证通过")
}

// TestVerifyPeerCertificate_Mismatch 测试 PeerID 不匹配
func TestVerifyPeerCertificate_Mismatch(t *testing.T) {
	id1, err := identity.Generate()
	require.NoError(t, err)

	id2, err := identity.Generate()
	require.NoError(t, err)

	// 使用 id1 的证书
	cert, err := GenerateCert(id1)
	require.NoError(t, err)

	// 期待 id2 的 PeerID
	wrongPeer := types.PeerID(id2.PeerID())

	// 验证应失败
	err = VerifyPeerCertificate(cert.Certificate, wrongPeer)
	assert.Error(t, err, "不匹配的 PeerID 应被拒绝")
	assert.ErrorIs(t, err, ErrPeerIDMismatch)

	t.Log("✅ PeerID 不匹配被正确拒绝")
}

// TestVerifyPeerCertificate_EmptyExpectedPeer 测试空期望 PeerID
func TestVerifyPeerCertificate_EmptyExpectedPeer(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	cert, err := GenerateCert(id)
	require.NoError(t, err)

	// 空 PeerID 应该被允许（入站握手时可能不知道对端）
	err = VerifyPeerCertificate(cert.Certificate, "")
	assert.NoError(t, err, "空 PeerID 应通过验证")
}

// TestVerifyPeerCertificate_EmptyCerts 测试空证书
func TestVerifyPeerCertificate_EmptyCerts(t *testing.T) {
	err := VerifyPeerCertificate([][]byte{}, "some-peer")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoCertificate)
}

// TestVerifyPeerCertificate_NilCerts 测试 nil 证书
func TestVerifyPeerCertificate_NilCerts(t *testing.T) {
	err := VerifyPeerCertificate(nil, "some-peer")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoCertificate)
}

// ============================================================================
// extractRemotePublicKeyFromConn 测试
// ============================================================================

// TestExtractRemotePublicKeyFromConn 测试从连接状态提取公钥
func TestExtractRemotePublicKeyFromConn(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	cert, err := GenerateCert(id)
	require.NoError(t, err)

	pubKey, err := extractRemotePublicKeyFromConn(cert.Certificate)
	require.NoError(t, err)
	require.NotNil(t, pubKey)

	// 验证公钥匹配
	expectedPubKey := id.PublicKey()
	assert.True(t, pubKey.Equals(expectedPubKey))
}

// TestExtractRemotePublicKeyFromConn_Empty 测试空证书
func TestExtractRemotePublicKeyFromConn_Empty(t *testing.T) {
	_, err := extractRemotePublicKeyFromConn([][]byte{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoCertificate)
}
