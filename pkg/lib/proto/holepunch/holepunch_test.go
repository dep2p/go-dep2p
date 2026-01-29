package holepunch_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/holepunch"
	"google.golang.org/protobuf/proto"
)

func TestHolePunch_Connect(t *testing.T) {
	hp := &holepunch.HolePunch{
		Type: holepunch.Type_CONNECT,
		ObsAddrs: [][]byte{
			[]byte("/ip4/1.2.3.4/tcp/4001"),
			[]byte("/ip6/::1/tcp/8080"),
		},
	}

	data, err := proto.Marshal(hp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded holepunch.HolePunch
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(hp, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestHolePunch_Sync(t *testing.T) {
	hp := &holepunch.HolePunch{
		Type:     holepunch.Type_SYNC,
		ObsAddrs: [][]byte{},
	}

	data, err := proto.Marshal(hp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded holepunch.HolePunch
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != holepunch.Type_SYNC {
		t.Errorf("Type = %v, want SYNC", decoded.Type)
	}
}
