package reuseport

import (
	"context"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// Dial 使用给定的多地址建立连接,尽可能复用当前正在监听的端口
// 参数:
//   - raddr: ma.Multiaddr 目标多地址
//
// 返回值:
//   - manet.Conn: 建立的网络连接
//   - error: 错误信息
//
// 注意:
//   - Dial 会智能选择源端口
//   - 如果我们正在拨号回环地址,且正在监听一个或多个回环端口,Dial 会随机选择一个回环端口和地址并复用它
func (t *Transport) Dial(raddr ma.Multiaddr) (manet.Conn, error) {
	return t.DialContext(context.Background(), raddr)
}

// DialContext 与 Dial 类似,但接受一个上下文参数
// 参数:
//   - ctx: context.Context 上下文对象
//   - raddr: ma.Multiaddr 目标多地址
//
// 返回值:
//   - manet.Conn: 建立的网络连接
//   - error: 错误信息
func (t *Transport) DialContext(ctx context.Context, raddr ma.Multiaddr) (manet.Conn, error) {
	// 从多地址解析出网络类型和地址
	network, addr, err := manet.DialArgs(raddr)
	if err != nil {
		log.Debugf("解析多地址失败: %v", err)
		return nil, err
	}

	// 根据网络类型选择拨号器
	var d *dialer
	switch network {
	case "tcp4":
		d = t.v4.getDialer(network)
	case "tcp6":
		d = t.v6.getDialer(network)
	default:
		log.Debugf("不支持的网络类型: %s", network)
		return nil, ErrWrongProto
	}

	// 使用拨号器建立连接
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		log.Debugf("拨号失败: %v", err)
		return nil, err
	}

	// 将网络连接包装为多地址连接
	maconn, err := manet.WrapNetConn(conn)
	if err != nil {
		conn.Close()
		log.Debugf("包装网络连接失败: %v", err)
		return nil, err
	}
	return maconn, nil
}

// getDialer 获取网络拨号器
// 参数:
//   - network: string 网络类型
//
// 返回值:
//   - *dialer: 拨号器对象
func (n *network) getDialer(network string) *dialer {
	// 尝试以只读锁获取拨号器
	n.mu.RLock()
	d := n.dialer
	n.mu.RUnlock()

	// 如果拨号器不存在,则创建新的拨号器
	if d == nil {
		n.mu.Lock()
		defer n.mu.Unlock()

		if n.dialer == nil {
			n.dialer = newDialer(n.listeners)
		}
		d = n.dialer
	}
	return d
}
