// Package quic 提供基于 QUIC 的传输层实现
package quic

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/quic-go/quic-go"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// Transport QUIC 传输层实现
type Transport struct {
	config    transportif.Config
	identity  identityif.Identity
	tlsConfig *tls.Config

	listeners   map[string]*Listener
	listenersMu sync.RWMutex

	conns   map[string]*Conn
	connsMu sync.RWMutex

	// 0-RTT 支持
	sessionStore *SessionStore
	enable0RTT   atomic.Bool

	// 连接迁移支持
	migrator   *ConnectionMigrator
	migratorMu sync.RWMutex

	closed atomic.Bool
}

// 确保实现 transport.Transport 接口
var _ transportif.Transport = (*Transport)(nil)

// NewTransport 创建 QUIC 传输层
func NewTransport(config transportif.Config, identity identityif.Identity) (*Transport, error) {
	// 生成 TLS 配置
	tlsConfigGen := NewTLSConfig(identity)
	tlsConfig, err := tlsConfigGen.GenerateConfig()
	if err != nil {
		return nil, fmt.Errorf("生成 TLS 配置失败: %w", err)
	}

	// 每个 Transport 使用独立的 Session Store，避免不同节点间的会话混淆
	// 注意：在测试场景中，多个节点可能运行在同一进程中，
	// 如果共享 Session Store，会导致错误的会话恢复
	sessionStore := NewSessionStore(DefaultSessionStoreConfig())

	// 配置 TLS Session Cache
	tlsConfig.ClientSessionCache = sessionStore

	t := &Transport{
		config:       config,
		identity:     identity,
		tlsConfig:    tlsConfig,
		listeners:    make(map[string]*Listener),
		conns:        make(map[string]*Conn),
		sessionStore: sessionStore,
	}
	t.enable0RTT.Store(true)
	return t, nil
}

// NewTransportWith0RTT 创建支持 0-RTT 的 QUIC 传输层
func NewTransportWith0RTT(config transportif.Config, identity identityif.Identity, sessionStoreConfig SessionStoreConfig) (*Transport, error) {
	// 生成 TLS 配置
	tlsConfigGen := NewTLSConfig(identity)
	tlsConfig, err := tlsConfigGen.GenerateConfig()
	if err != nil {
		return nil, fmt.Errorf("生成 TLS 配置失败: %w", err)
	}

	// 创建自定义 Session Store
	sessionStore := NewSessionStore(sessionStoreConfig)

	// 配置 TLS Session Cache
	tlsConfig.ClientSessionCache = sessionStore

	t := &Transport{
		config:       config,
		identity:     identity,
		tlsConfig:    tlsConfig,
		listeners:    make(map[string]*Listener),
		conns:        make(map[string]*Conn),
		sessionStore: sessionStore,
	}
	t.enable0RTT.Store(true)
	return t, nil
}

// NewTransportWithTLS 使用自定义 TLS 配置创建传输层
func NewTransportWithTLS(config transportif.Config, tlsConfig *tls.Config) *Transport {
	return &Transport{
		config:    config,
		tlsConfig: tlsConfig,
		listeners: make(map[string]*Listener),
		conns:     make(map[string]*Conn),
	}
}

// Dial 建立出站连接
func (t *Transport) Dial(ctx context.Context, addr endpoint.Address) (transportif.Conn, error) {
	return t.DialWithOptions(ctx, addr, transportif.DefaultDialOptions())
}

// DialWithOptions 使用选项建立连接
func (t *Transport) DialWithOptions(ctx context.Context, addr endpoint.Address, opts transportif.DialOptions) (transportif.Conn, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("传输层已关闭")
	}

	// 转换地址
	quicAddr, ok := addr.(*Address)
	if !ok {
		// 尝试从字符串解析
		addrStr := addr.String()
		var err error

		// 尝试解析多地址格式 (/ip4/127.0.0.1/udp/8080/quic-v1)
		if strings.HasPrefix(addrStr, "/") {
			quicAddr, err = ParseMultiaddr(addrStr)
		} else {
			// 尝试普通地址格式 (127.0.0.1:8080)
			quicAddr, err = ParseAddress(addrStr)
		}
		if err != nil {
			return nil, fmt.Errorf("无效的地址格式: %w", err)
		}
	}

	// 解析 UDP 地址
	udpAddr, err := quicAddr.ToUDPAddr()
	if err != nil {
		return nil, fmt.Errorf("解析 UDP 地址失败: %w", err)
	}

	// 设置超时
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// 创建 QUIC 配置
	quicConfig := t.getQuicConfig()

	// 建立 QUIC 连接
	quicConn, err := quic.DialAddr(ctx, udpAddr.String(), t.getClientTLSConfig(), quicConfig)
	if err != nil {
		return nil, fmt.Errorf("QUIC 拨号失败: %w", err)
	}

	// 创建连接封装
	conn := NewConn(quicConn, t)

	// 保存连接
	t.connsMu.Lock()
	t.conns[quicAddr.String()] = conn
	t.connsMu.Unlock()

	return conn, nil
}

