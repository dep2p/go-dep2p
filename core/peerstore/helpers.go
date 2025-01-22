package peerstore

import (
	"github.com/dep2p/go-dep2p/core/peer"
)

// AddrInfos 为每个指定的对等点ID按顺序返回一个AddrInfo
// 参数：
//   - ps: Peerstore 对等点存储接口，用于获取对等点信息
//   - peers: []peer.ID 对等点ID列表，需要获取信息的对等点集合
//
// 返回值：
//   - []peer.AddrInfo: 包含所有指定对等点信息的AddrInfo切片
func AddrInfos(ps Peerstore, peers []peer.ID) []peer.AddrInfo {
	// 创建一个与输入peers长度相同的AddrInfo切片
	pi := make([]peer.AddrInfo, len(peers))
	// 遍历peers切片，获取每个对等点的信息
	for i, p := range peers {
		// 通过Peerstore接口获取对等点信息并存储到切片中
		pi[i] = ps.PeerInfo(p)
	}
	// 返回包含所有对等点信息的切片
	return pi
}
