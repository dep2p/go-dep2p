// Package pubsub 实现发布订阅协议
package pubsub

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// topicSubscription 实现 TopicSubscription 接口
type topicSubscription struct {
	topic  *topic
	msgCh  chan *interfaces.Message
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

// 确保实现接口
var _ interfaces.TopicSubscription = (*topicSubscription)(nil)

// newTopicSubscription 创建主题订阅
func newTopicSubscription(t *topic, bufferSize int) *topicSubscription {
	ctx, cancel := context.WithCancel(context.Background())

	if bufferSize <= 0 {
		bufferSize = 32 // 默认缓冲区大小
	}

	return &topicSubscription{
		topic:  t,
		msgCh:  make(chan *interfaces.Message, bufferSize),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Next 获取下一条消息
func (ts *topicSubscription) Next(ctx context.Context) (*interfaces.Message, error) {
	select {
	case msg := <-ts.msgCh:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ts.ctx.Done():
		return nil, ErrSubscriptionCancelled
	}
}

// Cancel 取消订阅
func (ts *topicSubscription) Cancel() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.cancel != nil {
		ts.cancel()
		ts.cancel = nil
	}
}

// pushMessage 推送消息到订阅
func (ts *topicSubscription) pushMessage(msg *interfaces.Message) bool {
	// 先检查是否已取消，避免 select 随机选择导致取消后仍发送消息
	select {
	case <-ts.ctx.Done():
		return false
	default:
	}

	select {
	case ts.msgCh <- msg:
		return true
	case <-ts.ctx.Done():
		return false
	default:
		// 缓冲区满,丢弃消息
		return false
	}
}

// isCancelled 检查是否已取消
func (ts *topicSubscription) isCancelled() bool {
	select {
	case <-ts.ctx.Done():
		return true
	default:
		return false
	}
}