// Listen 监听入站连接
func (t *Transport) Listen(addr endpoint.Address) (transportif.Listener, error) {
	return t.ListenWithOptions(addr, transportif.DefaultListenOptions())
}

// ListenWithOptions 使用选项监听
func (t *Transport) ListenWithOptions(addr endpoint.Address, _ transportif.ListenOptions) (transportif.Listener, error) {
	if t.closed.Load() {
		return nil, fmt.Errorf("传输层已关闭")
	}

	// 转换地址
	quicAddr, ok := addr.(*Address)
	if !ok {
		// 尝试从字符串解析
		addrStr := addr.String()
		var err error

		// 尝试解析多地址格式 (/ip4/127.0.0.1/udp/8080/quic-v1)
		if strings.HasPrefix(addrStr, "/") {
			quicAddr, err = ParseMultiaddr(addrStr)
		} else {
			// 尝试普通地址格式 (127.0.0.1:8080)
			quicAddr, err = ParseAddress(addrStr)
		}
		if err != nil {
			return nil, fmt.Errorf("无效的地址格式: %w", err)
		}
	}

	// 解析 UDP 地址
	udpAddr, err := quicAddr.ToUDPAddr()
	if err != nil {
		return nil, fmt.Errorf("解析 UDP 地址失败: %w", err)
	}

	// 根据地址类型选择正确的网络协议
	network := "udp4"
	if quicAddr.Network() == "ip6" || (udpAddr.IP != nil && udpAddr.IP.To4() == nil) {
		network = "udp6"
	}

	// 监听 UDP
	udpConn, err := net.ListenUDP(network, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("监听 UDP 失败: %w", err)
	}

	// 创建 QUIC 配置
	quicConfig := t.getQuicConfig()

	// 创建 QUIC 监听器
	quicListener, err := quic.Listen(udpConn, t.tlsConfig, quicConfig)
	if err != nil {
		udpConn.Close()
		return nil, fmt.Errorf("创建 QUIC 监听器失败: %w", err)
	}

	// 获取实际监听地址
	actualAddr := udpConn.LocalAddr().(*net.UDPAddr)
	listenAddr := NewAddress(actualAddr.IP.String(), actualAddr.Port)

	// 创建监听器封装
	listener := NewListener(quicListener, listenAddr, t)

	// 保存监听器
	t.listenersMu.Lock()
	t.listeners[listenAddr.String()] = listener
	t.listenersMu.Unlock()

	return listener, nil
}

// Protocols 返回支持的协议
func (t *Transport) Protocols() []string {
	return []string{"quic", "quic-v1"}
}

// CanDial 检查是否可以拨号到指定地址
func (t *Transport) CanDial(addr endpoint.Address) bool {
	if addr == nil {
		return false
	}

	// QUIC 支持 IPv4 和 IPv6
	network := addr.Network()
	return network == "ip4" || network == "ip6" || network == "udp" || network == "udp4" || network == "udp6"
}

// Proxy 返回是否为代理传输
//
// QUIC 是直连传输，不通过中间节点转发
func (t *Transport) Proxy() bool {
	return false
}

