package eventbus

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/core/event"
	logging "github.com/dep2p/log"
)

// logInterface 定义日志接口
type logInterface interface {
	// Errorf 记录错误级别日志
	// 参数:
	//   - format: string 日志格式字符串
	//   - args: ...interface{} 格式化参数
	Errorf(format string, args ...interface{})
}

// log 全局日志对象
var log logInterface = logging.Logger("host-eventbus")

// slowConsumerWarningTimeout 慢速消费者警告超时时间
const slowConsumerWarningTimeout = time.Second

// /////////////////////
// 事件总线

// basicBus 是一个基于类型的事件分发系统
type basicBus struct {
	lk            sync.RWMutex           // 读写锁,保护 nodes 字段
	nodes         map[reflect.Type]*node // 事件类型到节点的映射
	wildcard      *wildcardNode          // 通配符订阅节点
	metricsTracer MetricsTracer          // 指标跟踪器
}

// 确保 basicBus 实现了 event.Bus 接口
var _ event.Bus = (*basicBus)(nil)

// emitter 事件发射器
type emitter struct {
	n             *node              // 事件节点
	w             *wildcardNode      // 通配符节点
	typ           reflect.Type       // 事件类型
	closed        atomic.Bool        // 是否已关闭
	dropper       func(reflect.Type) // 节点删除函数
	metricsTracer MetricsTracer      // 指标跟踪器
}

// Emit 发送事件
// 参数:
//   - evt: interface{} 要发送的事件
//
// 返回:
//   - error 发送错误
func (e *emitter) Emit(evt interface{}) error {
	// 检查发射器是否已关闭
	if e.closed.Load() {
		log.Errorf("发射器已关闭")
		return fmt.Errorf("发射器已关闭")
	}

	// 发送事件到普通节点
	e.n.emit(evt)
	// 发送事件到通配符节点
	e.w.emit(evt)

	// 记录指标
	if e.metricsTracer != nil {
		e.metricsTracer.EventEmitted(e.typ)
	}
	return nil
}

// Close 关闭发射器
// 返回:
//   - error 关闭错误
func (e *emitter) Close() error {
	// 检查是否已关闭
	if !e.closed.CompareAndSwap(false, true) {
		log.Errorf("多次关闭同一个发射器")
		return fmt.Errorf("多次关闭同一个发射器")
	}
	// 减少发射器计数,如果为0则删除节点
	if e.n.nEmitters.Add(-1) == 0 {
		e.dropper(e.typ)
	}
	return nil
}

// NewBus 创建新的事件总线
// 参数:
//   - opts: ...Option 配置选项
//
// 返回:
//   - event.Bus 事件总线实例
func NewBus(opts ...Option) event.Bus {
	bus := &basicBus{
		nodes:    map[reflect.Type]*node{},
		wildcard: &wildcardNode{},
	}
	// 应用配置选项
	for _, opt := range opts {
		opt(bus)
	}
	return bus
}

// withNode 对节点执行操作
// 参数:
//   - typ: reflect.Type 事件类型
//   - cb: func(*node) 同步回调函数
//   - async: func(*node) 异步回调函数
func (b *basicBus) withNode(typ reflect.Type, cb func(*node), async func(*node)) {
	b.lk.Lock()

	// 获取或创建节点
	n, ok := b.nodes[typ]
	if !ok {
		n = newNode(typ, b.metricsTracer)
		b.nodes[typ] = n
	}

	// 锁定节点
	n.lk.Lock()
	b.lk.Unlock()

	// 执行回调
	cb(n)

	// 同步或异步解锁
	if async == nil {
		n.lk.Unlock()
	} else {
		go func() {
			defer n.lk.Unlock()
			async(n)
		}()
	}
}

// tryDropNode 尝试删除节点
// 参数:
//   - typ: reflect.Type 要删除的节点类型
func (b *basicBus) tryDropNode(typ reflect.Type) {
	b.lk.Lock()
	n, ok := b.nodes[typ]
	if !ok { // 已经删除
		b.lk.Unlock()
		return
	}

	// 检查节点是否仍在使用
	n.lk.Lock()
	if n.nEmitters.Load() > 0 || len(n.sinks) > 0 {
		n.lk.Unlock()
		b.lk.Unlock()
		return // 仍在使用中
	}
	n.lk.Unlock()

	// 删除节点
	delete(b.nodes, typ)
	b.lk.Unlock()
}

