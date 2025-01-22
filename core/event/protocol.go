package event

import (
	peer "github.com/dep2p/go-dep2p/core/peer"
	protocol "github.com/dep2p/go-dep2p/core/protocol"
)

// EvtPeerProtocolsUpdated 应在我们连接的对等节点添加或移除其协议栈中的协议时发出。
type EvtPeerProtocolsUpdated struct {
	// Peer 是协议发生更新的对等节点。
	Peer peer.ID
	// Added 列举了该对等节点添加的协议。
	Added []protocol.ID
	// Removed 列举了该对等节点移除的协议。
	Removed []protocol.ID
}

// EvtLocalProtocolsUpdated 应在本地主机添加或移除流处理器时发出。
// 对于使用匹配谓词(host.SetStreamHandlerMatch())附加的处理器,此事件中仅包含协议ID。
type EvtLocalProtocolsUpdated struct {
	// Added 列举了本地添加的协议。
	Added []protocol.ID
	// Removed 列举了本地移除的协议。
	Removed []protocol.ID
}
