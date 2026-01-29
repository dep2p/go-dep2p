package relay_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/relay"
	"google.golang.org/protobuf/proto"
)

func TestHopMessage_Reserve(t *testing.T) {
	msg := &relay.HopMessage{
		Type: relay.HopMessage_RESERVE,
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded relay.HopMessage
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != relay.HopMessage_RESERVE {
		t.Errorf("Type = %v, want RESERVE", decoded.Type)
	}
}

func TestHopMessage_Connect(t *testing.T) {
	msg := &relay.HopMessage{
		Type: relay.HopMessage_CONNECT,
		Peer: &relay.Peer{
			Id:    []byte("target-peer"),
			Addrs: [][]byte{[]byte("/ip4/1.2.3.4/tcp/4001")},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded relay.HopMessage
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Peer == nil {
		t.Error("Peer is nil")
	}
}

func TestReservationVoucher(t *testing.T) {
	voucher := &relay.ReservationVoucher{
		Relay:      []byte("relay-peer-id"),
		Peer:       []byte("client-peer-id"),
		Expiration: 9999999999,
	}

	data, err := proto.Marshal(voucher)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded relay.ReservationVoucher
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(voucher, &decoded) {
		t.Error("Round trip failed")
	}
}
