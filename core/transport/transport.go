// Package transport 提供了 Transport 接口，它代表用于发送和接收数据的设备和网络协议。
package transport

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// CapableConn 表示一个提供 dep2p 所需基本功能的连接:流多路复用、加密和对等节点认证。
//
// 这些功能可以由传输层直接提供，也可以通过"连接升级"过程来实现，该过程通过添加加密通道和流多路复用器将"原始"网络连接转换为支持这些功能的连接。
//
// CapableConn 提供了访问用于建立连接的本地和远程多地址的方法，以及访问底层 Transport 的方法。
type CapableConn interface {
	network.MuxedConn
	network.ConnSecurity
	network.ConnMultiaddrs
	network.ConnScoper

	// Transport 返回此连接所属的传输层。
	Transport() Transport
}

// Transport 表示可以用来连接其他对等节点并接受其他对等节点连接的任何设备。
//
// Transport 接口允许你通过拨号连接其他对等节点，也允许你监听入站连接。
//
// Dial 返回的连接和传递给 Listeners 的连接都是 CapableConn 类型，这意味着它们已经升级为支持流多路复用和连接安全(加密和认证)。
//
// 如果传输层实现了 `io.Closer`(可选)，dep2p 将在关闭时调用 `Close`。注意：`Dial` 和 `Listen` 可能在 `Close` 之后或与 `Close` 并发调用。
//
// 除了 Transport 接口外，传输层还可以实现 Resolver 或 SkipResolver 接口。在包装/嵌入传输层时，你应该确保正确处理 Resolver/SkipResolver 接口。
type Transport interface {
	// Dial 拨号连接远程对等节点。它应该尽可能重用本地监听器地址，但也可以选择不重用。
	Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (CapableConn, error)

	// CanDial 如果此传输层知道如何拨号给定的多地址，则返回 true。
	//
	// 返回 true 并不保证拨号此多地址一定会成功。
	// 此函数*仅*应用于预先过滤掉我们无法拨号的地址。
	CanDial(addr ma.Multiaddr) bool

	// Listen 在传入的多地址上监听。
	Listen(laddr ma.Multiaddr) (Listener, error)

	// Protocol 返回此传输层处理的协议集。
	//
	// 有关如何使用的说明，请参见 Network 接口。
	Protocols() []int

	// Proxy 如果这是代理传输层则返回 true。
	//
	// 有关如何使用的说明，请参见 Network 接口。
	// TODO: 是否应该将其作为 go-multiaddr 协议的一部分？
	Proxy() bool
}

// Resolver 可以由想要解析或转换多地址的传输层选择性实现。
type Resolver interface {
	Resolve(ctx context.Context, maddr ma.Multiaddr) ([]ma.Multiaddr, error)
}

// SkipResolver 可以由不想解析或转换多地址的传输层选择性实现。
// 对于间接包装其他传输层的传输层(例如 p2p-circuit)很有用。
// 这让内部传输层可以指定稍后如何解析多地址。
type SkipResolver interface {
	SkipResolve(ctx context.Context, maddr ma.Multiaddr) bool
}

// Listener 是一个与 net.Listener 接口非常相似的接口。
// 唯一的实际区别是 Accept() 返回此包中类型的 Conn，并且暴露 Multiaddr 方法而不是常规的 Addr 方法
type Listener interface {
	// Accept 等待并返回下一个连接
	// 返回值:
	//   - CapableConn: 新的连接
	//   - error: 如果监听器已关闭,返回 ErrListenerClosed
	Accept() (CapableConn, error)

	// Close 关闭监听器
	// 返回值:
	//   - error: 如果关闭过程中发生错误,返回错误信息
	Close() error

	// Addr 返回监听器的底层网络地址
	// 返回值:
	//   - net.Addr: 底层网络地址
	Addr() net.Addr

	// Multiaddr 返回监听器的多地址
	// 返回值:
	//   - ma.Multiaddr: 监听器的多地址
	Multiaddr() ma.Multiaddr
}

// ErrListenerClosed 在监听器优雅关闭时由 Listener.Accept 返回。
var ErrListenerClosed = errors.New("监听器已关闭")

// TransportNetwork 是一个带有管理传输层方法的 inet.Network。
type TransportNetwork interface {
	network.Network

	// AddTransport 向此网络添加传输层。
	//
	// 在拨号时，此网络将遍历远程多地址中的协议，并选择第一个注册了代理传输层的协议(如果有)。
	// 否则，它将选择注册用于处理多地址中最后一个协议的传输层。
	//
	// 在监听时，此网络将遍历本地多地址中的协议，并选择*最后*一个注册了代理传输层的协议(如果有)。
	// 否则，它将选择注册用于处理多地址中最后一个协议的传输层。
	AddTransport(t Transport) error
}

// Upgrader 是一个多流升级器，可以将底层连接升级为完整的传输层连接(安全且多路复用)。
type Upgrader interface {
	// UpgradeListener 将传入的多地址网络监听器升级为完整的 dep2p 传输层监听器。
	UpgradeListener(Transport, manet.Listener) Listener
	// Upgrade 将多地址/网络连接升级为完整的 dep2p 传输层连接。
	Upgrade(ctx context.Context, t Transport, maconn manet.Conn, dir network.Direction, p peer.ID, scope network.ConnManagementScope) (CapableConn, error)
}

// DialUpdater 提供正在进行的拨号的更新。
type DialUpdater interface {
	// DialWithUpdates 拨号远程对等节点并在传入的通道上提供更新。
	DialWithUpdates(context.Context, ma.Multiaddr, peer.ID, chan<- DialUpdate) (CapableConn, error)
}

// DialUpdateKind 表示 DialUpdate 事件的类型。
type DialUpdateKind int

const (
	// UpdateKindDialFailed 表示拨号失败。
	UpdateKindDialFailed DialUpdateKind = iota
	// UpdateKindDialSuccessful 表示拨号成功。
	UpdateKindDialSuccessful
	// UpdateKindHandshakeProgressed 表示 TCP 三次握手成功完成
	UpdateKindHandshakeProgressed
)

func (k DialUpdateKind) String() string {
	switch k {
	case UpdateKindDialFailed:
		return "拨号失败"
	case UpdateKindDialSuccessful:
		return "拨号成功"
	case UpdateKindHandshakeProgressed:
		return "握手进行中"
	default:
		return fmt.Sprintf("拨号更新类型<未知-%d>", k)
	}
}

// DialUpdate 由 DialUpdater 用于提供拨号更新。
type DialUpdate struct {
	// Kind 是更新事件的类型。
	Kind DialUpdateKind
	// Addr 是对等节点的地址。
	Addr ma.Multiaddr
	// Conn 是成功时的结果连接。
	Conn CapableConn
	// Err 是拨号失败的原因。
	Err error
}
