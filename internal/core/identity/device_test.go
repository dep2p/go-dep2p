package identity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              Mock Identity
// ============================================================================

// mockIdentity 模拟身份实现
type mockIdentity struct {
	peerID string
}

func (m *mockIdentity) PeerID() string { return m.peerID }

func (m *mockIdentity) Sign(data []byte) ([]byte, error) { return nil, nil }

func (m *mockIdentity) Verify(data, sig []byte) (bool, error) { return true, nil }

func (m *mockIdentity) PrivateKey() pkgif.PrivateKey { return nil }

func (m *mockIdentity) PublicKey() pkgif.PublicKey { return nil }

func (m *mockIdentity) IDFromPublicKey() (string, error) { return m.peerID, nil }

// ============================================================================
//                              配置测试
// ============================================================================

// TestDefaultDeviceIdentityConfig 测试默认配置
func TestDefaultDeviceIdentityConfig(t *testing.T) {
	config := DefaultDeviceIdentityConfig()

	assert.Equal(t, DefaultDeviceCertValidity, config.CertValidity)
	assert.Empty(t, config.DeviceID)
}

// ============================================================================
//                              设备身份创建测试
// ============================================================================

// TestNewDeviceIdentity 测试创建设备身份
func TestNewDeviceIdentity(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)
	require.NotNil(t, di)

	assert.NotEmpty(t, di.DeviceID())
	assert.NotNil(t, di.PublicKey())
	assert.False(t, di.IsBound())
}

// TestNewDeviceIdentity_CustomDeviceID 测试自定义设备 ID
func TestNewDeviceIdentity_CustomDeviceID(t *testing.T) {
	config := DeviceIdentityConfig{
		DeviceID:     "custom-device-123",
		CertValidity: 24 * time.Hour,
	}
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	assert.Equal(t, "custom-device-123", di.DeviceID())
}

// TestNewDeviceIdentityFromKey 测试从密钥创建设备身份
func TestNewDeviceIdentityFromKey(t *testing.T) {
	// 创建原始设备身份
	config1 := DefaultDeviceIdentityConfig()
	di1, err := NewDeviceIdentity(config1)
	require.NoError(t, err)

	// 生成新密钥
	privKey, _, err := GenerateEd25519Key()
	require.NoError(t, err)

	// 从密钥创建
	config2 := DefaultDeviceIdentityConfig()
	di2, err := NewDeviceIdentityFromKey(privKey, config2)
	require.NoError(t, err)

	// 不同密钥应该有不同的设备 ID
	assert.NotEqual(t, di1.DeviceID(), di2.DeviceID())
}

// TestNewDeviceIdentityFromKey_NilKey 测试从 nil 密钥创建
func TestNewDeviceIdentityFromKey_NilKey(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	_, err := NewDeviceIdentityFromKey(nil, config)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNilPrivateKey)
}

// ============================================================================
//                              绑定测试
// ============================================================================

// TestDeviceIdentity_BindToPeer 测试绑定到节点
func TestDeviceIdentity_BindToPeer(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	// 创建模拟身份
	peerIdentity := &mockIdentity{peerID: "test-peer-id-123456"}

	err = di.BindToPeer(peerIdentity)
	assert.NoError(t, err)
	assert.True(t, di.IsBound())

	// 检查绑定的 PeerID
	peerID, ok := di.BoundPeerID()
	assert.True(t, ok)
	assert.Equal(t, "test-peer-id-123456", string(peerID))

	// 检查证书已创建
	cert := di.Certificate()
	assert.NotNil(t, cert)
	assert.Equal(t, di.DeviceID(), cert.DeviceID)
}

// TestDeviceIdentity_BindToPeer_AlreadyBound 测试重复绑定
func TestDeviceIdentity_BindToPeer_AlreadyBound(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	peerIdentity1 := &mockIdentity{peerID: "peer-1"}
	peerIdentity2 := &mockIdentity{peerID: "peer-2"}

	err = di.BindToPeer(peerIdentity1)
	assert.NoError(t, err)

	// 再次绑定应该失败
	err = di.BindToPeer(peerIdentity2)
	assert.ErrorIs(t, err, ErrDeviceAlreadyBound)
}

