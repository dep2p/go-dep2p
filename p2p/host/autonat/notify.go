package autonat

import (
	"github.com/dep2p/go-dep2p/core/network"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// 确保AmbientAutoNAT实现了network.Notifiee接口
var _ network.Notifiee = (*AmbientAutoNAT)(nil)

// Listen 实现network.Notifiee接口的监听方法
// 参数:
//   - net: network.Network 网络实例
//   - a: ma.Multiaddr 监听的多地址
func (as *AmbientAutoNAT) Listen(net network.Network, a ma.Multiaddr) {}

// ListenClose 实现network.Notifiee接口的监听关闭方法
// 参数:
//   - net: network.Network 网络实例
//   - a: ma.Multiaddr 关闭监听的多地址
func (as *AmbientAutoNAT) ListenClose(net network.Network, a ma.Multiaddr) {}

// Connected 实现network.Notifiee接口的连接建立方法
// 参数:
//   - net: network.Network 网络实例
//   - c: network.Conn 建立的连接
func (as *AmbientAutoNAT) Connected(net network.Network, c network.Conn) {
	// 如果是入站连接且远程地址是公网地址
	if c.Stat().Direction == network.DirInbound &&
		manet.IsPublicAddr(c.RemoteMultiaddr()) {
		// 尝试将连接发送到入站连接通道
		select {
		case as.inboundConn <- c: // 发送成功
		default: // 通道已满则丢弃
		}
	}
}

// Disconnected 实现network.Notifiee接口的连接断开方法
// 参数:
//   - net: network.Network 网络实例
//   - c: network.Conn 断开的连接
func (as *AmbientAutoNAT) Disconnected(net network.Network, c network.Conn) {}
