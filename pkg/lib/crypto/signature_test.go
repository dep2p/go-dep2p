package crypto

import (
	"crypto/rand"
	"testing"
)

func TestSign(t *testing.T) {
	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)
	data := []byte("test message")

	sig, err := Sign(priv, data)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	if sig == nil {
		t.Fatal("Sign() returned nil signature")
	}
	if sig.Type != KeyTypeEd25519 {
		t.Errorf("Sign() type = %v, want %v", sig.Type, KeyTypeEd25519)
	}
	if len(sig.Data) == 0 {
		t.Error("Sign() returned empty signature data")
	}
}

func TestSign_NilKey(t *testing.T) {
	_, err := Sign(nil, []byte("test"))
	if err == nil {
		t.Error("Sign(nil) should return error")
	}
}

func TestVerify(t *testing.T) {
	priv, pub, _ := GenerateKeyPair(KeyTypeEd25519)
	data := []byte("test message")

	sig, _ := Sign(priv, data)

	valid, err := Verify(pub, data, sig)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !valid {
		t.Error("Verify() = false, want true")
	}
}

func TestVerify_BadData(t *testing.T) {
	priv, pub, _ := GenerateKeyPair(KeyTypeEd25519)
	data := []byte("test message")
	badData := []byte("wrong message")

	sig, _ := Sign(priv, data)

	valid, err := Verify(pub, badData, sig)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if valid {
		t.Error("Verify(badData) = true, want false")
	}
}

func TestVerify_NilKey(t *testing.T) {
	_, err := Verify(nil, []byte("test"), &Signature{})
	if err == nil {
		t.Error("Verify(nil key) should return error")
	}
}

func TestVerify_NilSignature(t *testing.T) {
	_, pub, _ := GenerateKeyPair(KeyTypeEd25519)
	_, err := Verify(pub, []byte("test"), nil)
	if err == nil {
		t.Error("Verify(nil sig) should return error")
	}
}

func TestVerify_TypeMismatch(t *testing.T) {
	_, pub, _ := GenerateKeyPair(KeyTypeEd25519)
	sig := &Signature{Type: KeyTypeSecp256k1, Data: []byte("fake")}

	_, err := Verify(pub, []byte("test"), sig)
	if err == nil {
		t.Error("Verify(type mismatch) should return error")
	}
}

func TestSignedRecord(t *testing.T) {
	priv, pub, _ := GenerateKeyPair(KeyTypeEd25519)

	record, err := CreateSignedRecord(priv, "peer123", 1, []byte("data"))
	if err != nil {
		t.Fatalf("CreateSignedRecord() error = %v", err)
	}

	if record.PeerID != "peer123" {
		t.Errorf("PeerID = %q, want %q", record.PeerID, "peer123")
	}
	if record.Seq != 1 {
		t.Errorf("Seq = %d, want 1", record.Seq)
	}

	valid, err := VerifySignedRecord(pub, record)
	if err != nil {
		t.Fatalf("VerifySignedRecord() error = %v", err)
	}
	if !valid {
		t.Error("VerifySignedRecord() = false, want true")
	}
}

func TestVerifySignedRecord_NilRecord(t *testing.T) {
	_, pub, _ := GenerateKeyPair(KeyTypeEd25519)
	_, err := VerifySignedRecord(pub, nil)
	if err == nil {
		t.Error("VerifySignedRecord(nil) should return error")
	}
}

func TestSignedEnvelope(t *testing.T) {
	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)

	typeHint := []byte("test/envelope/v1")
	contents := []byte("envelope contents")

	envelope, err := Seal(priv, typeHint, contents)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	if string(envelope.TypeHint) != string(typeHint) {
		t.Errorf("TypeHint = %q, want %q", envelope.TypeHint, typeHint)
	}

	opened, err := envelope.Open()
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if string(opened) != string(contents) {
		t.Errorf("Open() = %q, want %q", opened, contents)
	}
}

func TestSignedEnvelope_InvalidSignature(t *testing.T) {
	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)

	envelope, _ := Seal(priv, []byte("type"), []byte("contents"))
	// 破坏签名
	envelope.Signature.Data[0] ^= 0xFF

	_, err := envelope.Open()
	if err == nil {
		t.Error("Open() with invalid signature should return error")
	}
}

func TestUint64ToBytes(t *testing.T) {
	tests := []struct {
		input uint64
		want  []byte
	}{
		{0, []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{1, []byte{0, 0, 0, 0, 0, 0, 0, 1}},
		{256, []byte{0, 0, 0, 0, 0, 0, 1, 0}},
		{0xFFFFFFFFFFFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
	}

	for _, tt := range tests {
		got := uint64ToBytes(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("uint64ToBytes(%d) len = %d, want %d", tt.input, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("uint64ToBytes(%d)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func BenchmarkSignature_Sign(b *testing.B) {
	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)
	data := make([]byte, 256)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Sign(priv, data)
	}
}

func BenchmarkSignature_Verify(b *testing.B) {
	priv, pub, _ := GenerateKeyPair(KeyTypeEd25519)
	data := make([]byte, 256)
	rand.Read(data)
	sig, _ := Sign(priv, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Verify(pub, data, sig)
	}
}