// TestDeviceIdentity_Unbind 测试解除绑定
func TestDeviceIdentity_Unbind(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	peerIdentity := &mockIdentity{peerID: "peer-1"}

	err = di.BindToPeer(peerIdentity)
	assert.NoError(t, err)
	assert.True(t, di.IsBound())

	// 解除绑定
	di.Unbind()
	assert.False(t, di.IsBound())
	assert.Nil(t, di.Certificate())

	// 解除绑定后应该可以重新绑定
	peerIdentity2 := &mockIdentity{peerID: "peer-2"}
	err = di.BindToPeer(peerIdentity2)
	assert.NoError(t, err)
}

// TestDeviceIdentity_BoundPeerID_NotBound 测试未绑定时获取 PeerID
func TestDeviceIdentity_BoundPeerID_NotBound(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	peerID, ok := di.BoundPeerID()
	assert.False(t, ok)
	assert.Empty(t, peerID)
}

// ============================================================================
//                              证书测试
// ============================================================================

// TestDeviceIdentity_Certificate 测试获取证书
func TestDeviceIdentity_Certificate(t *testing.T) {
	config := DeviceIdentityConfig{
		CertValidity: 24 * time.Hour,
	}
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	// 未绑定时没有证书
	assert.Nil(t, di.Certificate())

	// 绑定后有证书
	peerIdentity := &mockIdentity{peerID: "test-peer"}
	err = di.BindToPeer(peerIdentity)
	require.NoError(t, err)

	cert := di.Certificate()
	require.NotNil(t, cert)
	assert.Equal(t, DeviceCertVersion, int(cert.Version))
	assert.Equal(t, di.DeviceID(), cert.DeviceID)
	assert.NotEmpty(t, cert.PublicKey)
	assert.NotEmpty(t, cert.Signature)
	assert.False(t, cert.IsExpired())
}

// TestDeviceIdentity_RenewCertificate 测试证书续期
func TestDeviceIdentity_RenewCertificate(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	peerIdentity := &mockIdentity{peerID: "test-peer"}
	err = di.BindToPeer(peerIdentity)
	require.NoError(t, err)

	oldCert := di.Certificate()
	require.NotNil(t, oldCert)

	// 稍微等待一下确保时间戳不同
	time.Sleep(10 * time.Millisecond)

	// 续期
	err = di.RenewCertificate()
	assert.NoError(t, err)

	newCert := di.Certificate()
	require.NotNil(t, newCert)

	// 签发时间应该不同
	assert.GreaterOrEqual(t, newCert.IssuedAt, oldCert.IssuedAt)
}

// TestDeviceIdentity_RenewCertificate_NotBound 测试未绑定时续期
func TestDeviceIdentity_RenewCertificate_NotBound(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	err = di.RenewCertificate()
	assert.ErrorIs(t, err, ErrDeviceNotBound)
}

// ============================================================================
//                              证书验证测试
// ============================================================================

// TestVerifyCertificate 测试证书验证
func TestVerifyCertificate(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	peerIdentity := &mockIdentity{peerID: "test-peer"}
	err = di.BindToPeer(peerIdentity)
	require.NoError(t, err)

	cert := di.Certificate()
	require.NotNil(t, cert)

	// 验证应该成功
	err = VerifyCertificate(cert)
	assert.NoError(t, err)
}

// TestVerifyCertificate_Nil 测试验证 nil 证书
func TestVerifyCertificate_Nil(t *testing.T) {
	err := VerifyCertificate(nil)
	assert.ErrorIs(t, err, ErrInvalidDeviceCert)
}

// TestVerifyCertificate_InvalidSignature 测试验证无效签名
func TestVerifyCertificate_InvalidSignature(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	peerIdentity := &mockIdentity{peerID: "test-peer"}
	err = di.BindToPeer(peerIdentity)
	require.NoError(t, err)

	cert := di.Certificate()
	require.NotNil(t, cert)

	// 篡改设备 ID
	cert.DeviceID = "tampered-device-id"

	// 验证应该失败
	err = VerifyCertificate(cert)
	assert.ErrorIs(t, err, ErrInvalidDeviceSignature)
}

