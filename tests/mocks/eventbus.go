package mocks

import (
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// MockEventBus 模拟 EventBus 接口实现
//
// 用于测试需要事件总线依赖的组件。
type MockEventBus struct {
	mu sync.RWMutex

	// 存储
	subscriptions map[interface{}][]*MockSubscription
	emitters      map[interface{}][]*MockEmitter
	eventTypes    []interface{}

	// 可覆盖的方法
	SubscribeFunc        func(eventType interface{}, opts ...interfaces.SubscriptionOpt) (interfaces.Subscription, error)
	EmitterFunc          func(eventType interface{}, opts ...interfaces.EmitterOpt) (interfaces.Emitter, error)
	GetAllEventTypesFunc func() []interface{}

	// 调用记录
	SubscribeCalls []interface{}
	EmitterCalls   []interface{}
}

// MockSubscription 模拟 Subscription 接口实现
type MockSubscription struct {
	eventType interface{}
	eventCh   chan interface{}
	closed    bool
	mu        sync.RWMutex
	bus       *MockEventBus
}

// MockEmitter 模拟 Emitter 接口实现
type MockEmitter struct {
	eventType interface{}
	closed    bool
	mu        sync.RWMutex
	bus       *MockEventBus
	stateful  bool
	lastEvent interface{}
}

// NewMockEventBus 创建带有默认值的 MockEventBus
func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		subscriptions: make(map[interface{}][]*MockSubscription),
		emitters:      make(map[interface{}][]*MockEmitter),
		eventTypes:    make([]interface{}, 0),
	}
}

// Subscribe 订阅指定类型的事件
func (m *MockEventBus) Subscribe(eventType interface{}, opts ...interfaces.SubscriptionOpt) (interfaces.Subscription, error) {
	m.mu.Lock()
	m.SubscribeCalls = append(m.SubscribeCalls, eventType)
	m.mu.Unlock()

	if m.SubscribeFunc != nil {
		return m.SubscribeFunc(eventType, opts...)
	}

	// 应用选项
	settings := &interfaces.SubscriptionSettings{Buffer: 16}
	for _, opt := range opts {
		opt(settings)
	}

	sub := &MockSubscription{
		eventType: eventType,
		eventCh:   make(chan interface{}, settings.Buffer),
		bus:       m,
	}

	m.mu.Lock()
	m.subscriptions[eventType] = append(m.subscriptions[eventType], sub)
	m.mu.Unlock()

	return sub, nil
}

// Emitter 获取指定事件类型的发射器
func (m *MockEventBus) Emitter(eventType interface{}, opts ...interfaces.EmitterOpt) (interfaces.Emitter, error) {
	m.mu.Lock()
	m.EmitterCalls = append(m.EmitterCalls, eventType)
	m.mu.Unlock()

	if m.EmitterFunc != nil {
		return m.EmitterFunc(eventType, opts...)
	}

	// 应用选项
	settings := &interfaces.EmitterSettings{}
	for _, opt := range opts {
		opt(settings)
	}

	emitter := &MockEmitter{
		eventType: eventType,
		bus:       m,
		stateful:  settings.Stateful,
	}

	m.mu.Lock()
	m.emitters[eventType] = append(m.emitters[eventType], emitter)
	// 记录事件类型
	found := false
	for _, t := range m.eventTypes {
		if t == eventType {
			found = true
			break
		}
	}
	if !found {
		m.eventTypes = append(m.eventTypes, eventType)
	}
	m.mu.Unlock()

	return emitter, nil
}

// GetAllEventTypes 返回所有已注册的事件类型
func (m *MockEventBus) GetAllEventTypes() []interface{} {
	if m.GetAllEventTypesFunc != nil {
		return m.GetAllEventTypesFunc()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.eventTypes
}

// ============================================================================
// MockSubscription 方法
// ============================================================================

// Out 返回接收事件的通道
func (s *MockSubscription) Out() <-chan interface{} {
	return s.eventCh
}

// Close 取消订阅
func (s *MockSubscription) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	close(s.eventCh)

	// 从总线中移除
	s.bus.mu.Lock()
	subs := s.bus.subscriptions[s.eventType]
	for i, sub := range subs {
		if sub == s {
			s.bus.subscriptions[s.eventType] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	s.bus.mu.Unlock()

	return nil
}

// IsClosed 检查是否已关闭
func (s *MockSubscription) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// ============================================================================
// MockEmitter 方法
// ============================================================================

// Emit 发射事件
func (e *MockEmitter) Emit(event interface{}) error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	if e.stateful {
		e.lastEvent = event
	}
	e.mu.Unlock()

	// 发送给所有订阅者
	e.bus.mu.RLock()
	subs := e.bus.subscriptions[e.eventType]
	e.bus.mu.RUnlock()

	for _, sub := range subs {
		sub.mu.RLock()
		closed := sub.closed
		sub.mu.RUnlock()

		if !closed {
			select {
			case sub.eventCh <- event:
			default:
				// 缓冲区满，丢弃事件
			}
		}
	}

	return nil
}

// Close 关闭发射器
func (e *MockEmitter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true

	// 从总线中移除
	e.bus.mu.Lock()
	emitters := e.bus.emitters[e.eventType]
	for i, em := range emitters {
		if em == e {
			e.bus.emitters[e.eventType] = append(emitters[:i], emitters[i+1:]...)
			break
		}
	}
	e.bus.mu.Unlock()

	return nil
}

// IsClosed 检查是否已关闭
func (e *MockEmitter) IsClosed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closed
}

// LastEvent 返回最后发射的事件（仅适用于 stateful emitter）
func (e *MockEmitter) LastEvent() interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastEvent
}

// ============================================================================
// 测试辅助方法
// ============================================================================

// EmitEvent 直接发射事件到指定类型的订阅者（用于测试）
func (m *MockEventBus) EmitEvent(eventType interface{}, event interface{}) {
	m.mu.RLock()
	subs := m.subscriptions[eventType]
	m.mu.RUnlock()

	for _, sub := range subs {
		sub.mu.RLock()
		closed := sub.closed
		sub.mu.RUnlock()

		if !closed {
			select {
			case sub.eventCh <- event:
			default:
			}
		}
	}
}

// GetSubscribers 返回指定事件类型的订阅者数量
func (m *MockEventBus) GetSubscribers(eventType interface{}) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.subscriptions[eventType])
}

// 确保实现接口
var _ interfaces.EventBus = (*MockEventBus)(nil)
var _ interfaces.Subscription = (*MockSubscription)(nil)
var _ interfaces.Emitter = (*MockEmitter)(nil)
