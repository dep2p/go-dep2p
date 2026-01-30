// Package quic 实现 QUIC 传输
package quic

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/quic-go/quic-go"
)

// splitHostPort 拆分 "host:port" 格式的地址
func splitHostPort(addr string) (host, port string, err error) {
	return net.SplitHostPort(addr)
}

// 确保实现了接口
var _ pkgif.Connection = (*Connection)(nil)

// Connection QUIC 连接
type Connection struct {
	mu sync.RWMutex

	quicConn   *quic.Conn
	localPeer  types.PeerID
	remotePeer types.PeerID
	remoteAddr types.Multiaddr
	direction  pkgif.Direction

	streams []pkgif.Stream
	opened  time.Time
	closed  bool
}

// newConnection 创建新连接
func newConnection(quicConn *quic.Conn, local, remote types.PeerID, remoteAddr types.Multiaddr, dir pkgif.Direction) *Connection {
	return &Connection{
		quicConn:   quicConn,
		localPeer:  local,
		remotePeer: remote,
		remoteAddr: remoteAddr,
		direction:  dir,
		streams:    make([]pkgif.Stream, 0),
		opened:     time.Now(),
	}
}

// LocalPeer 返回本地节点 ID
func (c *Connection) LocalPeer() types.PeerID {
	return c.localPeer
}

// LocalMultiaddr 返回本地多地址
func (c *Connection) LocalMultiaddr() types.Multiaddr {
	localUDPAddr := c.quicConn.LocalAddr()
	if localUDPAddr == nil {
		return nil
	}

	// localUDPAddr.String() 返回 "ip:port" 格式，需要拆分
	host, port, err := splitHostPort(localUDPAddr.String())
	if err != nil {
		logger.Warn("解析本地地址失败", "addr", localUDPAddr.String(), "error", err)
		return nil
	}

	// 判断 IPv4 或 IPv6
	ipProto := "ip4"
	ip := net.ParseIP(host)
	if ip != nil && ip.To4() == nil {
		ipProto = "ip6"
	}

	// 构造正确的 multiaddr: /ip4|ip6/host/udp/port/quic-v1
	addrStr := fmt.Sprintf("/%s/%s/udp/%s/quic-v1", ipProto, host, port)
	addr, err := types.NewMultiaddr(addrStr)
	if err != nil {
		logger.Warn("创建本地多地址失败", "addrStr", addrStr, "error", err)
		return nil
	}
	return addr
}

// RemotePeer 返回远端节点 ID
func (c *Connection) RemotePeer() types.PeerID {
	return c.remotePeer
}

// RemoteMultiaddr 返回远端多地址
func (c *Connection) RemoteMultiaddr() types.Multiaddr {
	return c.remoteAddr
}

// NewStream 创建新流（默认优先级）
func (c *Connection) NewStream(ctx context.Context) (pkgif.Stream, error) {
	return c.NewStreamWithPriority(ctx, int(pkgif.StreamPriorityNormal))
}

// NewStreamWithPriority 创建新流（指定优先级）(v1.2 新增)
//
// 允许指定流优先级。QUIC (RFC 9000) 原生支持流优先级，
// 用于在网络拥塞时优先调度重要流。
//
// 参数:
//   - ctx: 上下文
//   - priority: 流优先级 (0=Critical, 1=High, 2=Normal, 3=Low)
//
// 注意：当前实现中优先级信息被记录但 quic-go 的优先级调度需要额外配置。
// 未来版本将与 quic-go 的 Stream.SetPriority() 集成。
func (c *Connection) NewStreamWithPriority(ctx context.Context, priority int) (pkgif.Stream, error) {
	// 先检查连接是否已关闭
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrConnectionClosed
	}
	c.mu.RUnlock()

	// 在锁外调用可能阻塞的 OpenStreamSync
	quicStream, err := c.quicConn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}

	// 重新获取写锁来添加 stream
	c.mu.Lock()
	defer c.mu.Unlock()

	// 再次检查（可能在等待期间被关闭）
	if c.closed {
		quicStream.Close()
		return nil, ErrConnectionClosed
	}

	stream := newStreamWithPriority(quicStream, c, priority)
	c.streams = append(c.streams, stream)

	return stream, nil
}

// SupportsStreamPriority 检查连接是否支持流优先级 (v1.2 新增)
//
// QUIC 连接支持流优先级。
func (c *Connection) SupportsStreamPriority() bool {
	return true
}

// AcceptStream 接受对方创建的流
func (c *Connection) AcceptStream() (pkgif.Stream, error) {
	// 先检查连接是否已关闭（不持锁调用阻塞操作）
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, ErrConnectionClosed
	}
	c.mu.RUnlock()

	// 在锁外调用可能阻塞的 AcceptStream
	quicStream, err := c.quicConn.AcceptStream(context.Background())
	if err != nil {
		return nil, err
	}

	// 重新获取写锁来添加 stream
	c.mu.Lock()
	defer c.mu.Unlock()

	// 再次检查（可能在等待期间被关闭）
	if c.closed {
		quicStream.Close()
		return nil, ErrConnectionClosed
	}

	stream := newStream(quicStream, c)
	c.streams = append(c.streams, stream)

	return stream, nil
}

// GetStreams 获取所有流
func (c *Connection) GetStreams() []pkgif.Stream {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return append([]pkgif.Stream{}, c.streams...)
}

// Stat 返回连接统计
func (c *Connection) Stat() pkgif.ConnectionStat {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return pkgif.ConnectionStat{
		Direction:  c.direction,
		Opened:     c.opened.Unix(),
		Transient:  false,
		NumStreams: len(c.streams),
	}
}

// Close 关闭连接
func (c *Connection) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// 在锁外调用可能阻塞的 QUIC 关闭操作，避免死锁
	return c.quicConn.CloseWithError(0, "connection closed")
}

// IsClosed 检查是否已关闭
func (c *Connection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.closed
}

// ConnType 返回连接类型（v2.0 新增）
//
// QUIC 连接始终为直连类型。
func (c *Connection) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}
