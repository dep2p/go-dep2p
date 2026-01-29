// Package delivery 提供可靠消息投递功能
package delivery

// ============================================================================
//                              错误定义
// ============================================================================

// 错误定义
var (
	// ErrQueueFull 队列已满
	ErrQueueFull = &DeliveryError{Message: "message queue is full"}

	// ErrMaxRetries 超过最大重试次数
	ErrMaxRetries = &DeliveryError{Message: "max retry attempts exceeded"}

	// ErrAckTimeout ACK 超时
	ErrAckTimeout = &DeliveryError{Message: "ack timeout"}

	// ErrAckDisabled ACK 未启用
	ErrAckDisabled = &DeliveryError{Message: "ack is disabled"}

	// ErrNoCriticalPeers 未配置关键节点
	ErrNoCriticalPeers = &DeliveryError{Message: "no critical peers configured"}

	// ErrNoUnderlying 未设置底层发布器
	ErrNoUnderlying = &DeliveryError{Message: "no underlying publisher"}

	// ErrAlreadyStarted 已启动
	ErrAlreadyStarted = &DeliveryError{Message: "publisher already started"}
)

// DeliveryError 投递错误
type DeliveryError struct {
	Message string
	Cause   error
}

func (e *DeliveryError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *DeliveryError) Unwrap() error {
	return e.Cause
}
