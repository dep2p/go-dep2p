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
var _ pkgif.Connection = (*Connection)(nil)

// Connection TCP 连接
// 注意：TCP 需要配合 Upgrader（Security + Muxer）使用
type Connection struct {
	mu sync.RWMutex

	conn       net.Conn
	localPeer  types.PeerID
	remotePeer types.PeerID
	remoteAddr types.Multiaddr
	direction  pkgif.Direction

	opened time.Time
	closed bool
}

// newConnection 创建新连接
func newConnection(conn net.Conn, local, remote types.PeerID, remoteAddr types.Multiaddr, dir pkgif.Direction) *Connection {
	return &Connection{
		conn:       conn,
		localPeer:  local,
		remotePeer: remote,
		remoteAddr: remoteAddr,
		direction:  dir,
		opened:     time.Now(),
	}
}

// LocalPeer 返回本地节点 ID
func (c *Connection) LocalPeer() types.PeerID {
	return c.localPeer
}

// LocalMultiaddr 返回本地多地址
func (c *Connection) LocalMultiaddr() types.Multiaddr {
	localTCPAddr := c.conn.LocalAddr().(*net.TCPAddr)
	addrStr := fmt.Sprintf("/ip4/%s/tcp/%d", localTCPAddr.IP.String(), localTCPAddr.Port)
	addr, _ := types.NewMultiaddr(addrStr)
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

// NewStream 创建新流
// 注意：原始 TCP 连接不支持多路复用，需要与 Muxer 配合
func (c *Connection) NewStream(_ context.Context) (pkgif.Stream, error) {
	return nil, ErrNoMuxer
}

// AcceptStream 接受对方创建的流
func (c *Connection) AcceptStream() (pkgif.Stream, error) {
	return nil, ErrNoMuxer
}

// GetStreams 获取所有流
func (c *Connection) GetStreams() []pkgif.Stream {
	return []pkgif.Stream{}
}

// Stat 返回连接统计
func (c *Connection) Stat() pkgif.ConnectionStat {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return pkgif.ConnectionStat{
		Direction:  c.direction,
		Opened:     c.opened.Unix(),
		Transient:  false,
		NumStreams: 0,
	}
}

// Close 关闭连接
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	return c.conn.Close()
}

// IsClosed 检查是否已关闭
func (c *Connection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.closed
}

// RawConn 返回原始连接（用于 Upgrader）
func (c *Connection) RawConn() net.Conn {
	return c.conn
}

// ConnType 返回连接类型（v2.0 新增）
//
// TCP 连接始终为直连类型。
func (c *Connection) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}
