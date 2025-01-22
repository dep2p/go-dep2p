package gostream

import (
	"context"
	"io"
	"net"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// conn 是 net.Conn 的一个实现,它封装了 dep2p 流
type conn struct {
	// Stream 是底层的 dep2p 流
	network.Stream
	// ignoreEOF 表示是否忽略 EOF 错误
	ignoreEOF bool
}

// Read 从流中读取数据
// 参数:
//   - b: 用于存储读取数据的字节切片
//
// 返回:
//   - int: 实际读取的字节数
//   - error: 读取过程中的错误,如果 ignoreEOF 为 true 则忽略 EOF 错误
func (c *conn) Read(b []byte) (int, error) {
	// 从底层流中读取数据
	n, err := c.Stream.Read(b)
	// 如果设置了忽略 EOF 且错误为 EOF,则返回读取的字节数和 nil
	if err != nil && c.ignoreEOF && err == io.EOF {
		return n, nil
	}
	// 返回读取结果
	return n, err
}

// newConn 根据给定的 dep2p 流创建一个新的连接
// 参数:
//   - s: dep2p 流
//   - ignoreEOF: 是否忽略 EOF 错误
//
// 返回:
//   - net.Conn: 标准网络连接接口
func newConn(s network.Stream, ignoreEOF bool) net.Conn {
	// 返回封装了流的连接对象
	return &conn{s, ignoreEOF}
}

// LocalAddr 返回本地网络地址
// 返回:
//   - net.Addr: 本地对等点的网络地址
func (c *conn) LocalAddr() net.Addr {
	// 返回本地对等点地址
	return &addr{c.Stream.Conn().LocalPeer()}
}

// RemoteAddr 返回远程网络地址
// 返回:
//   - net.Addr: 远程对等点的网络地址
func (c *conn) RemoteAddr() net.Addr {
	// 返回远程对等点地址
	return &addr{c.Stream.Conn().RemotePeer()}
}

// Dial 使用给定的主机打开到目标地址的流
// 参数:
//   - ctx: 上下文
//   - h: dep2p 主机
//   - pid: 目标对等点 ID
//   - tag: 协议标识
//
// 返回:
//   - net.Conn: 标准网络连接
//   - error: 打开流过程中的错误
func Dial(ctx context.Context, h host.Host, pid peer.ID, tag protocol.ID) (net.Conn, error) {
	// 创建新的流
	s, err := h.NewStream(ctx, pid, tag)
	if err != nil {
		log.Debugf("创建流失败: %v", err)
		return nil, err
	}
	// 返回封装了流的连接
	return newConn(s, false), nil
}
