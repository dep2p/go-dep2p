// Package tcp 提供基于 TCP 的传输层实现
//
// TCP 传输是 QUIC 的备选方案，用于以下场景：
// - UDP 被防火墙阻止
// - 需要与只支持 TCP 的节点通信
// - 调试和测试目的
//
// 注意：TCP 传输不提供原生多路复用，需要配合 Muxer 使用。
package tcp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// ============================================================================
//                              Transport 实现
// ============================================================================

// Transport TCP 传输层实现
type Transport struct {
	config transportif.Config

	listeners   map[string]*Listener
	listenersMu sync.RWMutex

	conns   map[string]*Conn
	connsMu sync.RWMutex

	closed atomic.Bool
}

// 确保实现 transport.Transport 接口
var _ transportif.Transport = (*Transport)(nil)

// NewTransport 创建 TCP 传输层
func NewTransport(config transportif.Config) *Transport {
	return &Transport{
		config:    config,
		listeners: make(map[string]*Listener),
		conns:     make(map[string]*Conn),
	}
}

// ============================================================================
//                              Transport 接口实现
// ============================================================================

// Dial 建立出站连接
func (t *Transport) Dial(ctx context.Context, addr endpoint.Address) (transportif.Conn, error) {
	return t.DialWithOptions(ctx, addr, transportif.DefaultDialOptions())
}

// DialWithOptions 使用选项建立连接
func (t *Transport) DialWithOptions(ctx context.Context, addr endpoint.Address, opts transportif.DialOptions) (transportif.Conn, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("传输层已关闭")
	}

	// 解析地址
	tcpAddr, ok := addr.(*Address)
	if !ok {
		// 尝试从字符串解析
		parsed, err := ParseAddress(addr.String())
		if err != nil {
			return nil, fmt.Errorf("无效的 TCP 地址: %w", err)
		}
		tcpAddr = parsed
	}

	// 创建拨号器
	dialer := &net.Dialer{
		Timeout:   opts.Timeout,
		KeepAlive: opts.KeepAlive,
	}

	// 拨号
	dialAddr := tcpAddr.NetDialString()
	conn, err := dialer.DialContext(ctx, "tcp", dialAddr)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("不是 TCP 连接")
	}

	// 设置连接选项
	if opts.NoDelay {
		tcpConn.SetNoDelay(true)
	}
	if opts.KeepAlive > 0 {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(opts.KeepAlive)
	}

	// 包装连接
	wrappedConn, err := NewConn(tcpConn)
	if err != nil {
		tcpConn.Close()
		return nil, err
	}

	// 记录连接
	t.connsMu.Lock()
	t.conns[wrappedConn.RemoteAddr().String()] = wrappedConn
	t.connsMu.Unlock()

	return wrappedConn, nil
}

// Listen 监听入站连接
func (t *Transport) Listen(addr endpoint.Address) (transportif.Listener, error) {
	return t.ListenWithOptions(addr, transportif.DefaultListenOptions())
}

// ListenWithOptions 使用选项监听
func (t *Transport) ListenWithOptions(addr endpoint.Address, opts transportif.ListenOptions) (transportif.Listener, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("传输层已关闭")
	}

	// 解析地址
	tcpAddr, ok := addr.(*Address)
	if !ok {
		// 尝试从字符串解析
		parsed, err := ParseAddress(addr.String())
		if err != nil {
			return nil, fmt.Errorf("无效的 TCP 地址: %w", err)
		}
		tcpAddr = parsed
	}

	// 创建监听器
	listener, err := NewListener(tcpAddr, opts)
	if err != nil {
		return nil, err
	}

	// 记录监听器
	t.listenersMu.Lock()
	t.listeners[listener.Addr().String()] = listener
	t.listenersMu.Unlock()

	return listener, nil
}

// Protocols 返回支持的协议
func (t *Transport) Protocols() []string {
	return []string{"tcp", "tcp4", "tcp6"}
}

// CanDial 检查是否可以拨号到指定地址
func (t *Transport) CanDial(addr endpoint.Address) bool {
	if t.closed.Load() {
		return false
	}

	addrStr := addr.String()

	// 检查是否是 TCP 地址格式
	if strings.Contains(addrStr, "/tcp/") {
		return true
	}

	// 检查网络类型
	network := addr.Network()
	return network == "tcp" || network == "tcp4" || network == "tcp6"
}

// Proxy 返回是否为代理传输
//
// TCP 是直连传输，不通过中间节点转发
func (t *Transport) Proxy() bool {
	return false
}

// Close 关闭传输层
func (t *Transport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}

	var lastErr error

	// 关闭所有监听器
	t.listenersMu.Lock()
	for _, l := range t.listeners {
		if err := l.Close(); err != nil {
			lastErr = err
		}
	}
	t.listeners = make(map[string]*Listener)
	t.listenersMu.Unlock()

	// 关闭所有连接
	t.connsMu.Lock()
	for _, c := range t.conns {
		if err := c.Close(); err != nil {
			lastErr = err
		}
	}
	t.conns = make(map[string]*Conn)
	t.connsMu.Unlock()

	return lastErr
}

// ============================================================================
//                              辅助方法
// ============================================================================

// RemoveConn 移除连接记录
func (t *Transport) RemoveConn(addr string) {
	t.connsMu.Lock()
	delete(t.conns, addr)
	t.connsMu.Unlock()
}

// RemoveListener 移除监听器记录
func (t *Transport) RemoveListener(addr string) {
	t.listenersMu.Lock()
	delete(t.listeners, addr)
	t.listenersMu.Unlock()
}

// ConnCount 返回连接数量
func (t *Transport) ConnCount() int {
	t.connsMu.RLock()
	defer t.connsMu.RUnlock()
	return len(t.conns)
}

// ListenerCount 返回监听器数量
func (t *Transport) ListenerCount() int {
	t.listenersMu.RLock()
	defer t.listenersMu.RUnlock()
	return len(t.listeners)
}

// IsClosed 检查是否已关闭
func (t *Transport) IsClosed() bool {
	return t.closed.Load()
}

// 注意：TransportFactory 接口已删除（v1.1 清理）。
// TCP transport 通过 NewTransport() 函数直接创建，无需工厂模式。

// ============================================================================
//                              辅助函数
// ============================================================================

// DialTimeout 使用超时拨号
func DialTimeout(addr string, timeout time.Duration) (*Conn, error) {
	tcpAddr, err := ParseAddress(addr)
	if err != nil {
		return nil, err
	}

	transport := NewTransport(transportif.DefaultConfig())
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := transport.DialWithOptions(ctx, tcpAddr, transportif.DialOptions{
		Timeout: timeout,
	})
	if err != nil {
		return nil, err
	}

	return conn.(*Conn), nil
}

