// Package quic 实现 QUIC 传输
package quic

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/quic-go/quic-go"
)

// 确保实现了接口
var _ pkgif.Transport = (*Transport)(nil)

// Transport QUIC 传输
//
// 使用共享的 UDP socket 进行监听和拨号，这对于 NAT 打洞至关重要：
// - 打洞时需要使用与监听相同的本地端口
// - quic.Transport 支持在同一个 socket 上同时监听和拨号
type Transport struct {
	mu sync.RWMutex

	localPeer     types.PeerID
	identity      pkgif.Identity
	serverTLSConf *tls.Config
	clientTLSConf *tls.Config
	config        *quic.Config

	// 共享的 QUIC Transport 和 UDP socket
	// 用于在同一端口上监听和拨号，提高 NAT 打洞成功率
	quicTransport *quic.Transport
	udpConn       *net.UDPConn

	listeners map[string]*Listener
	closed    bool

	// rebind 支持
	rebindSupport *RebindSupport
}

// New 创建 QUIC 传输
//
// 参数：
//   - localPeer: 本地节点 ID
//   - identity: 节点身份（用于生成 TLS 配置）
//
// 返回：
//   - *Transport: QUIC 传输实例
func New(localPeer types.PeerID, identity pkgif.Identity) *Transport {
	var serverTLS, clientTLS *tls.Config

	// 从 Identity 生成 TLS 配置
	if identity != nil {
		var err error
		serverTLS, clientTLS, err = NewTLSConfig(identity)
		if err != nil {
			// 回退到简化配置（仅用于测试）
			serverTLS = &tls.Config{
				MinVersion: tls.VersionTLS13,
				NextProtos: []string{"dep2p"},
			}
			clientTLS = serverTLS
		}
	} else {
		// 如果没有 Identity，使用默认配置（不推荐）
		serverTLS = &tls.Config{
			MinVersion: tls.VersionTLS13,
			NextProtos: []string{"dep2p"},
		}
		clientTLS = serverTLS
	}

	t := &Transport{
		localPeer:     localPeer,
		identity:      identity,
		serverTLSConf: serverTLS,
		clientTLSConf: clientTLS,
		config: &quic.Config{
			// MaxIdleTimeout: 连接空闲超时时间
			// 快速断开检测：设置为 6s，确保非优雅断开能在 ~9s 内检测到
			// 计算：KeepAlivePeriod(3s) + MaxIdleTimeout(6s) ≈ 9s 最大检测延迟
			// 参考：design/02_constraints/protocol/L2_transport/quic.md
			MaxIdleTimeout: 6 * time.Second,
			// KeepAlivePeriod: 客户端发送 KeepAlive 的间隔
			// 快速断开检测：设置为 3s，确保连接活跃性
			// 网络开销：~20 bytes/packet，100连接 = ~667 bytes/s，可忽略不计
			KeepAlivePeriod:       3 * time.Second,
			MaxIncomingStreams:    1024,
			MaxIncomingUniStreams: 1024,
			EnableDatagrams:       true,
			Allow0RTT:             true,
		},
		listeners:     make(map[string]*Listener),
		rebindSupport: NewRebindSupport(),
	}

	// 设置 rebind 函数
	t.rebindSupport.SetRebindFunc(t.doRebind)

	return t
}

// Dial 拨号连接
//
// 使用共享的 quic.Transport 拨号，复用监听端口。
// 这对于 NAT 打洞至关重要：打洞时需要使用与监听相同的本地端口，
// 否则 NAT 会分配新的外部端口映射，导致打洞失败。
func (t *Transport) Dial(ctx context.Context, raddr types.Multiaddr, peerID types.PeerID) (pkgif.Connection, error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, ErrTransportClosed
	}

	// 如果还没有 quicTransport（没有先 Listen），创建一个使用随机端口
	if t.quicTransport == nil {
		conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
		if err != nil {
			t.mu.Unlock()
			return nil, fmt.Errorf("listen udp for dial: %w", err)
		}
		t.udpConn = conn
		t.quicTransport = &quic.Transport{Conn: conn}
	}

	quicTransport := t.quicTransport
	t.mu.Unlock()

	// 解析地址
	udpAddr, err := parseMultiaddr(raddr)
	if err != nil {
		return nil, fmt.Errorf("parse address: %w", err)
	}

	// 使用共享 quic.Transport 拨号（复用监听端口！）
	quicConn, err := quicTransport.Dial(ctx, udpAddr, t.clientTLSConf, t.config)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	return newConnection(quicConn, t.localPeer, peerID, raddr, pkgif.DirOutbound), nil
}

// CanDial 检查是否支持拨号
func (t *Transport) CanDial(addr types.Multiaddr) bool {
	// 检查是否为 QUIC 地址
	_, err := addr.ValueForProtocol(types.ProtocolQUIC_V1)
	return err == nil
}

