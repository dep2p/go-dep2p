package proto_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/common"
	"github.com/dep2p/go-dep2p/pkg/lib/proto/key"
	"github.com/dep2p/go-dep2p/pkg/lib/proto/peer"
	"google.golang.org/protobuf/proto"
)

// TestCommonTimestamp 测试 Timestamp 消息
func TestCommonTimestamp(t *testing.T) {
	ts := &common.Timestamp{
		Seconds: 1234567890,
		Nanos:   123456789,
	}

	// Marshal
	data, err := proto.Marshal(ts)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var decoded common.Timestamp
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if decoded.Seconds != ts.Seconds {
		t.Errorf("Seconds = %d, want %d", decoded.Seconds, ts.Seconds)
	}
	if decoded.Nanos != ts.Nanos {
		t.Errorf("Nanos = %d, want %d", decoded.Nanos, ts.Nanos)
	}
}

// TestCommonError 测试 Error 消息
func TestCommonError(t *testing.T) {
	err := &common.Error{
		Code:    404,
		Message: "Not Found",
		Details: map[string]string{
			"resource": "peer",
		},
	}

	data, marshalErr := proto.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("Marshal failed: %v", marshalErr)
	}

	var decoded common.Error
	unmarshalErr := proto.Unmarshal(data, &decoded)
	if unmarshalErr != nil {
		t.Fatalf("Unmarshal failed: %v", unmarshalErr)
	}

	if decoded.Code != err.Code {
		t.Errorf("Code = %d, want %d", decoded.Code, err.Code)
	}
	if decoded.Message != err.Message {
		t.Errorf("Message = %s, want %s", decoded.Message, err.Message)
	}
}

// TestKeyPublicKey 测试 PublicKey 消息
func TestKeyPublicKey(t *testing.T) {
	pubKey := &key.PublicKey{
		Type: key.KeyType_Ed25519,
		Data: []byte("test-public-key-data"),
	}

	data, err := proto.Marshal(pubKey)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded key.PublicKey
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != pubKey.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, pubKey.Type)
	}
}

// TestPeerRecord 测试 PeerRecord 消息
func TestPeerRecord(t *testing.T) {
	pr := &peer.PeerRecord{
		PeerId: []byte("test-peer-id"),
		Seq:    1,
		Addresses: []*peer.AddressInfo{
			{Multiaddr: []byte("/ip4/127.0.0.1/tcp/4001")},
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

	if decoded.Seq != pr.Seq {
		t.Errorf("Seq = %d, want %d", decoded.Seq, pr.Seq)
	}
}

// TestRoundTrip 测试所有消息的往返编解码
func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  proto.Message
	}{
		{
			"Timestamp",
			&common.Timestamp{Seconds: 123, Nanos: 456},
		},
		{
			"Version",
			&common.Version{Major: 1, Minor: 2, Patch: 3},
		},
		{
			"PublicKey",
			&key.PublicKey{Type: key.KeyType_Ed25519, Data: []byte("test")},
		},
		{
			"PeerID",
			&peer.PeerID{Id: []byte("test-peer")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := proto.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// Unmarshal to new instance
			decoded := proto.Clone(tt.msg)
			proto.Reset(decoded)
			err = proto.Unmarshal(data, decoded)
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Compare
			if !proto.Equal(tt.msg, decoded) {
				t.Errorf("Round trip failed: got %v, want %v", decoded, tt.msg)
			}
		})
	}
}
