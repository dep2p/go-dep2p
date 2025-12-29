package endpoint

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// TestRegisterConnectionEventCallback 测试注册事件回调
func TestRegisterConnectionEventCallback(t *testing.T) {
	e := &Endpoint{
		conns: make(map[coreif.NodeID]*Connection),
	}

	callbackCalled := int32(0)
	e.RegisterConnectionEventCallback(func(event interface{}) {
		atomic.AddInt32(&callbackCalled, 1)
	})

	// 验证回调已注册
	e.eventCallbacksMu.RLock()
	count := len(e.eventCallbacks)
	e.eventCallbacksMu.RUnlock()

	if count != 1 {
		t.Errorf("expected 1 callback registered, got %d", count)
	}
}

// TestRegisterConnectionEventCallback_Multiple 测试注册多个回调
func TestRegisterConnectionEventCallback_Multiple(t *testing.T) {
	e := &Endpoint{
		conns: make(map[coreif.NodeID]*Connection),
	}

	for i := 0; i < 5; i++ {
		e.RegisterConnectionEventCallback(func(event interface{}) {})
	}

	e.eventCallbacksMu.RLock()
	count := len(e.eventCallbacks)
	e.eventCallbacksMu.RUnlock()

	if count != 5 {
		t.Errorf("expected 5 callbacks registered, got %d", count)
	}
}

// TestDispatchEvent 测试事件分发
func TestDispatchEvent(t *testing.T) {
	e := &Endpoint{
		conns: make(map[coreif.NodeID]*Connection),
	}

	var receivedEvent interface{}
	var wg sync.WaitGroup
	wg.Add(1)

	e.RegisterConnectionEventCallback(func(event interface{}) {
		receivedEvent = event
		wg.Done()
	})

	testEvent := coreif.ConnectionClosedEvent{
		Connection:  nil,
		Reason:      errors.New("test error"),
		IsRelayConn: true,
		RelayID:     coreif.NodeID{},
	}

	e.dispatchEvent(testEvent)

	// 等待回调被调用
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 验证收到的事件
		closed, ok := receivedEvent.(coreif.ConnectionClosedEvent)
		if !ok {
			t.Fatal("expected ConnectionClosedEvent")
		}
		if !closed.IsRelayConn {
			t.Error("expected IsRelayConn=true")
		}
	case <-time.After(time.Second):
		t.Fatal("callback not called within timeout")
	}
}

// TestDispatchEvent_MultipleCallbacks 测试多回调分发
func TestDispatchEvent_MultipleCallbacks(t *testing.T) {
	e := &Endpoint{
		conns: make(map[coreif.NodeID]*Connection),
	}

	callCount := int32(0)
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		e.RegisterConnectionEventCallback(func(event interface{}) {
			atomic.AddInt32(&callCount, 1)
			wg.Done()
		})
	}

	e.dispatchEvent(coreif.ConnectionOpenedEvent{})

	// 等待所有回调被调用
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if atomic.LoadInt32(&callCount) != 3 {
			t.Errorf("expected 3 callbacks called, got %d", callCount)
		}
	case <-time.After(time.Second):
		t.Fatalf("not all callbacks called within timeout, got %d", atomic.LoadInt32(&callCount))
	}
}

// TestDispatchEvent_NoCallbacks 测试无回调时分发
func TestDispatchEvent_NoCallbacks(t *testing.T) {
	e := &Endpoint{
		conns: make(map[coreif.NodeID]*Connection),
	}

	// 应该不会 panic
	e.dispatchEvent(coreif.ConnectionClosedEvent{})
}

