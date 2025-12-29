package tls

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func TestNewConfigBuilder(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	builder := NewConfigBuilder(ident)
	assert.NotNil(t, builder)
	assert.NotNil(t, builder.certManager)
}

func TestConfigBuilderWithOptions(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	// 生成证书
	certMgr := NewCertificateManager(ident)
	cert, err := certMgr.GenerateCertificateFromIdentity()
	require.NoError(t, err)

	builder := NewConfigBuilder(ident).
		WithCertificate(cert).
		WithMinVersion(tls.VersionTLS12).
		WithRequireClientAuth(false).
		WithInsecureSkipVerify(true).
		WithNextProtos([]string{"test/1.0"})

	assert.NotNil(t, builder)
	assert.Equal(t, uint16(tls.VersionTLS12), builder.minVersion)
	assert.False(t, builder.requireClientAuth)
	assert.True(t, builder.insecureSkipVerify)
	assert.Equal(t, []string{"test/1.0"}, builder.nextProtos)
}

func TestBuildServerConfig(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	builder := NewConfigBuilder(ident)

	// 构建服务端配置
	serverConfig, err := builder.BuildServerConfig()
	require.NoError(t, err)
	require.NotNil(t, serverConfig)

	// 验证配置
	assert.Equal(t, uint16(tls.VersionTLS13), serverConfig.MinVersion)
	assert.NotEmpty(t, serverConfig.Certificates)
	assert.True(t, serverConfig.InsecureSkipVerify) // P2P 场景使用自签名证书
	assert.Equal(t, tls.RequireAnyClientCert, serverConfig.ClientAuth)
	assert.NotNil(t, serverConfig.VerifyPeerCertificate)
	assert.Equal(t, []string{"dep2p/1.0.0"}, serverConfig.NextProtos)
}

func TestBuildClientConfig(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	builder := NewConfigBuilder(ident)

	// 创建另一个身份作为预期的服务端
	serverIdent, err := mgr.Create()
	require.NoError(t, err)

	// 构建客户端配置
	clientConfig, err := builder.BuildClientConfig(serverIdent.ID())
	require.NoError(t, err)
	require.NotNil(t, clientConfig)

	// 验证配置
	assert.Equal(t, uint16(tls.VersionTLS13), clientConfig.MinVersion)
	assert.NotEmpty(t, clientConfig.Certificates)
	assert.True(t, clientConfig.InsecureSkipVerify) // P2P 场景使用自签名证书
	assert.NotNil(t, clientConfig.VerifyPeerCertificate)
	assert.Equal(t, []string{"dep2p/1.0.0"}, clientConfig.NextProtos)
}

func TestBuildClientConfigWithEmptyExpectedID(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	builder := NewConfigBuilder(ident)

	// 使用空 NodeID 构建客户端配置
	clientConfig, err := builder.BuildClientConfig(types.EmptyNodeID)
	require.NoError(t, err)
	require.NotNil(t, clientConfig)
}

func TestTLSConfigProvider(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	provider := NewTLSConfigProvider(ident)
	assert.NotNil(t, provider)

	// 测试服务端配置
	serverConfig, err := provider.ServerConfig()
	require.NoError(t, err)
	require.NotNil(t, serverConfig)

	// 测试客户端配置
	serverIdent, err := mgr.Create()
	require.NoError(t, err)
	clientConfig, err := provider.ClientConfig(serverIdent.ID())
	require.NoError(t, err)
	require.NotNil(t, clientConfig)

	// 测试 InsecureSkipVerify
	assert.False(t, provider.InsecureSkipVerify())
}

func TestBuildServerConfigWithoutClientAuth(t *testing.T) {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)

	builder := NewConfigBuilder(ident).WithRequireClientAuth(false)

	serverConfig, err := builder.BuildServerConfig()
	require.NoError(t, err)
	assert.Equal(t, tls.RequestClientCert, serverConfig.ClientAuth)
}