// Listen 监听地址
//
// 使用共享 UDP socket 和 quic.Transport，使得后续 Dial 可以复用同一端口。
// 这对于 NAT 打洞至关重要：打洞时需要使用与监听相同的本地端口。
func (t *Transport) Listen(laddr types.Multiaddr) (pkgif.Listener, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, ErrTransportClosed
	}

	// 解析地址
	udpAddr, err := parseMultiaddr(laddr)
	if err != nil {
		return nil, fmt.Errorf("parse address: %w", err)
	}

	// 首次监听时创建共享 UDP socket 和 quic.Transport
	if t.udpConn == nil {
		conn, err := net.ListenUDP("udp", udpAddr)
		if err != nil {
			return nil, fmt.Errorf("listen udp: %w", err)
		}
		t.udpConn = conn
		t.quicTransport = &quic.Transport{Conn: conn}
	}

	// 使用共享 quic.Transport 监听
	quicListener, err := t.quicTransport.Listen(t.serverTLSConf, t.config)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	// 获取实际监听地址（可能端口为 0 时动态分配）
	actualAddr := t.udpConn.LocalAddr().(*net.UDPAddr)
	var actualAddrStr string
	if ip4 := actualAddr.IP.To4(); ip4 != nil {
		actualAddrStr = fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", ip4.String(), actualAddr.Port)
	} else if actualAddr.IP.IsUnspecified() {
		actualAddrStr = fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", actualAddr.Port)
	} else {
		actualAddrStr = fmt.Sprintf("/ip6/%s/udp/%d/quic-v1", actualAddr.IP.String(), actualAddr.Port)
	}
	actualMultiaddr, _ := types.NewMultiaddr(actualAddrStr)

	listener := &Listener{
		quicListener: quicListener,
		localAddr:    actualMultiaddr,
		localPeer:    t.localPeer,
		transport:    t,
	}

	t.listeners[actualAddrStr] = listener

	return listener, nil
}

// Protocols 返回支持的协议
func (t *Transport) Protocols() []int {
	return []int{types.ProtocolQUIC_V1}
}

// Close 关闭传输
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true

	// 关闭所有监听器
	for _, l := range t.listeners {
		l.Close()
	}

	// 关闭共享的 quicTransport（会关闭所有连接）
	if t.quicTransport != nil {
		t.quicTransport.Close()
		t.quicTransport = nil
	}

	// 关闭 UDP socket
	if t.udpConn != nil {
		t.udpConn.Close()
		t.udpConn = nil
	}

	return nil
}

// Rebind 重新绑定 socket
//
// 当网络接口变化时（如 4G→WiFi），需要重新绑定 socket 到新的网络接口。
// 参考 iroh-main 的实现：关闭旧 socket，创建新 socket 绑定到新地址。
func (t *Transport) Rebind(ctx context.Context) error {
	if t.rebindSupport == nil {
		return nil
	}
	return t.rebindSupport.Rebind(ctx)
}

// doRebind 执行实际的 rebind 操作
//
// 重新创建共享 UDP socket 和 quic.Transport：
// 1. 保存当前监听地址
// 2. 关闭旧的 quicTransport 和 udpConn
// 3. 创建新的 UDP socket 和 quic.Transport
// 4. 重新创建监听器
func (t *Transport) doRebind(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrTransportClosed
	}

	// 收集所有监听地址
	addrs := make([]types.Multiaddr, 0, len(t.listeners))
	for _, l := range t.listeners {
		addrs = append(addrs, l.localAddr)
	}

	// 关闭所有旧监听器
	for _, l := range t.listeners {
		l.Close()
	}
	t.listeners = make(map[string]*Listener)

	// 关闭旧的 quicTransport（会关闭所有连接）
	if t.quicTransport != nil {
		t.quicTransport.Close()
		t.quicTransport = nil
	}

	// 关闭旧的 UDP socket
	if t.udpConn != nil {
		t.udpConn.Close()
		t.udpConn = nil
	}

	// 为每个地址创建新的共享 socket 和监听器
	var lastErr error
	for _, addr := range addrs {
		// 解析地址
		udpAddr, err := parseMultiaddr(addr)
		if err != nil {
			lastErr = err
			continue
		}

		// 创建新的 UDP socket（仅首次）
		if t.udpConn == nil {
			conn, err := net.ListenUDP("udp", udpAddr)
			if err != nil {
				lastErr = err
				continue
			}
			t.udpConn = conn
			t.quicTransport = &quic.Transport{Conn: conn}
		}

		// 使用共享 quic.Transport 创建监听器
		quicListener, err := t.quicTransport.Listen(t.serverTLSConf, t.config)
		if err != nil {
			lastErr = err
			continue
		}

		listener := &Listener{
			quicListener: quicListener,
			localAddr:    addr,
			localPeer:    t.localPeer,
			transport:    t,
		}

		t.listeners[addr.String()] = listener
	}

	// 更新 rebind 支持的当前地址
	if t.udpConn != nil && t.rebindSupport != nil {
		t.rebindSupport.UpdateAddr(t.udpConn.LocalAddr().(*net.UDPAddr))
	}

	return lastErr
}

// GetRebindSupport 获取 rebind 支持
func (t *Transport) GetRebindSupport() *RebindSupport {
	return t.rebindSupport
}

// parseMultiaddr 解析 Multiaddr 到 UDP 地址
func parseMultiaddr(addr types.Multiaddr) (*net.UDPAddr, error) {
	// 提取 IP
	ip, err := addr.ValueForProtocol(types.ProtocolIP4)
	if err != nil {
		// 尝试 IPv6
		ip, err = addr.ValueForProtocol(types.ProtocolIP6)
		if err != nil {
			return nil, fmt.Errorf("no IP in address")
		}
	}

	// 提取 UDP 端口
	portStr, err := addr.ValueForProtocol(types.ProtocolUDP)
	if err != nil {
		return nil, fmt.Errorf("no UDP port in address")
	}

	// 解析端口
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	return &net.UDPAddr{
		IP:   net.ParseIP(ip),
		Port: port,
	}, nil
}
