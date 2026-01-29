package dht_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/dht"
	"google.golang.org/protobuf/proto"
)

func TestMessage_PutValue(t *testing.T) {
	msg := &dht.Message{
		Type:  dht.MessageType_PUT_VALUE,
		Key:   []byte("test-key"),
		Value: []byte("test-value"),
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded dht.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(msg, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestMessage_FindNode(t *testing.T) {
	msg := &dht.Message{
		Type: dht.MessageType_FIND_NODE,
		Key:  []byte("target-peer-id"),
		Closer: []*dht.Peer{
			{
				Id:         []byte("peer1"),
				Addrs:      [][]byte{[]byte("/ip4/1.2.3.4/tcp/4001")},
				Connection: dht.ConnectionType_CONNECTED,
			},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded dht.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Closer) != 1 {
		t.Errorf("Closer count = %d, want 1", len(decoded.Closer))
	}
}

func TestRecord(t *testing.T) {
	rec := &dht.Record{
		Key:          []byte("key"),
		Value:        []byte("value"),
		Author:       []byte("author-peer-id"),
		Signature:    []byte("signature"),
		TimeReceived: 1234567890,
	}

	data, err := proto.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded dht.Record
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(rec, &decoded) {
		t.Error("Round trip failed")
	}
}