// wildcardSub 通配符订阅
type wildcardSub struct {
	ch            chan interface{} // 事件通道
	w             *wildcardNode    // 通配符节点
	metricsTracer MetricsTracer    // 指标跟踪器
	name          string           // 订阅者名称
	closeOnce     sync.Once        // 确保只关闭一次
}

// Out 返回事件通道
// 返回:
//   - <-chan interface{} 只读事件通道
func (w *wildcardSub) Out() <-chan interface{} {
	return w.ch
}

// Close 关闭订阅
// 返回:
//   - error 关闭错误
func (w *wildcardSub) Close() error {
	w.closeOnce.Do(func() {
		// 移除订阅并更新指标
		w.w.removeSink(w.ch)
		if w.metricsTracer != nil {
			w.metricsTracer.RemoveSubscriber(reflect.TypeOf(event.WildcardSubscription))
		}
	})

	return nil
}

// Name 返回订阅者名称
// 返回:
//   - string 订阅者名称
func (w *wildcardSub) Name() string {
	return w.name
}

// namedSink 命名的事件接收器
type namedSink struct {
	name string           // 接收器名称
	ch   chan interface{} // 事件通道
}

// sub 普通订阅
type sub struct {
	ch            chan interface{}   // 事件通道
	nodes         []*node            // 订阅的节点列表
	dropper       func(reflect.Type) // 节点删除函数
	metricsTracer MetricsTracer      // 指标跟踪器
	name          string             // 订阅者名称
	closeOnce     sync.Once          // 确保只关闭一次
}

// Name 返回订阅者名称
// 返回:
//   - string 订阅者名称
func (s *sub) Name() string {
	return s.name
}

// Out 返回事件通道
// 返回:
//   - <-chan interface{} 只读事件通道
func (s *sub) Out() <-chan interface{} {
	return s.ch
}

// Close 关闭订阅
// 返回:
//   - error 关闭错误
func (s *sub) Close() error {
	go func() {
		// 清空事件通道,直到关闭并清空完成。
		// 这对于解除对此通道的发布阻塞是必要的。
		for range s.ch {
		}
	}()
	s.closeOnce.Do(func() {
		// 遍历所有节点
		for _, n := range s.nodes {
			n.lk.Lock()

			// 移除订阅
			for i := 0; i < len(n.sinks); i++ {
				if n.sinks[i].ch == s.ch {
					n.sinks[i], n.sinks[len(n.sinks)-1] = n.sinks[len(n.sinks)-1], nil
					n.sinks = n.sinks[:len(n.sinks)-1]

					// 更新指标
					if s.metricsTracer != nil {
						s.metricsTracer.RemoveSubscriber(n.typ)
					}
					break
				}
			}

			// 检查是否可以删除节点
			tryDrop := len(n.sinks) == 0 && n.nEmitters.Load() == 0

			n.lk.Unlock()

			if tryDrop {
				s.dropper(n.typ)
			}
		}
		close(s.ch)
	})
	return nil
}

// 确保 sub 实现了 event.Subscription 接口
var _ event.Subscription = (*sub)(nil)

// Subscribe 创建新的订阅。未能及时清空通道将导致发布者被阻塞。CancelFunc 保证在通道最后一次发送后返回
// 参数:
//   - evtTypes: interface{} 要订阅的事件类型
//   - opts: ...event.SubscriptionOpt 订阅选项
//
// 返回:
//   - event.Subscription 订阅对象
//   - error 订阅错误
func (b *basicBus) Subscribe(evtTypes interface{}, opts ...event.SubscriptionOpt) (_ event.Subscription, err error) {
	// 应用订阅设置
	settings := newSubSettings()
	for _, opt := range opts {
		if err := opt(&settings); err != nil {
			log.Errorf("订阅设置失败: %v", err)
			return nil, err
		}
	}

	// 处理通配符订阅
	if evtTypes == event.WildcardSubscription {
		out := &wildcardSub{
			ch:            make(chan interface{}, settings.buffer),
			w:             b.wildcard,
			metricsTracer: b.metricsTracer,
			name:          settings.name,
		}
		b.wildcard.addSink(&namedSink{ch: out.ch, name: out.name})
		return out, nil
	}

	// 转换事件类型为切片
	types, ok := evtTypes.([]interface{})
	if !ok {
		types = []interface{}{evtTypes}
	}

	// 检查通配符订阅
	if len(types) > 1 {
		for _, t := range types {
			if t == event.WildcardSubscription {
				log.Errorf("通配符订阅必须单独启动")
				return nil, fmt.Errorf("通配符订阅必须单独启动")
			}
		}
	}

	// 创建订阅对象
	out := &sub{
		ch:    make(chan interface{}, settings.buffer),
		nodes: make([]*node, len(types)),

		dropper:       b.tryDropNode,
		metricsTracer: b.metricsTracer,
		name:          settings.name,
	}

	// 检查类型
	for _, etyp := range types {
		if reflect.TypeOf(etyp).Kind() != reflect.Ptr {
			log.Errorf("使用非指针类型调用订阅")
			return nil, errors.New("使用非指针类型调用订阅")
		}
	}

	// 订阅每个类型
	for i, etyp := range types {
		typ := reflect.TypeOf(etyp)

		b.withNode(typ.Elem(), func(n *node) {
			// 添加接收器
			n.sinks = append(n.sinks, &namedSink{ch: out.ch, name: out.name})
			out.nodes[i] = n
			// 更新指标
			if b.metricsTracer != nil {
				b.metricsTracer.AddSubscriber(typ.Elem())
			}
		}, func(n *node) {
			// 发送最后一个事件(如果有)
			if n.keepLast {
				l := n.last
				if l == nil {
					return
				}
				out.ch <- l
			}
		})
	}

	return out, nil
}

