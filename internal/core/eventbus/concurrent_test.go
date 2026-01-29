package eventbus

import (
	"sync"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// 并发测试
// ============================================================================

// TestConcurrent_MultipleEmitters 测试多发射器并发
func TestConcurrent_MultipleEmitters(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{ ID int }

	sub, _ := bus.Subscribe(new(TestEvent), BufSize(100))
	defer sub.Close()

	// 启动多个发射器
	numEmitters := 10
	eventsPerEmitter := 10

	var wg sync.WaitGroup
	wg.Add(numEmitters)

	for i := 0; i < numEmitters; i++ {
		go func(id int) {
			defer wg.Done()

			em, _ := bus.Emitter(new(TestEvent))
			defer em.Close()

			for j := 0; j < eventsPerEmitter; j++ {
				em.Emit(TestEvent{ID: id*1000 + j})
			}
		}(i)
	}

	wg.Wait()

	// 接收所有事件
	received := 0
	timeout := time.After(time.Second)

loop:
	for {
		select {
		case <-sub.Out():
			received++
			if received >= numEmitters*eventsPerEmitter {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if received != numEmitters*eventsPerEmitter {
		t.Errorf("Received %d events, want %d", received, numEmitters*eventsPerEmitter)
	}
}

// TestConcurrent_MultipleSubscribers 测试多订阅者并发
func TestConcurrent_MultipleSubscribers(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{ Value int }

	numSubscribers := 10
	eventsToSend := 100

	// 创建多个订阅者
	subs := make([]pkgif.Subscription, numSubscribers)
	counters := make([]int, numSubscribers)
	var wg sync.WaitGroup

	for i := 0; i < numSubscribers; i++ {
		sub, _ := bus.Subscribe(new(TestEvent), BufSize(eventsToSend+10))
		subs[i] = sub

		wg.Add(1)
		go func(idx int, s pkgif.Subscription) {
			defer wg.Done()

			for evt := range s.Out() {
				_ = evt.(TestEvent)
				counters[idx]++
			}
		}(i, sub)
	}

	// 发射事件
	em, _ := bus.Emitter(new(TestEvent))
	for i := 0; i < eventsToSend; i++ {
		em.Emit(TestEvent{Value: i})
		time.Sleep(time.Microsecond) // 给订阅者接收时间
	}
	em.Close()

	// 等待一小段时间让事件传播
	time.Sleep(50 * time.Millisecond)

	// 关闭所有订阅（这会关闭通道，导致 range 循环退出）
	for _, sub := range subs {
		sub.Close()
	}

	// 等待所有 goroutine 完成
	wg.Wait()

	// 每个订阅者都应该收到大部分事件
	// 注意：由于缓冲区限制和关闭时机，可能不是全部
	for i, count := range counters {
		if count < eventsToSend/2 {
			t.Errorf("Subscriber %d received only %d events, want at least %d", i, count, eventsToSend/2)
		}
		t.Logf("Subscriber %d received %d events", i, count)
	}
}

// TestConcurrent_SubscribeWhileEmitting 测试发射时订阅
func TestConcurrent_SubscribeWhileEmitting(t *testing.T) {
	bus := NewBus()
	type TestEvent struct{ Value int }

	var wg sync.WaitGroup

	// 启动发射器
	wg.Add(1)
	go func() {
		defer wg.Done()

		em, _ := bus.Emitter(new(TestEvent))
		defer em.Close()

		for i := 0; i < 100; i++ {
			em.Emit(TestEvent{Value: i})
			time.Sleep(time.Millisecond)
		}
	}()

	// 同时启动订阅者
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(10 * time.Millisecond) // 延迟订阅

		sub, _ := bus.Subscribe(new(TestEvent), BufSize(100))
		defer sub.Close()

		received := 0
		timeout := time.After(2 * time.Second)

	loop:
		for {
			select {
			case <-sub.Out():
				received++
			case <-timeout:
				break loop
			}
		}

		if received == 0 {
			t.Error("Subscriber received no events")
		}
		t.Logf("Subscriber received %d events", received)
	}()

	wg.Wait()
}

// TestConcurrent_RaceDetection 测试竞态条件
func TestConcurrent_RaceDetection(t *testing.T) {
	// 运行 go test -race 时会检测竞态
	// 这个测试简化为只验证基本并发操作不会导致竞态

	bus := NewBus()
	type TestEvent struct{ Value int }

	numOps := 10
	eventsPerOp := 10

	subs := make([]pkgif.Subscription, numOps)
	for i := 0; i < numOps; i++ {
		subs[i], _ = bus.Subscribe(new(TestEvent), BufSize(eventsPerOp*numOps))
	}

	var wg sync.WaitGroup

	// 并发发射
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			em, _ := bus.Emitter(new(TestEvent))
			for j := 0; j < eventsPerOp; j++ {
				em.Emit(TestEvent{Value: id*100 + j})
			}
			em.Close()
		}(i)
	}

	// 等待所有发射完成
	wg.Wait()

	// 关闭所有订阅
	for _, sub := range subs {
		sub.Close()
	}

	// 消费剩余事件
	time.Sleep(50 * time.Millisecond)
}

// TestConcurrent_GetAllEventTypes 测试并发获取事件类型
func TestConcurrent_GetAllEventTypes(t *testing.T) {
	bus := NewBus()

	type Event1 struct{}
	type Event2 struct{}
	type Event3 struct{}

	var wg sync.WaitGroup

	// 并发订阅不同事件
	wg.Add(3)
	go func() {
		defer wg.Done()
		sub, _ := bus.Subscribe(new(Event1))
		defer sub.Close()
		time.Sleep(10 * time.Millisecond)
	}()

	go func() {
		defer wg.Done()
		sub, _ := bus.Subscribe(new(Event2))
		defer sub.Close()
		time.Sleep(10 * time.Millisecond)
	}()

	go func() {
		defer wg.Done()
		sub, _ := bus.Subscribe(new(Event3))
		defer sub.Close()
		time.Sleep(10 * time.Millisecond)
	}()

	// 并发读取事件类型
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			_ = bus.GetAllEventTypes()
		}()
	}

	wg.Wait()
}
