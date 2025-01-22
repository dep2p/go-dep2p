package quicreuse

import (
	"context"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// nonQUICPacketConn 是一个 net.PacketConn 接口的实现,用于在 quic.Transport 上读写非 QUIC 数据包
// 这允许我们将 UDP 端口复用于其他传输协议,如 WebRTC
type nonQUICPacketConn struct {
	// owningTransport 是引用计数的 QUIC 传输层,用于跟踪传输层的使用情况
	owningTransport refCountedQuicTransport
	// tr 是底层的 QUIC 传输层实例,用于实际的数据包传输
	tr *quic.Transport
	// ctx 是主上下文,用于控制整个连接的生命周期
	ctx context.Context
	// ctxCancel 是主上下文的取消函数,用于关闭连接
	ctxCancel context.CancelFunc
	// readCtx 是读取操作的上下文,用于控制单个读取操作的超时
	readCtx context.Context
	// readCancel 是读取上下文的取消函数,用于取消读取操作
	readCancel context.CancelFunc
}

// Close 实现 net.PacketConn 接口,关闭连接
// 返回值:
//   - error 错误信息
func (n *nonQUICPacketConn) Close() error {
	// 取消上下文
	n.ctxCancel()

	// 不实际关闭底层传输层,因为其他人可能正在使用它
	// reuse 有自己的垃圾回收机制来关闭未使用的传输层
	n.owningTransport.DecreaseCount()
	return nil
}

// LocalAddr 实现 net.PacketConn 接口,返回本地地址
// 返回值:
//   - net.Addr 本地网络地址
func (n *nonQUICPacketConn) LocalAddr() net.Addr {
	return n.tr.Conn.LocalAddr()
}

// ReadFrom 实现 net.PacketConn 接口,从连接读取数据
// 参数:
//   - p: []byte 用于存储读取数据的缓冲区
//
// 返回值:
//   - int 读取的字节数
//   - net.Addr 发送方地址
//   - error 错误信息
func (n *nonQUICPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	// 如果存在读取上下文则使用它,否则使用主上下文
	ctx := n.readCtx
	if ctx == nil {
		ctx = n.ctx
	}
	return n.tr.ReadNonQUICPacket(ctx, p)
}

// SetDeadline 实现 net.PacketConn 接口,设置读写超时时间
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error 错误信息
func (n *nonQUICPacketConn) SetDeadline(t time.Time) error {
	// 仅用于读取操作
	return n.SetReadDeadline(t)
}

// SetReadDeadline 实现 net.PacketConn 接口,设置读取超时时间
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error 错误信息
func (n *nonQUICPacketConn) SetReadDeadline(t time.Time) error {
	// 如果超时时间为零且存在读取上下文,则取消当前读取上下文
	if t.IsZero() && n.readCtx != nil {
		n.readCancel()
		n.readCtx = nil
	}
	// 创建新的带超时的读取上下文
	n.readCtx, n.readCancel = context.WithDeadline(n.ctx, t)
	return nil
}

// SetWriteDeadline 实现 net.PacketConn 接口,设置写入超时时间
// 参数:
//   - t: time.Time 超时时间
//
// 返回值:
//   - error 错误信息
func (n *nonQUICPacketConn) SetWriteDeadline(t time.Time) error {
	// 未使用,quic-go 不支持写入超时
	return nil
}

// WriteTo 实现 net.PacketConn 接口,向指定地址写入数据
// 参数:
//   - p: []byte 要写入的数据
//   - addr: net.Addr 目标地址
//
// 返回值:
//   - int 写入的字节数
//   - error 错误信息
func (n *nonQUICPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	return n.tr.WriteTo(p, addr)
}

// 确保 nonQUICPacketConn 实现了 net.PacketConn 接口
var _ net.PacketConn = &nonQUICPacketConn{}
