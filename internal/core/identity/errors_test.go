package identity

import (
	"testing"
)

// ============================================================================
// 错误测试
// ============================================================================

// TestErrors_Defined 测试错误定义
func TestErrors_Defined(t *testing.T) {
	errors := []error{
		ErrNilPrivateKey,
		ErrNilPublicKey,
		ErrKeyPairMismatch,
		ErrFailedToGenerateKey,
		ErrFailedToDerivePeerID,
		ErrInvalidKeyType,
		ErrInvalidKey,
		ErrInvalidPEM,
		ErrSigningFailed,
		ErrInvalidSignature,
		ErrInvalidPeerID,
		ErrEmptyPublicKey,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("Error is nil")
		}

		if err.Error() == "" {
			t.Errorf("Error %v has empty message", err)
		}
	}
}

// TestNew_NilPrivateKey 测试 nil 私钥错误
func TestNew_NilPrivateKey(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Error("New() should fail with nil private key")
	}
}

// TestSign_NilKey 测试签名时 nil 密钥
func TestSign_NilKey(t *testing.T) {
	_, err := Sign(nil, []byte("data"))
	if err == nil {
		t.Error("Sign() should fail with nil private key")
	}
}

// TestVerify_NilKey 测试验证时 nil 密钥
func TestVerify_NilKey(t *testing.T) {
	_, err := Verify(nil, []byte("data"), []byte("sig"))
	if err == nil {
		t.Error("Verify() should fail with nil public key")
	}
}

// TestVerify_EmptySignature 测试空签名
func TestVerify_EmptySignature(t *testing.T) {
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	_, err = Verify(pub, []byte("data"), []byte{})
	if err == nil {
		t.Error("Verify() should fail with empty signature")
	}
}

// TestPeerIDFromPublicKey_NilKey 测试 nil 公钥
func TestPeerIDFromPublicKey_NilKey(t *testing.T) {
	_, err := PeerIDFromPublicKey(nil)
	if err == nil {
		t.Error("PeerIDFromPublicKey() should fail with nil public key")
	}
}

// TestUnmarshalPrivateKeyPEM_InvalidPEM 测试无效 PEM
func TestUnmarshalPrivateKeyPEM_InvalidPEM(t *testing.T) {
	_, err := UnmarshalPrivateKeyPEM([]byte("invalid pem"))
	if err == nil {
		t.Error("UnmarshalPrivateKeyPEM() should fail with invalid PEM")
	}
}

// TestUnmarshalPrivateKeyPEM_WrongType 测试错误的 PEM 类型
func TestUnmarshalPrivateKeyPEM_WrongType(t *testing.T) {
	wrongPEM := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA...
-----END RSA PRIVATE KEY-----`

	_, err := UnmarshalPrivateKeyPEM([]byte(wrongPEM))
	if err == nil {
		t.Error("UnmarshalPrivateKeyPEM() should fail with wrong PEM type")
	}
}
