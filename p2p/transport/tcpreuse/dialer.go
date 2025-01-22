package tcpreuse

import (
	"context"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// DialContext 类似于 Dial 但接收一个上下文参数
// 参数:
//   - ctx: context.Context 上下文对象,用于控制超时和取消
//   - raddr: ma.Multiaddr 目标多地址
//
// 返回值:
//   - manet.Conn 建立的网络连接
//   - error 可能发生的错误
//
// 注意:
//   - 如果启用了端口重用,将使用重用端口的方式建立连接
//   - 否则使用普通的网络拨号方式
func (t *ConnMgr) DialContext(ctx context.Context, raddr ma.Multiaddr) (manet.Conn, error) {
	// 检查是否启用了端口重用
	if t.useReuseport() {
		// 使用重用端口的方式建立连接
		return t.reuse.DialContext(ctx, raddr)
	}
	// 创建普通的网络拨号器
	var d manet.Dialer
	// 使用普通拨号方式建立连接
	return d.DialContext(ctx, raddr)
}
