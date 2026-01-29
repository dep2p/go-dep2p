// Package tcp 实现 TCP 传输
package tcp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 确保实现了接口
var _ pkgif.Transport = (*Transport)(nil)

// Transport TCP 传输
type Transport struct {
	mu sync.RWMutex

	localPeer types.PeerID
	upgrader  pkgif.Upgrader
	listeners map[string]*Listener
	closed    bool
}

// New 创建 TCP 传输
func New(localPeer types.PeerID, upgrader pkgif.Upgrader) *Transport {
	return &Transport{
		localPeer: localPeer,
		upgrader:  upgrader,
		listeners: make(map[string]*Listener),
	}
}

// Dial 拨号连接
func (t *Transport) Dial(ctx context.Context, raddr types.Multiaddr, peerID types.PeerID) (pkgif.Connection, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil, ErrTransportClosed
	}
	t.mu.RUnlock()

	// 解析地址
	tcpAddr, err := parseMultiaddr(raddr)
	if err != nil {
		return nil, fmt.Errorf("parse address: %w", err)
	}

	// 建立 TCP 连接
	var d net.Dialer
	rawConn, err := d.DialContext(ctx, "tcp", tcpAddr.String())
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	// 如果有 Upgrader，进行连接升级（Security + Muxer）
	if t.upgrader != nil {
		upgradedConn, err := t.upgrader.Upgrade(ctx, rawConn, pkgif.DirOutbound, peerID)
		if err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("upgrade connection: %w", err)
		}
		
		// 将 UpgradedConn 转换为 Connection
		// UpgradedConn 实现了 MuxedConn，需要包装为 Connection
		return wrapUpgradedConn(upgradedConn, t.localPeer, raddr), nil
	}

	// 如果没有 Upgrader，返回原始 TCP 连接（不推荐，仅用于测试）
	return newConnection(rawConn, t.localPeer, peerID, raddr, pkgif.DirOutbound), nil
}

// CanDial 检查是否支持拨号
func (t *Transport) CanDial(addr types.Multiaddr) bool {
	// 检查是否为 TCP 地址
	_, err := addr.ValueForProtocol(types.ProtocolTCP)
	return err == nil
}

// Listen 监听地址
func (t *Transport) Listen(laddr types.Multiaddr) (pkgif.Listener, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, ErrTransportClosed
	}

	// 解析地址
	tcpAddr, err := parseMultiaddr(laddr)
	if err != nil {
		return nil, fmt.Errorf("parse address: %w", err)
	}

	// 创建 TCP 监听器
	tcpListener, err := net.Listen("tcp", tcpAddr.String())
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	listener := &Listener{
		tcpListener: tcpListener,
		localAddr:   laddr,
		localPeer:   t.localPeer,
		transport:   t,
	}

	t.listeners[laddr.String()] = listener

	return listener, nil
}

// Protocols 返回支持的协议
func (t *Transport) Protocols() []int {
	return []int{types.ProtocolTCP}
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

	return nil
}

// parseMultiaddr 解析 Multiaddr 到 TCP 地址
func parseMultiaddr(addr types.Multiaddr) (*net.TCPAddr, error) {
	// 提取 IP
	ip, err := addr.ValueForProtocol(types.ProtocolIP4)
	if err != nil {
		// 尝试 IPv6
		ip, err = addr.ValueForProtocol(types.ProtocolIP6)
		if err != nil {
			return nil, fmt.Errorf("no IP in address")
		}
	}

	// 提取 TCP 端口
	portStr, err := addr.ValueForProtocol(types.ProtocolTCP)
	if err != nil {
		return nil, fmt.Errorf("no TCP port in address")
	}

	// 解析端口
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	return &net.TCPAddr{
		IP:   net.ParseIP(ip),
		Port: port,
	}, nil
}

// wrapUpgradedConn 将 UpgradedConn 包装为 Connection
//
// UpgradedConn 是经过安全握手和多路复用升级后的连接，
// 需要包装为 pkgif.Connection 以供 Swarm 使用。
func wrapUpgradedConn(upgraded pkgif.UpgradedConn, localPeer types.PeerID, remoteAddr types.Multiaddr) pkgif.Connection {
	return &upgradedConnection{
		UpgradedConn: upgraded,
		localPeer:    localPeer,
		remoteAddr:   remoteAddr,
		opened:       time.Now(),
		streams:      make([]pkgif.Stream, 0),
	}
}

