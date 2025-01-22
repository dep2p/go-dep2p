package mocknet

import (
	"container/list"
	"context"
	"strconv"
	"sync"
	"sync/atomic"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// 连接计数器
var connCounter atomic.Int64

// conn 表示两个对等节点之间活跃连接的一端视角
// 它通过特定的链路进行通信
type conn struct {
	// 通知锁
	notifLk sync.Mutex

	// 连接ID
	id int64

	// 本地节点ID
	local peer.ID
	// 远程节点ID
	remote peer.ID

	// 本地地址
	localAddr ma.Multiaddr
	// 远程地址
	remoteAddr ma.Multiaddr

	// 本地私钥
	localPrivKey ic.PrivKey
	// 远程公钥
	remotePubKey ic.PubKey

	// 网络实例
	net *peernet
	// 链路实例
	link *link
	// 对端连接
	rconn *conn
	// 流列表
	streams list.List
	// 连接统计信息
	stat network.ConnStats

	// 确保只关闭一次的锁
	closeOnce sync.Once

	// 连接是否已关闭标志
	isClosed atomic.Bool

	// 读写锁
	sync.RWMutex
}

// newConn 创建一个新的连接
// 参数:
//   - ln: 本地网络
//   - rn: 远程网络
//   - l: 链路
//   - dir: 连接方向
//
// 返回:
//   - *conn: 新创建的连接
func newConn(ln, rn *peernet, l *link, dir network.Direction) *conn {
	// 创建连接实例
	c := &conn{net: ln, link: l}
	// 设置本地和远程节点ID
	c.local = ln.peer
	c.remote = rn.peer
	// 设置连接方向
	c.stat.Direction = dir
	// 生成连接ID
	c.id = connCounter.Add(1)

	// 设置本地地址
	c.localAddr = ln.ps.Addrs(ln.peer)[0]
	// 设置远程地址,优先选择非未指定IP的地址
	for _, a := range rn.ps.Addrs(rn.peer) {
		if !manet.IsIPUnspecified(a) {
			c.remoteAddr = a
			break
		}
	}
	if c.remoteAddr == nil {
		c.remoteAddr = rn.ps.Addrs(rn.peer)[0]
	}

	// 设置本地私钥和远程公钥
	c.localPrivKey = ln.ps.PrivKey(ln.peer)
	c.remotePubKey = rn.ps.PubKey(rn.peer)
	return c
}

// IsClosed 检查连接是否已关闭
// 返回:
//   - bool: 连接是否已关闭
func (c *conn) IsClosed() bool {
	return c.isClosed.Load()
}

// ID 获取连接ID
// 返回:
//   - string: 连接ID的字符串表示
func (c *conn) ID() string {
	return strconv.FormatInt(c.id, 10)
}

// Close 关闭连接
// 返回:
//   - error: 关闭过程中的错误
func (c *conn) Close() error {
	c.closeOnce.Do(func() {
		// 标记连接为已关闭
		c.isClosed.Store(true)
		// 异步关闭对端连接
		go c.rconn.Close()
		// 清理连接资源
		c.teardown()
	})
	return nil
}

// teardown 清理连接资源
func (c *conn) teardown() {
	// 重置所有流
	for _, s := range c.allStreams() {
		s.Reset()
	}
	// 从网络中移除连接
	c.net.removeConn(c)
}

// addStream 添加流到连接
// 参数:
//   - s: 要添加的流
func (c *conn) addStream(s *stream) {
	c.Lock()
	defer c.Unlock()
	s.conn = c
	c.streams.PushBack(s)
}

// removeStream 从连接中移除流
// 参数:
//   - s: 要移除的流
func (c *conn) removeStream(s *stream) {
	c.Lock()
	defer c.Unlock()
	for e := c.streams.Front(); e != nil; e = e.Next() {
		if s == e.Value {
			c.streams.Remove(e)
			return
		}
	}
}

// allStreams 获取所有流
// 返回:
//   - []network.Stream: 流列表
func (c *conn) allStreams() []network.Stream {
	c.RLock()
	defer c.RUnlock()

	strs := make([]network.Stream, 0, c.streams.Len())
	for e := c.streams.Front(); e != nil; e = e.Next() {
		s := e.Value.(*stream)
		strs = append(strs, s)
	}
	return strs
}

// remoteOpenedStream 处理远程打开的流
// 参数:
//   - s: 远程打开的流
func (c *conn) remoteOpenedStream(s *stream) {
	c.addStream(s)
	c.net.handleNewStream(s)
}

// openStream 打开新流
// 返回:
//   - *stream: 新打开的流
func (c *conn) openStream() *stream {
	sl, sr := newStreamPair()
	go c.rconn.remoteOpenedStream(sr)
	c.addStream(sl)
	return sl
}

// NewStream 创建新的流
// 参数:
//   - context.Context: 上下文
//
// 返回:
//   - network.Stream: 新创建的流
//   - error: 创建过程中的错误
func (c *conn) NewStream(context.Context) (network.Stream, error) {
	log.Debugf("Conn.NewStreamWithProtocol: %s --> %s", c.local, c.remote)

	s := c.openStream()
	return s, nil
}

// GetStreams 获取所有流
// 返回:
//   - []network.Stream: 流列表
func (c *conn) GetStreams() []network.Stream {
	return c.allStreams()
}

// LocalMultiaddr 获取本地多地址
// 返回:
//   - ma.Multiaddr: 本地多地址
func (c *conn) LocalMultiaddr() ma.Multiaddr {
	return c.localAddr
}

// LocalPeer 获取本地节点ID
// 返回:
//   - peer.ID: 本地节点ID
func (c *conn) LocalPeer() peer.ID {
	return c.local
}

// RemoteMultiaddr 获取远程多地址
// 返回:
//   - ma.Multiaddr: 远程多地址
func (c *conn) RemoteMultiaddr() ma.Multiaddr {
	return c.remoteAddr
}

// RemotePeer 获取远程节点ID
// 返回:
//   - peer.ID: 远程节点ID
func (c *conn) RemotePeer() peer.ID {
	return c.remote
}

// RemotePublicKey 获取远程节点的公钥
// 返回:
//   - ic.PubKey: 远程节点的公钥
func (c *conn) RemotePublicKey() ic.PubKey {
	return c.remotePubKey
}

// ConnState 获取安全连接的状态(如果不支持则为空)
// 返回:
//   - network.ConnectionState: 连接状态
func (c *conn) ConnState() network.ConnectionState {
	return network.ConnectionState{}
}

// Stat 获取连接的元数据
// 返回:
//   - network.ConnStats: 连接统计信息
func (c *conn) Stat() network.ConnStats {
	return c.stat
}

// Scope 获取连接作用域
// 返回:
//   - network.ConnScope: 连接作用域
func (c *conn) Scope() network.ConnScope {
	return &network.NullScope{}
}
