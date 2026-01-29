package crypto

import (
	"crypto/rand"
	"testing"
)

func TestRSA_Generate(t *testing.T) {
	priv, pub, err := GenerateRSAKey(2048, rand.Reader)
	if err != nil {
		t.Fatalf("GenerateRSAKey() error = %v", err)
	}

	if priv.Type() != KeyTypeRSA {
		t.Errorf("PrivateKey.Type() = %v, want %v", priv.Type(), KeyTypeRSA)
	}
	if pub.Type() != KeyTypeRSA {
		t.Errorf("PublicKey.Type() = %v, want %v", pub.Type(), KeyTypeRSA)
	}

	privRaw, err := priv.Raw()
	if err != nil {
		t.Fatalf("PrivateKey.Raw() error = %v", err)
	}
	if len(privRaw) == 0 {
		t.Error("PrivateKey.Raw() returned empty")
	}

	pubRaw, err := pub.Raw()
	if err != nil {
		t.Fatalf("PublicKey.Raw() error = %v", err)
	}
	if len(pubRaw) == 0 {
		t.Error("PublicKey.Raw() returned empty")
	}
}

func TestRSA_Generate_TooSmall(t *testing.T) {
	_, _, err := GenerateRSAKey(1024, rand.Reader)
	if err == nil {
		t.Error("GenerateRSAKey(1024) should return error")
	}
}

func TestRSA_Generate_TooLarge(t *testing.T) {
	_, _, err := GenerateRSAKey(16384, rand.Reader)
	if err == nil {
		t.Error("GenerateRSAKey(16384) should return error")
	}
}

func TestRSA_SignVerify(t *testing.T) {
	priv, pub, _ := GenerateRSAKey(2048, rand.Reader)
	data := []byte("test message for RSA")

	sig, err := priv.Sign(data)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	if len(sig) == 0 {
		t.Error("Sign() returned empty signature")
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
}

func TestRSA_Equals(t *testing.T) {
	priv1, pub1, _ := GenerateRSAKey(2048, rand.Reader)
	priv2, pub2, _ := GenerateRSAKey(2048, rand.Reader)

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

func TestRSA_GetPublic(t *testing.T) {
	priv, pub, _ := GenerateRSAKey(2048, rand.Reader)
	derivedPub := priv.GetPublic()

	if !pub.Equals(derivedPub) {
		t.Error("GetPublic() returned different key")
	}
}

func TestRSA_UnmarshalPublicKey(t *testing.T) {
	_, pub, _ := GenerateRSAKey(2048, rand.Reader)
	raw, _ := pub.Raw()

	pub2, err := UnmarshalRSAPublicKey(raw)
	if err != nil {
		t.Fatalf("UnmarshalRSAPublicKey() error = %v", err)
	}

	if !pub.Equals(pub2) {
		t.Error("Unmarshalled key does not equal original")
	}
}

func TestRSA_UnmarshalPublicKey_Invalid(t *testing.T) {
	_, err := UnmarshalRSAPublicKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalRSAPublicKey(invalid) should return error")
	}
}

func TestRSA_UnmarshalPrivateKey(t *testing.T) {
	priv, _, _ := GenerateRSAKey(2048, rand.Reader)
	raw, _ := priv.Raw()

	priv2, err := UnmarshalRSAPrivateKey(raw)
	if err != nil {
		t.Fatalf("UnmarshalRSAPrivateKey() error = %v", err)
	}

	if !priv.Equals(priv2) {
		t.Error("Unmarshalled key does not equal original")
	}
}

func TestRSA_UnmarshalPrivateKey_Invalid(t *testing.T) {
	_, err := UnmarshalRSAPrivateKey([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalRSAPrivateKey(invalid) should return error")
	}
}

func TestRSA_CrossVerify(t *testing.T) {
	priv1, _, _ := GenerateRSAKey(2048, rand.Reader)
	_, pub2, _ := GenerateRSAKey(2048, rand.Reader)

	data := []byte("test data")
	sig, _ := priv1.Sign(data)

	valid, _ := pub2.Verify(data, sig)
	if valid {
		t.Error("Verify with wrong key should return false")
	}
}

// 注意：RSA 基准测试因为密钥生成慢，所以只测试签名和验证
func BenchmarkRSA_Sign(b *testing.B) {
	priv, _, _ := GenerateRSAKey(2048, rand.Reader)
	data := make([]byte, 256)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = priv.Sign(data)
	}
}

func BenchmarkRSA_Verify(b *testing.B) {
	priv, pub, _ := GenerateRSAKey(2048, rand.Reader)
	data := make([]byte, 256)
	rand.Read(data)
	sig, _ := priv.Sign(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pub.Verify(data, sig)
	}
}
