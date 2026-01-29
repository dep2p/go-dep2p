package identify_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/identify"
	"google.golang.org/protobuf/proto"
)

func TestIdentify_Marshal(t *testing.T) {
	id := &identify.Identify{
		ProtocolVersion: []byte("dep2p/1.0.0"),
		AgentVersion:    []byte("go-dep2p/1.0.0"),
		PublicKey:       []byte("test-public-key"),
		ListenAddrs: [][]byte{
			[]byte("/ip4/127.0.0.1/tcp/4001"),
		},
		ObservedAddr: []byte("/ip4/1.2.3.4/tcp/5000"),
		Protocols: []string{
			"/dep2p/sys/ping/1.0.0",
			"/dep2p/sys/dht/1.0.0",
		},
	}

	data, err := proto.Marshal(id)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Marshal returned empty data")
	}
}

func TestIdentify_RoundTrip(t *testing.T) {
	original := &identify.Identify{
		ProtocolVersion: []byte("dep2p/1.0.0"),
		AgentVersion:    []byte("go-dep2p/1.0.0"),
		PublicKey:       []byte("test-key"),
		Protocols:       []string{"/test/1.0.0"},
	}

	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded identify.Identify
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(original, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestPush(t *testing.T) {
	push := &identify.Push{
		Protocols:          []string{"/new/1.0.0"},
		SignedPeerRecord: []byte("signed-record"),
	}

	data, err := proto.Marshal(push)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded identify.Push
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Protocols) != 1 || decoded.Protocols[0] != "/new/1.0.0" {
		t.Error("Protocols mismatch")
	}
}
