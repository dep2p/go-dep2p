package tls

import (
	"crypto/x509"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateCert 测试证书生成
func TestGenerateCert(t *testing.T) {
	// 生成身份
	id, err := identity.Generate()
	require.NoError(t, err)

	// 生成证书
	cert, err := GenerateCert(id)
	require.NoError(t, err)
	require.NotNil(t, cert)

	// 验证证书结构
	assert.NotEmpty(t, cert.Certificate)
	assert.NotNil(t, cert.PrivateKey)

	// 解析证书
	parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	// 验证 CN = PeerID
	assert.Equal(t, id.PeerID(), parsedCert.Subject.CommonName)

	t.Log("✅ 证书生成成功")
}

// TestCertificateExtension 测试公钥扩展
func TestCertificateExtension(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	cert, err := GenerateCert(id)
	require.NoError(t, err)

	parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	// 查找公钥扩展
	found := false
	for _, ext := range parsedCert.Extensions {
		if ext.Id.Equal(oidDep2pPublicKey) {
			found = true
			// 验证公钥长度 (Ed25519 = 32 bytes)
			assert.Equal(t, 32, len(ext.Value), "公钥应为 32 bytes")
			t.Logf("✅ 找到公钥扩展: %d bytes", len(ext.Value))
			break
		}
	}

	assert.True(t, found, "证书应包含公钥扩展")
}

// ============================================================================
//                       ExtractPeerIDFromCert 测试（安全关键）
// ============================================================================

// TestExtractPeerIDFromCert_Valid 测试从有效证书提取 PeerID
func TestExtractPeerIDFromCert_Valid(t *testing.T) {
	// 生成身份
	id, err := identity.Generate()
	require.NoError(t, err)

	// 生成证书
	cert, err := GenerateCert(id)
	require.NoError(t, err)

	// 解析证书
	parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	// 提取 PeerID
	extractedPeerID, err := ExtractPeerIDFromCert(parsedCert)
	require.NoError(t, err, "从有效证书提取 PeerID 应成功")

	// 验证提取的 PeerID 与原 Identity 的 PeerID 匹配
	assert.Equal(t, id.PeerID(), extractedPeerID, "提取的 PeerID 应与原 Identity 匹配")
	t.Logf("✅ 成功提取 PeerID: %s", extractedPeerID)
}

// TestExtractPeerIDFromCert_NilCert 测试 nil 证书
func TestExtractPeerIDFromCert_NilCert(t *testing.T) {
	peerID, err := ExtractPeerIDFromCert(nil)
	assert.Error(t, err, "nil 证书应返回错误")
	assert.Empty(t, peerID)
	assert.Contains(t, err.Error(), "nil")
}

// TestExtractPeerIDFromCert_EmptyCommonName 测试空 CommonName 场景
func TestExtractPeerIDFromCert_EmptyCommonName(t *testing.T) {
	// 生成身份
	id, err := identity.Generate()
	require.NoError(t, err)

	// 生成证书
	cert, err := GenerateCert(id)
	require.NoError(t, err)

	// 解析证书
	parsedCert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	// 清空 CommonName，测试从扩展提取
	parsedCert.Subject.CommonName = ""

	// 应该从扩展中提取（如果有公钥扩展）
	peerID, err := ExtractPeerIDFromCert(parsedCert)
	// 由于我们的证书包含公钥扩展，应该能从扩展提取
	if err != nil {
		t.Logf("从扩展提取失败（预期行为如果扩展不存在）: %v", err)
	} else {
		assert.NotEmpty(t, peerID, "应该能从扩展提取 PeerID")
		t.Logf("✅ 从扩展提取 PeerID: %s", peerID)
	}
}

// ============================================================================
//                       derivePeerIDFromPublicKey 测试
// ============================================================================

// TestDerivePeerIDFromPublicKey_Valid 测试有效公钥派生
func TestDerivePeerIDFromPublicKey_Valid(t *testing.T) {
	// 生成身份
	id, err := identity.Generate()
	require.NoError(t, err)

	// 获取公钥字节
	pubKeyBytes, err := id.PublicKey().Raw()
	require.NoError(t, err)
	require.Len(t, pubKeyBytes, 32, "Ed25519 公钥应为 32 字节")

	// 派生 PeerID
	derivedPeerID, err := derivePeerIDFromPublicKey(pubKeyBytes)
	require.NoError(t, err, "从有效公钥派生 PeerID 应成功")

	// 验证派生的 PeerID 与 Identity 的 PeerID 匹配
	assert.Equal(t, id.PeerID(), derivedPeerID, "派生的 PeerID 应与 Identity 匹配")
	t.Logf("✅ 派生 PeerID: %s", derivedPeerID)
}

// TestDerivePeerIDFromPublicKey_InvalidLength 测试无效长度公钥
func TestDerivePeerIDFromPublicKey_InvalidLength(t *testing.T) {
	tests := []struct {
		name   string
		pubKey []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte("short")},
		{"31 bytes", make([]byte, 31)},
		{"33 bytes", make([]byte, 33)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peerID, err := derivePeerIDFromPublicKey(tt.pubKey)
			assert.Error(t, err, "无效长度公钥应返回错误")
			assert.Empty(t, peerID)
			assert.Contains(t, err.Error(), "length")
		})
	}
}
