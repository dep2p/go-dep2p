package peer_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/peer"
	"google.golang.org/protobuf/proto"
)

func TestPeerID(t *testing.T) {
	id := &peer.PeerID{
		Id: []byte("12D3KooWTest"),
	}

	data, err := proto.Marshal(id)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded peer.PeerID
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(id, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestAddrInfo(t *testing.T) {
	ai := &peer.AddrInfo{
		Id: &peer.PeerID{Id: []byte("test-peer")},
		Addrs: [][]byte{
			[]byte("/ip4/127.0.0.1/tcp/4001"),
			[]byte("/ip6/::1/tcp/8080"),
		},
	}

	data, err := proto.Marshal(ai)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded peer.AddrInfo
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Addrs) != 2 {
		t.Errorf("Addrs count = %d, want 2", len(decoded.Addrs))
	}
}

func TestPeerRecord(t *testing.T) {
	pr := &peer.PeerRecord{
		PeerId: []byte("peer-id"),
		Seq:    123,
		Addresses: []*peer.AddressInfo{
			{Multiaddr: []byte("/ip4/1.2.3.4/tcp/4001")},
		},
	}

	data, err := proto.Marshal(pr)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded peer.PeerRecord
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(pr, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestSignedPeerRecord(t *testing.T) {
	spr := &peer.SignedPeerRecord{
		PeerId:    []byte("peer-id"),
		PublicKey: []byte("public-key"),
		Payload:   []byte("payload"),
		Signature: []byte("signature"),
	}

	data, err := proto.Marshal(spr)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded peer.SignedPeerRecord
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(spr, &decoded) {
		t.Error("Round trip failed")
	}
}
