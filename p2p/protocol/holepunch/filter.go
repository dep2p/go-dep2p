package holepunch

import (
	"github.com/dep2p/go-dep2p/core/peer"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// WithAddrFilter 是一个 Service 选项,用于启用多地址过滤功能
// 参数:
//   - f: AddrFilter 地址过滤器接口
//
// 返回值:
//   - Option Service 选项函数
//
// 说明:
//   - 允许只向远程对等节点发送观察到的地址的子集
//   - 例如,只宣告 TCP 或 QUIC 多地址而不是两者都宣告
//   - 也允许只考虑远程对等节点向我们宣告的多地址的子集
//   - 理论上,此 API 还允许在两种情况下添加多地址
func WithAddrFilter(f AddrFilter) Option {
	return func(hps *Service) error {
		// 设置服务的地址过滤器
		hps.filter = f
		return nil
	}
}

// AddrFilter 定义了多地址过滤的接口
type AddrFilter interface {
	// FilterLocal 过滤发送给远程对等节点的多地址
	// 参数:
	//   - remoteID: peer.ID 远程节点 ID
	//   - maddrs: []ma.Multiaddr 多地址列表
	//
	// 返回值:
	//   - []ma.Multiaddr 过滤后的多地址列表
	FilterLocal(remoteID peer.ID, maddrs []ma.Multiaddr) []ma.Multiaddr

	// FilterRemote 过滤从远程对等节点接收到的多地址
	// 参数:
	//   - remoteID: peer.ID 远程节点 ID
	//   - maddrs: []ma.Multiaddr 多地址列表
	//
	// 返回值:
	//   - []ma.Multiaddr 过滤后的多地址列表
	FilterRemote(remoteID peer.ID, maddrs []ma.Multiaddr) []ma.Multiaddr
}
