package reuseport

import (
	"context"
	"fmt"
	"math/rand"
	"net"

	"github.com/dep2p/go-dep2p/p2plib/netroute"
)

// dialer 结构体用于管理网络连接的拨号
type dialer struct {
	// specific 存储所有非回环和非未指定的地址(非0.0.0.0或::)
	specific []*net.TCPAddr
	// loopback 存储所有回环地址(127.*.*.*, ::1)
	loopback []*net.TCPAddr
	// unspecified 存储所有未指定地址(0.0.0.0, ::)
	unspecified []*net.TCPAddr
}

// Dial 使用默认上下文拨号到指定地址
// 参数:
//   - network: string 网络类型
//   - addr: string 目标地址
//
// 返回值:
//   - net.Conn 建立的网络连接
//   - error 可能的错误信息
func (d *dialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// randAddr 从给定的TCP地址列表中随机选择一个地址
// 参数:
//   - addrs: []*net.TCPAddr TCP地址列表
//
// 返回值:
//   - *net.TCPAddr 随机选择的TCP地址,如果列表为空则返回nil
func randAddr(addrs []*net.TCPAddr) *net.TCPAddr {
	if len(addrs) > 0 {
		return addrs[rand.Intn(len(addrs))]
	}
	return nil
}

// DialContext 使用指定上下文拨号到目标地址
//
// 按以下顺序尝试:
//
//  1. 如果我们明确监听目标地址的首选源地址(根据系统路由),
//     我们将使用该监听器的端口作为源端口
//  2. 如果我们监听一个或多个未指定地址(零地址),
//     我们将从这些监听器中选择一个源端口
//  3. 否则,让系统选择源端口
//
// 参数:
//   - ctx: context.Context 上下文对象
//   - network: string 网络类型
//   - addr: string 目标地址
//
// 返回值:
//   - net.Conn 建立的网络连接
//   - error 可能的错误信息
func (d *dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// 仅当用户监听特定地址(回环或其他)时才检查此情况。
	// 通常,用户会监听"未指定"地址(0.0.0.0或::),
	// 我们可以跳过这部分。
	//
	// 这让我们在大多数情况下避免重复解析地址。
	if len(d.specific) > 0 || len(d.loopback) > 0 {
		tcpAddr, err := net.ResolveTCPAddr(network, addr)
		if err != nil {
			log.Debugf("解析TCP地址失败: %v", err)
			return nil, err
		}
		ip := tcpAddr.IP
		if !ip.IsLoopback() && !ip.IsGlobalUnicast() {
			log.Debugf("无法拨号的IP地址: %s", ip)
			return nil, fmt.Errorf("无法拨号的IP地址: %s", ip)
		}

		// 如果我们正在监听某个特定地址,且该地址恰好是目标地址的首选源地址,
		// 我们尝试使用该地址/端口进行拨号。
		//
		// 如果我们没有监听任何特定地址,则跳过此检查,
		// 因为检查路由表可能很耗时,而用户很少监听特定IP地址。
		if len(d.specific) > 0 {
			if router, err := netroute.New(); err == nil {
				if _, _, preferredSrc, err := router.Route(ip); err == nil {
					for _, optAddr := range d.specific {
						if optAddr.IP.Equal(preferredSrc) {
							return reuseDial(ctx, optAddr, network, addr)
						}
					}
				}
			}
		}

		// 否则,如果我们正在监听回环地址且目标也是回环地址,
		// 使用我们回环监听器的端口。
		if len(d.loopback) > 0 && ip.IsLoopback() {
			return reuseDial(ctx, randAddr(d.loopback), network, addr)
		}
	}

	// 如果我们正在监听任何未指定地址,
	// 从这些监听器中随机选择一个端口使用
	if len(d.unspecified) > 0 {
		return reuseDial(ctx, randAddr(d.unspecified), network, addr)
	}

	// 最后,随机选择一个端口
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, addr)
}

// newDialer 创建一个新的dialer实例
// 参数:
//   - listeners: map[*listener]struct{} 监听器映射
//
// 返回值:
//   - *dialer 新创建的dialer实例
func newDialer(listeners map[*listener]struct{}) *dialer {
	specific := make([]*net.TCPAddr, 0)
	loopback := make([]*net.TCPAddr, 0)
	unspecified := make([]*net.TCPAddr, 0)

	for l := range listeners {
		addr := l.Addr().(*net.TCPAddr)
		if addr.IP.IsLoopback() {
			loopback = append(loopback, addr)
		} else if addr.IP.IsUnspecified() {
			unspecified = append(unspecified, addr)
		} else {
			specific = append(specific, addr)
		}
	}
	return &dialer{
		specific:    specific,
		loopback:    loopback,
		unspecified: unspecified,
	}
}
