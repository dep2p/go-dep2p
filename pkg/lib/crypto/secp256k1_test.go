package crypto

import (
	"crypto/rand"
	"testing"
)

func TestSecp256k1_Generate(t *testing.T) {
	priv, pub, err := GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateSecp256k1Key() error = %v", err)
	}

	if priv.Type() != KeyTypeSecp256k1 {
		t.Errorf("PrivateKey.Type() = %v, want %v", priv.Type(), KeyTypeSecp256k1)
	}
	if pub.Type() != KeyTypeSecp256k1 {
		t.Errorf("PublicKey.Type() = %v, want %v", pub.Type(), KeyTypeSecp256k1)
	}

	privRaw, _ := priv.Raw()
	if len(privRaw) != Secp256k1PrivateKeySize {
		t.Errorf("PrivateKey.Raw() len = %d, want %d", len(privRaw), Secp256k1PrivateKeySize)
	}

	pubRaw, _ := pub.Raw()
	if len(pubRaw) != Secp256k1PublicKeySize {
		t.Errorf("PublicKey.Raw() len = %d, want %d", len(pubRaw), Secp256k1PublicKeySize)
	}
}

func TestSecp256k1_SignVerify(t *testing.T) {
	priv, pub, _ := GenerateSecp256k1Key(rand.Reader)
	data := []byte("test message for secp256k1")

	sig, err := priv.Sign(data)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	if len(sig) != 64 {
		t.Errorf("Sign() len = %d, want 64", len(sig))
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

	// 验证短签名
	valid, _ = pub.Verify(data, []byte{1, 2, 3})
	if valid {
		t.Error("Verify(shortSig) = true, want false")
	}
}

func TestSecp256k1_Equals(t *testing.T) {
	priv1, pub1, _ := GenerateSecp256k1Key(rand.Reader)
	priv2, pub2, _ := GenerateSecp256k1Key(rand.Reader)

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

func TestSecp256k1_GetPublic(t *testing.T) {
	priv, pub, _ := GenerateSecp256k1Key(rand.Reader)
	derivedPub := priv.GetPublic()

	if !pub.Equals(derivedPub) {
		t.Error("GetPublic() returned different key")
	}
}

func TestSecp256k1_UnmarshalPublicKey(t *testing.T) {
	_, pub, _ := GenerateSecp256k1Key(rand.Reader)

	t.Run("compressed", func(t *testing.T) {
		raw, _ := pub.Raw()
		if len(raw) != Secp256k1PublicKeySize {
			t.Fatalf("Raw() len = %d, want %d", len(raw), Secp256k1PublicKeySize)
		}

		pub2, err := UnmarshalSecp256k1PublicKey(raw)
		if err != nil {
			t.Fatalf("UnmarshalSecp256k1PublicKey() error = %v", err)
		}

		if !pub.Equals(pub2) {
			t.Error("Unmarshalled key does not equal original")
		}
	})
}

func TestSecp256k1_UnmarshalPublicKey_InvalidSize(t *testing.T) {
	_, err := UnmarshalSecp256k1PublicKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalSecp256k1PublicKey(invalidSize) should return error")
	}
}

func TestSecp256k1_UnmarshalPrivateKey(t *testing.T) {
	priv, _, _ := GenerateSecp256k1Key(rand.Reader)
	raw, _ := priv.Raw()

	priv2, err := UnmarshalSecp256k1PrivateKey(raw)
	if err != nil {
		t.Fatalf("UnmarshalSecp256k1PrivateKey() error = %v", err)
	}

	if !priv.Equals(priv2) {
		t.Error("Unmarshalled key does not equal original")
	}
}

func TestSecp256k1_UnmarshalPrivateKey_InvalidSize(t *testing.T) {
	_, err := UnmarshalSecp256k1PrivateKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalSecp256k1PrivateKey(invalidSize) should return error")
	}
}

func TestSecp256k1_CrossVerify(t *testing.T) {
	// 测试：用一个密钥签名，用另一个密钥验证应该失败
	priv1, _, _ := GenerateSecp256k1Key(rand.Reader)
	_, pub2, _ := GenerateSecp256k1Key(rand.Reader)

	data := []byte("test data")
	sig, _ := priv1.Sign(data)

	valid, _ := pub2.Verify(data, sig)
	if valid {
		t.Error("Verify with wrong key should return false")
	}
}

func BenchmarkSecp256k1_Generate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, _ = GenerateSecp256k1Key(rand.Reader)
	}
}

func BenchmarkSecp256k1_Sign(b *testing.B) {
	priv, _, _ := GenerateSecp256k1Key(rand.Reader)
	data := make([]byte, 256)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = priv.Sign(data)
	}
}

func BenchmarkSecp256k1_Verify(b *testing.B) {
	priv, pub, _ := GenerateSecp256k1Key(rand.Reader)
	data := make([]byte, 256)
	rand.Read(data)
	sig, _ := priv.Sign(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pub.Verify(data, sig)
	}
}
