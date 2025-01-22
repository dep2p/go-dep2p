package client

import (
	"fmt"
	"net"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	tpt "github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// HopTagWeight 是用于携带中继跳跃流的连接管理器权重
var HopTagWeight = 5

type statLimitDuration struct{}
type statLimitData struct{}

var (
	StatLimitDuration = statLimitDuration{}
	StatLimitData     = statLimitData{}
)

// Conn 表示一个中继连接
type Conn struct {
	// stream 是底层的网络流
	stream network.Stream
	// remote 是远程对等节点的地址信息
	remote peer.AddrInfo
	// stat 保存连接的统计信息
	stat network.ConnStats
	// client 是关联的中继客户端
	client *Client
}

// NetAddr 表示网络地址
type NetAddr struct {
	// Relay 是中继节点的地址
	Relay string
	// Remote 是远程节点的地址
	Remote string
}

var _ net.Addr = (*NetAddr)(nil)

// Network 返回网络类型名称
func (n *NetAddr) Network() string {
	return "dep2p-circuit-relay"
}

// String 返回网络地址的字符串表示
func (n *NetAddr) String() string {
	return fmt.Sprintf("relay[%s-%s]", n.Remote, n.Relay)
}

// Conn 接口实现
var _ manet.Conn = (*Conn)(nil)

// Close 关闭连接
func (c *Conn) Close() error {
	c.untagHop()
	return c.stream.Reset()
}

// Read 从连接中读取数据
func (c *Conn) Read(buf []byte) (int, error) {
	return c.stream.Read(buf)
}

// Write 向连接写入数据
func (c *Conn) Write(buf []byte) (int, error) {
	return c.stream.Write(buf)
}

// SetDeadline 设置读写超时时间
func (c *Conn) SetDeadline(t time.Time) error {
	return c.stream.SetDeadline(t)
}

// SetReadDeadline 设置读取超时时间
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写入超时时间
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

// TODO: 将 c.Conn().RemotePeer() 转换为 multiaddr 是否安全？可能是"用户输入"
func (c *Conn) RemoteMultiaddr() ma.Multiaddr {
	// TODO: 我们应该能够直接完成这个操作，而不需要在字符串之间转换
	relayAddr, err := ma.NewComponent(
		ma.ProtocolWithCode(ma.P_P2P).Name,
		c.stream.Conn().RemotePeer().String(),
	)
	if err != nil {
		panic(err)
	}
	return ma.Join(c.stream.Conn().RemoteMultiaddr(), relayAddr, circuitAddr)
}

// LocalMultiaddr 返回本地多地址
func (c *Conn) LocalMultiaddr() ma.Multiaddr {
	return c.stream.Conn().LocalMultiaddr()
}

// LocalAddr 返回本地网络地址
func (c *Conn) LocalAddr() net.Addr {
	na, err := manet.ToNetAddr(c.stream.Conn().LocalMultiaddr())
	if err != nil {
		log.Debugf("转换本地多地址到网络地址失败: %v", err)
		return nil
	}
	return na
}

// RemoteAddr 返回远程网络地址
func (c *Conn) RemoteAddr() net.Addr {
	return &NetAddr{
		Relay:  c.stream.Conn().RemotePeer().String(),
		Remote: c.remote.ID.String(),
	}
}

// ConnStat 接口实现
var _ network.ConnStat = (*Conn)(nil)

// Stat 返回连接统计信息
func (c *Conn) Stat() network.ConnStats {
	return c.stat
}

// tagHop 标记底层中继连接，使其能够（在某种程度上）受到连接管理器的保护， 因为它是代理其他连接的重要连接。
// 这在这里处理是为了避免用户代码需要处理这个问题， 并避免出现高价值对等连接在中继连接后面而因连接管理器关闭底层中继连接而隐式断开的情况。
func (c *Conn) tagHop() {
	c.client.mx.Lock()
	defer c.client.mx.Unlock()

	p := c.stream.Conn().RemotePeer()
	c.client.hopCount[p]++
	if c.client.hopCount[p] == 1 {
		c.client.host.ConnManager().TagPeer(p, "relay-hop-stream", HopTagWeight)
	}
}

// untagHop 在必要时移除 relay-hop-stream 标记；当中继连接关闭时调用。
func (c *Conn) untagHop() {
	c.client.mx.Lock()
	defer c.client.mx.Unlock()

	p := c.stream.Conn().RemotePeer()
	c.client.hopCount[p]--
	if c.client.hopCount[p] == 0 {
		c.client.host.ConnManager().UntagPeer(p, "relay-hop-stream")
		delete(c.client.hopCount, p)
	}
}

// capableConnWithStat 定义了一个具有连接能力和统计功能的接口
type capableConnWithStat interface {
	tpt.CapableConn
	network.ConnStat
}

// capableConn 包装了一个具有连接能力和统计功能的连接
type capableConn struct {
	capableConnWithStat
}

var transportName = ma.ProtocolWithCode(ma.P_CIRCUIT).Name

// ConnState 返回连接状态
func (c capableConn) ConnState() network.ConnectionState {
	return network.ConnectionState{
		Transport: transportName,
	}
}
