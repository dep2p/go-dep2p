package identity

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 接口契约测试
// ============================================================================

// TestIdentity_ImplementsInterface 验证 Identity 实现接口
func TestIdentity_ImplementsInterface(t *testing.T) {
	var _ pkgif.Identity = (*Identity)(nil)
}

// TestIdentity_New 测试创建身份
func TestIdentity_New(t *testing.T) {
	// 生成密钥对
	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	// 创建身份
	id, err := New(priv)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// 验证 PeerID 非空
	if id.PeerID() == "" {
		t.Error("PeerID() returned empty string")
	}

	// 验证公钥一致
	if !id.PublicKey().Equals(pub) {
		t.Error("PublicKey() does not match generated key")
	}

	// 验证私钥一致
	if !id.PrivateKey().Equals(priv) {
		t.Error("PrivateKey() does not match generated key")
	}
}

// TestIdentity_PeerID 测试 PeerID 派生
func TestIdentity_PeerID(t *testing.T) {
	priv, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	id, err := New(priv)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// PeerID 应该稳定
	peerID1 := id.PeerID()
	peerID2 := id.PeerID()

	if peerID1 != peerID2 {
		t.Errorf("PeerID() unstable: %s != %s", peerID1, peerID2)
	}

	// PeerID 长度应该合理（Base58 编码的 Multihash）
	if len(peerID1) < 30 {
		t.Errorf("PeerID() too short: %s", peerID1)
	}
}

// TestIdentity_Sign 测试签名操作
func TestIdentity_Sign(t *testing.T) {
	priv, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	id, err := New(priv)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	data := []byte("test data")
	sig, err := id.Sign(data)
	if err != nil {
		t.Errorf("Sign() failed: %v", err)
	}

	if len(sig) == 0 {
		t.Error("Sign() returned empty signature")
	}

	// Ed25519 签名应该是 64 字节
	if len(sig) != 64 {
		t.Errorf("Sign() signature length = %d, want 64", len(sig))
	}
}

// TestIdentity_Verify 测试验证签名
func TestIdentity_Verify(t *testing.T) {
	priv, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	id, err := New(priv)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	data := []byte("test data")
	sig, err := id.Sign(data)
	if err != nil {
		t.Fatalf("Sign() failed: %v", err)
	}

	// 验证正确的签名
	valid, err := id.Verify(data, sig)
	if err != nil {
		t.Errorf("Verify() failed: %v", err)
	}

	if !valid {
		t.Error("Verify() returned false for valid signature")
	}

	// 验证错误的签名
	invalidSig := make([]byte, 64)
	valid, err = id.Verify(data, invalidSig)
	if err != nil {
		t.Errorf("Verify() failed: %v", err)
	}

	if valid {
		t.Error("Verify() returned true for invalid signature")
	}
}

// TestIdentity_SignVerifyRoundTrip 测试签名-验证循环
func TestIdentity_SignVerifyRoundTrip(t *testing.T) {
	priv, _, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	id, err := New(priv)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	testCases := [][]byte{
		[]byte("hello world"),
		[]byte(""),
		[]byte("中文测试"),
		make([]byte, 1024), // 大数据
	}

	for i, data := range testCases {
		sig, err := id.Sign(data)
		if err != nil {
			t.Errorf("case %d: Sign() failed: %v", i, err)
			continue
		}

		valid, err := id.Verify(data, sig)
		if err != nil {
			t.Errorf("case %d: Verify() failed: %v", i, err)
			continue
		}

		if !valid {
			t.Errorf("case %d: Verify() failed for valid signature", i)
		}
	}
}
