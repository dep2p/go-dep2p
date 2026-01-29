package config

import (
	"errors"
	"time"
)

// TransportConfig 传输层配置
//
// 配置节点支持的传输协议及其参数：
//   - QUIC: 基于 UDP 的现代传输协议（推荐）
//   - TCP: 传统 TCP 连接
//   - WebSocket: WebSocket 传输（用于浏览器）
type TransportConfig struct {
	// QUIC 配置
	EnableQUIC bool       `json:"enable_quic"`
	QUIC       QUICConfig `json:"quic,omitempty"`

	// TCP 配置
	EnableTCP bool      `json:"enable_tcp"`
	TCP       TCPConfig `json:"tcp,omitempty"`

	// WebSocket 配置
	EnableWebSocket bool            `json:"enable_websocket"`
	WebSocket       WebSocketConfig `json:"websocket,omitempty"`

	// 通用配置
	DialTimeout Duration `json:"dial_timeout"` // 拨号超时
}

// QUICConfig QUIC 传输配置
type QUICConfig struct {
	// MaxIdleTimeout 最大空闲超时
	MaxIdleTimeout Duration `json:"max_idle_timeout"`

	// MaxStreams 最大并发流数量
	MaxStreams int `json:"max_streams"`

	// MaxStreamReceiveWindow 流接收窗口大小
	MaxStreamReceiveWindow uint64 `json:"max_stream_receive_window,omitempty"`

	// MaxConnectionReceiveWindow 连接接收窗口大小
	MaxConnectionReceiveWindow uint64 `json:"max_connection_receive_window,omitempty"`

	// KeepAlive 是否启用 KeepAlive
	KeepAlive bool `json:"keep_alive"`

	// KeepAlivePeriod KeepAlive 周期
	KeepAlivePeriod Duration `json:"keep_alive_period"`
}

// TCPConfig TCP 传输配置
type TCPConfig struct {
	// Timeout TCP 连接超时
	Timeout Duration `json:"timeout"`

	// KeepAlive 是否启用 TCP KeepAlive
	KeepAlive bool `json:"keep_alive"`

	// KeepAlivePeriod KeepAlive 周期
	KeepAlivePeriod Duration `json:"keep_alive_period"`

	// NoDelay 是否禁用 Nagle 算法
	NoDelay bool `json:"no_delay"`

	// Linger SO_LINGER 设置（秒）
	// -1 表示使用系统默认值
	Linger int `json:"linger,omitempty"`
}

// WebSocketConfig WebSocket 传输配置
type WebSocketConfig struct {
	// ReadBufferSize 读缓冲区大小
	ReadBufferSize int `json:"read_buffer_size,omitempty"`

	// WriteBufferSize 写缓冲区大小
	WriteBufferSize int `json:"write_buffer_size,omitempty"`

	// HandshakeTimeout 握手超时
	HandshakeTimeout Duration `json:"handshake_timeout"`

	// EnableCompression 是否启用压缩
	EnableCompression bool `json:"enable_compression"`
}

