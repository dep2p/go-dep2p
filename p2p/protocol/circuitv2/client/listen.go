package client

import (
	"net"

	"github.com/dep2p/go-dep2p/core/transport"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// 确保 Listener 实现了 manet.Listener 接口
var _ manet.Listener = (*Listener)(nil)

// Listener 是 Client 的别名类型
type Listener Client

// Listener 返回当前客户端对应的监听器
// 返回值:
//   - *Listener 监听器对象
func (c *Client) Listener() *Listener {
	return (*Listener)(c)
}

// Accept 接受一个新的中继连接
// 返回值:
//   - manet.Conn 新接受的中继连接
//   - error 可能发生的错误
func (l *Listener) Accept() (manet.Conn, error) {
	for {
		select {
		case evt := <-l.incoming:
			// 向对端写入响应
			err := evt.writeResponse()
			if err != nil {
				log.Debugf("写入中继响应时发生错误: %s", err.Error())
				evt.conn.stream.Reset()
				continue
			}

			log.Debugf("已接受来自 %s 通过 %s 的中继连接", evt.conn.remote.ID, evt.conn.RemoteMultiaddr())

			// 标记连接为中继跳跃
			evt.conn.tagHop()
			return evt.conn, nil

		case <-l.ctx.Done():
			return nil, transport.ErrListenerClosed
		}
	}
}

// Addr 返回监听器的网络地址
// 返回值:
//   - net.Addr 网络地址对象
func (l *Listener) Addr() net.Addr {
	return &NetAddr{
		Relay:  "any",
		Remote: "any",
	}
}

// Multiaddr 返回监听器的多地址
// 返回值:
//   - ma.Multiaddr 多地址对象
func (l *Listener) Multiaddr() ma.Multiaddr {
	return circuitAddr
}

// Close 关闭监听器
// 返回值:
//   - error 可能发生的错误
func (l *Listener) Close() error {
	return (*Client)(l).Close()
}
