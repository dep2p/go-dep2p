package muxer

import (
	"errors"

	"github.com/libp2p/go-yamux/v5"
)

var (
	// ErrStreamReset 流被重置错误
	ErrStreamReset = errors.New("stream reset")

	// ErrConnClosed 连接已关闭错误
	ErrConnClosed = errors.New("connection closed")
)

// parseError 转换 yamux 错误为标准错误
func parseError(err error) error {
	if err == nil {
		return nil
	}

	// 检查流重置错误
	if errors.Is(err, yamux.ErrStreamReset) {
		return ErrStreamReset
	}

	// 检查会话关闭错误
	if errors.Is(err, yamux.ErrSessionShutdown) {
		return ErrConnClosed
	}

	// 返回原始错误
	return err
}
