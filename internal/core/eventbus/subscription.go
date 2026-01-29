// Package eventbus 实现事件总线
package eventbus

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
)

// ============================================================================
// Subscription 实现
// ============================================================================

// Subscription 订阅
type Subscription struct {
	bus       *Bus
	typ       reflect.Type
	out       chan interface{}
	closeOnce sync.Once
	closed    atomic.Bool
}

// Out 返回事件通道
func (s *Subscription) Out() <-chan interface{} {
	return s.out
}

// Close 取消订阅
//
// Close 是并发安全的，可以多次调用。
// 关闭后会：
//  1. 从总线移除订阅
//  2. 后台排空通道（防止阻塞发射者）
//  3. 关闭通道
func (s *Subscription) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)

		// 从总线移除
		s.bus.removeSub(s)

		// 后台排空通道，防止阻塞发射者
		go func() {
			for range s.out {
				// 丢弃剩余事件
			}
		}()

		// 关闭通道
		close(s.out)
	})

	return nil
}


// ============================================================================
// Emitter 实现
// ============================================================================

// Emitter 事件发射器
type Emitter struct {
	bus       *Bus
	node      *node
	typ       reflect.Type
	closed    atomic.Bool
	closeOnce sync.Once
}

// Emit 发射事件
func (e *Emitter) Emit(event interface{}) error {
	if e.closed.Load() {
		return errors.New("emitter is closed")
	}

	// 发射到节点
	e.node.emit(event)

	return nil
}

// Close 关闭发射器
//
// 关闭后：
//  1. 标记为已关闭
//  2. 减少引用计数
//  3. 如果计数为 0，尝试删除节点
func (e *Emitter) Close() error {
	e.closeOnce.Do(func() {
		e.closed.Store(true)

		// 减少引用计数
		if e.node.nEmitters.Add(-1) == 0 {
			e.bus.tryDropNode(e.typ)
		}
	})

	return nil
}
