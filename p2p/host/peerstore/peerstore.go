package peerstore

import (
	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"
)

// PeerInfos 获取对等节点的地址信息列表
// 参数:
//   - ps: 对等节点存储接口
//   - peers: 对等节点ID切片
//
// 返回:
//   - []peer.AddrInfo: 对等节点地址信息列表
func PeerInfos(ps pstore.Peerstore, peers peer.IDSlice) []peer.AddrInfo {
	pi := make([]peer.AddrInfo, len(peers)) // 创建地址信息切片
	for i, p := range peers {               // 遍历对等节点ID
		pi[i] = ps.PeerInfo(p) // 获取每个节点的地址信息
	}
	return pi // 返回地址信息列表
}

// PeerInfoIDs 从地址信息列表中提取对等节点ID列表
// 参数:
//   - pis: 对等节点地址信息列表
//
// 返回:
//   - peer.IDSlice: 对等节点ID切片
func PeerInfoIDs(pis []peer.AddrInfo) peer.IDSlice {
	ps := make(peer.IDSlice, len(pis)) // 创建节点ID切片
	for i, pi := range pis {           // 遍历地址信息列表
		ps[i] = pi.ID // 提取每个地址信息中的节点ID
	}
	return ps // 返回节点ID列表
}
