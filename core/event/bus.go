package event

import (
	"io"
	"reflect"
)

// SubscriptionOpt 表示订阅者选项。使用所选实现提供的选项。
type SubscriptionOpt = func(interface{}) error

// EmitterOpt 表示发射器选项。使用所选实现提供的选项。
type EmitterOpt = func(interface{}) error

// CancelFunc 用于关闭订阅者。
type CancelFunc = func()

// wildcardSubscriptionType 是一个虚拟类型,用于表示通配符订阅。
type wildcardSubscriptionType interface{}

// WildcardSubscription 是用于订阅事件总线上发出的所有事件的类型。
var WildcardSubscription = new(wildcardSubscriptionType)

// Emitter 表示在事件总线上发出事件的参与者。
type Emitter interface {
	io.Closer

	// Emit 在事件总线上发出一个事件。
	// 如果订阅该主题的任何通道被阻塞,Emit 的调用也会被阻塞。
	//
	// 使用错误的事件类型调用此函数将导致 panic。
	Emit(evt interface{}) error
}

// Subscription 表示对一个或多个事件类型的订阅。
type Subscription interface {
	io.Closer

	// Out 返回用于消费事件的通道。
	Out() <-chan interface{}

	// Name 返回订阅的名称
	Name() string
}

// Bus 是一个基于类型的事件传递系统的接口。
type Bus interface {
	// Subscribe 创建一个新的订阅。
	//
	// eventType 可以是指向单个事件类型的指针,也可以是指向多个事件类型的指针切片,
	// 以便一次性订阅多个事件类型(在单个订阅和通道下)。
	//
	// 未能及时消费通道可能导致发布者阻塞。
	//
	// 如果你想订阅总线上发出的所有事件,请使用 `WildcardSubscription` 作为 `eventType`:
	//
	//  eventbus.Subscribe(WildcardSubscription)
	//
	// 简单示例
	//
	//  sub, err := eventbus.Subscribe(new(EventType))
	//  defer sub.Close()
	//  for e := range sub.Out() {
	//    event := e.(EventType) // 保证安全
	//    [...]
	//  }
	//
	// 多类型示例
	//
	//  sub, err := eventbus.Subscribe([]interface{}{new(EventA), new(EventB)})
	//  defer sub.Close()
	//  for e := range sub.Out() {
	//    select e.(type):
	//      case EventA:
	//        [...]
	//      case EventB:
	//        [...]
	//    }
	//  }
	Subscribe(eventType interface{}, opts ...SubscriptionOpt) (Subscription, error)

	// Emitter 创建一个新的事件发射器。
	//
	// eventType 接受类型化的 nil 指针,并使用类型信息进行连接。
	//
	// 示例:
	//  em, err := eventbus.Emitter(new(EventT))
	//  defer em.Close() // 使用完发射器后必须调用此函数
	//  em.Emit(EventT{})
	Emitter(eventType interface{}, opts ...EmitterOpt) (Emitter, error)

	// GetAllEventTypes 返回此总线知道的所有事件类型(具有发射器和订阅者)。
	// 它会忽略 WildcardSubscription。
	//
	// 调用者可以保证此函数只返回值类型;
	// 不会返回指针类型。
	GetAllEventTypes() []reflect.Type
}
