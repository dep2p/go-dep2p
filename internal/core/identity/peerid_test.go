package identity

import (
	"testing"
)

// ============================================================================
// PeerID 派生测试
// ============================================================================

// TestPeerID_FromPublicKey 测试从公钥派生 PeerID
func TestPeerID_FromPublicKey(t *testing.T) {
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	peerID, err := PeerIDFromPublicKey(pub)
	if err != nil {
		t.Errorf("PeerIDFromPublicKey() failed: %v", err)
	}

	if peerID == "" {
		t.Error("PeerIDFromPublicKey() returned empty string")
	}

	// 验证 PeerID 稳定性（同一公钥应得到同一 PeerID）
	peerID2, err := PeerIDFromPublicKey(pub)
	if err != nil {
		t.Errorf("PeerIDFromPublicKey() failed: %v", err)
	}

	if peerID != peerID2 {
		t.Errorf("PeerID unstable: %s != %s", peerID, peerID2)
	}
}

// TestPeerID_DifferentKeys 测试不同密钥生成不同 PeerID
func TestPeerID_DifferentKeys(t *testing.T) {
	_, pub1, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	_, pub2, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	peerID1, _ := PeerIDFromPublicKey(pub1)
	peerID2, _ := PeerIDFromPublicKey(pub2)

	if peerID1 == peerID2 {
		t.Error("Different keys generated same PeerID")
	}
}

// TestPeerID_String 测试 PeerID Base58 编码
func TestPeerID_String(t *testing.T) {
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	peerID, err := PeerIDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("PeerIDFromPublicKey() failed: %v", err)
	}

	// PeerID 应该是 Base58 编码
	// 通常以 "12D3KooW" 开头（Multihash 前缀）
	if len(peerID) < 30 {
		t.Errorf("PeerID too short: %s (len=%d)", peerID, len(peerID))
	}

	// 应该只包含 Base58 字符
	// Base58: 123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz
	// （不包含 0, O, I, l）
	for _, ch := range peerID {
		valid := (ch >= '1' && ch <= '9') ||
			(ch >= 'A' && ch <= 'H') ||
			(ch >= 'J' && ch <= 'N') ||
			(ch >= 'P' && ch <= 'Z') ||
			(ch >= 'a' && ch <= 'k') ||
			(ch >= 'm' && ch <= 'z')

		if !valid {
			t.Errorf("PeerID contains invalid Base58 character: %c", ch)
			break
		}
	}
}

// TestPeerID_Validate 测试 PeerID 验证
func TestPeerID_Validate(t *testing.T) {
	// 生成有效的 PeerID
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	validPeerID, err := PeerIDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("PeerIDFromPublicKey() failed: %v", err)
	}

	testCases := []struct {
		peerID string
		valid  bool
		name   string
	}{
		{validPeerID, true, "valid PeerID"},
		{"", false, "empty string"},
		{"invalid!!!", false, "invalid format"},
	}

	for _, tc := range testCases {
		err := ValidatePeerID(tc.peerID)
		if tc.valid && err != nil {
			t.Errorf("%s: ValidatePeerID() failed for valid PeerID: %v", tc.name, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("%s: ValidatePeerID() succeeded for invalid PeerID", tc.name)
		}
	}
}

// TestParsePeerID 测试 PeerID 解析
func TestParsePeerID(t *testing.T) {
	_, pub, err := GenerateEd25519Key()
	if err != nil {
		t.Fatalf("GenerateEd25519Key() failed: %v", err)
	}

	validPeerID, err := PeerIDFromPublicKey(pub)
	if err != nil {
		t.Fatalf("PeerIDFromPublicKey() failed: %v", err)
	}

	// 解析有效 PeerID
	parsed, err := ParsePeerID(validPeerID)
	if err != nil {
		t.Errorf("ParsePeerID() failed: %v", err)
	}

	if parsed != validPeerID {
		t.Errorf("ParsePeerID() = %s, want %s", parsed, validPeerID)
	}

	// 解析无效 PeerID
	_, err = ParsePeerID("invalid")
	if err == nil {
		t.Error("ParsePeerID() should fail for invalid PeerID")
	}
}

// ============================================================================
// 基准测试
// ============================================================================

// BenchmarkPeerID_FromPublicKey PeerID 派生性能
func BenchmarkPeerID_FromPublicKey(b *testing.B) {
	_, pub, _ := GenerateEd25519Key()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := PeerIDFromPublicKey(pub)
		if err != nil {
			b.Fatalf("PeerIDFromPublicKey() failed: %v", err)
		}
	}
}
