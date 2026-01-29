// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 EventBus 接口，提供事件发布订阅功能。
package interfaces

// EventBus 定义事件总线接口
//
// EventBus 提供类型安全的事件发布/订阅机制。
type EventBus interface {
	// Subscribe 订阅指定类型的事件
	Subscribe(eventType interface{}, opts ...SubscriptionOpt) (Subscription, error)

	// Emitter 获取指定事件类型的发射器
	Emitter(eventType interface{}, opts ...EmitterOpt) (Emitter, error)

	// GetAllEventTypes 返回所有已注册的事件类型
	GetAllEventTypes() []interface{}
}

// Subscription 定义事件订阅接口
type Subscription interface {
	// Out 返回接收事件的通道
	Out() <-chan interface{}

	// Close 取消订阅
	Close() error
}

// Emitter 定义事件发射器接口
type Emitter interface {
	// Emit 发射事件
	Emit(event interface{}) error

	// Close 关闭发射器
	Close() error
}

// SubscriptionOpt 订阅选项函数类型
type SubscriptionOpt func(*SubscriptionSettings)

// EmitterOpt 发射器选项函数类型
type EmitterOpt func(*EmitterSettings)

// SubscriptionSettings 订阅设置（导出以供实现使用）
type SubscriptionSettings struct {
	Buffer int
}

// EmitterSettings 发射器设置（导出以供实现使用）
type EmitterSettings struct {
	Stateful bool
}

// BufSize 设置订阅缓冲区大小
func BufSize(size int) SubscriptionOpt {
	return func(s *SubscriptionSettings) {
		s.Buffer = size
	}
}

// Stateful 设置发射器为有状态模式
func Stateful() EmitterOpt {
	return func(s *EmitterSettings) {
		s.Stateful = true
	}
}
