package autonat_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/autonat"
	"google.golang.org/protobuf/proto"
)

func TestMessage_Dial(t *testing.T) {
	msg := &autonat.Message{
		Type: autonat.MessageType_DIAL,
		Dial: &autonat.Dial{
			Peer: &autonat.PeerInfo{
				Id:    []byte("test-peer-id"),
				Addrs: [][]byte{[]byte("/ip4/127.0.0.1/tcp/4001")},
			},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded autonat.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != autonat.MessageType_DIAL {
		t.Errorf("Type = %v, want DIAL", decoded.Type)
	}
}

func TestMessage_DialResponse(t *testing.T) {
	msg := &autonat.Message{
		Type: autonat.MessageType_DIAL_RESPONSE,
		DialResponse: &autonat.DialResponse{
			Status:     autonat.ResponseStatus_OK,
			StatusText: "Success",
			Addr:       []byte("/ip4/1.2.3.4/tcp/5000"),
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded autonat.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.DialResponse.Status != autonat.ResponseStatus_OK {
		t.Errorf("Status = %v, want OK", decoded.DialResponse.Status)
	}
}
