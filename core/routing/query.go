package routing

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/core/peer"
)

// QueryEventType 表示查询事件的类型
type QueryEventType int

// QueryEventBufferSize 定义事件缓冲区大小
var QueryEventBufferSize = 16

// 定义查询事件类型常量
const (
	// SendingQuery 表示正在向对等节点发送查询
	SendingQuery QueryEventType = iota
	// PeerResponse 表示收到对等节点的响应
	PeerResponse
	// FinalPeer 表示找到"最近"的对等节点(当前未使用)
	FinalPeer
	// QueryError 表示查询过程中发生错误
	QueryError
	// Provider 表示找到了提供者
	Provider
	// Value 表示找到了值
	Value
	// AddingPeer 表示正在添加对等节点到查询
	AddingPeer
	// DialingPeer 表示正在与对等节点建立连接
	DialingPeer
)

// QueryEvent 表示在 DHT 查询过程中发生的每个重要事件
// 包含事件相关的节点ID、类型、响应和额外信息
type QueryEvent struct {
	// ID 表示与事件相关的节点标识符
	ID peer.ID
	// Type 表示事件类型
	Type QueryEventType
	// Responses 包含对等节点的地址信息列表
	Responses []*peer.AddrInfo
	// Extra 包含额外的事件相关信息
	Extra string
}

// routingQueryKey 用于在 context 中存储事件通道的键
type routingQueryKey struct{}

// eventChannel 封装了事件通道及其相关的同步原语
type eventChannel struct {
	// mu 用于保护通道的并发访问
	mu sync.Mutex
	// ctx 用于控制通道的生命周期
	ctx context.Context
	// ch 用于发送查询事件
	ch chan<- *QueryEvent
}

// waitThenClose 在通道注册时在goroutine中启动
// 当context被取消时安全地清理通道
// 参数：
//   - 无
//
// 返回值：
//   - 无
func (e *eventChannel) waitThenClose() {
	<-e.ctx.Done()
	e.mu.Lock()
	close(e.ch)
	// 1. 表示我们已完成
	// 2. 释放内存(以防我们长时间持有它)
	e.ch = nil
	e.mu.Unlock()
}

// send 在事件通道上发送事件，如果传入的context或内部context过期则中止
// 参数：
//   - ctx: context.Context 用于控制发送操作
//   - ev: *QueryEvent 要发送的查询事件
//
// 返回值：
//   - 无
func (e *eventChannel) send(ctx context.Context, ev *QueryEvent) {
	e.mu.Lock()
	// 通道已关闭
	if e.ch == nil {
		e.mu.Unlock()
		return
	}
	// 如果传入的context无关，则等待两者
	select {
	case e.ch <- ev:
	case <-e.ctx.Done():
	case <-ctx.Done():
	}
	e.mu.Unlock()
}

// RegisterForQueryEvents 使用给定的context注册查询事件通道
// 返回的context可以传递给DHT查询以在返回的通道上接收查询事件
// 参数：
//   - ctx: context.Context 用于控制事件通道的生命周期
//
// 返回值：
//   - context.Context: 包含事件通道的新context
//   - <-chan *QueryEvent: 用于接收查询事件的通道
func RegisterForQueryEvents(ctx context.Context) (context.Context, <-chan *QueryEvent) {
	ch := make(chan *QueryEvent, QueryEventBufferSize)
	ech := &eventChannel{ch: ch, ctx: ctx}
	go ech.waitThenClose()
	return context.WithValue(ctx, routingQueryKey{}, ech), ch
}

// PublishQueryEvent 将查询事件发布到与给定context关联的查询事件通道(如果存在)
// 参数：
//   - ctx: context.Context 包含事件通道的context
//   - ev: *QueryEvent 要发布的查询事件
//
// 返回值：
//   - 无
func PublishQueryEvent(ctx context.Context, ev *QueryEvent) {
	ich := ctx.Value(routingQueryKey{})
	if ich == nil {
		return
	}

	// 我们希望在这里发生panic
	ech := ich.(*eventChannel)
	ech.send(ctx, ev)
}

// SubscribesToQueryEvents 检查context是否订阅了查询事件
// 如果此函数返回false，则在该context上调用PublishQueryEvent将不执行任何操作
// 参数：
//   - ctx: context.Context 要检查的context
//
// 返回值：
//   - bool: 如果context订阅了查询事件则返回true，否则返回false
func SubscribesToQueryEvents(ctx context.Context) bool {
	return ctx.Value(routingQueryKey{}) != nil
}
