package gossipsub_test

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
	"google.golang.org/protobuf/proto"
)

func TestRPC(t *testing.T) {
	rpc := &gossipsub.RPC{
		Subscriptions: []*gossipsub.SubOpts{
			{Subscribe: true, TopicId: "topic1"},
		},
		Publish: []*gossipsub.Message{
			{
				From:  []byte("sender"),
				Data:  []byte("hello"),
				Topic: "topic1",
			},
		},
	}

	data, err := proto.Marshal(rpc)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded gossipsub.RPC
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Subscriptions) != 1 {
		t.Errorf("Subscriptions count = %d, want 1", len(decoded.Subscriptions))
	}
}

func TestMessage(t *testing.T) {
	msg := &gossipsub.Message{
		From:      []byte("sender"),
		Data:      []byte("payload"),
		Seqno:     []byte{1, 2, 3, 4},
		Topic:     "test-topic",
		Signature: []byte("sig"),
		Key:       []byte("pubkey"),
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded gossipsub.Message
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(msg, &decoded) {
		t.Error("Round trip failed")
	}
}

func TestControlMessage(t *testing.T) {
	ctrl := &gossipsub.ControlMessage{
		Ihave: []*gossipsub.ControlIHave{
			{
				TopicId:    "topic1",
				MessageIds: [][]byte{[]byte("msg1"), []byte("msg2")},
			},
		},
		Iwant: []*gossipsub.ControlIWant{
			{MessageIds: [][]byte{[]byte("msg3")}},
		},
		Graft: []*gossipsub.ControlGraft{
			{TopicId: "topic2"},
		},
		Prune: []*gossipsub.ControlPrune{
			{TopicId: "topic3", Backoff: 60},
		},
	}

	data, err := proto.Marshal(ctrl)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded gossipsub.ControlMessage
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Ihave) != 1 {
		t.Errorf("Ihave count = %d, want 1", len(decoded.Ihave))
	}
}

func TestTopicDescriptor(t *testing.T) {
	td := &gossipsub.TopicDescriptor{
		Name: "test-topic",
		Auth: &gossipsub.AuthOpts{
			Mode: gossipsub.AuthOpts_KEY,
			Keys: [][]byte{[]byte("key1")},
		},
		Enc: &gossipsub.EncOpts{
			Mode:      gossipsub.EncOpts_SHAREDKEY,
			KeyHashes: [][]byte{[]byte("hash1")},
		},
	}

	data, err := proto.Marshal(td)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded gossipsub.TopicDescriptor
	err = proto.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !proto.Equal(td, &decoded) {
		t.Error("Round trip failed")
	}
}
