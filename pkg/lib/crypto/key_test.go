package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// TestKeyType 测试密钥类型
func TestKeyType(t *testing.T) {
	tests := []struct {
		kt   KeyType
		want string
	}{
		{KeyTypeUnspecified, "Unspecified"},
		{KeyTypeRSA, "RSA"},
		{KeyTypeEd25519, "Ed25519"},
		{KeyTypeSecp256k1, "Secp256k1"},
		{KeyTypeECDSA, "ECDSA"},
		{KeyType(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.kt.String(); got != tt.want {
			t.Errorf("KeyType(%d).String() = %q, want %q", tt.kt, got, tt.want)
		}
	}
}

// TestKeyTypeValues 测试密钥类型值与 proto 对齐
func TestKeyTypeValues(t *testing.T) {
	// 确保值与 pkg/proto/key/key.proto 对齐
	if KeyTypeUnspecified != 0 {
		t.Errorf("KeyTypeUnspecified = %d, want 0", KeyTypeUnspecified)
	}
	if KeyTypeRSA != 1 {
		t.Errorf("KeyTypeRSA = %d, want 1", KeyTypeRSA)
	}
	if KeyTypeEd25519 != 2 {
		t.Errorf("KeyTypeEd25519 = %d, want 2", KeyTypeEd25519)
	}
	if KeyTypeSecp256k1 != 3 {
		t.Errorf("KeyTypeSecp256k1 = %d, want 3", KeyTypeSecp256k1)
	}
	if KeyTypeECDSA != 4 {
		t.Errorf("KeyTypeECDSA = %d, want 4", KeyTypeECDSA)
	}
}

// TestGenerateKeyPair 测试密钥对生成
func TestGenerateKeyPair(t *testing.T) {
	tests := []struct {
		name    string
		keyType KeyType
		wantErr bool
	}{
		{"Ed25519", KeyTypeEd25519, false},
		{"Secp256k1", KeyTypeSecp256k1, false},
		{"ECDSA", KeyTypeECDSA, false},
		{"RSA", KeyTypeRSA, false},
		{"Unknown", KeyType(99), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priv, pub, err := GenerateKeyPair(tt.keyType)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateKeyPair(%v) error = %v, wantErr %v", tt.keyType, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if priv == nil {
					t.Error("GenerateKeyPair() returned nil private key")
				}
				if pub == nil {
					t.Error("GenerateKeyPair() returned nil public key")
				}
				if priv.Type() != tt.keyType {
					t.Errorf("PrivateKey.Type() = %v, want %v", priv.Type(), tt.keyType)
				}
				if pub.Type() != tt.keyType {
					t.Errorf("PublicKey.Type() = %v, want %v", pub.Type(), tt.keyType)
				}
			}
		})
	}
}

// TestSignAndVerify 测试签名和验证
func TestSignAndVerify(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			priv, pub, err := GenerateKeyPair(kt)
			if err != nil {
				t.Fatalf("GenerateKeyPair(%v) failed: %v", kt, err)
			}

			data := []byte("test message for signing")
			sig, err := priv.Sign(data)
			if err != nil {
				t.Fatalf("Sign() failed: %v", err)
			}

			valid, err := pub.Verify(data, sig)
			if err != nil {
				t.Fatalf("Verify() failed: %v", err)
			}
			if !valid {
				t.Error("Verify() returned false for valid signature")
			}

			// 验证错误数据
			badData := []byte("wrong message")
			valid, err = pub.Verify(badData, sig)
			if err != nil {
				t.Fatalf("Verify() with bad data failed: %v", err)
			}
			if valid {
				t.Error("Verify() returned true for invalid data")
			}
		})
	}
}

// TestKeyEqual 测试密钥相等性比较
func TestKeyEqual(t *testing.T) {
	priv1, pub1, _ := GenerateKeyPair(KeyTypeEd25519)
	priv2, pub2, _ := GenerateKeyPair(KeyTypeEd25519)

	// 相同密钥
	if !KeyEqual(pub1, pub1) {
		t.Error("KeyEqual() returned false for same key")
	}

	// 不同密钥
	if KeyEqual(pub1, pub2) {
		t.Error("KeyEqual() returned true for different keys")
	}

	// 私钥比较
	if !KeyEqual(priv1, priv1) {
		t.Error("KeyEqual() returned false for same private key")
	}
	if KeyEqual(priv1, priv2) {
		t.Error("KeyEqual() returned true for different private keys")
	}
}

