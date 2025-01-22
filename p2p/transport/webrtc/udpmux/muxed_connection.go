package udpmux

import (
	"context"
	"errors"
	"net"
	"time"

	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"
)

// packet 表示一个网络数据包
// 字段:
//   - buf: []byte 数据包内容
//   - addr: net.Addr 数据包来源地址
type packet struct {
	buf  []byte
	addr net.Addr
}

var _ net.PacketConn = &muxedConnection{}

const queueLen = 128 // 队列长度

// muxedConnection 提供了一个基于 packetQueue 的 net.PacketConn 抽象,并添加了存储该连接(由 ufrag 索引)接收数据的地址的功能
// 字段:
//   - ctx: context.Context 上下文对象
//   - cancel: context.CancelFunc 取消函数
//   - onClose: func() 关闭时的回调函数
//   - queue: chan packet 数据包队列
//   - mux: *UDPMux UDP 多路复用器
type muxedConnection struct {
	ctx     context.Context
	cancel  context.CancelFunc
	onClose func()
	queue   chan packet
	mux     *UDPMux
}

var _ net.PacketConn = &muxedConnection{}

// newMuxedConnection 创建一个新的多路复用连接
// 参数:
//   - mux: *UDPMux UDP 多路复用器
//   - onClose: func() 关闭时的回调函数
//
// 返回值:
//   - *muxedConnection 新创建的多路复用连接
func newMuxedConnection(mux *UDPMux, onClose func()) *muxedConnection {
	ctx, cancel := context.WithCancel(mux.ctx)
	return &muxedConnection{
		ctx:     ctx,
		cancel:  cancel,
		queue:   make(chan packet, queueLen),
		onClose: onClose,
		mux:     mux,
	}
}

// Push 将数据包推送到队列中
// 参数:
//   - buf: []byte 数据包内容
//   - addr: net.Addr 数据包来源地址
//
// 返回值:
//   - error 如果连接已关闭返回"已关闭"错误,如果队列已满返回"队列已满"错误
func (c *muxedConnection) Push(buf []byte, addr net.Addr) error {
	select {
	case <-c.ctx.Done():
		return errors.New("已关闭")
	default:
	}
	select {
	case c.queue <- packet{buf: buf, addr: addr}:
		return nil
	default:
		return errors.New("队列已满")
	}
}

// ReadFrom 从队列中读取数据包
// 参数:
//   - buf: []byte 用于存储读取数据的缓冲区
//
// 返回值:
//   - int 读取的字节数
//   - net.Addr 数据包来源地址
//   - error 如果连接已关闭则返回上下文错误
func (c *muxedConnection) ReadFrom(buf []byte) (int, net.Addr, error) {
	select {
	case p := <-c.queue:
		n := copy(buf, p.buf) // 如果 buf 太短可能会丢弃部分数据包
		if n < len(p.buf) {
			log.Debugf("读取不完整,原有 %d 字节,实际读取 %d 字节", len(p.buf), n)
		}
		pool.Put(p.buf)
		return n, p.addr, nil
	case <-c.ctx.Done():
		return 0, nil, c.ctx.Err()
	}
}

// WriteTo 将数据写入到指定地址
// 参数:
//   - p: []byte 要写入的数据
//   - addr: net.Addr 目标地址
//
// 返回值:
//   - n: int 写入的字节数
//   - err: error 写入过程中的错误
func (c *muxedConnection) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return c.mux.writeTo(p, addr)
}

// Close 关闭连接并清空队列
// 返回值:
//   - error 始终返回 nil
func (c *muxedConnection) Close() error {
	select {
	case <-c.ctx.Done():
		return nil
	default:
	}
	c.onClose()
	c.cancel()
	// 清空数据包队列
	for {
		select {
		case p := <-c.queue:
			pool.Put(p.buf)
		default:
			return nil
		}
	}
}

// LocalAddr 返回本地地址
// 返回值:
//   - net.Addr 本地网络地址
func (c *muxedConnection) LocalAddr() net.Addr { return c.mux.socket.LocalAddr() }

// SetDeadline 设置读写超时时间(此处不需要)
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error 始终返回 nil
func (*muxedConnection) SetDeadline(t time.Time) error {
	// 此处不需要设置超时
	return nil
}

// SetReadDeadline 设置读取超时时间(此处不需要)
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error 始终返回 nil
func (*muxedConnection) SetReadDeadline(t time.Time) error {
	// 此处不需要设置读取超时
	return nil
}

// SetWriteDeadline 设置写入超时时间(此处不需要)
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error 始终返回 nil
func (*muxedConnection) SetWriteDeadline(t time.Time) error {
	// 此处不需要设置写入超时
	return nil
}
