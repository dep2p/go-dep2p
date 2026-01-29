package crypto

import (
	"testing"
)

func TestPeerIDFromPublicKey(t *testing.T) {
	keyTypes := []KeyType{KeyTypeEd25519, KeyTypeSecp256k1, KeyTypeECDSA, KeyTypeRSA}

	for _, kt := range keyTypes {
		t.Run(kt.String(), func(t *testing.T) {
			_, pub, err := GenerateKeyPair(kt)
			if err != nil {
				t.Fatalf("GenerateKeyPair() error = %v", err)
			}

			id, err := PeerIDFromPublicKey(pub)
			if err != nil {
				t.Fatalf("PeerIDFromPublicKey() error = %v", err)
			}

			if id.IsEmpty() {
				t.Error("PeerIDFromPublicKey() returned empty ID")
			}

			// 相同公钥应产生相同 ID
			id2, _ := PeerIDFromPublicKey(pub)
			if id != id2 {
				t.Error("PeerIDFromPublicKey() not deterministic")
			}
		})
	}
}

func TestPeerIDFromPublicKey_Nil(t *testing.T) {
	_, err := PeerIDFromPublicKey(nil)
	if err == nil {
		t.Error("PeerIDFromPublicKey(nil) should return error")
	}
}

func TestPeerIDFromPrivateKey(t *testing.T) {
	priv, pub, _ := GenerateKeyPair(KeyTypeEd25519)

	id1, err := PeerIDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("PeerIDFromPrivateKey() error = %v", err)
	}

	id2, _ := PeerIDFromPublicKey(pub)

	if id1 != id2 {
		t.Error("PeerIDFromPrivateKey() != PeerIDFromPublicKey()")
	}
}

func TestPeerIDFromPrivateKey_Nil(t *testing.T) {
	_, err := PeerIDFromPrivateKey(nil)
	if err == nil {
		t.Error("PeerIDFromPrivateKey(nil) should return error")
	}
}

func TestIDFromPublicKey(t *testing.T) {
	_, pub, _ := GenerateKeyPair(KeyTypeEd25519)

	id1, _ := IDFromPublicKey(pub)
	id2, _ := PeerIDFromPublicKey(pub)

	if id1 != id2 {
		t.Error("IDFromPublicKey() != PeerIDFromPublicKey()")
	}
}

func TestIDFromPrivateKey(t *testing.T) {
	priv, _, _ := GenerateKeyPair(KeyTypeEd25519)

	id1, _ := IDFromPrivateKey(priv)
	id2, _ := PeerIDFromPrivateKey(priv)

	if id1 != id2 {
		t.Error("IDFromPrivateKey() != PeerIDFromPrivateKey()")
	}
}

func TestPublicKeyHash(t *testing.T) {
	_, pub, _ := GenerateKeyPair(KeyTypeEd25519)

	hash, err := PublicKeyHash(pub)
	if err != nil {
		t.Fatalf("PublicKeyHash() error = %v", err)
	}

	if hash == [32]byte{} {
		t.Error("PublicKeyHash() returned zero hash")
	}

	// 相同公钥应产生相同哈希
	hash2, _ := PublicKeyHash(pub)
	if hash != hash2 {
		t.Error("PublicKeyHash() not deterministic")
	}
}

func TestPublicKeyHash_Nil(t *testing.T) {
	_, err := PublicKeyHash(nil)
	if err == nil {
		t.Error("PublicKeyHash(nil) should return error")
	}
}

func TestVerifyPeerID(t *testing.T) {
	_, pub, _ := GenerateKeyPair(KeyTypeEd25519)
	id, _ := PeerIDFromPublicKey(pub)

	valid, err := VerifyPeerID(pub, id)
	if err != nil {
		t.Fatalf("VerifyPeerID() error = %v", err)
	}
	if !valid {
		t.Error("VerifyPeerID() = false, want true")
	}
}

func TestVerifyPeerID_Wrong(t *testing.T) {
	_, pub1, _ := GenerateKeyPair(KeyTypeEd25519)
	_, pub2, _ := GenerateKeyPair(KeyTypeEd25519)

	id1, _ := PeerIDFromPublicKey(pub1)

	valid, _ := VerifyPeerID(pub2, id1)
	if valid {
		t.Error("VerifyPeerID(wrong) = true, want false")
	}
}

func TestDifferentKeysProduceDifferentIDs(t *testing.T) {
	_, pub1, _ := GenerateKeyPair(KeyTypeEd25519)
	_, pub2, _ := GenerateKeyPair(KeyTypeEd25519)

	id1, _ := PeerIDFromPublicKey(pub1)
	id2, _ := PeerIDFromPublicKey(pub2)

	if id1 == id2 {
		t.Error("Different keys produced same ID")
	}
}

func BenchmarkPeerIDFromPublicKey(b *testing.B) {
	_, pub, _ := GenerateKeyPair(KeyTypeEd25519)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PeerIDFromPublicKey(pub)
	}
}