// DefaultTransportConfig 返回默认传输配置
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		// ════════════════════════════════════════════════════════════════════
		// QUIC 配置（推荐，基于 UDP，支持 0-RTT、多路复用、连接迁移）
		// ════════════════════════════════════════════════════════════════════
		EnableQUIC: true, // 默认启用：现代 P2P 首选传输协议
		QUIC: QUICConfig{
			MaxIdleTimeout:             Duration(30 * time.Second), // 空闲超时：30 秒，超时后关闭连接
			MaxStreams:                 1024,                       // 最大并发流：1024，足够大多数场景
			MaxStreamReceiveWindow:     6 * 1024 * 1024,            // 流接收窗口：6 MB，控制流量
			MaxConnectionReceiveWindow: 15 * 1024 * 1024,           // 连接接收窗口：15 MB，所有流共享
			KeepAlive:                  true,                       // 启用 KeepAlive：保持连接活跃
			KeepAlivePeriod:            Duration(15 * time.Second), // KeepAlive 间隔：15 秒
		},

		// ════════════════════════════════════════════════════════════════════
		// TCP 配置（兼容性好，适合防火墙严格的环境）
		// ════════════════════════════════════════════════════════════════════
		EnableTCP: true, // 默认启用：提供 QUIC 的后备选项
		TCP: TCPConfig{
			Timeout:         Duration(10 * time.Second), // 连接超时：10 秒
			KeepAlive:       true,                       // 启用 TCP KeepAlive：检测死连接
			KeepAlivePeriod: Duration(15 * time.Second), // KeepAlive 间隔：15 秒
			NoDelay:         true,                       // 禁用 Nagle 算法：减少延迟
			Linger:          -1,                         // SO_LINGER：-1 使用系统默认
		},

		// ════════════════════════════════════════════════════════════════════
		// WebSocket 配置（用于浏览器客户端，HTTP/HTTPS 兼容）
		// ════════════════════════════════════════════════════════════════════
		EnableWebSocket: false, // 默认禁用：仅浏览器场景需要
		WebSocket: WebSocketConfig{
			ReadBufferSize:    4096,                      // 读缓冲区：4 KB
			WriteBufferSize:   4096,                      // 写缓冲区：4 KB
			HandshakeTimeout:  Duration(10 * time.Second), // 握手超时：10 秒
			EnableCompression: false,                     // 禁用压缩：避免 CPU 开销
		},

		// ════════════════════════════════════════════════════════════════════
		// 通用传输配置
		// ════════════════════════════════════════════════════════════════════
		DialTimeout: Duration(30 * time.Second), // 拨号超时：30 秒，包括 DNS 解析、TCP 握手等
	}
}

// Validate 验证传输配置
func (c TransportConfig) Validate() error {
	// 至少启用一种传输协议
	if !c.EnableQUIC && !c.EnableTCP && !c.EnableWebSocket {
		return errors.New("at least one transport must be enabled")
	}

	// 验证 QUIC 配置
	if c.EnableQUIC {
		if c.QUIC.MaxIdleTimeout <= 0 {
			return errors.New("QUIC max idle timeout must be positive")
		}
		if c.QUIC.MaxStreams <= 0 {
			return errors.New("QUIC max streams must be positive")
		}
		if c.QUIC.MaxStreamReceiveWindow == 0 {
			return errors.New("QUIC max stream receive window must be positive")
		}
		if c.QUIC.MaxConnectionReceiveWindow == 0 {
			return errors.New("QUIC max connection receive window must be positive")
		}
		if c.QUIC.KeepAlive && c.QUIC.KeepAlivePeriod <= 0 {
			return errors.New("QUIC keep alive period must be positive when enabled")
		}
	}

	// 验证 TCP 配置
	if c.EnableTCP {
		if c.TCP.Timeout <= 0 {
			return errors.New("TCP timeout must be positive")
		}
		if c.TCP.KeepAlive && c.TCP.KeepAlivePeriod <= 0 {
			return errors.New("TCP keep alive period must be positive when enabled")
		}
	}

	// 验证 WebSocket 配置
	if c.EnableWebSocket {
		if c.WebSocket.ReadBufferSize <= 0 {
			return errors.New("WebSocket read buffer size must be positive")
		}
		if c.WebSocket.WriteBufferSize <= 0 {
			return errors.New("WebSocket write buffer size must be positive")
		}
		if c.WebSocket.HandshakeTimeout <= 0 {
			return errors.New("WebSocket handshake timeout must be positive")
		}
	}

	// 验证拨号超时
	if c.DialTimeout <= 0 {
		return errors.New("dial timeout must be positive")
	}

	return nil
}

// WithQUIC 设置是否启用 QUIC
func (c TransportConfig) WithQUIC(enabled bool) TransportConfig {
	c.EnableQUIC = enabled
	return c
}

// WithTCP 设置是否启用 TCP
func (c TransportConfig) WithTCP(enabled bool) TransportConfig {
	c.EnableTCP = enabled
	return c
}

// WithWebSocket 设置是否启用 WebSocket
func (c TransportConfig) WithWebSocket(enabled bool) TransportConfig {
	c.EnableWebSocket = enabled
	return c
}

// WithDialTimeout 设置拨号超时
func (c TransportConfig) WithDialTimeout(timeout time.Duration) TransportConfig {
	c.DialTimeout = Duration(timeout)
	return c
}
