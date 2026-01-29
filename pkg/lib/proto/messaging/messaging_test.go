package messaging_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/messaging"
	"google.golang.org/protobuf/proto"
)

func TestMessage_Direct(t *testing.T) {
	msg := &messaging.Message{
		Id:        []byte("msg-uuid"),
		From:      []byte("sender"),
		To:        []byte("receiver"),
		Type:      messaging.MessageType_DIRECT,
		Priority:  messaging.Priority_NORMAL,
		Payload:   []byte("hello world"),
		Timestamp: 1234567890,
		Ttl:       3600,
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded messaging.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(msg, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestMessage_Broadcast(t *testing.T) {
	msg := &messaging.Message{
		Id:        []byte("broadcast-id"),
		From:      []byte("sender"),
		Topic:     "announcements",
		Type:      messaging.MessageType_BROADCAST,
		Priority:  messaging.Priority_HIGH,
		Payload:   []byte("important message"),
		Timestamp: 1234567890,
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded messaging.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Type != messaging.MessageType_BROADCAST {
		t.Errorf("Type = %v, want BROADCAST", decoded.Type)
	}
}

func TestAck(t *testing.T) {
	ack := &messaging.Ack{
		MessageId: []byte("msg-id"),
		From:      []byte("receiver"),
		Status:    messaging.Ack_PROCESSED,
		Timestamp: 1234567890,
	}

	data, err := proto.Marshal(ack)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded messaging.Ack
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Status != messaging.Ack_PROCESSED {
		t.Errorf("Status = %v, want PROCESSED", decoded.Status)
	}
}
