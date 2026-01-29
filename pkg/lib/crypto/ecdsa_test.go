package crypto

import (
	"crypto/rand"
	"testing"
)

func TestECDSA_Generate(t *testing.T) {
	priv, pub, err := GenerateECDSAKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateECDSAKey() error = %v", err)
	}

	if priv.Type() != KeyTypeECDSA {
		t.Errorf("PrivateKey.Type() = %v, want %v", priv.Type(), KeyTypeECDSA)
	}
	if pub.Type() != KeyTypeECDSA {
		t.Errorf("PublicKey.Type() = %v, want %v", pub.Type(), KeyTypeECDSA)
	}

	privRaw, _ := priv.Raw()
	if len(privRaw) != ECDSAPrivateKeySize {
		t.Errorf("PrivateKey.Raw() len = %d, want %d", len(privRaw), ECDSAPrivateKeySize)
	}

	pubRaw, _ := pub.Raw()
	if len(pubRaw) != ECDSAPublicKeySize {
		t.Errorf("PublicKey.Raw() len = %d, want %d", len(pubRaw), ECDSAPublicKeySize)
	}
}

func TestECDSA_SignVerify(t *testing.T) {
	priv, pub, _ := GenerateECDSAKey(rand.Reader)
	data := []byte("test message for ECDSA")

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

func TestECDSA_Equals(t *testing.T) {
	priv1, pub1, _ := GenerateECDSAKey(rand.Reader)
	priv2, pub2, _ := GenerateECDSAKey(rand.Reader)

	if !priv1.Equals(priv1) {
		t.Error("priv1.Equals(priv1) = false")
	}
	if !pub1.Equals(pub1) {
		t.Error("pub1.Equals(pub1) = false")
	}

	if priv1.Equals(priv2) {
		t.Error("priv1.Equals(priv2) = true")
	}
	if pub1.Equals(pub2) {
		t.Error("pub1.Equals(pub2) = true")
	}
}

func TestECDSA_GetPublic(t *testing.T) {
	priv, pub, _ := GenerateECDSAKey(rand.Reader)
	derivedPub := priv.GetPublic()

	if !pub.Equals(derivedPub) {
		t.Error("GetPublic() returned different key")
	}
}

func TestECDSA_UnmarshalPublicKey(t *testing.T) {
	_, pub, _ := GenerateECDSAKey(rand.Reader)

	t.Run("compressed", func(t *testing.T) {
		raw, _ := pub.Raw()
		if len(raw) != ECDSAPublicKeySize {
			t.Fatalf("Raw() len = %d, want %d", len(raw), ECDSAPublicKeySize)
		}

		pub2, err := UnmarshalECDSAPublicKey(raw)
		if err != nil {
			t.Fatalf("UnmarshalECDSAPublicKey() error = %v", err)
		}

		if !pub.Equals(pub2) {
			t.Error("Unmarshalled key does not equal original")
		}
	})
}

func TestECDSA_UnmarshalPublicKey_InvalidSize(t *testing.T) {
	_, err := UnmarshalECDSAPublicKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalECDSAPublicKey(invalidSize) should return error")
	}
}

func TestECDSA_UnmarshalPrivateKey(t *testing.T) {
	priv, _, _ := GenerateECDSAKey(rand.Reader)
	raw, _ := priv.Raw()

	priv2, err := UnmarshalECDSAPrivateKey(raw)
	if err != nil {
		t.Fatalf("UnmarshalECDSAPrivateKey() error = %v", err)
	}

	if !priv.Equals(priv2) {
		t.Error("Unmarshalled key does not equal original")
	}
}

func TestECDSA_CrossVerify(t *testing.T) {
	priv1, _, _ := GenerateECDSAKey(rand.Reader)
	_, pub2, _ := GenerateECDSAKey(rand.Reader)

	data := []byte("test data")
	sig, _ := priv1.Sign(data)

	valid, _ := pub2.Verify(data, sig)
	if valid {
		t.Error("Verify with wrong key should return false")
	}
}

func BenchmarkECDSA_Generate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, _ = GenerateECDSAKey(rand.Reader)
	}
}

func BenchmarkECDSA_Sign(b *testing.B) {
	priv, _, _ := GenerateECDSAKey(rand.Reader)
	data := make([]byte, 256)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = priv.Sign(data)
	}
}

func BenchmarkECDSA_Verify(b *testing.B) {
	priv, pub, _ := GenerateECDSAKey(rand.Reader)
	data := make([]byte, 256)
	rand.Read(data)
	sig, _ := priv.Sign(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pub.Verify(data, sig)
	}
}
