// Package pubsub 实现发布订阅协议
package pubsub

import (
	"crypto/sha256"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
)

// messageCache 消息缓存
type messageCache struct {
	mu       sync.RWMutex
	messages map[string]*cacheEntry
	maxSize  int
}

// cacheEntry 缓存条目
type cacheEntry struct {
	msg       *pb.Message
	timestamp time.Time
}

// newMessageCache 创建消息缓存
func newMessageCache(maxSize int) *messageCache {
	return &messageCache{
		messages: make(map[string]*cacheEntry),
		maxSize:  maxSize,
	}
}

// Put 添加消息到缓存
func (mc *messageCache) Put(msg *pb.Message) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	msgID := messageID(string(msg.From), msg.Seqno)
	mc.messages[msgID] = &cacheEntry{
		msg:       msg,
		timestamp: time.Now(),
	}

	// 简单的大小控制,删除最旧的条目
	if len(mc.messages) > mc.maxSize {
		mc.evict()
	}
}

// Get 从缓存获取消息
func (mc *messageCache) Get(msgID string) (*pb.Message, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	entry, exists := mc.messages[msgID]
	if !exists {
		return nil, false
	}
	return entry.msg, true
}

// Has 检查消息是否在缓存中
func (mc *messageCache) Has(msgID string) bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	_, exists := mc.messages[msgID]
	return exists
}

// evict 驱逐最旧的条目
func (mc *messageCache) evict() {
	// 找到最旧的条目
	var oldest string
	var oldestTime time.Time

	for id, entry := range mc.messages {
		if oldestTime.IsZero() || entry.timestamp.Before(oldestTime) {
			oldest = id
			oldestTime = entry.timestamp
		}
	}

	if oldest != "" {
		delete(mc.messages, oldest)
	}
}

// CleanupOld 清理过期条目
func (mc *messageCache) CleanupOld(ttl time.Duration) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	for id, entry := range mc.messages {
		if now.Sub(entry.timestamp) > ttl {
			delete(mc.messages, id)
		}
	}
}

// seenMessages 已见消息追踪
type seenMessages struct {
	mu   sync.RWMutex
	seen map[string]time.Time
	ttl  time.Duration
}

// newSeenMessages 创建已见消息追踪器
func newSeenMessages(ttl time.Duration) *seenMessages {
	return &seenMessages{
		seen: make(map[string]time.Time),
		ttl:  ttl,
	}
}

// Has 检查消息是否已见
func (sm *seenMessages) Has(msgID string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	_, exists := sm.seen[msgID]
	return exists
}

// Add 添加已见消息
func (sm *seenMessages) Add(msgID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.seen[msgID] = time.Now()
}

// Cleanup 清理过期记录
func (sm *seenMessages) Cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for id, timestamp := range sm.seen {
		if now.Sub(timestamp) > sm.ttl {
			delete(sm.seen, id)
		}
	}
}

// protoToInterface 转换 protobuf 消息到接口消息
func protoToInterface(msg *pb.Message) *interfaces.Message {
	return &interfaces.Message{
		From:         string(msg.From),
		Data:         msg.Data,
		Topic:        msg.Topic,
		Seqno:        msg.Seqno,
		ID:           messageID(string(msg.From), msg.Seqno),
		ReceivedFrom: "", // 在接收时填充
		// P1-1: 设置接收时间戳
		RecvTimeNano: time.Now().UnixNano(),
	}
}

// generateSeqno 生成序列号
func generateSeqno() []byte {
	h := sha256.New()
	h.Write([]byte(time.Now().String()))
	return h.Sum(nil)[:8] // 使用前8字节
}
