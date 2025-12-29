package tls

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dep2p/go-dep2p/pkg/types"
)

func TestBuildConnectionState(t *testing.T) {
	tests := []struct {
		name         string
		tlsVersion   uint16
		expectVer    string
		cipherSuite  uint16
		didResume    bool
	}{
		{
			name:        "TLS 1.0",
			tlsVersion:  tls.VersionTLS10,
			expectVer:   "1.0",
			cipherSuite: tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		},
		{
			name:        "TLS 1.1",
			tlsVersion:  tls.VersionTLS11,
			expectVer:   "1.1",
			cipherSuite: tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		{
			name:        "TLS 1.2",
			tlsVersion:  tls.VersionTLS12,
			expectVer:   "1.2",
			cipherSuite: tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		{
			name:        "TLS 1.3",
			tlsVersion:  tls.VersionTLS13,
			expectVer:   "1.3",
			cipherSuite: tls.TLS_AES_128_GCM_SHA256,
		},
		{
			name:        "Unknown Version",
			tlsVersion:  0x0999,
			expectVer:   "0x0999",
			cipherSuite: tls.TLS_AES_256_GCM_SHA384,
		},
		{
			name:        "With Resume",
			tlsVersion:  tls.VersionTLS13,
			expectVer:   "1.3",
			cipherSuite: tls.TLS_AES_128_GCM_SHA256,
			didResume:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tlsState := tls.ConnectionState{
				Version:     tc.tlsVersion,
				CipherSuite: tc.cipherSuite,
				DidResume:   tc.didResume,
			}

			state := buildConnectionState(tlsState)

			assert.Equal(t, "tls", state.Protocol)
			assert.Equal(t, tc.expectVer, state.Version)
			assert.NotEmpty(t, state.CipherSuite)
			assert.Equal(t, tc.didResume, state.DidResume)
		})
	}
}

func TestTLSPublicKey(t *testing.T) {
	// 创建一个空的 tlsPublicKey
	key := &tlsPublicKey{cert: nil}

	// 测试空证书的情况
	assert.Nil(t, key.Bytes())
	assert.Nil(t, key.Raw())
	assert.Equal(t, types.KeyTypeUnknown, key.Type())

	// 测试 Equal - 两个空 key 比较
	otherKey := &tlsPublicKey{cert: nil}
	// 两个空的 key 应该相等（都是 nil cert）
	assert.True(t, key.Equal(otherKey))

	// 测试 Equal - nil key 与 nil 比较
	assert.True(t, key.Equal(nil))

	// 测试 Verify - 空证书应该返回错误
	ok, err := key.Verify([]byte("data"), []byte("sig"))
	assert.False(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "证书为空")
}

