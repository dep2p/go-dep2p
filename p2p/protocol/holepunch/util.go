package holepunch

import (
	"context"
	"slices"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// removeRelayAddrs 移除中继地址
// 参数:
//   - addrs: []ma.Multiaddr 多地址列表
//
// 返回值:
//   - []ma.Multiaddr 移除中继地址后的地址列表
func removeRelayAddrs(addrs []ma.Multiaddr) []ma.Multiaddr {
	// 删除中继地址
	return slices.DeleteFunc(addrs, isRelayAddress)
}

// isRelayAddress 判断是否为中继地址
// 参数:
//   - a: ma.Multiaddr 待检查的多地址
//
// 返回值:
//   - bool 是否为中继地址
func isRelayAddress(a ma.Multiaddr) bool {
	// 检查是否包含中继协议
	_, err := a.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

// addrsToBytes 将多地址列表转换为字节数组列表
// 参数:
//   - as: []ma.Multiaddr 多地址列表
//
// 返回值:
//   - [][]byte 字节数组列表
func addrsToBytes(as []ma.Multiaddr) [][]byte {
	// 创建字节数组切片
	bzs := make([][]byte, 0, len(as))
	// 遍历地址列表并转换为字节数组
	for _, a := range as {
		bzs = append(bzs, a.Bytes())
	}
	return bzs
}

// addrsFromBytes 将字节数组列表转换为多地址列表
// 参数:
//   - bzs: [][]byte 字节数组列表
//
// 返回值:
//   - []ma.Multiaddr 多地址列表
func addrsFromBytes(bzs [][]byte) []ma.Multiaddr {
	// 创建多地址切片
	addrs := make([]ma.Multiaddr, 0, len(bzs))
	// 遍历字节数组并转换为多地址
	for _, bz := range bzs {
		a, err := ma.NewMultiaddrBytes(bz)
		if err == nil {
			addrs = append(addrs, a)
		}
	}
	return addrs
}

// getDirectConnection 获取与指定节点的直连连接
// 参数:
//   - h: host.Host 本地节点
//   - p: peer.ID 目标节点ID
//
// 返回值:
//   - network.Conn 直连连接,如果不存在则返回nil
func getDirectConnection(h host.Host, p peer.ID) network.Conn {
	// 遍历与目标节点的所有连接
	for _, c := range h.Network().ConnsToPeer(p) {
		// 返回第一个非中继连接
		if !isRelayAddress(c.RemoteMultiaddr()) {
			return c
		}
	}
	return nil
}

// holePunchConnect 尝试与目标节点建立打洞连接
// 参数:
//   - ctx: context.Context 上下文
//   - host: host.Host 本地节点
//   - pi: peer.AddrInfo 目标节点信息
//   - isClient: bool 是否为客户端
//
// 返回值:
//   - error 连接错误,成功则返回nil
func holePunchConnect(ctx context.Context, host host.Host, pi peer.AddrInfo, isClient bool) error {
	// 创建支持同时连接的上下文
	holePunchCtx := network.WithSimultaneousConnect(ctx, isClient, "hole-punching")
	// 创建强制直连的上下文
	forceDirectConnCtx := network.WithForceDirectDial(holePunchCtx, "hole-punching")
	// 创建带超时的上下文
	dialCtx, cancel := context.WithTimeout(forceDirectConnCtx, dialTimeout)
	defer cancel()

	// 尝试连接
	if err := host.Connect(dialCtx, pi); err != nil {
		log.Debugf("打洞尝试失败", "peer ID", pi.ID, "error", err)
		return err
	}
	log.Debugw("打洞成功", "peer", pi.ID)
	return nil
}
