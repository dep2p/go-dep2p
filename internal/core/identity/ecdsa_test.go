package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              ECDSA 密钥生成测试
// ============================================================================

func TestGenerateECDSAKeyPair_P256(t *testing.T) {
	priv, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)
	require.NotNil(t, priv)
	require.NotNil(t, pub)

	// 验证密钥类型
	assert.Equal(t, types.KeyTypeECDSAP256, priv.Type())
	assert.Equal(t, types.KeyTypeECDSAP256, pub.Type())
}

func TestGenerateECDSAKeyPair_P384(t *testing.T) {
	priv, pub, err := GenerateECDSAP384KeyPair()
	require.NoError(t, err)
	require.NotNil(t, priv)
	require.NotNil(t, pub)

	// 验证密钥类型
	assert.Equal(t, types.KeyTypeECDSAP384, priv.Type())
	assert.Equal(t, types.KeyTypeECDSAP384, pub.Type())
}

func TestGenerateECDSAKeyPair_DefaultCurve(t *testing.T) {
	priv, pub, err := GenerateECDSAKeyPair(nil)
	require.NoError(t, err)

	// 默认应该是 P-256
	assert.Equal(t, types.KeyTypeECDSAP256, priv.Type())
	assert.Equal(t, types.KeyTypeECDSAP256, pub.Type())
}

// ============================================================================
//                              签名和验证测试
// ============================================================================

