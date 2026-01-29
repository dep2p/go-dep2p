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

	// RegisterHandler 注册消息处理器
	RegisterHandler(protocol string, handler MessageHandler) error

	// UnregisterHandler 注销消息处理器
	UnregisterHandler(protocol string) error

	// Close 关闭服务
	Close() error
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