// upgradedConnection 升级后的连接（包装 UpgradedConn）
type upgradedConnection struct {
	pkgif.UpgradedConn
	localPeer  types.PeerID
	remoteAddr types.Multiaddr
	opened     time.Time
	
	mu      sync.RWMutex
	streams []pkgif.Stream
	closed  bool
}

// 确保实现接口
var _ pkgif.Connection = (*upgradedConnection)(nil)

// LocalMultiaddr 返回本地多地址
func (c *upgradedConnection) LocalMultiaddr() types.Multiaddr {
	// 升级后的连接没有保存本地地址，返回空
	return nil
}

// RemoteMultiaddr 返回远程多地址
func (c *upgradedConnection) RemoteMultiaddr() types.Multiaddr {
	return c.remoteAddr
}

// GetStreams 获取所有流
func (c *upgradedConnection) GetStreams() []pkgif.Stream {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// 返回副本
	result := make([]pkgif.Stream, len(c.streams))
	copy(result, c.streams)
	return result
}

// Stat 返回连接统计信息
func (c *upgradedConnection) Stat() pkgif.ConnectionStat {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return pkgif.ConnectionStat{
		Direction:  pkgif.DirOutbound,
		Opened:     c.opened.Unix(),
		Transient:  false,
		NumStreams: len(c.streams),
	}
}

// IsClosed 检查连接是否已关闭
func (c *upgradedConnection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// NewStream 创建新流
func (c *upgradedConnection) NewStream(ctx context.Context) (pkgif.Stream, error) {
	// UpgradedConn 的 OpenStream 返回 MuxedStream
	muxedStream, err := c.OpenStream(ctx)
	if err != nil {
		return nil, err
	}
	
	// 包装 MuxedStream 为 Stream
	stream := wrapMuxedStream(muxedStream, c)
	
	// 记录流
	c.mu.Lock()
	c.streams = append(c.streams, stream)
	c.mu.Unlock()
	
	return stream, nil
}

// AcceptStream 接受新流
func (c *upgradedConnection) AcceptStream() (pkgif.Stream, error) {
	// UpgradedConn 的 AcceptStream 返回 MuxedStream
	muxedStream, err := c.UpgradedConn.AcceptStream()
	if err != nil {
		return nil, err
	}
	
	// 包装 MuxedStream 为 Stream
	stream := wrapMuxedStream(muxedStream, c)
	
	// 记录流
	c.mu.Lock()
	c.streams = append(c.streams, stream)
	c.mu.Unlock()
	
	return stream, nil
}

// ConnType 返回连接类型（v2.0 新增）
//
// 升级后的 TCP 连接始终为直连类型。
func (c *upgradedConnection) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}

// wrapMuxedStream 将 MuxedStream 包装为 Stream
func wrapMuxedStream(muxed pkgif.MuxedStream, conn pkgif.Connection) pkgif.Stream {
	return &tcpStream{
		MuxedStream: muxed,
		conn:        conn,
	}
}

// tcpStream 将 MuxedStream 包装为 Stream
type tcpStream struct {
	pkgif.MuxedStream
	conn     pkgif.Connection
	protocol string
}

// 确保实现接口
var _ pkgif.Stream = (*tcpStream)(nil)

// Conn 返回所属连接
func (s *tcpStream) Conn() pkgif.Connection {
	return s.conn
}

// Protocol 返回协议 ID
func (s *tcpStream) Protocol() string {
	return s.protocol
}

// SetProtocol 设置协议 ID
func (s *tcpStream) SetProtocol(protocol string) {
	s.protocol = protocol
}

// Stat 返回流统计
func (s *tcpStream) Stat() types.StreamStat {
	connStat := s.conn.Stat()
	direction := types.DirUnknown
	switch connStat.Direction {
	case pkgif.DirInbound:
		direction = types.DirInbound
	case pkgif.DirOutbound:
		direction = types.DirOutbound
	}
	return types.StreamStat{
		Direction:    direction,
		Opened:       time.Now(), // TCP stream 没有记录打开时间
		Protocol:     types.ProtocolID(s.protocol),
		BytesRead:    0,
		BytesWritten: 0,
	}
}

// IsClosed 检查流是否已关闭
func (s *tcpStream) IsClosed() bool {
	// MuxedStream 没有 IsClosed 方法，通过检查连接状态判断
	return s.conn.IsClosed()
}

// State 返回流当前状态
func (s *tcpStream) State() types.StreamState {
	if s.conn.IsClosed() {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}
