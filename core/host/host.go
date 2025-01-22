// Package host 提供了 dep2p 的核心 Host 接口。
//
// Host 表示对等网络中的单个 dep2p 节点。
package host

import (
	"context"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// Host 是一个参与p2p网络的对象,它实现了协议或提供服务。
// 它像Server一样处理请求,像Client一样发出请求。
// 它被称为Host,因为它既是Server又是Client(Peer可能令人困惑)。
type Host interface {
	// ID 返回与此主机关联的(本地)对等节点ID。
	ID() peer.ID

	// Peerstore 返回此主机的对等节点地址和密钥的存储库。
	Peerstore() peerstore.Peerstore

	// Addrs 返回此主机的监听地址。
	Addrs() []ma.Multiaddr

	// Network 返回此主机的网络接口。
	Network() network.Network

	// Mux 返回此主机的流多路复用器。
	Mux() protocol.Switch

	// Connect 确保此主机与给定对等节点ID的对等节点之间存在连接。
	// Connect 将pi中的地址吸收到其内部对等节点存储库中。
	// 如果没有活动的连接,Connect将发出h.Network.Dial,并阻塞直到连接打开或返回错误。
	Connect(ctx context.Context, pi peer.AddrInfo) error

	// SetStreamHandler 设置主机的流处理器。
	// 这等同于:
	//   host.Mux().SetHandler(proto, handler)
	// (Thread-safe)
	SetStreamHandler(pid protocol.ID, handler network.StreamHandler)

	// SetStreamHandlerMatch 设置主机的流处理器,使用匹配函数进行协议选择。
	SetStreamHandlerMatch(protocol.ID, func(protocol.ID) bool, network.StreamHandler)

	// RemoveStreamHandler 移除主机的流处理器。
	RemoveStreamHandler(pid protocol.ID)

	// NewStream 打开到给定对等节点p的新流,并写入带有给定ProtocolID的p2p/protocol头。
	// 如果没有到p的连接,尝试创建一个。如果ProtocolID为空,则不写头。
	// (Thread-safe)
	NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error)

	// Close 关闭主机、其网络和服务的连接。
	Close() error

	// ConnManager 返回此主机的连接管理器。
	ConnManager() connmgr.ConnManager

	// EventBus 返回此主机的EventBus。
	EventBus() event.Bus
}
