package rendezvous_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/rendezvous"
	"google.golang.org/protobuf/proto"
)

func TestMessage_Register(t *testing.T) {
	msg := &rendezvous.Message{
		Type: rendezvous.Message_REGISTER,
		Register: &rendezvous.Message_Register{
			Ns:                "test-namespace",
			SignedPeerRecord: []byte("signed-record"),
			Ttl:              3600,
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded rendezvous.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != rendezvous.Message_REGISTER {
		t.Errorf("Type = %v, want REGISTER", decoded.Type)
	}
}

func TestMessage_Discover(t *testing.T) {
	msg := &rendezvous.Message{
		Type: rendezvous.Message_DISCOVER,
		Discover: &rendezvous.Message_Discover{
			Ns:     "test-namespace",
			Limit:  100,
			Cookie: []byte("pagination-cookie"),
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded rendezvous.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Discover.Limit != 100 {
		t.Errorf("Limit = %d, want 100", decoded.Discover.Limit)
	}
}

func TestMessage_DiscoverResponse(t *testing.T) {
	msg := &rendezvous.Message{
		Type: rendezvous.Message_DISCOVER_RESPONSE,
		DiscoverResponse: &rendezvous.Message_DiscoverResponse{
			Registrations: []*rendezvous.Message_Registration{
				{
					Ns:                "test-ns",
					SignedPeerRecord: []byte("record"),
					Ttl:              3600,
				},
			},
			Status:     rendezvous.Message_OK,
			StatusText: "Success",
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded rendezvous.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.DiscoverResponse.Registrations) != 1 {
		t.Errorf("Registrations count = %d, want 1", len(decoded.DiscoverResponse.Registrations))
	}
}
