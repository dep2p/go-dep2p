package pubsub

import (
	"sync"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
	"github.com/stretchr/testify/assert"
)

func TestConcurrent_MeshAdd(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			peer := string(rune('a' + (id % 10)))
			mesh.Add("topic1", peer)
		}(i)
	}

	wg.Wait()

	// 应该有一些节点被添加
	count := mesh.Count("topic1")
	assert.Greater(t, count, 0)
	assert.LessOrEqual(t, count, 12) // 不超过上限
}

func TestConcurrent_MeshRemove(t *testing.T) {
	mesh := newMeshPeers(6, 4, 12)

	// 先添加节点
	for i := 0; i < 10; i++ {
		mesh.Add("topic1", string(rune('a'+i)))
	}

	// 并发移除
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			peer := string(rune('a' + id))
			mesh.Remove("topic1", peer)
		}(i)
	}

	wg.Wait()

	// 所有节点应该被移除
	count := mesh.Count("topic1")
	assert.Equal(t, 0, count)
}

func TestConcurrent_MessageCache(t *testing.T) {
	cache := newMessageCache(100)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := createTestMessage("peer-1", "test", []byte{byte(id)})
			cache.Put(msg)
		}(i)
	}

	wg.Wait()

	// 应该有一些消息被缓存
	cache.mu.RLock()
	size := len(cache.messages)
	cache.mu.RUnlock()
	assert.Greater(t, size, 0)
	assert.LessOrEqual(t, size, 100)
}

func TestConcurrent_SeenMessages(t *testing.T) {
	seen := newSeenMessages(60)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msgID := string(rune('a' + (id % 26)))
			seen.Add(msgID)
		}(i)
	}

	wg.Wait()

	// 应该有一些消息被标记为已见
	seen.mu.RLock()
	size := len(seen.seen)
	seen.mu.RUnlock()
	assert.Greater(t, size, 0)
	assert.LessOrEqual(t, size, 26)
}

func TestConcurrent_SubscriptionPush(t *testing.T) {
	topicObj := &topic{
		name: "test",
	}

	sub := newTopicSubscription(topicObj, 100)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := createInterfaceMessage("peer-1", "test", []byte{byte(id)})
			sub.pushMessage(msg)
		}(i)
	}

	wg.Wait()

	// 应该有一些消息被推送
	assert.Greater(t, len(sub.msgCh), 0)
}

func TestConcurrent_TopicNotify(t *testing.T) {
	topicObj := &topic{
		name:  "test",
		peers: make(map[string]bool),
	}

	handler1 := newTopicEventHandler(topicObj)
	handler2 := newTopicEventHandler(topicObj)
	topicObj.eventHandlers = []*topicEventHandler{handler1, handler2}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			peer := string(rune('a' + id))
			topicObj.notifyPeerJoin(peer)
		}(i)
	}

	wg.Wait()

	// 应该有一些事件被推送
	assert.Greater(t, len(handler1.eventCh), 0)
	assert.Greater(t, len(handler2.eventCh), 0)
}

// 辅助函数
func createTestMessage(from, topic string, seqno []byte) *pb.Message {
	return &pb.Message{
		From:  []byte(from),
		Data:  []byte("test"),
		Topic: topic,
		Seqno: seqno,
	}
}

func createInterfaceMessage(from, topic string, seqno []byte) *interfaces.Message {
	return &interfaces.Message{
		From:  from,
		Data:  []byte("test"),
		Topic: topic,
		Seqno: seqno,
		ID:    messageID(from, seqno),
	}
}
