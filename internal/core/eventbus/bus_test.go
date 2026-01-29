package eventbus

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 接口契约测试
// ============================================================================

// TestBus_ImplementsInterface 验证 Bus 实现接口
func TestBus_ImplementsInterface(t *testing.T) {
	var _ pkgif.EventBus = (*Bus)(nil)
}

// ============================================================================
// 基础功能测试
// ============================================================================

// TestBus_NewBus 测试创建事件总线
func TestBus_NewBus(t *testing.T) {
	bus := NewBus()

	if bus == nil {
		t.Fatal("NewBus() returned nil")
	}

	if bus.nodes == nil {
		t.Error("NewBus() nodes map is nil")
	}
}

// TestBus_Subscribe 测试订阅事件
func TestBus_Subscribe(t *testing.T) {
	bus := NewBus()

	type TestEvent struct {
		Value int
	}

	sub, err := bus.Subscribe(new(TestEvent))
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}

	if sub == nil {
		t.Fatal("Subscribe() returned nil subscription")
	}

	if sub.Out() == nil {
		t.Error("Subscribe() subscription has nil output channel")
	}
}

// TestBus_Emitter 测试获取发射器
func TestBus_Emitter(t *testing.T) {
	bus := NewBus()

	type TestEvent struct {
		Value int
	}

	em, err := bus.Emitter(new(TestEvent))
	if err != nil {
		t.Fatalf("Emitter() failed: %v", err)
	}

	if em == nil {
		t.Fatal("Emitter() returned nil emitter")
	}
}

// TestBus_EmitAndReceive 测试事件发射和接收
func TestBus_EmitAndReceive(t *testing.T) {
	bus := NewBus()

	type TestEvent struct {
		Value int
	}

	// 订阅
	sub, err := bus.Subscribe(new(TestEvent))
	if err != nil {
		t.Fatalf("Subscribe() failed: %v", err)
	}
	defer sub.Close()

	// 发射器
	em, err := bus.Emitter(new(TestEvent))
	if err != nil {
		t.Fatalf("Emitter() failed: %v", err)
	}
	defer em.Close()

	// 发射事件
	testValue := 42
	err = em.Emit(TestEvent{Value: testValue})
	if err != nil {
		t.Errorf("Emit() failed: %v", err)
	}

	// 接收事件
	evt := <-sub.Out()
	received, ok := evt.(TestEvent)
	if !ok {
		t.Fatalf("Received wrong event type: %T", evt)
	}

	if received.Value != testValue {
		t.Errorf("Received event value = %d, want %d", received.Value, testValue)
	}
}

// TestBus_MultipleSubscribers 测试多个订阅者
func TestBus_MultipleSubscribers(t *testing.T) {
	bus := NewBus()

	type TestEvent struct {
		Value int
	}

	// 创建 3 个订阅者
	sub1, _ := bus.Subscribe(new(TestEvent))
	defer sub1.Close()

	sub2, _ := bus.Subscribe(new(TestEvent))
	defer sub2.Close()

	sub3, _ := bus.Subscribe(new(TestEvent))
	defer sub3.Close()

	// 发射事件
	em, _ := bus.Emitter(new(TestEvent))
	defer em.Close()

	testValue := 100
	em.Emit(TestEvent{Value: testValue})

	// 所有订阅者都应收到事件
	evt1 := <-sub1.Out()
	evt2 := <-sub2.Out()
	evt3 := <-sub3.Out()

	if evt1.(TestEvent).Value != testValue {
		t.Errorf("Subscriber 1 received wrong value")
	}
	if evt2.(TestEvent).Value != testValue {
		t.Errorf("Subscriber 2 received wrong value")
	}
	if evt3.(TestEvent).Value != testValue {
		t.Errorf("Subscriber 3 received wrong value")
	}
}

// TestBus_GetAllEventTypes 测试获取所有事件类型
func TestBus_GetAllEventTypes(t *testing.T) {
	bus := NewBus()

	type Event1 struct{}
	type Event2 struct{}

	// 初始应该为空
	types := bus.GetAllEventTypes()
	if len(types) != 0 {
		t.Errorf("GetAllEventTypes() initial length = %d, want 0", len(types))
	}

	// 订阅一些事件
	sub1, _ := bus.Subscribe(new(Event1))
	defer sub1.Close()

	sub2, _ := bus.Subscribe(new(Event2))
	defer sub2.Close()

	// 应该有 2 个事件类型
	types = bus.GetAllEventTypes()
	if len(types) != 2 {
		t.Errorf("GetAllEventTypes() length = %d, want 2", len(types))
	}
}

// TestBus_DifferentEventTypes 测试不同事件类型隔离
func TestBus_DifferentEventTypes(t *testing.T) {
	bus := NewBus()

	type Event1 struct{ Value int }
	type Event2 struct{ Value string }

	// 订阅两种事件
	sub1, _ := bus.Subscribe(new(Event1))
	defer sub1.Close()

	sub2, _ := bus.Subscribe(new(Event2))
	defer sub2.Close()

	// 发射 Event1
	em1, _ := bus.Emitter(new(Event1))
	defer em1.Close()
	em1.Emit(Event1{Value: 42})

	// sub1 应该收到，sub2 不应该收到
	select {
	case evt := <-sub1.Out():
		if evt.(Event1).Value != 42 {
			t.Error("sub1 received wrong value")
		}
	default:
		t.Error("sub1 did not receive Event1")
	}

	select {
	case <-sub2.Out():
		t.Error("sub2 should not receive Event1")
	default:
		// 正确：sub2 没有收到 Event1
	}
}
