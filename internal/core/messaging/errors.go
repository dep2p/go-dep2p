// Package messaging 提供消息服务模块的实现
package messaging

import "errors"

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrServiceClosed 服务已关闭
	ErrServiceClosed = errors.New("messaging service closed")

	// ErrTimeout 请求超时
	ErrTimeout = errors.New("request timeout")

	// ErrNoConnection 无法连接到节点
	ErrNoConnection = errors.New("no connection to peer")

	// ErrStreamFailed 打开流失败
	ErrStreamFailed = errors.New("failed to open stream")

	// ErrNoHandler 没有处理器
	ErrNoHandler = errors.New("no handler for protocol")

	// ErrInvalidResponse 无效响应
	ErrInvalidResponse = errors.New("invalid response")
)

