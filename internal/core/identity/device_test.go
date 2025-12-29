package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestIdentity 创建用于测试的 Ed25519 身份
func createTestIdentity(t *testing.T) *identity {
	t.Helper()
	priv, _, err := GenerateEd25519KeyPair()
	require.NoError(t, err)
	return NewIdentity(priv)
}

// ============================================================================
//                              DeviceIdentity 测试
// ============================================================================

func TestDeviceIdentity_NewDeviceIdentity(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	assert.True(t, deviceIdentity.IsMaster())
	assert.Equal(t, 0, deviceIdentity.DeviceCount())
	assert.Equal(t, masterIdentity.ID(), deviceIdentity.MasterID())
}

func TestDeviceIdentity_IssueDeviceCertificate(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 生成设备密钥
	devicePubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// 签发证书
	cert, err := deviceIdentity.IssueDeviceCertificate("TestDevice", devicePubKey)
	require.NoError(t, err)
	require.NotNil(t, cert)

	// 验证证书内容
	assert.Equal(t, masterIdentity.ID(), cert.MasterID)
	assert.Equal(t, "TestDevice", cert.DeviceName)
	assert.False(t, cert.IsExpired())
	assert.Equal(t, 1, deviceIdentity.DeviceCount())
}

func TestDeviceIdentity_GenerateDeviceKeyPair(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 生成设备密钥对并签发证书
	cert, privKey, err := deviceIdentity.GenerateDeviceKeyPair("TestDevice")
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.NotNil(t, privKey)

	// 验证私钥可以签名
	testData := []byte("test message")
	signature := ed25519.Sign(privKey, testData)
	assert.True(t, ed25519.Verify(cert.DevicePublicKey, testData, signature))
}

func TestDeviceIdentity_VerifyDeviceCertificate(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 签发证书
	cert, _, err := deviceIdentity.GenerateDeviceKeyPair("TestDevice")
	require.NoError(t, err)

	// 验证证书
	err = deviceIdentity.VerifyDeviceCertificate(cert)
	assert.NoError(t, err)
}

func TestDeviceIdentity_VerifyDeviceCertificate_InvalidSignature(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 签发证书
	cert, _, err := deviceIdentity.GenerateDeviceKeyPair("TestDevice")
	require.NoError(t, err)

	// 篡改签名
	cert.Signature[0] ^= 0xFF

	// 验证应该失败
	err = deviceIdentity.VerifyDeviceCertificate(cert)
	assert.Equal(t, ErrInvalidDeviceCert, err)
}

func TestDeviceIdentity_RevokeDevice(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 签发证书
	cert, _, err := deviceIdentity.GenerateDeviceKeyPair("TestDevice")
	require.NoError(t, err)
	assert.Equal(t, 1, deviceIdentity.DeviceCount())

	// 撤销设备
	err = deviceIdentity.RevokeDevice(cert.DeviceID)
	require.NoError(t, err)

	// 验证已撤销
	assert.True(t, deviceIdentity.IsDeviceRevoked(cert.DeviceID))
	assert.Equal(t, 0, deviceIdentity.DeviceCount())

	// 验证证书失败
	err = deviceIdentity.VerifyDeviceCertificate(cert)
	assert.Equal(t, ErrDeviceRevoked, err)
}

func TestDeviceIdentity_MaxDevices(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器（限制为 2 个设备）
	config := DeviceConfig{
		MaxDevices:    2,
		CertValidity:  time.Hour,
		AllowSelfSign: true,
	}
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 签发 2 个证书
	_, _, err = deviceIdentity.GenerateDeviceKeyPair("Device1")
	require.NoError(t, err)
	_, _, err = deviceIdentity.GenerateDeviceKeyPair("Device2")
	require.NoError(t, err)

	// 第 3 个应该失败
	_, _, err = deviceIdentity.GenerateDeviceKeyPair("Device3")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum device limit")
}

