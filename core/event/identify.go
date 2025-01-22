package event

import (
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/record"
	"github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// EvtPeerIdentificationCompleted 在对等节点的初始身份识别完成时发出。
type EvtPeerIdentificationCompleted struct {
	// Peer 是身份识别成功的对等节点的ID。
	Peer peer.ID

	// Conn 是我们识别的连接。
	Conn network.Conn

	// ListenAddrs 是对等节点正在监听的地址列表。
	ListenAddrs []multiaddr.Multiaddr

	// Protocols 是对等节点在此连接上公布的协议列表。
	Protocols []protocol.ID

	// SignedPeerRecord 是对等节点提供的已签名对等记录。可能为nil。
	SignedPeerRecord *record.Envelope

	// AgentVersion 类似于浏览器中的UserAgent字符串,或者BitTorrent中的客户端版本,
	// 包含客户端名称和客户端信息。
	AgentVersion string

	// ProtocolVersion 是identify消息中的协议版本字段。
	ProtocolVersion string

	// ObservedAddr 是对等节点观察到的我方连接地址。
	// 这未经验证,对等节点可能在此返回任何内容。
	ObservedAddr multiaddr.Multiaddr
}

// EvtPeerIdentificationFailed 在对等节点的初始身份识别失败时发出。
type EvtPeerIdentificationFailed struct {
	// Peer 是身份识别失败的对等节点的ID。
	Peer peer.ID
	// Reason 是身份识别失败的原因。
	Reason error
}
