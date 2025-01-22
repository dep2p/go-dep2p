package dep2p

import (
	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/control"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// filtersConnectionGater 是一个适配器，将 multiaddr.Filter 转换为 connmgr.ConnectionGater
type filtersConnectionGater ma.Filters

// 确保 filtersConnectionGater 实现了 connmgr.ConnectionGater 接口
var _ connmgr.ConnectionGater = (*filtersConnectionGater)(nil)

// InterceptAddrDial 拦截地址拨号请求，检查目标地址是否被过滤
// 参数：
//   - _ peer.ID: 目标节点ID(未使用)
//   - addr ma.Multiaddr: 目标多地址
//
// 返回：
//   - bool: 如果地址未被过滤则返回 true，否则返回 false
func (f *filtersConnectionGater) InterceptAddrDial(_ peer.ID, addr ma.Multiaddr) (allow bool) {
	return !(*ma.Filters)(f).AddrBlocked(addr)
}

// InterceptPeerDial 拦截节点拨号请求
// 参数：
//   - p peer.ID: 目标节点ID
//
// 返回：
//   - bool: 始终返回 true，允许所有节点拨号
func (f *filtersConnectionGater) InterceptPeerDial(p peer.ID) (allow bool) {
	return true
}

// InterceptAccept 拦截连接接受请求，检查远程地址是否被过滤
// 参数：
//   - connAddr network.ConnMultiaddrs: 连接的多地址信息
//
// 返回：
//   - bool: 如果远程地址未被过滤则返回 true，否则返回 false
func (f *filtersConnectionGater) InterceptAccept(connAddr network.ConnMultiaddrs) (allow bool) {
	return !(*ma.Filters)(f).AddrBlocked(connAddr.RemoteMultiaddr())
}

// InterceptSecured 拦截已建立安全连接的请求，检查远程地址是否被过滤
// 参数：
//   - _ network.Direction: 连接方向(未使用)
//   - _ peer.ID: 节点ID(未使用)
//   - connAddr network.ConnMultiaddrs: 连接的多地址信息
//
// 返回：
//   - bool: 如果远程地址未被过滤则返回 true，否则返回 false
func (f *filtersConnectionGater) InterceptSecured(_ network.Direction, _ peer.ID, connAddr network.ConnMultiaddrs) (allow bool) {
	return !(*ma.Filters)(f).AddrBlocked(connAddr.RemoteMultiaddr())
}

// InterceptUpgraded 拦截已升级的连接请求
// 参数：
//   - _ network.Conn: 网络连接(未使用)
//
// 返回：
//   - bool: 始终返回 true，允许所有升级的连接
//   - control.DisconnectReason: 断开连接的原因，返回 0 表示无原因
func (f *filtersConnectionGater) InterceptUpgraded(_ network.Conn) (allow bool, reason control.DisconnectReason) {
	return true, 0
}