// Emitter 创建新的发射器
//
// eventType 接受类型化的 nil 指针，并使用类型信息来选择输出类型
//
// 示例:
// emit, err := eventbus.Emitter(new(EventT))
// defer emit.Close() // 使用完发射器后必须调用此函数
//
// emit(EventT{})
//
// 参数:
//   - evtType: interface{} 事件类型
//   - opts: ...event.EmitterOpt 发射器选项
//
// 返回:
//   - event.Emitter 发射器对象
//   - error 创建错误
func (b *basicBus) Emitter(evtType interface{}, opts ...event.EmitterOpt) (e event.Emitter, err error) {
	// 检查通配符
	if evtType == event.WildcardSubscription {
		log.Errorf("通配符订阅的非法发射器")
		return nil, fmt.Errorf("通配符订阅的非法发射器")
	}

	// 应用设置
	var settings emitterSettings
	for _, opt := range opts {
		if err := opt(&settings); err != nil {
			log.Errorf("发射器设置失败: %v", err)
			return nil, err
		}
	}

	// 获取类型
	typ := reflect.TypeOf(evtType)
	if typ.Kind() != reflect.Ptr {
		log.Errorf("使用非指针类型调用发射器")
		return nil, errors.New("使用非指针类型调用发射器")
	}
	typ = typ.Elem()

	// 创建发射器
	b.withNode(typ, func(n *node) {
		n.nEmitters.Add(1)
		n.keepLast = n.keepLast || settings.makeStateful
		e = &emitter{n: n, typ: typ, dropper: b.tryDropNode, w: b.wildcard, metricsTracer: b.metricsTracer}
	}, nil)
	return
}

// GetAllEventTypes 返回此总线拥有的所有事件类型的发射器或订阅者。
// 返回:
//   - []reflect.Type 事件类型列表
func (b *basicBus) GetAllEventTypes() []reflect.Type {
	b.lk.RLock()
	defer b.lk.RUnlock()

	types := make([]reflect.Type, 0, len(b.nodes))
	for t := range b.nodes {
		types = append(types, t)
	}
	return types
}

// /////////////////////
// 节点

// wildcardNode 通配符节点
type wildcardNode struct {
	sync.RWMutex                // 读写锁
	nSinks        atomic.Int32  // 接收器数量
	sinks         []*namedSink  // 接收器列表
	metricsTracer MetricsTracer // 指标跟踪器

	slowConsumerTimer *time.Timer // 慢速消费者定时器
}

// addSink 添加接收器
// 参数:
//   - sink: *namedSink 要添加的接收器
func (n *wildcardNode) addSink(sink *namedSink) {
	n.nSinks.Add(1) // 在锁外执行是安全的
	n.Lock()
	n.sinks = append(n.sinks, sink)
	n.Unlock()

	// 更新指标
	if n.metricsTracer != nil {
		n.metricsTracer.AddSubscriber(reflect.TypeOf(event.WildcardSubscription))
	}
}

// removeSink 移除接收器
// 参数:
//   - ch: chan interface{} 要移除的通道
func (n *wildcardNode) removeSink(ch chan interface{}) {
	go func() {
		// 清空事件通道,直到关闭并清空完成。
		// 这对于解除对此通道的发布阻塞是必要的。
		for range ch {
		}
	}()
	n.nSinks.Add(-1) // 在锁外执行是安全的
	n.Lock()
	for i := 0; i < len(n.sinks); i++ {
		if n.sinks[i].ch == ch {
			n.sinks[i], n.sinks[len(n.sinks)-1] = n.sinks[len(n.sinks)-1], nil
			n.sinks = n.sinks[:len(n.sinks)-1]
			break
		}
	}
	n.Unlock()
}

