package identity

import (
	"os"
	"path/filepath"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 辅助函数测试
// ============================================================================

// TestGenerate 测试 Generate 函数
func TestGenerate(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	if id == nil {
		t.Fatal("Generate() returned nil")
	}

	if id.PeerID() == "" {
		t.Error("Generate() created identity with empty PeerID")
	}

	if id.PublicKey() == nil {
		t.Error("Generate() created identity with nil public key")
	}

	if id.PrivateKey() == nil {
		t.Error("Generate() created identity with nil private key")
	}
}

// TestFromKeyPair 测试 FromKeyPair 函数
func TestFromKeyPair(t *testing.T) {
	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	id, err := FromKeyPair(priv, pub)
	if err != nil {
		t.Fatalf("FromKeyPair() failed: %v", err)
	}

	if id == nil {
		t.Fatal("FromKeyPair() returned nil")
	}

	if !id.PrivateKey().Equals(priv) {
		t.Error("FromKeyPair() private key mismatch")
	}

	if !id.PublicKey().Equals(pub) {
		t.Error("FromKeyPair() public key mismatch")
	}
}

// TestFromKeyPair_NilKeys 测试空密钥错误
func TestFromKeyPair_NilKeys(t *testing.T) {
	priv, pub, _ := GenerateEd25519Key()

	// 测试 nil 私钥
	_, err := FromKeyPair(nil, pub)
	if err == nil {
		t.Error("FromKeyPair() should fail with nil private key")
	}

	// 测试 nil 公钥
	_, err = FromKeyPair(priv, nil)
	if err == nil {
		t.Error("FromKeyPair() should fail with nil public key")
	}

	// 测试两个都是 nil
	_, err = FromKeyPair(nil, nil)
	if err == nil {
		t.Error("FromKeyPair() should fail with nil keys")
	}
}

// TestFromKeyPair_Mismatch 测试密钥对不匹配
func TestFromKeyPair_Mismatch(t *testing.T) {
	priv1, _, _ := GenerateEd25519Key()
	_, pub2, _ := GenerateEd25519Key()

	// 密钥对不匹配应该失败
	_, err := FromKeyPair(priv1, pub2)
	if err == nil {
		t.Error("FromKeyPair() should fail with mismatched key pair")
	}
}

// TestIdentity_String 测试 String 方法
func TestIdentity_String(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	str := id.String()
	if str != id.PeerID() {
		t.Errorf("String() = %s, want %s", str, id.PeerID())
	}
}

// TestIdentity_KeyType 测试 KeyType 方法
func TestIdentity_KeyType(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	keyType := id.KeyType()
	if keyType != pkgif.KeyTypeEd25519 {
		t.Errorf("KeyType() = %v, want Ed25519", keyType)
	}
}

// TestSaveAndLoad 测试保存和加载
func TestSaveAndLoad(t *testing.T) {
	// 创建临时文件
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test_key.pem")

	// 生成身份
	id1, err := Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// 保存
	err = saveIdentityToFile(id1, keyPath)
	if err != nil {
		t.Fatalf("saveIdentityToFile() failed: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("Key file was not created")
	}

	// 加载
	id2, err := loadIdentityFromFile(keyPath)
	if err != nil {
		t.Fatalf("loadIdentityFromFile() failed: %v", err)
	}

	// 验证 PeerID 相同
	if id1.PeerID() != id2.PeerID() {
		t.Errorf("Loaded identity PeerID mismatch: %s != %s", id1.PeerID(), id2.PeerID())
	}

	// 验证密钥相同
	if !id1.PrivateKey().Equals(id2.PrivateKey()) {
		t.Error("Loaded identity private key mismatch")
	}
}

// TestPrivateKeyFromBytes 测试从字节创建私钥
func TestPrivateKeyFromBytes(t *testing.T) {
	priv, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	// 获取原始字节
	privBytes, err := priv.Raw()
	if err != nil {
		t.Fatalf("Raw() failed: %v", err)
	}

	// 从字节重建
	restored, err := PrivateKeyFromBytes(privBytes, pkgif.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes() failed: %v", err)
	}

	// 验证相等
	if !restored.Equals(priv) {
		t.Error("Restored private key does not match original")
	}
}

// TestPublicKeyFromBytes 测试从字节创建公钥
func TestPublicKeyFromBytes(t *testing.T) {
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	// 获取原始字节
	pubBytes, err := pub.Raw()
	if err != nil {
		t.Fatalf("Raw() failed: %v", err)
	}

	// 从字节重建
	restored, err := PublicKeyFromBytes(pubBytes, pkgif.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("PublicKeyFromBytes() failed: %v", err)
	}

	// 验证相等
	if !restored.Equals(pub) {
		t.Error("Restored public key does not match original")
	}
}

// TestPrivateKeyFromBytes_InvalidSize 测试无效密钥大小
func TestPrivateKeyFromBytes_InvalidSize(t *testing.T) {
	// Ed25519 私钥支持 32/64/96 字节，使用其他大小应该失败
	invalidKey := make([]byte, 48) // 无效大小（不是 32/64/96）

	_, err := PrivateKeyFromBytes(invalidKey, pkgif.KeyTypeEd25519)
	if err == nil {
		t.Error("PrivateKeyFromBytes() should fail with invalid key size")
	}
}

// TestPublicKeyFromBytes_InvalidSize 测试无效公钥大小
func TestPublicKeyFromBytes_InvalidSize(t *testing.T) {
	// Ed25519 公钥应该是 32 字节
	invalidKey := make([]byte, 64) // 错误大小

	_, err := PublicKeyFromBytes(invalidKey, pkgif.KeyTypeEd25519)
	if err == nil {
		t.Error("PublicKeyFromBytes() should fail with invalid key size")
	}
}

// TestMarshalUnmarshalPEM 测试 PEM 编码解码
func TestMarshalUnmarshalPEM(t *testing.T) {
	priv, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	// Marshal
	pemBytes, err := MarshalPrivateKeyPEM(priv)
	if err != nil {
		t.Fatalf("MarshalPrivateKeyPEM() failed: %v", err)
	}

	if len(pemBytes) == 0 {
		t.Fatal("MarshalPrivateKeyPEM() returned empty bytes")
	}

	// Unmarshal
	restored, err := UnmarshalPrivateKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("UnmarshalPrivateKeyPEM() failed: %v", err)
	}

	// 验证相等
	if !restored.Equals(priv) {
		t.Error("Unmarshaled key does not match original")
	}
}
