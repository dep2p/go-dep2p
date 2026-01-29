package bootstrap

import (
	"errors"
	"fmt"
)

// 预定义错误
var (
	// ErrNoBootstrapPeers 没有配置引导节点
	ErrNoBootstrapPeers = errors.New("bootstrap: no bootstrap peers configured")
	
	// ErrMinPeersNotMet 最小成功连接数未达到
	ErrMinPeersNotMet = errors.New("bootstrap: minimum peers not met")
	
	// ErrAllConnectionsFailed 所有连接都失败
	ErrAllConnectionsFailed = errors.New("bootstrap: all connections failed")
	
	// ErrTimeout 连接超时
	ErrTimeout = errors.New("bootstrap: timeout")
	
	// ErrAlreadyStarted 服务已启动
	ErrAlreadyStarted = errors.New("bootstrap: already started")
	
	// ErrAlreadyClosed 服务已关闭
	ErrAlreadyClosed = errors.New("bootstrap: already closed")
	
	// ErrNotSupported 操作不支持
	ErrNotSupported = errors.New("bootstrap: operation not supported")

	// ErrNotPubliclyReachable 节点不可公网访问
	ErrNotPubliclyReachable = errors.New("bootstrap: node is not publicly reachable")

	// ErrNotEnabled Bootstrap 能力未启用
	ErrNotEnabled = errors.New("bootstrap: capability not enabled")

	// ErrNodeNotFound 节点未找到
	ErrNodeNotFound = errors.New("bootstrap: node not found")
)

// BootstrapError Bootstrap 错误类型
type BootstrapError struct {
	Op      string // 操作名称
	PeerID  string // 节点 ID（如果适用）
	Err     error  // 底层错误
	Message string // 错误消息
}

// Error 实现 error 接口
func (e *BootstrapError) Error() string {
	if e.PeerID != "" {
		return fmt.Sprintf("bootstrap %s failed for peer %s: %s", e.Op, e.PeerID, e.Message)
	}
	return fmt.Sprintf("bootstrap %s failed: %s", e.Op, e.Message)
}

// Unwrap 支持 errors.Unwrap
func (e *BootstrapError) Unwrap() error {
	return e.Err
}

// NewBootstrapError 创建 Bootstrap 错误
func NewBootstrapError(op string, peerID string, err error, message string) *BootstrapError {
	return &BootstrapError{
		Op:      op,
		PeerID:  peerID,
		Err:     err,
		Message: message,
	}
}
