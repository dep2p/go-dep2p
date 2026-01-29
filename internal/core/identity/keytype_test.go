package identity

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// KeyType 测试
// ============================================================================

// TestKeyType_String 测试 KeyType.String()
func TestKeyType_String(t *testing.T) {
	testCases := []struct {
		keyType pkgif.KeyType
		want    string
	}{
		{pkgif.KeyTypeEd25519, "Ed25519"},
		{pkgif.KeyTypeSecp256k1, "Secp256k1"},
		{pkgif.KeyTypeRSA, "RSA"},
		{pkgif.KeyTypeECDSA, "ECDSA"},
	}

	for _, tc := range testCases {
		got := tc.keyType.String()
		if got != tc.want {
			t.Errorf("KeyType(%d).String() = %s, want %s", tc.keyType, got, tc.want)
		}
	}
}

// TestPrivateKey_Type 测试私钥类型
func TestPrivateKey_Type(t *testing.T) {
	priv, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	if priv.Type() != pkgif.KeyTypeEd25519 {
		t.Errorf("PrivateKey.Type() = %v, want Ed25519", priv.Type())
	}
}

// TestPublicKey_Type 测试公钥类型
func TestPublicKey_Type(t *testing.T) {
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	if pub.Type() != pkgif.KeyTypeEd25519 {
		t.Errorf("PublicKey.Type() = %v, want Ed25519", pub.Type())
	}
}

// TestPrivateKeyFromBytes_InvalidKeyType 测试无效密钥类型
func TestPrivateKeyFromBytes_InvalidKeyType(t *testing.T) {
	invalidType := pkgif.KeyType(999)

	_, err := PrivateKeyFromBytes(make([]byte, 64), invalidType)
	if err != ErrInvalidKeyType {
		t.Errorf("PrivateKeyFromBytes() error = %v, want ErrInvalidKeyType", err)
	}
}

// TestPublicKeyFromBytes_InvalidKeyType 测试无效密钥类型
func TestPublicKeyFromBytes_InvalidKeyType(t *testing.T) {
	invalidType := pkgif.KeyType(999)

	_, err := PublicKeyFromBytes(make([]byte, 32), invalidType)
	if err != ErrInvalidKeyType {
		t.Errorf("PublicKeyFromBytes() error = %v, want ErrInvalidKeyType", err)
	}
}

// TestMarshalPrivateKeyPEM_InvalidType 测试 PEM 编码时无效类型
func TestMarshalPrivateKeyPEM_InvalidType(t *testing.T) {
	// 创建一个 mock 私钥，返回不支持的类型
	mockKey := &mockPrivateKey{keyType: pkgif.KeyTypeRSA}

	_, err := MarshalPrivateKeyPEM(mockKey)
	if err != ErrInvalidKeyType {
		t.Errorf("MarshalPrivateKeyPEM() error = %v, want ErrInvalidKeyType", err)
	}
}

// mockPrivateKey 用于测试的 mock 私钥
type mockPrivateKey struct {
	keyType pkgif.KeyType
}

func (m *mockPrivateKey) Raw() ([]byte, error) {
	return make([]byte, 64), nil
}

func (m *mockPrivateKey) Type() pkgif.KeyType {
	return m.keyType
}

func (m *mockPrivateKey) PublicKey() pkgif.PublicKey {
	return nil
}

func (m *mockPrivateKey) Equals(other pkgif.PrivateKey) bool {
	return false
}

func (m *mockPrivateKey) Sign(data []byte) ([]byte, error) {
	return make([]byte, 64), nil
}
