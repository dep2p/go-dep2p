package types

import (
	"testing"
)

func TestPeerID(t *testing.T) {
	t.Run("ParsePeerID", func(t *testing.T) {
		tests := []struct {
			name    string
			input   string
			wantErr bool
		}{
			{"valid", "12D3KooWTest", false},
			{"empty", "", true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := ParsePeerID(tt.input)
				if (err != nil) != tt.wantErr {
					t.Errorf("ParsePeerID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				}
			})
		}
	})

	t.Run("String", func(t *testing.T) {
		id := PeerID("12D3KooWTest")
		if id.String() != "12D3KooWTest" {
			t.Errorf("PeerID.String() = %q, want %q", id.String(), "12D3KooWTest")
		}
	})

	t.Run("ShortString", func(t *testing.T) {
		// 长 ID：前8...后3
		id := PeerID("12D3KooWTestLongPeerID")
		short := id.ShortString()
		expected := "12D3KooW...rID"
		if short != expected {
			t.Errorf("PeerID.ShortString() = %q, want %q", short, expected)
		}

		// 短 ID：原样返回
		shortID := PeerID("12D3KooW")
		if shortID.ShortString() != "12D3KooW" {
			t.Errorf("短 ID 应原样返回")
		}
	})

	t.Run("IsEmpty", func(t *testing.T) {
		if !EmptyPeerID.IsEmpty() {
			t.Error("EmptyPeerID.IsEmpty() = false, want true")
		}
		id := PeerID("test")
		if id.IsEmpty() {
			t.Error("PeerID(\"test\").IsEmpty() = true, want false")
		}
	})

	t.Run("Equal", func(t *testing.T) {
		id1 := PeerID("test")
		id2 := PeerID("test")
		id3 := PeerID("other")

		if !id1.Equal(id2) {
			t.Error("PeerID.Equal() = false, want true for same IDs")
		}
		if id1.Equal(id3) {
			t.Error("PeerID.Equal() = true, want false for different IDs")
		}
	})
}

func TestPSK(t *testing.T) {
	t.Run("GeneratePSK", func(t *testing.T) {
		psk := GeneratePSK()
		if len(psk) != PSKLength {
			t.Errorf("GeneratePSK() len = %d, want %d", len(psk), PSKLength)
		}
		if psk.IsEmpty() {
			t.Error("GeneratePSK() is empty")
		}
	})

	t.Run("PSKFromBytes", func(t *testing.T) {
		// Valid
		data := make([]byte, PSKLength)
		for i := range data {
			data[i] = byte(i)
		}
		psk, err := PSKFromBytes(data)
		if err != nil {
			t.Errorf("PSKFromBytes() error = %v", err)
		}
		if len(psk) != PSKLength {
			t.Errorf("PSKFromBytes() len = %d, want %d", len(psk), PSKLength)
		}

		// Invalid length
		_, err = PSKFromBytes([]byte{1, 2, 3})
		if err == nil {
			t.Error("PSKFromBytes() with invalid length should return error")
		}

		// Empty
		_, err = PSKFromBytes(nil)
		if err == nil {
			t.Error("PSKFromBytes(nil) should return error")
		}
	})

	t.Run("PSKFromHex", func(t *testing.T) {
		// Valid hex (64 chars = 32 bytes)
		hexStr := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		psk, err := PSKFromHex(hexStr)
		if err != nil {
			t.Errorf("PSKFromHex() error = %v", err)
		}
		if len(psk) != PSKLength {
			t.Errorf("PSKFromHex() len = %d, want %d", len(psk), PSKLength)
		}

		// Invalid length
		_, err = PSKFromHex("0123456789abcdef")
		if err == nil {
			t.Error("PSKFromHex() with invalid length should return error")
		}

		// Invalid hex
		_, err = PSKFromHex("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")
		if err == nil {
			t.Error("PSKFromHex() with invalid hex should return error")
		}
	})

	t.Run("Equal", func(t *testing.T) {
		psk1 := GeneratePSK()
		psk2 := make(PSK, PSKLength)
		copy(psk2, psk1)
		psk3 := GeneratePSK()

		if !psk1.Equal(psk2) {
			t.Error("PSK.Equal() = false, want true for same PSKs")
		}
		if psk1.Equal(psk3) {
			t.Error("PSK.Equal() = true, want false for different PSKs")
		}
	})

	t.Run("DeriveRealmID", func(t *testing.T) {
		psk := GeneratePSK()
		realmID := psk.DeriveRealmID()
		if realmID.IsEmpty() {
			t.Error("PSK.DeriveRealmID() returned empty RealmID")
		}

		// Same PSK should produce same RealmID
		realmID2 := psk.DeriveRealmID()
		if realmID != realmID2 {
			t.Error("PSK.DeriveRealmID() not deterministic")
		}
	})
}

func TestRealmKey(t *testing.T) {
	t.Run("GenerateRealmKey", func(t *testing.T) {
		key := GenerateRealmKey()
		if key.IsEmpty() {
			t.Error("GenerateRealmKey() is empty")
		}
	})

	t.Run("RealmKeyFromBytes", func(t *testing.T) {
		data := make([]byte, 32)
		for i := range data {
			data[i] = byte(i)
		}
		key, err := RealmKeyFromBytes(data)
		if err != nil {
			t.Errorf("RealmKeyFromBytes() error = %v", err)
		}
		if key.IsEmpty() {
			t.Error("RealmKeyFromBytes() returned empty key")
		}

		// Invalid length
		_, err = RealmKeyFromBytes([]byte{1, 2, 3})
		if err == nil {
			t.Error("RealmKeyFromBytes() with invalid length should return error")
		}
	})

	t.Run("DeriveRealmKeyFromName", func(t *testing.T) {
		key1 := DeriveRealmKeyFromName("test-realm")
		key2 := DeriveRealmKeyFromName("test-realm")
		key3 := DeriveRealmKeyFromName("other-realm")

		if key1 != key2 {
			t.Error("DeriveRealmKeyFromName() not deterministic")
		}
		if key1 == key3 {
			t.Error("DeriveRealmKeyFromName() same for different names")
		}
	})

	t.Run("ToPSK", func(t *testing.T) {
		key := GenerateRealmKey()
		psk := key.ToPSK()
		if len(psk) != PSKLength {
			t.Errorf("RealmKey.ToPSK() len = %d, want %d", len(psk), PSKLength)
		}
	})
}

func TestProtocolID(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		proto := ProtocolID("/dep2p/sys/ping/1.0.0")
		if proto.String() != "/dep2p/sys/ping/1.0.0" {
			t.Errorf("ProtocolID.String() = %q", proto.String())
		}
	})

	t.Run("IsEmpty", func(t *testing.T) {
		empty := ProtocolID("")
		if !empty.IsEmpty() {
			t.Error("empty ProtocolID.IsEmpty() = false")
		}
		proto := ProtocolID("/test")
		if proto.IsEmpty() {
			t.Error("ProtocolID.IsEmpty() = true for non-empty")
		}
	})
}

