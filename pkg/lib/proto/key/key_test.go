package key_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/key"
	"google.golang.org/protobuf/proto"
)

func TestPublicKey(t *testing.T) {
	pub := &key.PublicKey{
		Type: key.KeyType_Ed25519,
		Data: []byte("ed25519-public-key-32-bytes"),
	}

	data, err := proto.Marshal(pub)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded key.PublicKey
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(pub, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestPrivateKey(t *testing.T) {
	priv := &key.PrivateKey{
		Type: key.KeyType_Secp256k1,
		Data: []byte("secp256k1-private-key"),
	}

	data, err := proto.Marshal(priv)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded key.PrivateKey
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != key.KeyType_Secp256k1 {
		t.Errorf("Type = %v, want Secp256k1", decoded.Type)
	}
}

func TestKeyPair(t *testing.T) {
	kp := &key.KeyPair{
		PublicKey: &key.PublicKey{
			Type: key.KeyType_Ed25519,
			Data: []byte("public"),
		},
		PrivateKey: &key.PrivateKey{
			Type: key.KeyType_Ed25519,
			Data: []byte("private"),
		},
	}

	data, err := proto.Marshal(kp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded key.KeyPair
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(kp, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestSignature(t *testing.T) {
	sig := &key.Signature{
		Type: key.KeyType_ECDSA,
		Data: []byte("signature-bytes"),
	}

	data, err := proto.Marshal(sig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded key.Signature
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(sig, &decoded) {
		t.Error("Round trip failed")
	}
}
