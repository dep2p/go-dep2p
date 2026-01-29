package eventbus

import (
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 接口契约测试
// ============================================================================

// TestEmitter_ImplementsInterface 验证 Emitter 实现接口
func TestEmitter_ImplementsInterface(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{}

	em, err := bus.Emitter(new(TestEvent))
	if err != nil {
		t.Fatalf("Emitter() failed: %v", err)
	}
	defer em.Close()

	var _ pkgif.Emitter = em
}

// ============================================================================
// Emitter 测试
// ============================================================================

// TestEmitter_Emit 测试发射事件
func TestEmitter_Emit(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{ Value int }

	sub, _ := bus.Subscribe(new(TestEvent))
	defer sub.Close()

	em, _ := bus.Emitter(new(TestEvent))
	defer em.Close()

	// 发射事件
	err := em.Emit(TestEvent{Value: 999})
	if err != nil {
		t.Errorf("Emit() failed: %v", err)
	}

	// 验证接收
	select {
	case evt := <-sub.Out():
		if evt.(TestEvent).Value != 999 {
			t.Error("Received wrong event value")
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for event")
	}
}

// TestEmitter_Close 测试关闭发射器
func TestEmitter_Close(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{}

	em, _ := bus.Emitter(new(TestEvent))

	err := em.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

// TestEmitter_CloseTwice 测试重复关闭发射器
func TestEmitter_CloseTwice(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{}

	em, _ := bus.Emitter(new(TestEvent))

	// 第一次关闭
	err1 := em.Close()
	if err1 != nil {
		t.Errorf("First Close() failed: %v", err1)
	}

	// 第二次关闭应该不会 panic
	err2 := em.Close()
	if err2 != nil {
		t.Logf("Second Close() returned: %v", err2)
	}
}

// TestEmitter_EmitAfterClose 测试关闭后发射
func TestEmitter_EmitAfterClose(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{}

	em, _ := bus.Emitter(new(TestEvent))
	em.Close()

	// 关闭后发射应该失败
	err := em.Emit(TestEvent{})
	if err == nil {
		t.Error("Emit() should fail after Close()")
	}
}

// TestEmitter_MultipleEmitters 测试同一事件类型的多个发射器
func TestEmitter_MultipleEmitters(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{ ID int }

	sub, _ := bus.Subscribe(new(TestEvent))
	defer sub.Close()

	em1, _ := bus.Emitter(new(TestEvent))
	defer em1.Close()

	em2, _ := bus.Emitter(new(TestEvent))
	defer em2.Close()

	// 两个发射器都发射事件
	em1.Emit(TestEvent{ID: 1})
	em2.Emit(TestEvent{ID: 2})

	// 应该收到两个事件
	received := 0
	timeout := time.After(time.Second)

loop:
	for received < 2 {
		select {
		case evt := <-sub.Out():
			received++
			id := evt.(TestEvent).ID
			if id != 1 && id != 2 {
				t.Errorf("Received unexpected event ID: %d", id)
			}
		case <-timeout:
			break loop
		}
	}

	if received != 2 {
		t.Errorf("Received %d events, want 2", received)
	}
}

// TestEmitter_Stateful 已移除
// TODO: 实现 Stateful emitter 功能后添加此测试
// Stateful emitter 是高级功能，允许新订阅者接收最后发射的事件
// 参考：pkg/interfaces/eventbus.go Stateful() EmitterOption