// wildcardType 通配符类型
var wildcardType = reflect.TypeOf(event.WildcardSubscription)

// emit 发送事件
// 参数:
//   - evt: interface{} 要发送的事件
func (n *wildcardNode) emit(evt interface{}) {
	if n.nSinks.Load() == 0 {
		return
	}

	n.RLock()
	for _, sink := range n.sinks {

		// 在发送到通道之前发送指标,允许我们在阻塞之前记录通道已满事件
		sendSubscriberMetrics(n.metricsTracer, sink)

		select {
		case sink.ch <- evt:
		default:
			slowConsumerTimer := emitAndLogError(n.slowConsumerTimer, wildcardType, evt, sink)
			defer func() {
				n.Lock()
				n.slowConsumerTimer = slowConsumerTimer
				n.Unlock()
			}()
		}
	}
	n.RUnlock()
}

// node 事件节点
type node struct {
	// 注意：当持有此锁时，确保永远不要锁定 basicBus.lk
	lk sync.Mutex

	typ reflect.Type // 事件类型

	// 发射器引用计数
	nEmitters atomic.Int32

	keepLast bool        // 是否保留最后一个事件
	last     interface{} // 最后一个事件

	sinks         []*namedSink  // 接收器列表
	metricsTracer MetricsTracer // 指标跟踪器

	slowConsumerTimer *time.Timer // 慢速消费者定时器
}

// newNode 创建新节点
// 参数:
//   - typ: reflect.Type 事件类型
//   - metricsTracer: MetricsTracer 指标跟踪器
//
// 返回:
//   - *node 新节点
func newNode(typ reflect.Type, metricsTracer MetricsTracer) *node {
	return &node{
		typ:           typ,
		metricsTracer: metricsTracer,
	}
}

// emit 发送事件
// 参数:
//   - evt: interface{} 要发送的事件
func (n *node) emit(evt interface{}) {
	typ := reflect.TypeOf(evt)
	if typ != n.typ {
		log.Errorf("使用错误类型调用 Emit。预期: %s, 实际: %s", n.typ, typ)
		panic(fmt.Sprintf("使用错误类型调用 Emit。预期: %s, 实际: %s", n.typ, typ))
	}

	n.lk.Lock()
	if n.keepLast {
		n.last = evt
	}

	for _, sink := range n.sinks {

		// 在发送到通道之前发送指标,允许我们在阻塞之前记录通道已满事件
		sendSubscriberMetrics(n.metricsTracer, sink)
		select {
		case sink.ch <- evt:
		default:
			n.slowConsumerTimer = emitAndLogError(n.slowConsumerTimer, n.typ, evt, sink)
		}
	}
	n.lk.Unlock()
}

// emitAndLogError 发送事件并记录错误
// 参数:
//   - timer: *time.Timer 定时器
//   - typ: reflect.Type 事件类型
//   - evt: interface{} 事件
//   - sink: *namedSink 接收器
//
// 返回:
//   - *time.Timer 更新后的定时器
func emitAndLogError(timer *time.Timer, typ reflect.Type, evt interface{}, sink *namedSink) *time.Timer {
	// 慢速消费者。如果超时则记录警告
	if timer == nil {
		timer = time.NewTimer(slowConsumerWarningTimeout)
	} else {
		timer.Reset(slowConsumerWarningTimeout)
	}

	select {
	case sink.ch <- evt:
		if !timer.Stop() {
			<-timer.C
		}
	case <-timer.C:
		log.Errorf("名为 \"%s\" 的订阅者是 %s 的慢速消费者。这可能导致 dep2p 停滞和难以调试的问题。", sink.name, typ)
		// 继续阻塞因为我们无能为力
		sink.ch <- evt
	}

	return timer
}

// sendSubscriberMetrics 发送订阅者指标
// 参数:
//   - metricsTracer: MetricsTracer 指标跟踪器
//   - sink: *namedSink 接收器
func sendSubscriberMetrics(metricsTracer MetricsTracer, sink *namedSink) {
	if metricsTracer != nil {
		metricsTracer.SubscriberQueueLength(sink.name, len(sink.ch)+1)
		metricsTracer.SubscriberQueueFull(sink.name, len(sink.ch)+1 >= cap(sink.ch))
		metricsTracer.SubscriberEventQueued(sink.name)
	}
}
