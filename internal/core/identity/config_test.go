package identity

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 配置测试
// ============================================================================

// TestDefaultConfig 测试默认配置
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if cfg.KeyType != pkgif.KeyTypeEd25519 {
		t.Errorf("DefaultConfig().KeyType = %v, want Ed25519", cfg.KeyType)
	}
}

// TestConfig_CustomKeyType 测试自定义密钥类型
func TestConfig_CustomKeyType(t *testing.T) {
	cfg := &Config{
		KeyType: pkgif.KeyTypeSecp256k1,
	}

	if cfg.KeyType != pkgif.KeyTypeSecp256k1 {
		t.Errorf("Config.KeyType = %v, want Secp256k1", cfg.KeyType)
	}
}

// TestConfig_WithPath 测试配置密钥路径
func TestConfig_WithPath(t *testing.T) {
	cfg := &Config{
		KeyType:     pkgif.KeyTypeEd25519,
		PrivKeyPath: "/path/to/key.pem",
	}

	if cfg.PrivKeyPath != "/path/to/key.pem" {
		t.Errorf("Config.PrivKeyPath = %s, want /path/to/key.pem", cfg.PrivKeyPath)
	}
}