func TestDeviceIdentity_DuplicateDevice(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 生成设备密钥
	devicePubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// 签发证书
	_, err = deviceIdentity.IssueDeviceCertificate("Device1", devicePubKey)
	require.NoError(t, err)

	// 尝试重复签发
	_, err = deviceIdentity.IssueDeviceCertificate("Device1Again", devicePubKey)
	assert.Equal(t, ErrDeviceAlreadyExists, err)
}

func TestDeviceIdentity_RevokedDeviceCannotReissue(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 生成设备密钥
	devicePubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// 签发证书
	cert, err := deviceIdentity.IssueDeviceCertificate("Device1", devicePubKey)
	require.NoError(t, err)

	// 撤销
	err = deviceIdentity.RevokeDevice(cert.DeviceID)
	require.NoError(t, err)

	// 尝试重新签发（同一个公钥）
	_, err = deviceIdentity.IssueDeviceCertificate("Device1Again", devicePubKey)
	assert.Equal(t, ErrDeviceRevoked, err)
}

func TestDeviceIdentity_GetDevice(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 签发证书
	cert, _, err := deviceIdentity.GenerateDeviceKeyPair("TestDevice")
	require.NoError(t, err)

	// 获取设备
	gotCert, ok := deviceIdentity.GetDevice(cert.DeviceID)
	assert.True(t, ok)
	assert.Equal(t, cert.DeviceName, gotCert.DeviceName)

	// 获取不存在的设备
	_, ok = deviceIdentity.GetDevice(types.EmptyNodeID)
	assert.False(t, ok)
}

func TestDeviceIdentity_ListDevices(t *testing.T) {
	// 创建主身份
	masterIdentity := createTestIdentity(t)

	// 创建设备身份管理器
	config := DefaultDeviceConfig()
	deviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	// 签发多个证书
	_, _, err = deviceIdentity.GenerateDeviceKeyPair("Device1")
	require.NoError(t, err)
	_, _, err = deviceIdentity.GenerateDeviceKeyPair("Device2")
	require.NoError(t, err)
	_, _, err = deviceIdentity.GenerateDeviceKeyPair("Device3")
	require.NoError(t, err)

	// 列出设备
	devices := deviceIdentity.ListDevices()
	assert.Len(t, devices, 3)
}

func TestDeviceIdentity_NonMasterCannotIssue(t *testing.T) {
	// 创建主身份并签发设备证书
	masterIdentity := createTestIdentity(t)

	config := DefaultDeviceConfig()
	masterDeviceIdentity, err := NewDeviceIdentity(masterIdentity, config)
	require.NoError(t, err)

	cert, privKey, err := masterDeviceIdentity.GenerateDeviceKeyPair("SubDevice")
	require.NoError(t, err)

	// 创建从设备身份
	subDeviceIdentity, err := NewSubDeviceIdentity(cert, privKey)
	require.NoError(t, err)

	// 从设备不能签发证书
	devicePubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	_, err = subDeviceIdentity.IssueDeviceCertificate("AnotherDevice", devicePubKey)
	assert.Equal(t, ErrNotMasterIdentity, err)
}

func TestDeviceCertificate_Bytes(t *testing.T) {
	// 创建证书
	devicePubKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	cert := &DeviceCertificate{
		DeviceID:        types.NodeID{1, 2, 3},
		MasterID:        types.NodeID{4, 5, 6},
		DevicePublicKey: devicePubKey,
		IssuedAt:        time.Now(),
		ExpiresAt:       time.Now().Add(time.Hour),
		DeviceName:      "TestDevice",
	}

	// 确保 Bytes 返回一致的值
	bytes1 := cert.Bytes()
	bytes2 := cert.Bytes()
	assert.Equal(t, bytes1, bytes2)
}

func TestDeviceCertificate_IsExpired(t *testing.T) {
	cert := &DeviceCertificate{
		ExpiresAt: time.Now().Add(-time.Hour), // 已过期
	}
	assert.True(t, cert.IsExpired())

	cert.ExpiresAt = time.Now().Add(time.Hour) // 未过期
	assert.False(t, cert.IsExpired())
}

