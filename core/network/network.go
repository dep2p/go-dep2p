// Package network 提供了dep2p的核心网络抽象层。
//
// network包提供了与其他dep2p节点交互的高级Network接口,这是用于发起和接受远程节点连接的主要公共API。
package network

import (
	"context"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// MessageSizeMax 是网络消息的软限制(建议)最大值。
// 由于接口是流式的,可以写入更多数据。
// 但当整个消息是单个大的序列化对象时,建议将其分成多个读/写操作。
const MessageSizeMax = 1 << 22 // 4 MB

// Direction 表示在流中哪个节点发起了连接。
type Direction int

const (
	// DirUnknown 是默认的方向。
	DirUnknown Direction = iota
	// DirInbound 表示远程节点发起了连接。
	DirInbound
	// DirOutbound 表示本地节点发起了连接。
	DirOutbound
)

// unrecognized 表示未识别的方向。
const unrecognized = "(unrecognized)"

// String 返回Direction的字符串表示。
//
// 返回值：
//   - string：Direction的字符串描述
func (d Direction) String() string {
	str := [...]string{"Unknown", "Inbound", "Outbound"}
	if d < 0 || int(d) >= len(str) {
		return unrecognized
	}
	return str[d]
}

// Connectedness 表示与给定节点的连接能力。
// 用于向服务和其他节点发出节点是否可达的信号。
type Connectedness int

const (
	// NotConnected 表示没有与节点的连接,也没有额外信息(默认)
	NotConnected Connectedness = iota

	// Connected 表示与节点有开放的、活跃的连接
	Connected

	// Deprecated: CanConnect 已弃用,将在未来版本中移除
	//
	// CanConnect 表示最近与节点连接过,正常终止
	CanConnect

	// Deprecated: CannotConnect 已弃用,将在未来版本中移除
	//
	// CannotConnect 表示最近尝试连接但失败
	// (表示"尝试过但失败了")
	CannotConnect

	// Limited 表示与节点有临时连接,但未完全连接
	Limited
)

// String 返回Connectedness的字符串表示。
//
// 返回值：
//   - string：Connectedness的字符串描述
func (c Connectedness) String() string {
	str := [...]string{"NotConnected", "Connected", "CanConnect", "CannotConnect", "Limited"}
	if c < 0 || int(c) >= len(str) {
		return unrecognized
	}
	return str[c]
}

// Reachability 表示节点的可达性状态。
type Reachability int

const (
	// ReachabilityUnknown 表示节点的可达性状态未知。
	ReachabilityUnknown Reachability = iota

	// ReachabilityPublic 表示节点可以从公共互联网访问。
	ReachabilityPublic

	// ReachabilityPrivate 表示节点不能从公共互联网访问。
	//
	// 注意:此节点仍可能通过中继可达。
	ReachabilityPrivate
)

// String 返回Reachability的字符串表示。
//
// 返回值：
//   - string：Reachability的字符串描述
func (r Reachability) String() string {
	str := [...]string{"Unknown", "Public", "Private"}
	if r < 0 || int(r) >= len(str) {
		return unrecognized
	}
	return str[r]
}

// ConnStats 存储与给定Conn相关的元数据。
type ConnStats struct {
	Stats
	// NumStreams 是连接上的流数量。
	NumStreams int
}

// Stats 存储与给定Stream/Conn相关的元数据。
type Stats struct {
	// Direction 指定这是入站还是出站连接。
	Direction Direction
	// Opened 是此连接打开的时间戳。
	Opened time.Time
	// Limited 表示此连接是受限的。
	// 可能受字节或时间限制。实际上,这是通过circuit v2中继形成的连接。
	Limited bool
	// Extra 存储关于此连接的额外元数据。
	Extra map[interface{}]interface{}
}

// StreamHandler 是用于监听远程端打开的流的函数类型。
type StreamHandler func(Stream)

// Network 是用于连接外部世界的接口。
// 它用于拨号和监听连接。它使用Swarm来池化连接(参见swarm包和peerstream.Swarm)。
// 连接使用类TLS协议加密。
type Network interface {
	Dialer
	io.Closer

	// SetStreamHandler 设置远程端打开新流时的处理程序。此操作是线程安全的。
	//
	// 参数：
	//   - handler：StreamHandler 处理新流的函数
	SetStreamHandler(StreamHandler)

	// NewStream 返回到给定节点p的新流。
	// 如果没有到p的连接,则尝试创建一个。
	//
	// 参数：
	//   - ctx：context.Context 上下文
	//   - p：peer.ID 目标节点ID
	//
	// 返回值：
	//   - Stream：新创建的流
	//   - error：错误信息
	NewStream(context.Context, peer.ID) (Stream, error)

	// Listen 告诉网络开始在给定的多地址上监听。
	//
	// 参数：
	//   - addrs：...ma.Multiaddr 要监听的多地址列表
	//
	// 返回值：
	//   - error：错误信息
	Listen(...ma.Multiaddr) error

	// ListenAddresses 返回此网络监听的地址列表。
	//
	// 返回值：
	//   - []ma.Multiaddr：监听地址列表
	ListenAddresses() []ma.Multiaddr

	// InterfaceListenAddresses 返回此网络监听的地址列表。
	// 它展开"任意接口"地址(/ip4/0.0.0.0, /ip6/::)以使用已知的本地接口。
	//
	// 返回值：
	//   - []ma.Multiaddr：接口监听地址列表
	//   - error：错误信息
	InterfaceListenAddresses() ([]ma.Multiaddr, error)

	// ResourceManager 返回与此网络关联的ResourceManager
	//
	// 返回值：
	//   - ResourceManager：资源管理器
	ResourceManager() ResourceManager
}

// MultiaddrDNSResolver 定义了DNS解析相关的接口。
type MultiaddrDNSResolver interface {
	// ResolveDNSAddr 解析多地址中的第一个/dnsaddr组件。
	// 递归解析DNSADDRs直到达到递归限制
	//
	// 参数：
	//   - ctx：context.Context 上下文
	//   - expectedPeerID：peer.ID 预期的节点ID
	//   - maddr：ma.Multiaddr 要解析的多地址
	//   - recursionLimit：int 递归限制
	//   - outputLimit：int 输出限制
	//
	// 返回值：
	//   - []ma.Multiaddr：解析后的多地址列表
	//   - error：错误信息
	ResolveDNSAddr(ctx context.Context, expectedPeerID peer.ID, maddr ma.Multiaddr, recursionLimit, outputLimit int) ([]ma.Multiaddr, error)

	// ResolveDNSComponent 解析多地址中的第一个/{dns,dns4,dns6}组件。
	//
	// 参数：
	//   - ctx：context.Context 上下文
	//   - maddr：ma.Multiaddr 要解析的多地址
	//   - outputLimit：int 输出限制
	//
	// 返回值：
	//   - []ma.Multiaddr：解析后的多地址列表
	//   - error：错误信息
	ResolveDNSComponent(ctx context.Context, maddr ma.Multiaddr, outputLimit int) ([]ma.Multiaddr, error)
}

// Dialer 表示可以向节点拨号的服务
// (这通常就是一个Network,但其他服务可能不需要整个堆栈,因此更容易模拟)
type Dialer interface {
	// Peerstore 返回内部节点存储。这对于告诉拨号器节点的新地址很有用。
	// 或使用在网络上发现的公钥之一。
	//
	// 返回值：
	//   - peerstore.Peerstore：节点存储
	Peerstore() peerstore.Peerstore

	// LocalPeer 返回与此网络关联的本地节点
	//
	// 返回值：
	//   - peer.ID：本地节点ID
	LocalPeer() peer.ID

	// DialPeer 建立与给定节点的连接
	//
	// 参数：
	//   - ctx：context.Context 上下文
	//   - p：peer.ID 目标节点ID
	//
	// 返回值：
	//   - Conn：建立的连接
	//   - error：错误信息
	DialPeer(context.Context, peer.ID) (Conn, error)

	// ClosePeer 关闭与给定节点的连接
	//
	// 参数：
	//   - p：peer.ID 要关闭连接的节点ID
	//
	// 返回值：
	//   - error：错误信息
	ClosePeer(peer.ID) error

	// Connectedness 返回表示连接能力的状态
	//
	// 参数：
	//   - p：peer.ID 要检查连接状态的节点ID
	//
	// 返回值：
	//   - Connectedness：连接状态
	Connectedness(peer.ID) Connectedness

	// Peers 返回已连接的节点
	//
	// 返回值：
	//   - []peer.ID：已连接节点ID列表
	Peers() []peer.ID

	// Conns 返回此Network中的连接
	//
	// 返回值：
	//   - []Conn：连接列表
	Conns() []Conn

	// ConnsToPeer 返回此Network中到给定节点的连接。
	//
	// 参数：
	//   - p：peer.ID 目标节点ID
	//
	// 返回值：
	//   - []Conn：到目标节点的连接列表
	ConnsToPeer(p peer.ID) []Conn

	// Notify/StopNotify 注册和注销信号的通知接收者
	//
	// 参数：
	//   - notifiee：Notifiee 通知接收者
	Notify(Notifiee)
	StopNotify(Notifiee)

	// CanDial 返回拨号器是否可以在addr地址拨号节点p
	//
	// 参数：
	//   - p：peer.ID 目标节点ID
	//   - addr：ma.Multiaddr 目标地址
	//
	// 返回值：
	//   - bool：是否可以拨号
	CanDial(p peer.ID, addr ma.Multiaddr) bool
}

// AddrDelay 提供一个地址以及在该地址应该被拨号之前的延迟
type AddrDelay struct {
	// Addr 是要拨号的地址
	Addr ma.Multiaddr
	// Delay 是拨号前的延迟时间
	Delay time.Duration
}

// DialRanker 提供拨号提供的地址的调度
//
// 参数：
//   - addrs：[]ma.Multiaddr 要调度的地址列表
//
// 返回值：
//   - []AddrDelay：带延迟的地址列表
type DialRanker func([]ma.Multiaddr) []AddrDelay