// TestUnmarshalPublicKey 测试公钥反序列化
func TestUnmarshalPublicKey(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			_, pub, err := GenerateKeyPair(kt)
			if err != nil {
				t.Fatalf("GenerateKeyPair() failed: %v", err)
			}

			raw, err := pub.Raw()
			if err != nil {
				t.Fatalf("Raw() failed: %v", err)
			}

			pub2, err := UnmarshalPublicKey(kt, raw)
			if err != nil {
				t.Fatalf("UnmarshalPublicKey() failed: %v", err)
			}

			if !KeyEqual(pub, pub2) {
				t.Error("Unmarshalled key does not equal original")
			}
		})
	}
}

// TestUnmarshalPrivateKey 测试私钥反序列化
func TestUnmarshalPrivateKey(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			priv, _, err := GenerateKeyPair(kt)
			if err != nil {
				t.Fatalf("GenerateKeyPair() failed: %v", err)
			}

			raw, err := priv.Raw()
			if err != nil {
				t.Fatalf("Raw() failed: %v", err)
			}

			priv2, err := UnmarshalPrivateKey(kt, raw)
			if err != nil {
				t.Fatalf("UnmarshalPrivateKey() failed: %v", err)
			}

			if !KeyEqual(priv, priv2) {
				t.Error("Unmarshalled key does not equal original")
			}
		})
	}
}

// TestGetPublic 测试从私钥获取公钥
func TestGetPublic(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			priv, pub, err := GenerateKeyPair(kt)
			if err != nil {
				t.Fatalf("GenerateKeyPair() failed: %v", err)
			}

			derivedPub := priv.GetPublic()
			if !KeyEqual(pub, derivedPub) {
				t.Error("GetPublic() returned different key than GenerateKeyPair()")
			}
		})
	}
}

// TestDeterministicGeneration 测试确定性密钥生成
func TestDeterministicGeneration(t *testing.T) {
	seed := make([]byte, 64)
	for i := range seed {
		seed[i] = byte(i)
	}

	// 使用相同种子应生成相同密钥
	reader1 := bytes.NewReader(seed)
	reader2 := bytes.NewReader(seed)

	priv1, _, err := GenerateKeyPairWithReader(KeyTypeEd25519, reader1)
	if err != nil {
		t.Fatalf("GenerateKeyPairWithReader() failed: %v", err)
	}

	priv2, _, err := GenerateKeyPairWithReader(KeyTypeEd25519, reader2)
	if err != nil {
		t.Fatalf("GenerateKeyPairWithReader() failed: %v", err)
	}

	if !KeyEqual(priv1, priv2) {
		t.Error("Deterministic generation produced different keys")
	}
}

// BenchmarkGenerateKeyPair 基准测试密钥生成
func BenchmarkGenerateKeyPair(b *testing.B) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA}

	for _, kt := range keyTypes {
		b.Run(kt.String(), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, _ = GenerateKeyPair(kt)
			}
		})
	}
}

// BenchmarkSign 基准测试签名
func BenchmarkSign(b *testing.B) {
	data := make([]byte, 256)
	rand.Read(data)

	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA}

	for _, kt := range keyTypes {
		priv, _, _ := GenerateKeyPair(kt)
		b.Run(kt.String(), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = priv.Sign(data)
			}
		})
	}
}

// BenchmarkVerify 基准测试验证
func BenchmarkVerify(b *testing.B) {
	data := make([]byte, 256)
	rand.Read(data)

	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA}

	for _, kt := range keyTypes {
		priv, pub, _ := GenerateKeyPair(kt)
		sig, _ := priv.Sign(data)
		b.Run(kt.String(), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = pub.Verify(data, sig)
			}
		})
	}
}
