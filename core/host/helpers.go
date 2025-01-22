package host

import "github.com/dep2p/go-dep2p/core/peer"

// InfoFromHost 从主机获取对等节点地址信息。
//
// 参数:
//   - h: 主机实例,用于获取其ID和监听地址。
//
// 返回值:
//   - *peer.AddrInfo: 包含主机ID和所有监听地址的对等节点地址信息结构体。
func InfoFromHost(h Host) *peer.AddrInfo {
	// 创建并返回一个新的 AddrInfo 结构体
	return &peer.AddrInfo{
		ID:    h.ID(),    // 设置主机的对等节点ID
		Addrs: h.Addrs(), // 设置主机的所有监听地址
	}
}
