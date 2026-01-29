package tls

import (
	"crypto/tls"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTLSConfig_MinVersion 测试 TLS 1.3 强制
func TestTLSConfig_MinVersion(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	// 验证证书存在
	assert.NotNil(t, transport.cert)
	assert.NotEmpty(t, transport.cert.Certificate)
}

// TestTLSConfig_Certificates 测试证书配置
func TestTLSConfig_Certificates(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	cert, err := GenerateCert(id)
	require.NoError(t, err)

	// 验证证书可以用于 TLS 配置
	config := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		MinVersion:   tls.VersionTLS13,
	}

	assert.NotNil(t, config)
	assert.Equal(t, uint16(tls.VersionTLS13), config.MinVersion)
	assert.Len(t, config.Certificates, 1)
}

// TestTransport_New_NilIdentity 测试空身份错误
func TestTransport_New_NilIdentity(t *testing.T) {
	transport, err := New(nil)
	assert.Error(t, err)
	assert.Nil(t, transport)
	assert.Contains(t, err.Error(), "nil")
}

// ============================================================================
//                       Config 测试（安全关键）
// ============================================================================

// TestNewFromIdentity_Valid 测试从有效 Identity 创建配置
func TestNewFromIdentity_Valid(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	config, err := NewFromIdentity(id)
	require.NoError(t, err, "从有效 Identity 创建配置应成功")
	require.NotNil(t, config)

	// 验证配置字段
	assert.Equal(t, id, config.Identity)
	assert.NotNil(t, config.Certificate)
	assert.Equal(t, "dep2p", config.ServerName)
	assert.Contains(t, config.NextProtos, "dep2p")
	assert.Equal(t, uint16(tls.VersionTLS13), config.MinVersion)
	assert.True(t, config.RequireClientCert)

	t.Log("✅ NewFromIdentity 配置创建成功")
}

// TestNewFromIdentity_NilIdentity 测试 nil Identity
func TestNewFromIdentity_NilIdentity(t *testing.T) {
	config, err := NewFromIdentity(nil)
	assert.Error(t, err, "nil Identity 应返回错误")
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "nil")
}

// TestDefaultConfig 测试默认配置
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	require.NotNil(t, config)

	// 验证默认值
	assert.Nil(t, config.Identity, "默认配置不应有 Identity")
	assert.Nil(t, config.Certificate, "默认配置不应有证书")
	assert.Equal(t, "dep2p", config.ServerName)
	assert.Contains(t, config.NextProtos, "dep2p")
	assert.Equal(t, uint16(tls.VersionTLS13), config.MinVersion)
	assert.True(t, config.RequireClientCert)

	t.Log("✅ DefaultConfig 默认配置验证通过")
}

// TestConfig_WithIdentity 测试设置 Identity
func TestConfig_WithIdentity(t *testing.T) {
	config := DefaultConfig()
	require.NotNil(t, config)
	require.Nil(t, config.Identity)

	// 生成 Identity
	id, err := identity.Generate()
	require.NoError(t, err)

	// 设置 Identity
	err = config.WithIdentity(id)
	require.NoError(t, err, "设置有效 Identity 应成功")

	// 验证设置结果
	assert.Equal(t, id, config.Identity)
	assert.NotNil(t, config.Certificate)

	t.Log("✅ WithIdentity 设置成功")
}

// TestConfig_WithIdentity_Nil 测试设置 nil Identity
func TestConfig_WithIdentity_Nil(t *testing.T) {
	config := DefaultConfig()
	err := config.WithIdentity(nil)
	assert.Error(t, err, "设置 nil Identity 应返回错误")
	assert.Contains(t, err.Error(), "nil")
}

// TestConfig_ServerConfig 测试生成服务端配置
func TestConfig_ServerConfig(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	config, err := NewFromIdentity(id)
	require.NoError(t, err)

	// 生成服务端配置
	remotePeerID := types.PeerID("QmRemotePeer12345")
	tlsConfig := config.ServerConfig(remotePeerID)
	require.NotNil(t, tlsConfig)

	// 验证服务端配置
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.Equal(t, tls.RequireAnyClientCert, tlsConfig.ClientAuth)
	assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	assert.True(t, tlsConfig.InsecureSkipVerify) // P2P 使用自定义验证
	assert.NotNil(t, tlsConfig.VerifyPeerCertificate)
	assert.Contains(t, tlsConfig.NextProtos, "dep2p")

	t.Log("✅ ServerConfig 配置生成成功")
}

// TestConfig_ClientConfig 测试生成客户端配置
func TestConfig_ClientConfig(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	config, err := NewFromIdentity(id)
	require.NoError(t, err)

	// 生成客户端配置
	remotePeerID := types.PeerID("QmRemotePeer12345")
	tlsConfig := config.ClientConfig(remotePeerID)
	require.NotNil(t, tlsConfig)

	// 验证客户端配置
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	assert.True(t, tlsConfig.InsecureSkipVerify) // P2P 使用自定义验证
	assert.Equal(t, "dep2p", tlsConfig.ServerName)
	assert.NotNil(t, tlsConfig.VerifyPeerCertificate)
	assert.Contains(t, tlsConfig.NextProtos, "dep2p")

	t.Log("✅ ClientConfig 配置生成成功")
}

// TestConfig_Clone 测试配置克隆
func TestConfig_Clone(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	original, err := NewFromIdentity(id)
	require.NoError(t, err)

	// 克隆配置
	cloned := original.Clone()
	require.NotNil(t, cloned)

	// 验证克隆内容相同
	assert.Equal(t, original.Identity, cloned.Identity)
	assert.Equal(t, original.Certificate, cloned.Certificate)
	assert.Equal(t, original.ServerName, cloned.ServerName)
	assert.Equal(t, original.MinVersion, cloned.MinVersion)
	assert.Equal(t, original.RequireClientCert, cloned.RequireClientCert)
	assert.Equal(t, original.NextProtos, cloned.NextProtos)

	// 验证是独立副本（修改克隆不影响原始）
	cloned.ServerName = "modified"
	cloned.NextProtos = append(cloned.NextProtos, "extra")
	assert.NotEqual(t, original.ServerName, cloned.ServerName)
	assert.NotEqual(t, len(original.NextProtos), len(cloned.NextProtos))

	t.Log("✅ Clone 配置克隆成功")
}
