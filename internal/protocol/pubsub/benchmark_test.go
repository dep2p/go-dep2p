package pubsub

import (
	"context"
	"testing"

	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
)

func BenchmarkMessageCache_Put(b *testing.B) {
	cache := newMessageCache(1000)
	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  make([]byte, 1024),
		Topic: "topic1",
		Seqno: []byte{1, 2, 3},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(msg)
	}
}

func BenchmarkMessageCache_Get(b *testing.B) {
	cache := newMessageCache(1000)
	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test"),
		Topic: "topic1",
		Seqno: []byte{1, 2, 3},
	}
	cache.Put(msg)

	msgID := messageID("peer-1", msg.Seqno)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get(msgID)
	}
}

func BenchmarkSeenMessages_Add(b *testing.B) {
	seen := newSeenMessages(60)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgID := string(rune('a' + (i % 26)))
		seen.Add(msgID)
	}
}

func BenchmarkSeenMessages_Has(b *testing.B) {
	seen := newSeenMessages(60)
	seen.Add("test-message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = seen.Has("test-message")
	}
}

func BenchmarkMeshPeers_Add(b *testing.B) {
	mesh := newMeshPeers(6, 4, 12)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer := string(rune('a' + (i % 10)))
		mesh.Add("topic1", peer)
		mesh.Remove("topic1", peer)
	}
}

func BenchmarkMeshPeers_List(b *testing.B) {
	mesh := newMeshPeers(6, 4, 12)
	for i := 0; i < 6; i++ {
		mesh.Add("topic1", string(rune('a'+i)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mesh.List("topic1")
	}
}

func BenchmarkProtoToInterface(b *testing.B) {
	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  make([]byte, 1024),
		Topic: "topic1",
		Seqno: []byte{1, 2, 3},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = protoToInterface(msg)
	}
}

func BenchmarkProtoToInterface_Large(b *testing.B) {
	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  make([]byte, 10240), // 10KB
		Topic: "topic1",
		Seqno: []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = protoToInterface(msg)
	}
}

func BenchmarkMessageID(b *testing.B) {
	from := "peer-1"
	seqno := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = messageID(from, seqno)
	}
}

func BenchmarkGenerateSeqno(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateSeqno()
	}
}

func BenchmarkValidator_Validate(b *testing.B) {
	realmMgr := newMockRealmManager()
	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realmMgr.AddRealm(realm)

	validator := newMessageValidator(realmMgr, 1024*1024)

	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test"),
		Topic: "topic1",
		Seqno: []byte{1, 2, 3},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(ctx, "peer-1", msg)
	}
}
