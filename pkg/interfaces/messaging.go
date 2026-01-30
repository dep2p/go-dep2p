// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Messaging 接口，提供请求/响应消息模式。
package interfaces

import (
	"context"
	"time"
)

// Messaging 定义消息服务接口
//
// Messaging 提供简单的请求/响应消息模式。
type Messaging interface {
	// Send 发送消息并等待响应
	Send(ctx context.Context, peerID string, protocol string, data []byte) ([]byte, error)

	// SendAsync 异步发送消息
	SendAsync(ctx context.Context, peerID string, protocol string, data []byte) (<-chan *Response, error)

	// ════════════════════════════════════════════════════════════════════════
	// 批量发送 API (v1.1 新增)
	// ════════════════════════════════════════════════════════════════════════

	// SendToMany 批量发送消息（并行执行）
	//
	// 向多个节点发送相同消息，内部并行化以最小化总延迟。
	// 返回每个节点的发送结果。
	//
	// 示例：
	//
	//	results := messaging.SendToMany(ctx, peers, "chat", []byte("hello"))
	//	for _, r := range results {
	//	    if r.Error != nil {
	//	        fmt.Printf("发送到 %s 失败: %v\n", r.PeerID[:8], r.Error)
	//	    }
	//	}
	SendToMany(ctx context.Context, peers []string, protocol string, data []byte) []SendResult

	// Broadcast 广播消息给所有 Realm 成员
	//
	// 向 Realm 内所有成员（排除自己）发送消息。
	// 内部并行化以最小化总延迟。
	//
	// 返回 BroadcastResult，包含成功/失败统计和详细结果。
	//
	// 示例：
	//
	//	result := messaging.Broadcast(ctx, "announce", []byte("important message"))
	//	if result.FailedCount > 0 {
	//	    fmt.Printf("广播部分失败: %d/%d\n", result.FailedCount, result.TotalCount)
	//	}
	Broadcast(ctx context.Context, protocol string, data []byte) *BroadcastResult

	// BroadcastAsync 异步广播（立即返回）
	//
	// 启动广播但不等待完成，适用于火忘场景。
	// 返回一个 channel，可用于异步接收每个节点的发送结果。
	//
	// 示例：
	//
	//	resultsCh := messaging.BroadcastAsync(ctx, "heartbeat", []byte("ping"))
	//	// 可选：稍后检查结果
	//	go func() {
	//	    for r := range resultsCh {
	//	        if r.Error != nil {
	//	            log.Printf("广播失败: %s", r.PeerID)
	//	        }
	//	    }
	//	}()
	BroadcastAsync(ctx context.Context, protocol string, data []byte) <-chan SendResult

	// RegisterHandler 注册消息处理器
	RegisterHandler(protocol string, handler MessageHandler) error

	// UnregisterHandler 注销消息处理器
	UnregisterHandler(protocol string) error

	// Close 关闭服务
	Close() error
}

// ════════════════════════════════════════════════════════════════════════════
// 批量发送结果类型 (v1.1 新增)
// ════════════════════════════════════════════════════════════════════════════

// SendResult 单个发送结果
type SendResult struct {
	// PeerID 目标节点 ID
	PeerID string

	// Response 响应数据
	Response []byte

	// Error 发送错误（如有）
	Error error

	// Latency 发送延迟
	Latency time.Duration
}

// BroadcastResult 广播结果
type BroadcastResult struct {
	// TotalCount 总目标数量
	TotalCount int

	// SuccessCount 成功数量
	SuccessCount int

	// FailedCount 失败数量
	FailedCount int

	// Results 每个节点的详细结果
	Results []SendResult
}

// Success 检查广播是否完全成功
func (r *BroadcastResult) Success() bool {
	return r.FailedCount == 0
}

// PartialSuccess 检查广播是否部分成功
func (r *BroadcastResult) PartialSuccess() bool {
	return r.SuccessCount > 0 && r.FailedCount > 0
}

// AllFailed 检查广播是否完全失败
func (r *BroadcastResult) AllFailed() bool {
	return r.SuccessCount == 0 && r.TotalCount > 0
}

// MessageHandler 消息处理函数类型
type MessageHandler func(ctx context.Context, req *Request) (*Response, error)

// Request 消息请求
type Request struct {
	// ID 请求唯一标识
	ID string

	// From 发送方节点 ID
	From string

	// Protocol 协议标识
	Protocol string

	// Data 请求数据
	Data []byte

	// Timestamp 时间戳
	Timestamp time.Time

	// Metadata 元数据
	Metadata map[string]string
}

// Response 消息响应
type Response struct {
	// ID 对应请求的 ID
	ID string

	// From 响应方节点 ID
	From string

	// Data 响应数据
	Data []byte

	// Error 错误信息
	Error error

	// Timestamp 时间戳
	Timestamp time.Time

	// Latency 响应延迟
	Latency time.Duration

	// Metadata 元数据
	Metadata map[string]string
}

// MessagingOption 消息选项
type MessagingOption func(*MessagingOptions)

// MessagingOptions 消息选项集合
type MessagingOptions struct {
	// Timeout 超时时间
	Timeout time.Duration

	// RetryCount 重试次数
	RetryCount int

	// RetryDelay 重试延迟
	RetryDelay time.Duration
}

// WithTimeout 设置超时时间
func WithTimeout(timeout time.Duration) MessagingOption {
	return func(o *MessagingOptions) {
		o.Timeout = timeout
	}
}

// WithRetry 设置重试参数
func WithRetry(count int, delay time.Duration) MessagingOption {
	return func(o *MessagingOptions) {
		o.RetryCount = count
		o.RetryDelay = delay
	}
}
