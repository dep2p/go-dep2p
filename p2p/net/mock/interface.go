// Package mocknet 提供了一个用于测试的模拟网络 net.Network。
//
// - 一个 Mocknet 包含多个 network.Networks
// - 一个 Mocknet 包含多个 Links
// - 一个 Link 连接两个 network.Networks
// - network.Conns 和 network.Streams 由 network.Networks 创建
package mocknet

import (
	"io"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// PeerOptions 定义了添加对等点时的配置选项
type PeerOptions struct {
	// ps 是添加对等点时使用的 Peerstore
	// 如果为 nil,将创建一个默认的 peerstore
	ps peerstore.Peerstore

	// gater 是添加对等点时使用的连接过滤器
	// 如果为 nil,将不使用连接过滤器
	gater connmgr.ConnectionGater
}

// Mocknet 定义了一个模拟网络的接口
type Mocknet interface {
	// GenPeer 在 Mocknet 中生成一个对等点及其网络
	// 返回:
	//   - host.Host: 生成的主机
	//   - error: 生成过程中的错误
	GenPeer() (host.Host, error)

	// GenPeerWithOptions 使用指定选项生成对等点
	// 参数:
	//   - opts: 对等点配置选项
	// 返回:
	//   - host.Host: 生成的主机
	//   - error: 生成过程中的错误
	GenPeerWithOptions(PeerOptions) (host.Host, error)

	// AddPeer 添加一个现有的对等点
	// 参数:
	//   - pk: 对等点的私钥
	//   - addr: 对等点的多地址
	// 返回:
	//   - host.Host: 添加的主机
	//   - error: 添加过程中的错误
	AddPeer(ic.PrivKey, ma.Multiaddr) (host.Host, error)

	// AddPeerWithPeerstore 使用指定的 peerstore 添加对等点
	// 参数:
	//   - id: 对等点ID
	//   - ps: peerstore 实例
	// 返回:
	//   - host.Host: 添加的主机
	//   - error: 添加过程中的错误
	AddPeerWithPeerstore(peer.ID, peerstore.Peerstore) (host.Host, error)

	// AddPeerWithOptions 使用指定选项添加对等点
	// 参数:
	//   - id: 对等点ID
	//   - opts: 对等点配置选项
	// 返回:
	//   - host.Host: 添加的主机
	//   - error: 添加过程中的错误
	AddPeerWithOptions(peer.ID, PeerOptions) (host.Host, error)

	// Peers 返回所有对等点ID(以随机顺序)
	Peers() []peer.ID

	// Net 返回指定对等点的网络
	// 参数:
	//   - pid: 对等点ID
	Net(peer.ID) network.Network

	// Nets 返回所有网络
	Nets() []network.Network

	// Host 返回指定对等点的主机
	// 参数:
	//   - pid: 对等点ID
	Host(peer.ID) host.Host

	// Hosts 返回所有主机
	Hosts() []host.Host

	// Links 返回所有链路映射
	Links() LinkMap

	// LinksBetweenPeers 返回两个对等点之间的所有链路
	// 参数:
	//   - a,b: 两个对等点的ID
	LinksBetweenPeers(a, b peer.ID) []Link

	// LinksBetweenNets 返回两个网络之间的所有链路
	// 参数:
	//   - a,b: 两个网络实例
	LinksBetweenNets(a, b network.Network) []Link

	// LinkPeers 在两个对等点之间创建链路
	// 参数:
	//   - a,b: 两个对等点的ID
	// 返回:
	//   - Link: 创建的链路
	//   - error: 创建过程中的错误
	LinkPeers(peer.ID, peer.ID) (Link, error)

	// LinkNets 在两个网络之间创建链路
	// 参数:
	//   - a,b: 两个网络实例
	// 返回:
	//   - Link: 创建的链路
	//   - error: 创建过程中的错误
	LinkNets(network.Network, network.Network) (Link, error)

	// Unlink 移除指定链路
	// 参数:
	//   - l: 要移除的链路
	Unlink(Link) error

	// UnlinkPeers 移除两个对等点之间的链路
	// 参数:
	//   - a,b: 两个对等点的ID
	UnlinkPeers(peer.ID, peer.ID) error

	// UnlinkNets 移除两个网络之间的链路
	// 参数:
	//   - a,b: 两个网络实例
	UnlinkNets(network.Network, network.Network) error

	// SetLinkDefaults 设置链路的默认选项
	// 参数:
	//   - opts: 链路默认选项
	SetLinkDefaults(LinkOptions)

	// LinkDefaults 返回链路的默认选项
	LinkDefaults() LinkOptions

	// ConnectPeers 连接两个对等点
	// 参数:
	//   - a,b: 两个对等点的ID
	// 返回:
	//   - network.Conn: 建立的连接
	//   - error: 连接过程中的错误
	ConnectPeers(peer.ID, peer.ID) (network.Conn, error)

	// ConnectNets 连接两个网络
	// 参数:
	//   - a,b: 两个网络实例
	// 返回:
	//   - network.Conn: 建立的连接
	//   - error: 连接过程中的错误
	ConnectNets(network.Network, network.Network) (network.Conn, error)

	// DisconnectPeers 断开两个对等点的连接
	// 参数:
	//   - a,b: 两个对等点的ID
	DisconnectPeers(peer.ID, peer.ID) error

	// DisconnectNets 断开两个网络的连接
	// 参数:
	//   - a,b: 两个网络实例
	DisconnectNets(network.Network, network.Network) error

	// LinkAll 创建所有可能的链路
	LinkAll() error

	// ConnectAllButSelf 连接除自身外的所有对等点
	ConnectAllButSelf() error

	// Close 关闭模拟网络
	io.Closer
}

// LinkOptions 定义了链路的配置选项
// LinkOptions 用于更改链路的各个方面。
// 抱歉，但它们目前还不能工作 :(
type LinkOptions struct {
	// Latency 定义链路延迟
	Latency time.Duration

	// Bandwidth 定义链路带宽(字节/秒)
	Bandwidth float64
	// 我们以后可以将这些值做成分布式的。
}

// Link 定义了对等点之间的物理链路接口
// Link 表示两个对等点之间连接的**可能性**。
// 将其视为物理网络链路。
// 没有它们，对等点可以一直尝试但无法连接。
// 这允许构建特定节点无法直接相互通信的拓扑结构。:)
type Link interface {
	// Networks 返回链路连接的网络
	Networks() []network.Network

	// Peers 返回链路连接的对等点
	Peers() []peer.ID

	// SetOptions 设置链路选项
	// 参数:
	//   - opts: 链路选项
	SetOptions(LinkOptions)

	// Options 返回链路选项
	Options() LinkOptions
}

// LinkMap 是一个三维映射,用于跟踪链路
// （哇，好多映射。这样的数据结构。如何组合。啊指针）

type LinkMap map[string]map[string]map[Link]struct{}

// Printer 定义了用于检查和打印网络内容的接口
// Printer 让你检查内容 :)
type Printer interface {
	// MocknetLinks 打印整个模拟网络的链路表
	// MocknetLinks 显示整个 Mocknet 的链路表 :)
	// 参数:
	//   - mn: 模拟网络实例
	MocknetLinks(mn Mocknet)

	// NetworkConns 打印网络的连接信息
	// 参数:
	//   - ni: 网络实例
	NetworkConns(ni network.Network)
}

// PrinterTo 返回一个写入指定 io.Writer 的打印器
// 参数:
//   - w: 输出写入器
//
// 返回:
//   - Printer: 打印器实例
func PrinterTo(w io.Writer) Printer {
	return &printer{w}
}