func TestECDSAPrivateKey_SignAndVerify_P256(t *testing.T) {
	priv, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	testData := []byte("test message for ECDSA P-256 signing")

	// 签名
	signature, err := priv.Sign(testData)
	require.NoError(t, err)
	require.NotEmpty(t, signature)

	// 验证
	valid, err := pub.Verify(testData, signature)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestECDSAPrivateKey_SignAndVerify_P384(t *testing.T) {
	priv, pub, err := GenerateECDSAP384KeyPair()
	require.NoError(t, err)

	testData := []byte("test message for ECDSA P-384 signing")

	// 签名
	signature, err := priv.Sign(testData)
	require.NoError(t, err)
	require.NotEmpty(t, signature)

	// 验证
	valid, err := pub.Verify(testData, signature)
	require.NoError(t, err)
	assert.True(t, valid)
}

func TestECDSAPublicKey_Verify_InvalidSignature(t *testing.T) {
	priv, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	testData := []byte("test message")

	// 签名
	signature, err := priv.Sign(testData)
	require.NoError(t, err)

	// 篡改签名
	signature[0] ^= 0xFF

	// 验证应该失败
	valid, err := pub.Verify(testData, signature)
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestECDSAPublicKey_Verify_WrongData(t *testing.T) {
	priv, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	testData := []byte("original message")
	wrongData := []byte("different message")

	// 签名原始数据
	signature, err := priv.Sign(testData)
	require.NoError(t, err)

	// 用错误的数据验证
	valid, err := pub.Verify(wrongData, signature)
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestECDSAPublicKey_Verify_InvalidSignatureLength(t *testing.T) {
	_, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	testData := []byte("test message")

	// 无效长度的签名
	invalidSig := []byte("short")

	_, err = pub.Verify(testData, invalidSig)
	assert.Error(t, err)
}

// ============================================================================
//                              序列化测试
// ============================================================================

func TestECDSAPrivateKey_Bytes(t *testing.T) {
	priv, _, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	bytes := priv.Bytes()
	require.NotEmpty(t, bytes)

	// PKCS8 编码的私钥应该是 DER 格式
	assert.NotNil(t, bytes)
}

func TestECDSAPublicKey_Bytes(t *testing.T) {
	_, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	bytes := pub.Bytes()
	require.NotEmpty(t, bytes)

	// P-256 未压缩公钥是 65 字节 (0x04 + X(32) + Y(32))
	assert.Equal(t, 65, len(bytes))
	assert.Equal(t, byte(0x04), bytes[0]) // 未压缩标记
}

func TestNewECDSAPublicKeyFromBytes_P256(t *testing.T) {
	_, origPub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	bytes := origPub.Bytes()

	// 从字节恢复
	restoredPub, err := NewECDSAPublicKeyFromBytes(bytes, nil) // 自动检测
	require.NoError(t, err)
	require.NotNil(t, restoredPub)

	assert.True(t, origPub.Equal(restoredPub))
}

func TestNewECDSAPublicKeyFromBytes_P384(t *testing.T) {
	_, origPub, err := GenerateECDSAP384KeyPair()
	require.NoError(t, err)

	bytes := origPub.Bytes()

	// 从字节恢复
	restoredPub, err := NewECDSAPublicKeyFromBytes(bytes, nil) // 自动检测
	require.NoError(t, err)
	require.NotNil(t, restoredPub)

	assert.True(t, origPub.Equal(restoredPub))
}

func TestNewECDSAPublicKeyFromBytes_InvalidData(t *testing.T) {
	// 无效长度
	_, err := NewECDSAPublicKeyFromBytes([]byte("invalid"), nil)
	assert.Equal(t, ErrInvalidKeyData, err)

	// 无效数据但正确长度
	invalidBytes := make([]byte, 65)
	invalidBytes[0] = 0x04
	_, err = NewECDSAPublicKeyFromBytes(invalidBytes, elliptic.P256())
	assert.Equal(t, ErrInvalidKeyData, err)
}

// ============================================================================
//                              PEM 序列化测试
// ============================================================================

func TestECDSAPrivateKeyToPEM(t *testing.T) {
	priv, _, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	pemData, err := ECDSAPrivateKeyToPEM(priv)
	require.NoError(t, err)
	require.NotEmpty(t, pemData)

	// 验证是有效的 PEM
	assert.Contains(t, string(pemData), "-----BEGIN PRIVATE KEY-----")
	assert.Contains(t, string(pemData), "-----END PRIVATE KEY-----")
}

func TestECDSAPrivateKeyFromPEM(t *testing.T) {
	origPriv, origPub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	pemData, err := ECDSAPrivateKeyToPEM(origPriv)
	require.NoError(t, err)

	// 从 PEM 恢复
	restoredPriv, err := ECDSAPrivateKeyFromPEM(pemData)
	require.NoError(t, err)
	require.NotNil(t, restoredPriv)

	// 验证私钥相等
	assert.True(t, origPriv.Equal(restoredPriv))

	// 验证公钥也匹配
	assert.True(t, origPub.Equal(restoredPriv.PublicKey()))
}

func TestECDSAPublicKeyToPEM(t *testing.T) {
	_, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	pemData, err := ECDSAPublicKeyToPEM(pub)
	require.NoError(t, err)
	require.NotEmpty(t, pemData)

	// 验证是有效的 PEM
	assert.Contains(t, string(pemData), "-----BEGIN PUBLIC KEY-----")
	assert.Contains(t, string(pemData), "-----END PUBLIC KEY-----")
}

func TestECDSAPublicKeyFromPEM(t *testing.T) {
	_, origPub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	pemData, err := ECDSAPublicKeyToPEM(origPub)
	require.NoError(t, err)

	// 从 PEM 恢复
	restoredPub, err := ECDSAPublicKeyFromPEM(pemData)
	require.NoError(t, err)
	require.NotNil(t, restoredPub)

	// 验证公钥相等
	assert.True(t, origPub.Equal(restoredPub))
}

func TestECDSAPrivateKeyFromPEM_Invalid(t *testing.T) {
	// 无效的 PEM
	_, err := ECDSAPrivateKeyFromPEM([]byte("not a PEM"))
	assert.Error(t, err)

	// 有效 PEM 但不是 ECDSA 密钥
	invalidPEM := []byte(`-----BEGIN PRIVATE KEY-----
aW52YWxpZA==
-----END PRIVATE KEY-----`)
	_, err = ECDSAPrivateKeyFromPEM(invalidPEM)
	assert.Error(t, err)
}

func TestECDSAPublicKeyFromPEM_Invalid(t *testing.T) {
	// 无效的 PEM
	_, err := ECDSAPublicKeyFromPEM([]byte("not a PEM"))
	assert.Error(t, err)
}

// ============================================================================
//                              相等性测试
// ============================================================================

func TestECDSAPublicKey_Equal(t *testing.T) {
	_, pub1, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	_, pub2, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	// 不同的密钥
	assert.False(t, pub1.Equal(pub2))

	// 相同的密钥（通过序列化恢复）
	bytes := pub1.Bytes()
	restoredPub, err := NewECDSAPublicKeyFromBytes(bytes, nil)
	require.NoError(t, err)
	assert.True(t, pub1.Equal(restoredPub))

	// nil
	assert.False(t, pub1.Equal(nil))
}

func TestECDSAPrivateKey_Equal(t *testing.T) {
	priv1, _, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	priv2, _, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	// 不同的密钥
	assert.False(t, priv1.Equal(priv2))

	// 相同的密钥（通过序列化恢复）
	pemData, _ := ECDSAPrivateKeyToPEM(priv1)
	restoredPriv, err := ECDSAPrivateKeyFromPEM(pemData)
	require.NoError(t, err)
	assert.True(t, priv1.Equal(restoredPriv))

	// nil
	assert.False(t, priv1.Equal(nil))
}

// ============================================================================
//                              Raw 方法测试
// ============================================================================

func TestECDSAPublicKey_Raw(t *testing.T) {
	_, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	raw := pub.Raw()
	require.NotNil(t, raw)

	// 验证返回的是 *ecdsa.PublicKey
	ecdsaPub, ok := raw.(*ecdsa.PublicKey)
	assert.True(t, ok)
	assert.Equal(t, elliptic.P256(), ecdsaPub.Curve)
}

func TestECDSAPrivateKey_Raw(t *testing.T) {
	priv, _, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	raw := priv.Raw()
	require.NotNil(t, raw)

	// 验证返回的是 *ecdsa.PrivateKey
	ecdsaPriv, ok := raw.(*ecdsa.PrivateKey)
	assert.True(t, ok)
	assert.Equal(t, elliptic.P256(), ecdsaPriv.Curve)
}

// ============================================================================
//                              PublicKey 方法测试
// ============================================================================

func TestECDSAPrivateKey_PublicKey(t *testing.T) {
	priv, expectedPub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	gotPub := priv.PublicKey()
	require.NotNil(t, gotPub)

	// 验证公钥匹配
	ecdsaGotPub, ok := gotPub.(*ECDSAPublicKey)
	assert.True(t, ok)
	assert.True(t, ecdsaGotPub.Equal(expectedPub))
}

// ============================================================================
//                              包装器测试
// ============================================================================

func TestNewECDSAPublicKey(t *testing.T) {
	// 生成原始 ecdsa 密钥
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// 包装公钥
	pub := NewECDSAPublicKey(&key.PublicKey)
	require.NotNil(t, pub)

	assert.Equal(t, types.KeyTypeECDSAP256, pub.Type())
}

func TestNewECDSAPrivateKey(t *testing.T) {
	// 生成原始 ecdsa 密钥
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// 包装私钥
	priv := NewECDSAPrivateKey(key)
	require.NotNil(t, priv)

	assert.Equal(t, types.KeyTypeECDSAP256, priv.Type())
	assert.NotNil(t, priv.PublicKey())
}

// ============================================================================
//                              签名编码测试
// ============================================================================

func TestSignatureEncoding_Consistency(t *testing.T) {
	priv, pub, err := GenerateECDSAP256KeyPair()
	require.NoError(t, err)

	testData := []byte("test data for signature consistency")

	// 多次签名（由于 ECDSA 使用随机数，每次签名不同）
	sig1, err := priv.Sign(testData)
	require.NoError(t, err)

	sig2, err := priv.Sign(testData)
	require.NoError(t, err)

	// 签名应该不同（随机性）
	assert.NotEqual(t, sig1, sig2)

	// 但都应该验证成功
	valid1, err := pub.Verify(testData, sig1)
	require.NoError(t, err)
	assert.True(t, valid1)

	valid2, err := pub.Verify(testData, sig2)
	require.NoError(t, err)
	assert.True(t, valid2)
}

func TestSignatureLength(t *testing.T) {
	tests := []struct {
		name         string
		genFunc      func() (*ECDSAPrivateKey, *ECDSAPublicKey, error)
		expectedLen  int
	}{
		{
			name:        "P-256",
			genFunc:     GenerateECDSAP256KeyPair,
			expectedLen: 64, // 32 bytes R + 32 bytes S
		},
		{
			name:        "P-384",
			genFunc:     GenerateECDSAP384KeyPair,
			expectedLen: 96, // 48 bytes R + 48 bytes S
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priv, _, err := tt.genFunc()
			require.NoError(t, err)

			sig, err := priv.Sign([]byte("test"))
			require.NoError(t, err)

			assert.Equal(t, tt.expectedLen, len(sig))
		})
	}
}