// TestDispatchEvent_ConcurrentSafe 测试并发安全
func TestDispatchEvent_ConcurrentSafe(t *testing.T) {
	e := &Endpoint{
		conns: make(map[coreif.NodeID]*Connection),
	}

	callCount := int32(0)

	// 并发注册回调
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.RegisterConnectionEventCallback(func(event interface{}) {
				atomic.AddInt32(&callCount, 1)
			})
		}()
	}
	wg.Wait()

	// 并发分发事件
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.dispatchEvent(coreif.ConnectionClosedEvent{})
		}()
	}
	wg.Wait()

	// 等待异步回调完成
	time.Sleep(100 * time.Millisecond)

	// 验证所有回调都被调用（10 个回调 * 10 次事件 = 100 次调用）
	count := atomic.LoadInt32(&callCount)
	if count != 100 {
		t.Errorf("expected 100 callback calls, got %d", count)
	}
}

// TestConnectionClosedEvent_Fields 测试连接关闭事件字段
func TestConnectionClosedEvent_Fields(t *testing.T) {
	relayID := coreif.NodeID{}
	copy(relayID[:], []byte("relay_node_id_123456789012"))

	event := coreif.ConnectionClosedEvent{
		Connection:  nil,
		Reason:      errors.New("connection timeout"),
		IsRelayConn: true,
		RelayID:     relayID,
	}

	if event.Type() != coreif.EventConnectionClosed {
		t.Errorf("expected event type %s, got %s", coreif.EventConnectionClosed, event.Type())
	}

	if !event.IsRelayConn {
		t.Error("expected IsRelayConn=true")
	}

	if event.RelayID != relayID {
		t.Error("RelayID mismatch")
	}

	if event.Reason == nil || event.Reason.Error() != "connection timeout" {
		t.Error("Reason mismatch")
	}
}

// TestIsRelayConnection 测试 relay 连接检测
func TestIsRelayConnection(t *testing.T) {
	e := &Endpoint{
		conns: make(map[coreif.NodeID]*Connection),
	}

	// 测试 nil 连接
	isRelay, relayID := e.isRelayConnection(nil)
	if isRelay {
		t.Error("nil connection should not be relay")
	}
	if !relayID.IsEmpty() {
		t.Error("relayID should be empty for nil connection")
	}
}

// TestConnectionEventType 测试事件类型常量
func TestConnectionEventType(t *testing.T) {
	if coreif.EventConnectionOpened != "connection.opened" {
		t.Errorf("unexpected EventConnectionOpened: %s", coreif.EventConnectionOpened)
	}
	if coreif.EventConnectionClosed != "connection.closed" {
		t.Errorf("unexpected EventConnectionClosed: %s", coreif.EventConnectionClosed)
	}
	if coreif.EventConnectionFailed != "connection.failed" {
		t.Errorf("unexpected EventConnectionFailed: %s", coreif.EventConnectionFailed)
	}
}

// TestConnectionOpenedEvent 测试连接建立事件
func TestConnectionOpenedEvent(t *testing.T) {
	event := coreif.ConnectionOpenedEvent{
		Connection: nil,
		Direction:  coreif.DirOutbound,
	}

	if event.Type() != coreif.EventConnectionOpened {
		t.Errorf("expected event type %s, got %s", coreif.EventConnectionOpened, event.Type())
	}

	if event.Direction != coreif.DirOutbound {
		t.Error("Direction mismatch")
	}
}

// TestConnectionFailedEvent 测试连接失败事件
func TestConnectionFailedEvent(t *testing.T) {
	nodeID := coreif.NodeID{}
	copy(nodeID[:], []byte("test_node_id_1234567890123"))

	event := coreif.ConnectionFailedEvent{
		NodeID: nodeID,
		Addrs:  nil,
		Error:  errors.New("dial timeout"),
	}

	if event.Type() != coreif.EventConnectionFailed {
		t.Errorf("expected event type %s, got %s", coreif.EventConnectionFailed, event.Type())
	}

	if event.NodeID != nodeID {
		t.Error("NodeID mismatch")
	}

	if event.Error == nil || event.Error.Error() != "dial timeout" {
		t.Error("Error mismatch")
	}
}

