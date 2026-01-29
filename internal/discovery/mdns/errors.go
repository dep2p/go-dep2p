package mdns

import (
	"errors"
	"fmt"
)

// 预定义错误
var (
	// ErrNotStarted 服务未启动
	ErrNotStarted = errors.New("mdns: service not started")

	// ErrAlreadyStarted 服务已启动
	ErrAlreadyStarted = errors.New("mdns: already started")

	// ErrAlreadyClosed 服务已关闭
	ErrAlreadyClosed = errors.New("mdns: already closed")

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("mdns: invalid config")

	// ErrNoValidAddresses 没有有效地址
	ErrNoValidAddresses = errors.New("mdns: no valid addresses for advertisement")

	// ErrNilHost Host 为 nil
	ErrNilHost = errors.New("mdns: host is nil")

	// ErrServerStart 服务器启动失败
	ErrServerStart = errors.New("mdns: failed to start server")

	// ErrResolverStart 解析器启动失败
	ErrResolverStart = errors.New("mdns: failed to start resolver")
)

// MDNSError 自定义错误类型
type MDNSError struct {
	Op      string // 操作名称
	Err     error  // 原始错误
	Message string // 错误信息
}

// Error 实现 error 接口
func (e *MDNSError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("mdns: %s: %s: %v", e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("mdns: %s: %s", e.Op, e.Message)
}

// Unwrap 支持 errors.Unwrap
func (e *MDNSError) Unwrap() error {
	return e.Err
}

// NewMDNSError 创建自定义错误
func NewMDNSError(op string, err error, message string) *MDNSError {
	return &MDNSError{
		Op:      op,
		Err:     err,
		Message: message,
	}
}
