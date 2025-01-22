package gostream

import (
	"context"
	"net"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// listener 是一个 net.Listener 的实现,用于处理来自 dep2p 连接的 http 标记流
// 可以通过 Listen() 函数创建一个 listener
type listener struct {
	host     host.Host           // dep2p 主机实例
	ctx      context.Context     // 上下文对象,用于控制生命周期
	tag      protocol.ID         // 协议标识符
	cancel   func()              // 取消函数,用于停止监听
	streamCh chan network.Stream // 流通道,用于接收新的流连接
	// ignoreEOF 是一个标志,告诉监听器返回忽略 EOF 错误的连接
	// 这是必要的,因为默认的 responsewriter 在读取到 EOF 时会认为连接已关闭
	// 但在流上,我们可以读取到 EOF,但仍然可以继续写入
	ignoreEOF bool
}

// Accept 返回此监听器的下一个连接
// 如果没有连接,它会阻塞。在底层,连接是 dep2p 流
// 返回值:
//   - net.Conn: 新的网络连接
//   - error: 接收过程中的错误
func (l *listener) Accept() (net.Conn, error) {
	select {
	case s := <-l.streamCh: // 从流通道接收新的流
		return newConn(s, l.ignoreEOF), nil // 创建并返回新的连接
	case <-l.ctx.Done(): // 如果上下文被取消
		return nil, l.ctx.Err() // 返回上下文错误
	}
}

// Close 终止此监听器。它将不再处理任何传入的流
// 返回值:
//   - error: 关闭过程中的错误
func (l *listener) Close() error {
	l.cancel()                        // 调用取消函数
	l.host.RemoveStreamHandler(l.tag) // 移除流处理器
	return nil
}

// Addr 返回此监听器的地址,即其 dep2p 对等点 ID
// 返回值:
//   - net.Addr: 监听器的网络地址
func (l *listener) Addr() net.Addr {
	return &addr{l.host.ID()} // 返回包含主机 ID 的地址
}

// Listen 提供一个标准的 net.Listener,准备接受"连接"
// 在底层,这些连接是带有给定 protocol.ID 标记的 dep2p 流
//
// 参数:
//   - h host.Host: dep2p 主机实例
//   - tag protocol.ID: 协议标识符
//   - opts ...ListenerOption: 监听器选项列表
//
// 返回值:
//   - net.Listener: 监听器接口
//   - error: 错误信息
func Listen(h host.Host, tag protocol.ID, opts ...ListenerOption) (net.Listener, error) {
	ctx, cancel := context.WithCancel(context.Background()) // 创建可取消的上下文

	l := &listener{ // 创建新的监听器实例
		host:     h,
		ctx:      ctx,
		cancel:   cancel,
		tag:      tag,
		streamCh: make(chan network.Stream),
	}
	for _, opt := range opts { // 应用所有监听器选项
		if err := opt(l); err != nil {
			log.Debugf("应用监听器选项失败: %v", err)
			return nil, err
		}
	}

	h.SetStreamHandler(tag, func(s network.Stream) { // 设置流处理器
		select {
		case l.streamCh <- s: // 将新流发送到通道
		case <-ctx.Done(): // 如果上下文被取消
			s.Reset() // 重置流
		}
	})

	return l, nil
}

// ListenerOption 是监听器选项函数类型
// 参数:
//   - l *listener: 监听器实例
//
// 返回值:
//   - error: 应用选项时的错误
type ListenerOption func(*listener) error

// IgnoreEOF 返回一个设置 ignoreEOF 标志的监听器选项
// 返回值:
//   - ListenerOption: 监听器选项函数
func IgnoreEOF() ListenerOption {
	return func(l *listener) error {
		l.ignoreEOF = true // 设置忽略 EOF 标志
		return nil
	}
}