// Close 关闭传输层
func (t *Transport) Close() error {
	if t.closed.Swap(true) {
		return nil // 已经关闭
	}

	var errs []error

	// 关闭所有连接
	t.connsMu.Lock()
	for _, conn := range t.conns {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	t.conns = make(map[string]*Conn)
	t.connsMu.Unlock()

	// 关闭所有监听器
	t.listenersMu.Lock()
	for _, listener := range t.listeners {
		if err := listener.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	t.listeners = make(map[string]*Listener)
	t.listenersMu.Unlock()

	// 关闭连接迁移器
	t.migratorMu.Lock()
	if t.migrator != nil {
		if err := t.migrator.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	t.migratorMu.Unlock()

	// 注意：sessionStore 可能是全局共享的，不在这里关闭
	// 如果是自定义的非全局 sessionStore，调用者应负责关闭

	if len(errs) > 0 {
		return fmt.Errorf("关闭时发生 %d 个错误: %v", len(errs), errs[0])
	}

	return nil
}

// getQuicConfig 获取 QUIC 配置
func (t *Transport) getQuicConfig() *quic.Config {
	config := &quic.Config{
		MaxIdleTimeout:        t.config.IdleTimeout,
		KeepAlivePeriod:       t.config.IdleTimeout / 2,
		MaxIncomingStreams:    int64(t.config.MaxStreamsPerConn),
		MaxIncomingUniStreams: int64(t.config.MaxStreamsPerConn / 2),
	}

	// 启用 0-RTT
	if t.enable0RTT.Load() {
		config.Allow0RTT = true
	}

	return config
}

// getClientTLSConfig 获取客户端 TLS 配置
func (t *Transport) getClientTLSConfig() *tls.Config {
	// 复制 TLS 配置，客户端不需要 ClientAuth
	clientConfig := t.tlsConfig.Clone()
	clientConfig.ClientAuth = tls.NoClientCert

	// 使用 Session Cache 实现 0-RTT
	if t.sessionStore != nil {
		clientConfig.ClientSessionCache = t.sessionStore
	}

	return clientConfig
}

// SessionStore 返回 Session Store
func (t *Transport) SessionStore() *SessionStore {
	return t.sessionStore
}

// Is0RTTEnabled 检查是否启用 0-RTT
func (t *Transport) Is0RTTEnabled() bool {
	return t.enable0RTT.Load()
}

// Enable0RTT 启用 0-RTT（线程安全）
func (t *Transport) Enable0RTT(enable bool) {
	t.enable0RTT.Store(enable)
}

// ============================================================================
//                              连接迁移
// ============================================================================

// Migrator 返回连接迁移器
func (t *Transport) Migrator() *ConnectionMigrator {
	t.migratorMu.RLock()
	defer t.migratorMu.RUnlock()
	return t.migrator
}

// EnableMigration 启用连接迁移
func (t *Transport) EnableMigration(ctx context.Context, config MigratorConfig) error {
	t.migratorMu.Lock()
	if t.migrator == nil {
		t.migrator = NewConnectionMigrator(t)
	}
	migrator := t.migrator
	t.migratorMu.Unlock()

	return migrator.Start(ctx, config)
}

// OnAddressChange 注册地址变更回调
func (t *Transport) OnAddressChange(callback AddressChangeCallback) {
	t.migratorMu.Lock()
	if t.migrator == nil {
		t.migrator = NewConnectionMigrator(t)
	}
	migrator := t.migrator
	t.migratorMu.Unlock()

	migrator.OnAddressChange(callback)
}

// GetListener 获取指定地址的监听器
func (t *Transport) GetListener(addr string) *Listener {
	t.listenersMu.RLock()
	defer t.listenersMu.RUnlock()
	return t.listeners[addr]
}

// GetConn 获取指定地址的连接
func (t *Transport) GetConn(addr string) *Conn {
	t.connsMu.RLock()
	defer t.connsMu.RUnlock()
	return t.conns[addr]
}

// RemoveConn 移除连接
func (t *Transport) RemoveConn(addr string) {
	t.connsMu.Lock()
	defer t.connsMu.Unlock()
	delete(t.conns, addr)
}

// Identity 返回身份
func (t *Transport) Identity() identityif.Identity {
	return t.identity
}

// Config 返回配置
func (t *Transport) Config() transportif.Config {
	return t.config
}

// IsClosed 检查传输层是否已关闭
func (t *Transport) IsClosed() bool {
	return t.closed.Load()
}

// ListenersCount 返回监听器数量
func (t *Transport) ListenersCount() int {
	t.listenersMu.RLock()
	defer t.listenersMu.RUnlock()
	return len(t.listeners)
}

// ConnsCount 返回连接数量
func (t *Transport) ConnsCount() int {
	t.connsMu.RLock()
	defer t.connsMu.RUnlock()
	return len(t.conns)
}
