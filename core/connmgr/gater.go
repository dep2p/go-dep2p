package connmgr

import (
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"

	"github.com/dep2p/go-dep2p/core/control"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
)

// ConnectionGater 可以由支持主动入站或出站连接控制的类型实现。
//
// ConnectionGater 是主动的,而 ConnManager 倾向于被动。
//
// 在连接建立/升级生命周期的不同阶段,将会调用 ConnectionGater 进行咨询。
// 在整个过程中会调用特定的函数,允许你在该阶段拦截连接。
//
//	InterceptPeerDial 在即将进行出站对等节点拨号请求时调用,在该对等节点的地址可用/解析之前。
//	在这个阶段阻止连接通常用于黑名单场景。
//
//	InterceptAddrDial 在即将向特定地址的对等节点进行出站拨号时调用。
//	在这个阶段阻止连接通常用于地址过滤。
//
//	InterceptAccept 在传输监听器收到入站连接请求时立即调用,在任何升级发生之前。
//	接受已经安全和/或多路复用的连接的传输(例如可能是 QUIC)必须调用此方法,以确保正确性/一致性。
//
//	InterceptSecured 在完成安全握手并验证对等节点身份后,对入站和出站连接都会调用。
//
//	InterceptUpgraded 在 dep2p 完全将连接升级为安全、多路复用的通道后,
//	对入站和出站连接都会调用。
//
// 此接口可用于实现*严格/主动*的连接管理策略,例如:
// - 达到最大连接数后的硬限制
// - 维护对等节点黑名单
// - 按传输配额限制连接
//
// 实验性功能:未来将支持 DISCONNECT 协议/消息。
// 这允许控制器和其他组件传达连接关闭的意图,以减少潜在的重连尝试。
//
// 目前,InterceptUpgraded 在阻止连接时可以返回非零的 DisconnectReason,
// 但随着我们完善此功能,此接口可能在未来发生变化。
// 只有此方法可以处理 DisconnectReason 的原因是,
// 我们需要流多路复用功能来打开控制协议流以传输消息。
type ConnectionGater interface {
	// InterceptPeerDial 测试是否允许我们拨号指定的对等节点。
	//
	// 当拨号对等节点时,由 network.Network 实现调用。
	InterceptPeerDial(p peer.ID) (allow bool)

	// InterceptAddrDial 测试是否允许我们为给定对等节点拨号指定的多地址。
	//
	// 在 network.Network 实现解析对等节点地址后、拨号每个地址之前调用。
	InterceptAddrDial(peer.ID, ma.Multiaddr) (allow bool)

	// InterceptAccept 测试是否允许新生的入站连接。
	//
	// 由升级器调用,或由传输直接调用(例如 QUIC、蓝牙),
	// 在从其套接字接受连接后立即调用。
	InterceptAccept(network.ConnMultiaddrs) (allow bool)

	// InterceptSecured 测试是否允许给定的已认证连接。
	//
	// 由升级器在执行安全握手之后、协商多路复用器之前调用,
	// 或由传输在完全相同的检查点直接调用。
	InterceptSecured(network.Direction, peer.ID, network.ConnMultiaddrs) (allow bool)

	// InterceptUpgraded 测试是否允许完全功能的连接。
	//
	// 此时已选择多路复用器。
	// 当拒绝连接时,控制器可以返回一个 DisconnectReason。
	// 更多信息请参考 ConnectionGater 类型的 godoc。
	//
	// 注意:go-dep2p 实现目前忽略断开连接原因。
	InterceptUpgraded(network.Conn) (allow bool, reason control.DisconnectReason)
}
