package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEd25519_Generate(t *testing.T) {
	priv, pub, err := GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateEd25519Key() error = %v", err)
	}

	if priv.Type() != KeyTypeEd25519 {
		t.Errorf("PrivateKey.Type() = %v, want %v", priv.Type(), KeyTypeEd25519)
	}
	if pub.Type() != KeyTypeEd25519 {
		t.Errorf("PublicKey.Type() = %v, want %v", pub.Type(), KeyTypeEd25519)
	}

	privRaw, _ := priv.Raw()
	if len(privRaw) != Ed25519PrivateKeySize {
		t.Errorf("PrivateKey.Raw() len = %d, want %d", len(privRaw), Ed25519PrivateKeySize)
	}

	pubRaw, _ := pub.Raw()
	if len(pubRaw) != Ed25519PublicKeySize {
		t.Errorf("PublicKey.Raw() len = %d, want %d", len(pubRaw), Ed25519PublicKeySize)
	}
}

func TestEd25519_SignVerify(t *testing.T) {
	priv, pub, _ := GenerateEd25519Key(rand.Reader)
	data := []byte("test message")

	sig, err := priv.Sign(data)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	if len(sig) != Ed25519SignatureSize {
		t.Errorf("Sign() len = %d, want %d", len(sig), Ed25519SignatureSize)
	}

	valid, err := pub.Verify(data, sig)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !valid {
		t.Error("Verify() = false, want true")
	}

	// 验证错误数据
	badData := []byte("wrong message")
	valid, _ = pub.Verify(badData, sig)
	if valid {
		t.Error("Verify(badData) = true, want false")
	}

	// 验证错误签名
	badSig := make([]byte, Ed25519SignatureSize)
	valid, _ = pub.Verify(data, badSig)
	if valid {
		t.Error("Verify(badSig) = true, want false")
	}

	// 验证短签名
	valid, _ = pub.Verify(data, []byte{1, 2, 3})
	if valid {
		t.Error("Verify(shortSig) = true, want false")
	}
}

func TestEd25519_Equals(t *testing.T) {
	priv1, pub1, _ := GenerateEd25519Key(rand.Reader)
	priv2, pub2, _ := GenerateEd25519Key(rand.Reader)

	// 相同密钥
	if !priv1.Equals(priv1) {
		t.Error("priv1.Equals(priv1) = false")
	}
	if !pub1.Equals(pub1) {
		t.Error("pub1.Equals(pub1) = false")
	}

	// 不同密钥
	if priv1.Equals(priv2) {
		t.Error("priv1.Equals(priv2) = true")
	}
	if pub1.Equals(pub2) {
		t.Error("pub1.Equals(pub2) = true")
	}
}

func TestEd25519_GetPublic(t *testing.T) {
	priv, pub, _ := GenerateEd25519Key(rand.Reader)
	derivedPub := priv.GetPublic()

	if !pub.Equals(derivedPub) {
		t.Error("GetPublic() returned different key")
	}
}

func TestEd25519_UnmarshalPublicKey(t *testing.T) {
	_, pub, _ := GenerateEd25519Key(rand.Reader)
	raw, _ := pub.Raw()

	pub2, err := UnmarshalEd25519PublicKey(raw)
	if err != nil {
		t.Fatalf("UnmarshalEd25519PublicKey() error = %v", err)
	}

	if !pub.Equals(pub2) {
		t.Error("Unmarshalled key does not equal original")
	}
}

func TestEd25519_UnmarshalPublicKey_InvalidSize(t *testing.T) {
	_, err := UnmarshalEd25519PublicKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalEd25519PublicKey(invalidSize) should return error")
	}
}

func TestEd25519_UnmarshalPrivateKey(t *testing.T) {
	priv, _, _ := GenerateEd25519Key(rand.Reader)
	raw, _ := priv.Raw()

	t.Run("64 bytes", func(t *testing.T) {
		priv2, err := UnmarshalEd25519PrivateKey(raw)
		if err != nil {
			t.Fatalf("UnmarshalEd25519PrivateKey() error = %v", err)
		}
		if !priv.Equals(priv2) {
			t.Error("Unmarshalled key does not equal original")
		}
	})

	t.Run("32 bytes seed", func(t *testing.T) {
		ed25519Priv := priv.(*Ed25519PrivateKey)
		seed := ed25519Priv.Seed()

		priv2, err := UnmarshalEd25519PrivateKey(seed)
		if err != nil {
			t.Fatalf("UnmarshalEd25519PrivateKey(seed) error = %v", err)
		}
		if !priv.Equals(priv2) {
			t.Error("Unmarshalled from seed does not equal original")
		}
	})

	t.Run("96 bytes with redundant pubkey", func(t *testing.T) {
		// 创建 96 字节格式：64 字节私钥 + 32 字节冗余公钥
		pubRaw, _ := priv.GetPublic().Raw()
		data96 := append(raw, pubRaw...)

		priv2, err := UnmarshalEd25519PrivateKey(data96)
		if err != nil {
			t.Fatalf("UnmarshalEd25519PrivateKey(96) error = %v", err)
		}
		if !priv.Equals(priv2) {
			t.Error("Unmarshalled from 96 bytes does not equal original")
		}
	})
}

func TestEd25519_UnmarshalPrivateKey_InvalidSize(t *testing.T) {
	_, err := UnmarshalEd25519PrivateKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalEd25519PrivateKey(invalidSize) should return error")
	}
}

func TestEd25519_DeterministicGeneration(t *testing.T) {
	seed := make([]byte, 64)
	for i := range seed {
		seed[i] = byte(i)
	}

	reader1 := bytes.NewReader(seed)
	reader2 := bytes.NewReader(seed)

	priv1, _, _ := GenerateEd25519Key(reader1)
	priv2, _, _ := GenerateEd25519Key(reader2)

	if !priv1.Equals(priv2) {
		t.Error("Deterministic generation produced different keys")
	}
}

func BenchmarkEd25519_Generate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, _ = GenerateEd25519Key(rand.Reader)
	}
}

func BenchmarkEd25519_Sign(b *testing.B) {
	priv, _, _ := GenerateEd25519Key(rand.Reader)
	data := make([]byte, 256)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = priv.Sign(data)
	}
}

func BenchmarkEd25519_Verify(b *testing.B) {
	priv, pub, _ := GenerateEd25519Key(rand.Reader)
	data := make([]byte, 256)
	rand.Read(data)
	sig, _ := priv.Sign(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pub.Verify(data, sig)
	}
}
