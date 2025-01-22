package reuseport

import (
	"context"
	"net"

	"github.com/dep2p/go-dep2p/p2plib/reuseport"
)

// 默认的回退拨号器
var fallbackDialer net.Dialer

// reuseDial 使用端口重用进行拨号,如果失败则使用普通拨号重试
// 参数:
//   - ctx: context.Context 上下文对象
//   - laddr: *net.TCPAddr 本地地址
//   - network: string 网络类型
//   - raddr: string 远程地址
//
// 返回值:
//   - con: net.Conn 建立的网络连接
//   - err: error 错误信息
func reuseDial(ctx context.Context, laddr *net.TCPAddr, network, raddr string) (con net.Conn, err error) {
	// 如果本地地址为空,直接使用默认拨号器
	if laddr == nil {
		return fallbackDialer.DialContext(ctx, network, raddr)
	}

	// 创建支持端口重用的拨号器
	d := net.Dialer{
		LocalAddr: laddr,
		Control:   reuseport.Control,
	}

	// 尝试使用端口重用进行拨号
	con, err = d.DialContext(ctx, network, raddr)
	if err == nil {
		return con, nil
	}

	// 如果是可重试的错误且上下文未取消,则使用随机端口重试
	if reuseErrShouldRetry(err) && ctx.Err() == nil {
		// 可能存在已打开的socket或处于TIME-WAIT状态的socket
		log.Debugf("端口重用失败,将使用随机端口重试: %s", err)
		con, err = fallbackDialer.DialContext(ctx, network, raddr)
	}
	return con, err
}