// ============================================================================
//                              证书序列化测试
// ============================================================================

// TestDeviceCertificate_Marshal 测试证书序列化
func TestDeviceCertificate_Marshal(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	peerIdentity := &mockIdentity{peerID: "test-peer"}
	err = di.BindToPeer(peerIdentity)
	require.NoError(t, err)

	cert := di.Certificate()
	require.NotNil(t, cert)

	// 序列化
	data, err := cert.Marshal()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 反序列化
	cert2, err := UnmarshalDeviceCertificate(data)
	require.NoError(t, err)

	assert.Equal(t, cert.Version, cert2.Version)
	assert.Equal(t, cert.DeviceID, cert2.DeviceID)
	assert.Equal(t, cert.PeerID, cert2.PeerID)
	assert.Equal(t, cert.PublicKey, cert2.PublicKey)
	assert.Equal(t, cert.IssuedAt, cert2.IssuedAt)
	assert.Equal(t, cert.ExpiresAt, cert2.ExpiresAt)
	assert.Equal(t, cert.Signature, cert2.Signature)
}

// TestUnmarshalDeviceCertificate_Invalid 测试反序列化无效数据
func TestUnmarshalDeviceCertificate_Invalid(t *testing.T) {
	_, err := UnmarshalDeviceCertificate(nil)
	assert.Error(t, err)

	_, err = UnmarshalDeviceCertificate([]byte{})
	assert.Error(t, err)

	_, err = UnmarshalDeviceCertificate([]byte("short"))
	assert.Error(t, err)
}

// TestDeviceIdentity_CertificateBytes 测试获取证书字节
func TestDeviceIdentity_CertificateBytes(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	// 未绑定时应该返回错误
	_, err = di.CertificateBytes()
	assert.ErrorIs(t, err, ErrDeviceNotBound)

	// 绑定后应该返回证书字节
	peerIdentity := &mockIdentity{peerID: "test-peer"}
	err = di.BindToPeer(peerIdentity)
	require.NoError(t, err)

	certBytes, err := di.CertificateBytes()
	assert.NoError(t, err)
	assert.NotEmpty(t, certBytes)
}

// ============================================================================
//                              证书过期测试
// ============================================================================

// TestDeviceCertificate_IsExpired 测试证书过期检查
func TestDeviceCertificate_IsExpired(t *testing.T) {
	t.Run("not expired", func(t *testing.T) {
		cert := &DeviceCertificate{
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		}
		assert.False(t, cert.IsExpired())
	})

	t.Run("expired", func(t *testing.T) {
		cert := &DeviceCertificate{
			IssuedAt:  time.Now().Add(-48 * time.Hour).Unix(),
			ExpiresAt: time.Now().Add(-24 * time.Hour).Unix(),
		}
		assert.True(t, cert.IsExpired())
	})
}

// TestDeviceCertificate_IsValid 测试证书有效性检查
func TestDeviceCertificate_IsValid(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	peerIdentity := &mockIdentity{peerID: "test-peer"}
	err = di.BindToPeer(peerIdentity)
	require.NoError(t, err)

	cert := di.Certificate()
	require.NotNil(t, cert)

	assert.True(t, cert.IsValid())

	// 无效版本
	cert.Version = 99
	assert.False(t, cert.IsValid())
}

// ============================================================================
//                              设备 ID 测试
// ============================================================================

// TestDeviceIdentity_DeviceID_Consistency 测试设备 ID 一致性
func TestDeviceIdentity_DeviceID_Consistency(t *testing.T) {
	config := DefaultDeviceIdentityConfig()
	di, err := NewDeviceIdentity(config)
	require.NoError(t, err)

	id1 := di.DeviceID()
	id2 := di.DeviceID()

	assert.Equal(t, id1, id2)
	assert.NotEmpty(t, id1)
}

// TestDeviceIdentity_Different 测试不同设备身份
func TestDeviceIdentity_Different(t *testing.T) {
	config := DefaultDeviceIdentityConfig()

	di1, _ := NewDeviceIdentity(config)
	di2, _ := NewDeviceIdentity(config)

	assert.NotEqual(t, di1.DeviceID(), di2.DeviceID())
}
