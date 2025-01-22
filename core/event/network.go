package event

import (
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
)

// EvtPeerConnectednessChanged 应在每次与给定对等节点的"连接状态"发生变化时发出。具体来说,在以下情况下会发出此事件:
//
//   - Connectedness = Connected: 每当我们从与对等节点没有连接过渡到至少有一个连接时。
//   - Connectedness = NotConnected: 每当我们从与对等节点至少有一个连接过渡到没有连接时。
//
// 未来可能会添加其他连接状态。此列表不应被视为详尽无遗。
//
// 需要注意:
//
//   - 可以与给定对等节点建立多个连接。
//   - dep2p 和网络都是异步的。
//
// 这意味着以下所有情况都是可能的:
//
// 连接断开并重新建立:
//
//   - 对等节点 A 观察到从 Connected -> NotConnected -> Connected 的转换
//   - 对等节点 B 观察到从 Connected -> NotConnected -> Connected 的转换
//
// 解释: 两个对等节点都观察到连接断开。这是"理想"情况。
//
// 连接断开并重新建立:
//
//   - 对等节点 A 观察到从 Connected -> NotConnected -> Connected 的转换。
//   - 对等节点 B 没有观察到转换。
//
// 解释: 对等节点 A 重新建立了断开的连接。
// 对等节点 B 在观察到旧连接断开之前就观察到新连接形成。
//
// 连接断开:
//
//   - 对等节点 A 没有观察到转换。
//   - 对等节点 B 没有观察到转换。
//
// 解释: 原本有两个连接,其中一个断开了。
// 这个连接可能正在被使用,但两个对等节点都不会观察到"连接状态"的变化。
// 对等节点应始终确保重试网络请求。
type EvtPeerConnectednessChanged struct {
	// Peer 是连接状态发生变化的远程对等节点。
	Peer peer.ID
	// Connectedness 是新的连接状态。
	Connectedness network.Connectedness
}
