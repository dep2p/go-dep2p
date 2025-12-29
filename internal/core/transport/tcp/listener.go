// Package tcp 提供基于 TCP 的传输层实现
package tcp

import (
	"fmt"
	"net"
	"sync/atomic"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// ============================================================================
//                              Listener 实现
// ============================================================================

// Listener TCP 监听器
type Listener struct {
	listener *net.TCPListener
	addr     *Address
	closed   atomic.Bool
}

// 确保实现接口
var _ transportif.Listener = (*Listener)(nil)

// NewListener 创建 TCP 监听器
func NewListener(addr *Address, _ transportif.ListenOptions) (*Listener, error) {
	// 构建监听地址
	listenAddr := fmt.Sprintf("%s:%d", addr.host, addr.port)
	if addr.network == "tcp6" {
		listenAddr = fmt.Sprintf("[%s]:%d", addr.host, addr.port)
	}

	// 监听
	network := "tcp"
	if addr.network == "tcp4" {
		network = "tcp4"
	} else if addr.network == "tcp6" {
		network = "tcp6"
	}

	// 创建监听配置
	lc := net.ListenConfig{
		// KeepAlive 默认启用
	}

	// 创建监听器
	l, err := lc.Listen(nil, network, listenAddr)
	if err != nil {
		return nil, fmt.Errorf("监听失败: %w", err)
	}

	tcpListener, ok := l.(*net.TCPListener)
	if !ok {
		_ = l.Close()
		return nil, fmt.Errorf("不是 TCP 监听器")
	}

	// 获取实际监听地址（端口可能是 0）
	actualAddr, err := NewAddressFromNetAddr(tcpListener.Addr())
	if err != nil {
		_ = tcpListener.Close()
		return nil, fmt.Errorf("获取监听地址失败: %w", err)
	}

	return &Listener{
		listener: tcpListener,
		addr:     actualAddr,
	}, nil
}

// ============================================================================
//                              transport.Listener 接口实现
// ============================================================================

// Accept 接受连接
func (l *Listener) Accept() (transportif.Conn, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("不是 TCP 连接")
	}

	// 设置连接选项
	tcpConn.SetNoDelay(true)
	tcpConn.SetKeepAlive(true)

	return NewConn(tcpConn)
}

// Addr 返回监听地址
func (l *Listener) Addr() endpoint.Address {
	return l.addr
}

// Close 关闭监听器
func (l *Listener) Close() error {
	if l.closed.CompareAndSwap(false, true) {
		return l.listener.Close()
	}
	return nil
}

// Multiaddr 返回多地址格式的监听地址
func (l *Listener) Multiaddr() string {
	return l.addr.String()
}

// ============================================================================
//                              辅助方法
// ============================================================================

// NetListener 返回底层 net.Listener
func (l *Listener) NetListener() *net.TCPListener {
	return l.listener
}

// IsClosed 检查监听器是否已关闭
func (l *Listener) IsClosed() bool {
	return l.closed.Load()
}