func TestStreamID(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		id := StreamID(12345)
		s := id.String()
		if len(s) != 16 { // 8 bytes * 2 hex chars
			t.Errorf("StreamID.String() len = %d, want 16", len(s))
		}
	})
}

func TestPeerIDSlice(t *testing.T) {
	slice := PeerIDSlice{
		PeerID("c"),
		PeerID("a"),
		PeerID("b"),
	}

	if slice.Len() != 3 {
		t.Errorf("PeerIDSlice.Len() = %d, want 3", slice.Len())
	}

	if !slice.Less(1, 0) { // "a" < "c"
		t.Error("PeerIDSlice.Less() failed")
	}

	slice.Swap(0, 1)
	if slice[0] != PeerID("a") {
		t.Error("PeerIDSlice.Swap() failed")
	}
}

// TestPeerID_XOR 测试 XOR 距离计算
func TestPeerID_XOR(t *testing.T) {
	id1 := PeerID("peer1")
	id2 := PeerID("peer2")
	id3 := PeerID("peer1") // 与 id1 相同

	t.Run("Hash", func(t *testing.T) {
		h1 := id1.Hash()
		h2 := id1.Hash()
		if h1 != h2 {
			t.Error("Hash() 对相同 ID 应返回相同结果")
		}

		h3 := id2.Hash()
		if h1 == h3 {
			t.Error("Hash() 对不同 ID 应返回不同结果")
		}
	})

	t.Run("XOR", func(t *testing.T) {
		// 相同 ID 的 XOR 应为全 0
		xorSame := id1.XOR(id3)
		allZero := true
		for _, b := range xorSame {
			if b != 0 {
				allZero = false
				break
			}
		}
		if !allZero {
			t.Error("相同 ID 的 XOR 应为全 0")
		}

		// 不同 ID 的 XOR 应非 0
		xorDiff := id1.XOR(id2)
		anyNonZero := false
		for _, b := range xorDiff {
			if b != 0 {
				anyNonZero = true
				break
			}
		}
		if !anyNonZero {
			t.Error("不同 ID 的 XOR 应非 0")
		}

		// XOR 对称性：a XOR b == b XOR a
		xor12 := id1.XOR(id2)
		xor21 := id2.XOR(id1)
		if xor12 != xor21 {
			t.Error("XOR 应对称")
		}
	})

	t.Run("DistanceCmp", func(t *testing.T) {
		target := PeerID("target")
		closer := PeerID("targetX") // 可能更接近
		farther := PeerID("zzzzzzz")

		// 自己到自己距离为 0
		cmp := target.DistanceCmp(target, target)
		if cmp != 0 {
			t.Errorf("DistanceCmp(self, self) = %d, want 0", cmp)
		}

		// 与自己的距离最近
		cmp = target.DistanceCmp(target, farther)
		if cmp != -1 {
			t.Errorf("DistanceCmp(self, other) = %d, want -1", cmp)
		}

		// 测试相反顺序
		cmp = target.DistanceCmp(farther, target)
		if cmp != 1 {
			t.Errorf("DistanceCmp(other, self) = %d, want 1", cmp)
		}

		// 确保有传递性
		_ = closer // 抑制未使用警告
	})

	t.Run("CommonPrefixLen", func(t *testing.T) {
		// 相同 ID 应有最大公共前缀
		cpl := id1.CommonPrefixLen(id3)
		if cpl != 256 {
			t.Errorf("相同 ID 的 CommonPrefixLen = %d, want 256", cpl)
		}

		// 不同 ID 应有较小的公共前缀
		cpl = id1.CommonPrefixLen(id2)
		if cpl >= 256 {
			t.Errorf("不同 ID 的 CommonPrefixLen = %d, 应 < 256", cpl)
		}
		if cpl < 0 {
			t.Errorf("CommonPrefixLen 不应为负数: %d", cpl)
		}
	})
}

func BenchmarkPeerIDFromPublicKey(b *testing.B) {
	pubKey := make([]byte, 32)
	for i := range pubKey {
		pubKey[i] = byte(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PeerIDFromPublicKey(pubKey)
	}
}

func BenchmarkGeneratePSK(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GeneratePSK()
	}
}
