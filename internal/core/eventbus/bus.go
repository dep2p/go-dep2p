// Package eventbus 实现事件总线
package eventbus

import (
	"errors"
	"reflect"
	"sync"
	"sync/atomic"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/eventbus")

// ============================================================================
// 错误定义
// ============================================================================

var (
	// ErrClosed 事件总线已关闭
	ErrClosed = errors.New("eventbus closed")
	// ErrInvalidEventType 无效的事件类型
	ErrInvalidEventType = errors.New("invalid event type")
	// ErrNonPointerType 非指针类型
	ErrNonPointerType = errors.New("subscribe called with non-pointer type")
)

// ============================================================================
// Bus 实现
// ============================================================================

// Bus 事件总线
type Bus struct {
	mu sync.RWMutex

	// nodes 事件类型节点映射
	nodes map[reflect.Type]*node
}

// node 事件类型节点
type node struct {
	lk        sync.Mutex
	typ       reflect.Type
	sinks     []*Subscription         // 订阅者列表
	nEmitters atomic.Int32              // 发射器引用计数
	keepLast  bool                     // 是否保持最后一个事件（Stateful）
	last      interface{}              // 最后一个事件
	dropCount atomic.Int64              // 丢弃事件计数（用于慢消费者警告）
}

// NewBus 创建新的事件总线
func NewBus() *Bus {
	return &Bus{
		nodes: make(map[reflect.Type]*node),
	}
}

// ============================================================================
// EventBus 接口实现
// ============================================================================

// Subscribe 订阅事件
func (b *Bus) Subscribe(eventType interface{}, opts ...pkgif.SubscriptionOpt) (pkgif.Subscription, error) {
	if eventType == nil {
		return nil, ErrInvalidEventType
	}

	// 创建设置结构
	settings := &subscriptionSettings{
		Buffer: 16, // 默认缓冲区大小
	}

	// 应用选项
	for _, opt := range opts {
		opt(settings)
	}

	// 获取事件类型
	typ := reflect.TypeOf(eventType)
	if typ == nil {
		return nil, ErrInvalidEventType
	}

	// 必须是指针类型
	if typ.Kind() != reflect.Ptr {
		return nil, ErrNonPointerType
	}

	// 获取元素类型
	elemType := typ.Elem()

	// 创建订阅
	sub := &Subscription{
		bus: b,
		typ: elemType,
		out: make(chan interface{}, settings.Buffer),
	}

	// 添加到节点
	b.withNode(elemType, func(n *node) {
		n.sinks = append(n.sinks, sub)

		// 如果是有状态节点，发送最后的事件
		if n.keepLast && n.last != nil {
			select {
			case sub.out <- n.last:
			default:
				// 缓冲区满，跳过
			}
		}
	})

	return sub, nil
}

// Emitter 获取发射器
func (b *Bus) Emitter(eventType interface{}, opts ...pkgif.EmitterOpt) (pkgif.Emitter, error) {
	if eventType == nil {
		return nil, ErrInvalidEventType
	}

	// 创建设置结构
	settings := &emitterSettings{
		Stateful: false,
	}

	// 应用选项
	for _, opt := range opts {
		opt(settings)
	}

	// 获取事件类型
	typ := reflect.TypeOf(eventType)
	if typ == nil {
		return nil, ErrInvalidEventType
	}

	// 必须是指针类型
	if typ.Kind() != reflect.Ptr {
		return nil, ErrNonPointerType
	}

	// 获取元素类型
	elemType := typ.Elem()

	var n *node
	b.withNode(elemType, func(node *node) {
		n = node
		n.nEmitters.Add(1)

		// 设置有状态模式
		if settings.Stateful {
			n.keepLast = true
		}
	})

	e := &Emitter{
		bus:  b,
		node: n,
		typ:  elemType,
	}

	return e, nil
}

// GetAllEventTypes 返回所有已注册的事件类型
func (b *Bus) GetAllEventTypes() []interface{} {
	b.mu.RLock()
	defer b.mu.RUnlock()

	types := make([]interface{}, 0, len(b.nodes))
	for typ := range b.nodes {
		// 返回零值实例
		types = append(types, reflect.Zero(typ).Interface())
	}

	return types
}

// ============================================================================
// 内部方法
// ============================================================================

// withNode 在节点上执行操作
func (b *Bus) withNode(typ reflect.Type, cb func(*node)) {
	b.mu.Lock()

	n, ok := b.nodes[typ]
	if !ok {
		n = &node{
			typ:   typ,
			sinks: make([]*Subscription, 0),
		}
		b.nodes[typ] = n
	}

	n.lk.Lock()
	b.mu.Unlock()

	cb(n)
	n.lk.Unlock()
}

// tryDropNode 尝试删除节点（如果没有订阅者和发射器）
func (b *Bus) tryDropNode(typ reflect.Type) {
	b.mu.Lock()
	n, ok := b.nodes[typ]
	if !ok {
		b.mu.Unlock()
		return
	}

	n.lk.Lock()
	// 检查是否还有活跃的订阅者或发射器
	if len(n.sinks) > 0 || n.nEmitters.Load() > 0 {
		n.lk.Unlock()
		b.mu.Unlock()
		return
	}
	n.lk.Unlock()

	// 删除节点
	delete(b.nodes, typ)
	b.mu.Unlock()
}

// removeSub 移除订阅
func (b *Bus) removeSub(sub *Subscription) {
	b.mu.Lock()
	n, ok := b.nodes[sub.typ]
	if !ok {
		b.mu.Unlock()
		return
	}

	n.lk.Lock()
	b.mu.Unlock()

	// 从 sinks 中移除
	for i, s := range n.sinks {
		if s == sub {
			n.sinks = append(n.sinks[:i], n.sinks[i+1:]...)
			break
		}
	}

	// 检查是否需要删除节点
	shouldDrop := len(n.sinks) == 0 && n.nEmitters.Load() == 0
	n.lk.Unlock()

	if shouldDrop {
		b.tryDropNode(sub.typ)
	}
}

// emit 发射事件到所有订阅者
func (n *node) emit(event interface{}) {
	n.lk.Lock()
	defer n.lk.Unlock()

	// 保存最后的事件（如果是有状态模式）
	if n.keepLast {
		n.last = event
	}

	// 发送到所有订阅者
	for _, sub := range n.sinks {
		select {
		case sub.out <- event:
			// 成功发送
		default:
			// 缓冲区满，丢弃事件
			dropped := n.dropCount.Add(1)
			
			// 每丢弃 100 个事件警告一次，避免日志泛滥
			if dropped%100 == 1 {
				logger.Warn("慢消费者检测", 
					"dropped", dropped,
					"type", n.typ,
					"reason", "subscriber buffer full")
			}
		}
	}
}

// ============================================================================
// 选项设置
// ============================================================================

