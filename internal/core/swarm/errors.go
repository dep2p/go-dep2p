package swarm

import (
	"errors"
	"fmt"
)

var (
	// ErrSwarmClosed Swarm 已关闭
	ErrSwarmClosed = errors.New("swarm closed")

	// ErrNoAddresses 没有可用地址
	ErrNoAddresses = errors.New("no addresses")

	// ErrDialTimeout 拨号超时
	ErrDialTimeout = errors.New("dial timeout")

	// ErrNoTransport 没有可用传输层
	ErrNoTransport = errors.New("no transport for address")

	// ErrAllDialsFailed 所有拨号都失败
	ErrAllDialsFailed = errors.New("all dials failed")

	// ErrNoConnection 没有连接
	ErrNoConnection = errors.New("no connection to peer")

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("invalid config")

	// ErrDialToSelf 尝试拨号自己
	ErrDialToSelf = errors.New("dial to self attempted")

	// ErrNoRelayAvailable 没有可用的 Relay
	ErrNoRelayAvailable = errors.New("no relay available")
)

// DialError 拨号错误，包含多个地址的错误信息
type DialError struct {
	Peer   string
	Errors []error
}

func (e *DialError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("failed to dial %s: unknown error", e.Peer)
	}
	if len(e.Errors) == 1 {
		return fmt.Sprintf("failed to dial %s: %v", e.Peer, e.Errors[0])
	}
	return fmt.Sprintf("failed to dial %s: %d errors: %v", e.Peer, len(e.Errors), e.Errors)
}

// Unwrap 返回第一个错误
func (e *DialError) Unwrap() error {
	if len(e.Errors) == 0 {
		return nil
	}
	return e.Errors[0]
}
