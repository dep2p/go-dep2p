package pubsub

import (
	"testing"
	"time"

	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageCache_Put(t *testing.T) {
	cache := newMessageCache(10)

	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test"),
		Topic: "topic1",
		Seqno: []byte{1, 2, 3},
	}

	cache.Put(msg)

	// 验证已缓存
	msgID := messageID("peer-1", msg.Seqno)
	assert.True(t, cache.Has(msgID))
}

func TestMessageCache_Get(t *testing.T) {
	cache := newMessageCache(10)

	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test"),
		Topic: "topic1",
		Seqno: []byte{1, 2, 3},
	}

	cache.Put(msg)

	msgID := messageID("peer-1", msg.Seqno)
	retrieved, exists := cache.Get(msgID)
	require.True(t, exists)
	assert.Equal(t, msg.Data, retrieved.Data)
}

func TestMessageCache_Evict(t *testing.T) {
	cache := newMessageCache(3) // 小缓存

	// 添加超过上限的消息
	for i := 0; i < 5; i++ {
		msg := &pb.Message{
			From:  []byte("peer-1"),
			Data:  []byte("test"),
			Topic: "topic1",
			Seqno: []byte{byte(i)},
		}
		cache.Put(msg)
		time.Sleep(time.Millisecond) // 确保时间戳不同
	}

	// 缓存大小应该不超过上限
	cache.mu.RLock()
	size := len(cache.messages)
	cache.mu.RUnlock()
	assert.LessOrEqual(t, size, 3)
}

func TestSeenMessages_Add(t *testing.T) {
	seen := newSeenMessages(time.Minute)

	msgID := "test-message-id"
	seen.Add(msgID)

	assert.True(t, seen.Has(msgID))
}

func TestSeenMessages_Cleanup(t *testing.T) {
	seen := newSeenMessages(10 * time.Millisecond)

	msgID := "test-message-id"
	seen.Add(msgID)
	assert.True(t, seen.Has(msgID))

	// 等待过期
	time.Sleep(20 * time.Millisecond)

	// 清理
	seen.Cleanup()

	// 应该已被清理
	seen.mu.RLock()
	_, exists := seen.seen[msgID]
	seen.mu.RUnlock()
	assert.False(t, exists)
}

func TestProtoToInterface(t *testing.T) {
	pbMsg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test data"),
		Topic: "topic1",
		Seqno: []byte{1, 2, 3},
	}

	interfaceMsg := protoToInterface(pbMsg)

	assert.Equal(t, "peer-1", interfaceMsg.From)
	assert.Equal(t, []byte("test data"), interfaceMsg.Data)
	assert.Equal(t, "topic1", interfaceMsg.Topic)
	assert.Equal(t, []byte{1, 2, 3}, interfaceMsg.Seqno)
}

func TestProtoToInterface_RoundTrip(t *testing.T) {
	// 创建原始 proto 消息
	original := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test data"),
		Topic: "topic1",
		Seqno: []byte{1, 2, 3},
	}

	// 转换为接口消息
	interfaceMsg := protoToInterface(original)

	// 验证转换正确
	assert.Equal(t, "peer-1", interfaceMsg.From)
	assert.Equal(t, []byte("test data"), interfaceMsg.Data)
	assert.Equal(t, "topic1", interfaceMsg.Topic)
	assert.Equal(t, []byte{1, 2, 3}, interfaceMsg.Seqno)
}

func TestGenerateSeqno(t *testing.T) {
	seqno1 := generateSeqno()
	seqno2 := generateSeqno()

	// 应该生成不同的序列号
	assert.NotEqual(t, seqno1, seqno2)

	// 长度应该是8字节
	assert.Len(t, seqno1, 8)
	assert.Len(t, seqno2, 8)
}

func TestMessageID(t *testing.T) {
	from := "peer-1"
	seqno := []byte{1, 2, 3, 4}

	id1 := messageID(from, seqno)
	id2 := messageID(from, seqno)

	// 相同输入应产生相同ID
	assert.Equal(t, id1, id2)

	// 不同输入应产生不同ID
	id3 := messageID("peer-2", seqno)
	assert.NotEqual(t, id1, id3)

	id4 := messageID(from, []byte{5, 6, 7, 8})
	assert.NotEqual(t, id1, id4)
}
