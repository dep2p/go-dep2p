package identity

import (
	"os"
	"path/filepath"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// ProvideIdentity 测试
// ============================================================================

// TestProvideIdentity_DefaultConfig 测试默认配置
func TestProvideIdentity_DefaultConfig(t *testing.T) {
	params := Params{
		Config: nil, // 使用默认配置
	}

	result, err := ProvideIdentity(params)
	if err != nil {
		t.Fatalf("ProvideIdentity() failed: %v", err)
	}

	if result.Identity == nil {
		t.Error("ProvideIdentity() returned nil Identity")
	}

	if result.Identity.PeerID() == "" {
		t.Error("ProvideIdentity() Identity has empty PeerID")
	}
}

// TestProvideIdentity_AutoCreate 测试自动创建
func TestProvideIdentity_AutoCreate(t *testing.T) {
	cfg := &Config{
		KeyType:    pkgif.KeyTypeEd25519,
		AutoCreate: true,
	}

	params := Params{Config: cfg}

	result, err := ProvideIdentity(params)
	if err != nil {
		t.Fatalf("ProvideIdentity() failed: %v", err)
	}

	if result.Identity == nil {
		t.Error("ProvideIdentity() returned nil Identity")
	}
}

// TestProvideIdentity_WithPath 测试指定路径
func TestProvideIdentity_WithPath(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "identity.pem")

	cfg := &Config{
		KeyType:      pkgif.KeyTypeEd25519,
		PrivKeyPath:  keyPath,
		AutoCreate:   true,
	}

	params := Params{Config: cfg}

	// 第一次调用应该创建新身份并保存
	result1, err := ProvideIdentity(params)
	if err != nil {
		t.Fatalf("ProvideIdentity() failed: %v", err)
	}

	peerID1 := result1.Identity.PeerID()

	// 验证文件已创建
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Fatal("Key file was not created")
	}

	// 第二次调用应该加载现有身份
	result2, err := ProvideIdentity(params)
	if err != nil {
		t.Fatalf("ProvideIdentity() second call failed: %v", err)
	}

	peerID2 := result2.Identity.PeerID()

	// PeerID 应该相同
	if peerID1 != peerID2 {
		t.Errorf("PeerID mismatch after reload: %s != %s", peerID1, peerID2)
	}
}

// TestProvideIdentity_LoadExisting 测试加载现有身份
func TestProvideIdentity_LoadExisting(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "existing.pem")

	// 先创建并保存一个身份
	id1, err := Generate()
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	err = saveIdentityToFile(id1, keyPath)
	if err != nil {
		t.Fatalf("saveIdentityToFile() failed: %v", err)
	}

	// 配置加载现有身份
	cfg := &Config{
		KeyType:     pkgif.KeyTypeEd25519,
		PrivKeyPath: keyPath,
	}

	params := Params{Config: cfg}

	result, err := ProvideIdentity(params)
	if err != nil {
		t.Fatalf("ProvideIdentity() failed: %v", err)
	}

	// 验证加载的身份与原身份相同
	if result.Identity.PeerID() != id1.PeerID() {
		t.Errorf("Loaded PeerID mismatch: %s != %s", result.Identity.PeerID(), id1.PeerID())
	}
}

// TestProvideIdentity_NoConfigNoAutoCreate 测试无配置且不自动创建
func TestProvideIdentity_NoConfigNoAutoCreate(t *testing.T) {
	// 创建一个不存在的文件路径
	tmpDir := t.TempDir()
	nonExistPath := filepath.Join(tmpDir, "nonexistent.pem")

	cfg := &Config{
		KeyType:     pkgif.KeyTypeEd25519,
		PrivKeyPath: nonExistPath,
		AutoCreate:  false, // 不自动创建
	}

	params := Params{Config: cfg}

	_, err := ProvideIdentity(params)
	if err == nil {
		t.Error("ProvideIdentity() should fail when loading non-existent file with AutoCreate=false")
	}
}
