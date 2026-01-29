package swarm

import (
	"context"
	"sync"
	
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// SwarmConn Swarm 连接封装
type SwarmConn struct {
	swarm *Swarm
	conn  pkgif.Connection // 升级后的连接
	
	streamsMu sync.Mutex
	streams   []pkgif.Stream
	
	closed bool
}

// newSwarmConn 创建 Swarm 连接
func newSwarmConn(swarm *Swarm, conn pkgif.Connection) *SwarmConn {
	return &SwarmConn{
		swarm:   swarm,
		conn:    conn,
		streams: make([]pkgif.Stream, 0),
		closed:  false,
	}
}

// LocalPeer 返回本地节点 ID
func (c *SwarmConn) LocalPeer() types.PeerID {
	return c.conn.LocalPeer()
}

// RemotePeer 返回远程节点 ID
func (c *SwarmConn) RemotePeer() types.PeerID {
	return c.conn.RemotePeer()
}

// LocalMultiaddr 返回本地地址
func (c *SwarmConn) LocalMultiaddr() types.Multiaddr {
	return c.conn.LocalMultiaddr()
}

// RemoteMultiaddr 返回远程地址
func (c *SwarmConn) RemoteMultiaddr() types.Multiaddr {
	return c.conn.RemoteMultiaddr()
}

// AcceptStream 接受入站流（被动等待远程创建流）
func (c *SwarmConn) AcceptStream() (pkgif.Stream, error) {
	// 检查连接是否已关闭（加锁保护，修复 B24 数据竞争）
	c.streamsMu.Lock()
	if c.closed {
		c.streamsMu.Unlock()
		return nil, ErrSwarmClosed
	}
	c.streamsMu.Unlock()
	
	// 委托给底层连接
	stream, err := c.conn.AcceptStream()
	if err != nil {
		return nil, err
	}
	
	// 封装为 SwarmStream
	swarmStream := newSwarmStream(c, stream)
	
	// 记录流
	c.streamsMu.Lock()
	// 再次检查，防止在等待 Accept 期间连接被关闭
	if c.closed {
		c.streamsMu.Unlock()
		stream.Close()
		return nil, ErrSwarmClosed
	}
	c.streams = append(c.streams, swarmStream)
	c.streamsMu.Unlock()
	
	return swarmStream, nil
}

// NewStream 创建新流
func (c *SwarmConn) NewStream(ctx context.Context) (pkgif.Stream, error) {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	
	if c.closed {
		return nil, ErrSwarmClosed
	}
	
	// 创建底层流
	stream, err := c.conn.NewStream(ctx)
	if err != nil {
		return nil, err
	}
	
	// 封装为 SwarmStream
	swarmStream := newSwarmStream(c, stream)
	
	// 记录流
	c.streams = append(c.streams, swarmStream)
	
	return swarmStream, nil
}

// GetStreams 获取所有流
func (c *SwarmConn) GetStreams() []pkgif.Stream {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	
	// 返回副本
	streams := make([]pkgif.Stream, len(c.streams))
	copy(streams, c.streams)
	return streams
}

// IsClosed 返回连接是否已关闭
func (c *SwarmConn) IsClosed() bool {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	return c.closed
}

// Stat 返回连接统计信息
func (c *SwarmConn) Stat() pkgif.ConnectionStat {
	// 委托给底层连接
	return c.conn.Stat()
}

// Close 关闭连接
func (c *SwarmConn) Close() error {
	c.streamsMu.Lock()
	if c.closed {
		c.streamsMu.Unlock()
		return nil
	}
	c.closed = true
	
	// 关闭所有流
	streams := c.streams
	c.streams = nil
	c.streamsMu.Unlock()
	
	for _, stream := range streams {
		stream.Close()
	}
	
	// 从 Swarm 移除
	c.swarm.removeConn(c)
	
	// 触发断开事件
	c.swarm.notifyDisconnected(c)
	
	// 关闭底层连接
	return c.conn.Close()
}

// removeStream 移除流（内部方法）
func (c *SwarmConn) removeStream(stream pkgif.Stream) {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	
	for i, s := range c.streams {
		if s == stream {
			c.streams = append(c.streams[:i], c.streams[i+1:]...)
			break
		}
	}
}

// getBandwidthCounter 获取带宽计数器（内部方法）
func (c *SwarmConn) getBandwidthCounter() pkgif.BandwidthCounter {
	return c.swarm.getBandwidthCounter()
}

// ConnType 返回连接类型（v2.0 新增）
//
// SwarmConn 代理底层连接的 ConnType 方法。
func (c *SwarmConn) ConnType() pkgif.ConnectionType {
	return c.conn.ConnType()
}
