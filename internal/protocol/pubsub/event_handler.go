// Package pubsub 实现发布订阅协议
package pubsub

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// topicEventHandler 实现 TopicEventHandler 接口
type topicEventHandler struct {
	topic   *topic
	eventCh chan interfaces.PeerEvent
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
}

// 确保实现接口
var _ interfaces.TopicEventHandler = (*topicEventHandler)(nil)

// newTopicEventHandler 创建主题事件处理器
func newTopicEventHandler(t *topic) *topicEventHandler {
	ctx, cancel := context.WithCancel(context.Background())

	return &topicEventHandler{
		topic:   t,
		eventCh: make(chan interfaces.PeerEvent, 32),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// NextPeerEvent 获取下一个节点事件
func (h *topicEventHandler) NextPeerEvent(ctx context.Context) (interfaces.PeerEvent, error) {
	select {
	case event := <-h.eventCh:
		return event, nil
	case <-ctx.Done():
		return interfaces.PeerEvent{}, ctx.Err()
	case <-h.ctx.Done():
		return interfaces.PeerEvent{}, ErrSubscriptionCancelled
	}
}

// Cancel 取消事件处理
func (h *topicEventHandler) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

// pushEvent 推送事件
func (h *topicEventHandler) pushEvent(event interfaces.PeerEvent) bool {
	select {
	case h.eventCh <- event:
		return true
	case <-h.ctx.Done():
		return false
	default:
		// 缓冲区满,丢弃事件
		return false
	}
}

// isCancelled 检查是否已取消
func (h *topicEventHandler) isCancelled() bool {
	select {
	case <-h.ctx.Done():
		return true
	default:
		return false
	}
}
