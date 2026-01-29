package crypto

import (
	"testing"
)

func TestMarshalPublicKey(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			_, pub, err := GenerateKeyPair(kt)
			if err != nil {
				t.Fatalf("GenerateKeyPair() error = %v", err)
			}

			data, err := MarshalPublicKey(pub)
			if err != nil {
				t.Fatalf("MarshalPublicKey() error = %v", err)
			}

			if len(data) < marshalHeaderSize {
				t.Errorf("MarshalPublicKey() len = %d, want >= %d", len(data), marshalHeaderSize)
			}

			// 验证头部
			if KeyType(data[0]) != kt {
				t.Errorf("MarshalPublicKey() type = %v, want %v", KeyType(data[0]), kt)
			}
		})
	}
}

func TestMarshalPublicKey_Nil(t *testing.T) {
	_, err := MarshalPublicKey(nil)
	if err == nil {
		t.Error("MarshalPublicKey(nil) should return error")
	}
}

func TestUnmarshalPublicKeyBytes(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			_, pub, _ := GenerateKeyPair(kt)

			data, _ := MarshalPublicKey(pub)
			pub2, err := UnmarshalPublicKeyBytes(data)
			if err != nil {
				t.Fatalf("UnmarshalPublicKeyBytes() error = %v", err)
			}

			if !KeyEqual(pub, pub2) {
				t.Error("Unmarshalled key does not equal original")
			}
		})
	}
}

func TestUnmarshalPublicKeyBytes_TooShort(t *testing.T) {
	_, err := UnmarshalPublicKeyBytes([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalPublicKeyBytes(short) should return error")
	}
}

func TestMarshalPrivateKey(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			priv, _, err := GenerateKeyPair(kt)
			if err != nil {
				t.Fatalf("GenerateKeyPair() error = %v", err)
			}

			data, err := MarshalPrivateKey(priv)
			if err != nil {
				t.Fatalf("MarshalPrivateKey() error = %v", err)
			}

			if len(data) < marshalHeaderSize {
				t.Errorf("MarshalPrivateKey() len = %d, want >= %d", len(data), marshalHeaderSize)
			}
		})
	}
}

func TestMarshalPrivateKey_Nil(t *testing.T) {
	_, err := MarshalPrivateKey(nil)
	if err == nil {
		t.Error("MarshalPrivateKey(nil) should return error")
	}
}

func TestUnmarshalPrivateKeyBytes(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			priv, _, _ := GenerateKeyPair(kt)

			data, _ := MarshalPrivateKey(priv)
			priv2, err := UnmarshalPrivateKeyBytes(data)
			if err != nil {
				t.Fatalf("UnmarshalPrivateKeyBytes() error = %v", err)
			}

			if !KeyEqual(priv, priv2) {
				t.Error("Unmarshalled key does not equal original")
			}
		})
	}
}

func TestMarshalSignature(t *testing.T) {
	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)
	sig, _ := priv.Sign([]byte("test"))

	data, err := MarshalSignature(KeyTypeEd25519, sig)
	if err != nil {
		t.Fatalf("MarshalSignature() error = %v", err)
	}

	if len(data) < marshalHeaderSize {
		t.Errorf("MarshalSignature() len = %d, want >= %d", len(data), marshalHeaderSize)
	}
}

func TestMarshalSignature_Nil(t *testing.T) {
	_, err := MarshalSignature(KeyTypeEd25519, nil)
	if err == nil {
		t.Error("MarshalSignature(nil) should return error")
	}
}

func TestUnmarshalSignature(t *testing.T) {
	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)
	sig, _ := priv.Sign([]byte("test"))

	data, _ := MarshalSignature(KeyTypeEd25519, sig)
	kt, sig2, err := UnmarshalSignature(data)
	if err != nil {
		t.Fatalf("UnmarshalSignature() error = %v", err)
	}

	if kt != KeyTypeEd25519 {
		t.Errorf("UnmarshalSignature() type = %v, want %v", kt, KeyTypeEd25519)
	}
	if string(sig) != string(sig2) {
		t.Error("Unmarshalled signature does not equal original")
	}
}

func TestUnmarshalSignature_TooShort(t *testing.T) {
	_, _, err := UnmarshalSignature([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalSignature(short) should return error")
	}
}

func TestMarshalKeyPair(t *testing.T) {
	priv, pub, _ := GenerateKeyPair(KeyTypeEd25519)

	data, err := MarshalKeyPair(priv, pub)
	if err != nil {
		t.Fatalf("MarshalKeyPair() error = %v", err)
	}

	priv2, pub2, err := UnmarshalKeyPair(data)
	if err != nil {
		t.Fatalf("UnmarshalKeyPair() error = %v", err)
	}

	if !KeyEqual(priv, priv2) {
		t.Error("Unmarshalled private key does not equal original")
	}
	if !KeyEqual(pub, pub2) {
		t.Error("Unmarshalled public key does not equal original")
	}
}

func TestUnmarshalKeyPair_TooShort(t *testing.T) {
	_, _, err := UnmarshalKeyPair([]byte{1, 2, 3})
	if err == nil {
		t.Error("UnmarshalKeyPair(short) should return error")
	}
}
