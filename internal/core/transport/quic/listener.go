// Package quic 提供基于 QUIC 的传输层实现
package quic

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/quic-go/quic-go"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// 错误定义
var (
	// ErrListenerClosed 监听器已关闭
	ErrListenerClosed = errors.New("listener closed")
)

// Listener QUIC 监听器实现
type Listener struct {
	quicListener *quic.Listener
	addr         *Address
	transport    *Transport
	closed       atomic.Bool
}

// 确保实现 transport.Listener 接口
var _ transportif.Listener = (*Listener)(nil)

// NewListener 创建监听器
func NewListener(ql *quic.Listener, addr *Address, transport *Transport) *Listener {
	return &Listener{
		quicListener: ql,
		addr:         addr,
		transport:    transport,
	}
}

// Accept 接受连接
// 注意: 此方法会阻塞直到有新连接或监听器关闭
// 推荐使用 AcceptWithContext 以支持超时和取消
func (l *Listener) Accept() (transportif.Conn, error) {
	if l.closed.Load() {
		return nil, ErrListenerClosed
	}

	// 接受 QUIC 连接
	// 当监听器关闭时，Accept 会返回错误
	quicConn, err := l.quicListener.Accept(context.Background())
	if err != nil {
		// 检查是否是监听器关闭导致的错误
		if l.closed.Load() {
			return nil, ErrListenerClosed
		}
		return nil, fmt.Errorf("接受连接失败: %w", err)
	}

	// 创建连接封装
	conn := NewConn(quicConn, l.transport)

	return conn, nil
}

// AcceptWithContext 使用上下文接受连接
// 支持通过 context 进行超时控制和取消
func (l *Listener) AcceptWithContext(ctx context.Context) (transportif.Conn, error) {
	if l.closed.Load() {
		return nil, ErrListenerClosed
	}

	// 接受 QUIC 连接
	quicConn, err := l.quicListener.Accept(ctx)
	if err != nil {
		// 检查是否是监听器关闭导致的错误
		if l.closed.Load() {
			return nil, ErrListenerClosed
		}
		return nil, fmt.Errorf("接受连接失败: %w", err)
	}

	// 创建连接封装
	conn := NewConn(quicConn, l.transport)

	return conn, nil
}

// Addr 返回监听地址
func (l *Listener) Addr() endpoint.Address {
	return l.addr
}

// Close 关闭监听器
func (l *Listener) Close() error {
	if l.closed.Swap(true) {
		return nil // 已经关闭
	}
	return l.quicListener.Close()
}

// Multiaddr 返回多地址格式的监听地址
func (l *Listener) Multiaddr() string {
	return l.addr.Multiaddr()
}

// QuicListener 返回底层 QUIC 监听器
func (l *Listener) QuicListener() *quic.Listener {
	return l.quicListener
}

// IsClosed 检查监听器是否已关闭
func (l *Listener) IsClosed() bool {
	return l.closed.Load()
}
