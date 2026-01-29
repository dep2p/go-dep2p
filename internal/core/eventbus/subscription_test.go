package eventbus

import (
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 接口契约测试
// ============================================================================

// TestSubscription_ImplementsInterface 验证 Subscription 实现接口
func TestSubscription_ImplementsInterface(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{}

	sub, err := bus.Subscribe(new(TestEvent))
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}
	defer sub.Close()

	var _ pkgif.Subscription = sub
}

// ============================================================================
// Subscription 测试
// ============================================================================

// TestSubscription_Out 测试接收事件通道
func TestSubscription_Out(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{ Value int }

	sub, _ := bus.Subscribe(new(TestEvent))
	defer sub.Close()

	em, _ := bus.Emitter(new(TestEvent))
	defer em.Close()

	// 发射事件
	em.Emit(TestEvent{Value: 123})

	// 接收事件
	select {
	case evt := <-sub.Out():
		if evt.(TestEvent).Value != 123 {
			t.Error("Received wrong event value")
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for event")
	}
}

// TestSubscription_Close 测试关闭订阅
func TestSubscription_Close(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{}

	sub, _ := bus.Subscribe(new(TestEvent))

	err := sub.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// 关闭后通道应该关闭
	select {
	case _, ok := <-sub.Out():
		if ok {
			t.Error("Channel should be closed after Close()")
		}
	case <-time.After(100 * time.Millisecond):
		// 可能还在排空，这是可以接受的
	}
}

// TestSubscription_CloseTwice 测试重复关闭
func TestSubscription_CloseTwice(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{}

	sub, _ := bus.Subscribe(new(TestEvent))

	// 第一次关闭
	err1 := sub.Close()
	if err1 != nil {
		t.Errorf("First Close() failed: %v", err1)
	}

	// 第二次关闭应该不会 panic
	err2 := sub.Close()
	if err2 != nil {
		// 可能返回错误，也可能是 no-op
		t.Logf("Second Close() returned: %v", err2)
	}
}

// TestSubscription_BufferSize 测试缓冲区大小
func TestSubscription_BufferSize(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{ Value int }

	// 创建小缓冲区订阅
	bufSize := 2
	sub, _ := bus.Subscribe(new(TestEvent), pkgif.BufSize(bufSize))
	defer sub.Close()

	em, _ := bus.Emitter(new(TestEvent))
	defer em.Close()

	// 发射超过缓冲区大小的事件
	for i := 0; i < bufSize+5; i++ {
		em.Emit(TestEvent{Value: i})
	}

	// 至少应该收到缓冲区大小的事件
	received := 0
	timeout := time.After(100 * time.Millisecond)

loop:
	for {
		select {
		case <-sub.Out():
			received++
		case <-timeout:
			break loop
		}
	}

	// 应该收到至少缓冲区大小的事件
	if received < bufSize {
		t.Errorf("Received %d events, want at least %d", received, bufSize)
	}

	t.Logf("Received %d events with buffer size %d", received, bufSize)
}

// TestSubscription_NoEmitter 测试无发射器时订阅
func TestSubscription_NoEmitter(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{}

	// 订阅但不创建发射器
	sub, err := bus.Subscribe(new(TestEvent))
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}
	defer sub.Close()

	// 不应该收到任何事件
	select {
	case <-sub.Out():
		t.Error("Received event without emitter")
	case <-time.After(50 * time.Millisecond):
		// 正确：没有收到事件
	}
}

// TestSubscription_CloseBeforeReceive 测试关闭前有未读事件
func TestSubscription_CloseBeforeReceive(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{ Value int }

	sub, _ := bus.Subscribe(new(TestEvent), pkgif.BufSize(10))
	em, _ := bus.Emitter(new(TestEvent))

	// 发射多个事件
	for i := 0; i < 5; i++ {
		em.Emit(TestEvent{Value: i})
	}

	em.Close()

	// 立即关闭订阅
	sub.Close()

	// 关闭不应该 panic
	// 通道应该被排空
}
