package identity

import (
	"bytes"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 接口契约测试
// ============================================================================

// TestPrivateKey_ImplementsInterface 验证 PrivateKey 实现接口
func TestPrivateKey_ImplementsInterface(t *testing.T) {
	priv, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	var _ pkgif.PrivateKey = priv
}

// TestPublicKey_ImplementsInterface 验证 PublicKey 实现接口
func TestPublicKey_ImplementsInterface(t *testing.T) {
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	var _ pkgif.PublicKey = pub
}

// ============================================================================
// Ed25519 密钥测试
// ============================================================================

// TestEd25519_Generate 测试密钥生成
func TestEd25519_Generate(t *testing.T) {
	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	if priv == nil {
		t.Error("GenerateEd25519Key() returned nil private key")
	}

	if pub == nil {
		t.Error("GenerateEd25519Key() returned nil public key")
	}

	// 验证密钥类型
	if priv.Type() != pkgif.KeyTypeEd25519 {
		t.Errorf("PrivateKey.Type() = %v, want Ed25519", priv.Type())
	}

	if pub.Type() != pkgif.KeyTypeEd25519 {
		t.Errorf("PublicKey.Type() = %v, want Ed25519", pub.Type())
	}
}

// TestEd25519_Marshal 测试序列化
func TestEd25519_Marshal(t *testing.T) {
	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	// 测试私钥序列化
	privBytes, err := priv.Raw()
	if err != nil {
		t.Errorf("PrivateKey.Raw() failed: %v", err)
	}

	// Ed25519 私钥应该是 64 字节
	if len(privBytes) != 64 {
		t.Errorf("PrivateKey.Raw() length = %d, want 64", len(privBytes))
	}

	// 测试公钥序列化
	pubBytes, err := pub.Raw()
	if err != nil {
		t.Errorf("PublicKey.Raw() failed: %v", err)
	}

	// Ed25519 公钥应该是 32 字节
	if len(pubBytes) != 32 {
		t.Errorf("PublicKey.Raw() length = %d, want 32", len(pubBytes))
	}
}

// TestEd25519_PublicKeyFromPrivate 测试从私钥获取公钥
func TestEd25519_PublicKeyFromPrivate(t *testing.T) {
	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	// 从私钥获取公钥
	derivedPub := priv.PublicKey()

	// 比较公钥
	if !derivedPub.Equals(pub) {
		t.Error("PublicKey from private key does not match")
	}

	// 比较原始字节
	derivedBytes, _ := derivedPub.Raw()
	originalBytes, _ := pub.Raw()

	if !bytes.Equal(derivedBytes, originalBytes) {
		t.Error("PublicKey bytes do not match")
	}
}

// TestPrivateKey_Equals 测试私钥比较
func TestPrivateKey_Equals(t *testing.T) {
	priv1, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	priv2, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	// 同一个密钥应该相等
	if !priv1.Equals(priv1) {
		t.Error("PrivateKey.Equals() should return true for same key")
	}

	// 不同密钥应该不相等
	if priv1.Equals(priv2) {
		t.Error("PrivateKey.Equals() should return false for different keys")
	}
}

// TestPublicKey_Equals 测试公钥比较
func TestPublicKey_Equals(t *testing.T) {
	_, pub1, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	_, pub2, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	// 同一个公钥应该相等
	if !pub1.Equals(pub1) {
		t.Error("PublicKey.Equals() should return true for same key")
	}

	// 不同公钥应该不相等
	if pub1.Equals(pub2) {
		t.Error("PublicKey.Equals() should return false for different keys")
	}
}

// TestPrivateKey_PEM 已移除（冗余测试）
// PEM 格式测试已在 helpers_test.go 的 TestMarshalUnmarshalPEM 中完整覆盖

// ============================================================================
// 基准测试
// ============================================================================

// BenchmarkEd25519_Generate 密钥生成性能
func BenchmarkEd25519_Generate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, err := GenerateEd25519Key()
		if err != nil {
			b.Fatalf("GenerateEd25519Key() failed: %v", err)
		}
	}
}

// BenchmarkPrivateKey_Raw 私钥序列化性能
func BenchmarkPrivateKey_Raw(b *testing.B) {
	priv, _, _ := GenerateEd25519Key()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := priv.Raw()
		if err != nil {
			b.Fatalf("Raw() failed: %v", err)
		}
	}
}
