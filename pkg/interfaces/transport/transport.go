// Package transport 定义传输层接口
//
// 传输模块负责底层网络通信，包括：
// - 传输协议抽象
// - 连接建立和管理
// - 监听和接受连接
package transport

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/interfaces/netaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Transport 接口
// ============================================================================

// Transport 传输层接口
//
// Transport 提供底层网络传输能力，抽象了不同的传输协议（如 QUIC、TCP、Relay）。
type Transport interface {
	// Dial 建立出站连接
	// 连接到指定地址
	Dial(ctx context.Context, addr netaddr.Address) (Conn, error)

	// DialWithOptions 使用选项建立连接
	DialWithOptions(ctx context.Context, addr netaddr.Address, opts DialOptions) (Conn, error)

	// Listen 监听入站连接
	// 在指定地址上开始监听
	Listen(addr netaddr.Address) (Listener, error)

	// ListenWithOptions 使用选项监听
	ListenWithOptions(addr netaddr.Address, opts ListenOptions) (Listener, error)

	// Protocols 返回支持的协议
	// 如 ["quic", "quic-v1", "p2p-circuit"]
	Protocols() []string

	// CanDial 检查是否可以拨号到指定地址
	CanDial(addr netaddr.Address) bool

	// Proxy 返回该传输是否为代理传输
	//
	// 代理传输（如 Relay）通过中间节点转发流量，可能需要额外的安全协商。
	// 直连传输（如 QUIC、TCP）返回 false。
	Proxy() bool

	// Close 关闭传输层
	Close() error
}

// ============================================================================
//                              Listener 接口
// ============================================================================

// Listener 监听器接口
//
// Listener 监听入站连接，类似于 net.Listener。
type Listener interface {
	// Accept 接受连接
	// 阻塞直到有新连接到达
	Accept() (Conn, error)

	// Addr 返回监听地址
	Addr() netaddr.Address

	// Close 关闭监听器
	Close() error

	// Multiaddr 返回多地址格式的监听地址
	Multiaddr() string
}

// ============================================================================
//                              Conn 接口
// ============================================================================

// Conn 传输层连接接口
//
// Conn 代表底层传输连接，提供基本的读写能力。
type Conn interface {
	io.ReadWriteCloser

	// LocalAddr 本地地址
	LocalAddr() netaddr.Address

	// RemoteAddr 远程地址
	RemoteAddr() netaddr.Address

	// LocalNetAddr 返回底层 net.Addr
	LocalNetAddr() net.Addr

	// RemoteNetAddr 返回底层远程 net.Addr
	RemoteNetAddr() net.Addr

	// SetDeadline 设置读写超时
	SetDeadline(t time.Time) error

	// SetReadDeadline 设置读超时
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline 设置写超时
	SetWriteDeadline(t time.Time) error

	// IsClosed 检查连接是否已关闭
	IsClosed() bool

	// Transport 返回传输协议名称
	Transport() string
}

// ============================================================================
//                              UpgradedConn 接口
// ============================================================================

// 注意：Upgrader 接口已删除（v1.1 清理）。
// QUIC 内置 TLS 1.3 和多路复用，无需外部升级器。

// UpgradedConn 升级后的连接
type UpgradedConn interface {
	Conn

	// RemoteID 返回远程节点 ID
	RemoteID() types.NodeID

	// RemotePublicKey 返回远程公钥
	RemotePublicKey() identity.PublicKey

	// LocalID 返回本地节点 ID
	LocalID() types.NodeID

	// OpenStream 打开新流
	OpenStream(ctx context.Context) (Stream, error)

	// AcceptStream 接受新流
	AcceptStream(ctx context.Context) (Stream, error)
}

// Stream 传输层流接口
type Stream interface {
	io.ReadWriteCloser

	// ID 返回流 ID
	ID() uint64

	// SetDeadline 设置超时
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error

	// CloseRead 关闭读端
	CloseRead() error

	// CloseWrite 关闭写端
	CloseWrite() error
}

// ============================================================================
//                              选项
// ============================================================================

// DialOptions 拨号选项
type DialOptions struct {
	// Timeout 连接超时
	Timeout time.Duration

	// KeepAlive 保活间隔
	KeepAlive time.Duration

	// NoDelay 禁用 Nagle 算法
	NoDelay bool
}

// DefaultDialOptions 返回默认拨号选项
func DefaultDialOptions() DialOptions {
	return DialOptions{
		Timeout:   30 * time.Second,
		KeepAlive: 15 * time.Second,
		NoDelay:   true,
	}
}

// ListenOptions 监听选项
type ListenOptions struct {
	// Backlog 连接队列大小
	Backlog int

	// ReuseAddr 允许地址重用
	ReuseAddr bool

	// ReusePort 允许端口重用
	ReusePort bool
}

// DefaultListenOptions 返回默认监听选项
func DefaultListenOptions() ListenOptions {
	return ListenOptions{
		Backlog:   128,
		ReuseAddr: true,
		ReusePort: false,
	}
}

// ============================================================================
//                              配置
// ============================================================================

// 注意：TransportFactory 接口已删除（v1.1 清理）。
// 传输层通过 Fx DI 直接提供 Transport 实例，无需工厂模式。

// Config 传输配置
type Config struct {
	// MaxConnections 最大连接数
	MaxConnections int

	// MaxStreamsPerConn 每连接最大流数
	MaxStreamsPerConn int

	// IdleTimeout 空闲超时
	IdleTimeout time.Duration

	// HandshakeTimeout 握手超时
	HandshakeTimeout time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MaxConnections:    1000,
		MaxStreamsPerConn: 100,
		IdleTimeout:       30 * time.Second,
		HandshakeTimeout:  10 * time.Second,
	}
}
