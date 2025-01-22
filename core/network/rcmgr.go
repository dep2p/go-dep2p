package network

import (
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// ResourceManager 是网络资源管理子系统的接口。
// 它跟踪和记录资源使用情况,从内部到应用程序,并提供一种机制来根据用户配置的策略限制资源使用。
//
// 资源管理基于资源管理范围的概念,其中资源使用受限于一个范围的 DAG,
// 以下图示说明了资源约束 DAG 的结构:
// System
//
//	+------------> Transient.............+................+
//	|                                    .                .
//	+------------>  Service------------- . ----------+    .
//	|                                    .           |    .
//	+------------->  Protocol----------- . ----------+    .
//	|                                    .           |    .
//	+-------------->  Peer               \           |    .
//	                   +------------> Connection     |    .
//	                   |                             \    \
//	                   +--------------------------->  Stream
//
// ResourceManager 跟踪的基本资源包括内存、流、连接和文件描述符。
// 这些资源直接影响系统的可用性和性能。
//
// ResourceManager 的工作方式是在资源预留时限制资源使用。当系统组件需要使用资源时,
// 它会在适当的范围内预留资源。ResourceManager 根据适用的范围限制来控制预留;
// 如果超出限制,则返回错误(包装 ErrResourceLimitExceeded),由组件相应处理。
// 在系统的较低层,这通常会导致某种失败,如无法打开流或连接,并传播给程序员。
// 某些组件可能能够更优雅地处理资源预留失败;
// 例如,多路复用器尝试为窗口更改增加缓冲区时,将只保留现有窗口大小并继续正常运行,
// 尽管吞吐量可能会降低。
//
// 在某个范围内预留的所有资源在范围关闭时释放。
// 对于低级范围(主要是连接和流范围),这发生在连接或流关闭时。
//
// 服务程序员通常使用资源管理器为其子系统预留内存。
// 这通过两种方式实现:程序员可以将流附加到服务,流预留的资源自动计入服务预算;
// 或者程序员可以通过资源管理器接口直接与服务范围交互,使用 ViewService。
//
// 应用程序员也可以在某些适用范围内直接预留内存。
// 为了便于控制流限定的资源记账,系统中定义的所有范围都允许用户创建跨度。
// 跨度是根植于其他范围的临时范围,当程序员完成时释放其资源。
// 跨度范围可以形成树,具有嵌套跨度。
//
// 典型用法:
//   - 系统的低级组件(传输、多路复用器)都可以访问资源管理器,并通过它创建连接和流范围。
//     这些范围可通过 Conn 和 Stream 对象的 Scope 方法供用户访问,尽管接口更窄。
//   - 服务通常围绕流进行,程序员可以将流附加到特定服务。
//     他们也可以使用 ResourceManager 接口访问服务范围,直接为服务预留内存。
//   - 想要记录其网络资源使用的应用程序可以预留内存,通常使用跨度,直接在系统或服务范围内;
//     他们也可以选择对创建或拥有的流使用适当的流范围。
//
// 用户可维护部分:用户可以指定自己的接口实现。
// 我们在 dep2p-resource-manager 包中提供了一个规范实现。
// 该包的用户可以为各种范围指定限制,可以是静态的或动态的。
//
// 警告:ResourceManager 接口被视为实验性的,在后续版本中可能会更改。
type ResourceManager interface {
	ResourceScopeViewer

	// OpenConnection 创建一个新的连接范围,尚未与任何对等点关联;连接限定在临时范围内。
	// 调用者拥有返回的范围,负责调用 Done 以表示范围的跨度结束。
	//
	// 参数:
	//   - dir: Direction 连接方向
	//   - usefd: bool 是否使用文件描述符
	//   - endpoint: multiaddr.Multiaddr 端点地址
	//
	// 返回值:
	//   - ConnManagementScope: 连接管理范围
	//   - error: 如果发生错误,返回错误信息
	OpenConnection(dir Direction, usefd bool, endpoint multiaddr.Multiaddr) (ConnManagementScope, error)

	// OpenStream 创建一个新的流范围,最初未协商。
	// 未协商的流最初不会附加到任何协议范围,并受临时范围约束。
	// 调用者拥有返回的范围,负责调用 Done 以表示范围的跨度结束。
	//
	// 参数:
	//   - p: peer.ID 对等点ID
	//   - dir: Direction 流方向
	//
	// 返回值:
	//   - StreamManagementScope: 流管理范围
	//   - error: 如果发生错误,返回错误信息
	OpenStream(p peer.ID, dir Direction) (StreamManagementScope, error)

	// Close 关闭资源管理器
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	Close() error
}

// ResourceScopeViewer 是一个混合接口,提供访问顶级范围的视图方法。
type ResourceScopeViewer interface {
	// ViewSystem 查看系统范围的资源范围。
	// 系统范围是最高级别的范围,负责所有级别的全局资源使用。
	// 此范围约束所有其他范围并制定全局硬限制。
	//
	// 参数:
	//   - func(ResourceScope) error: 用于查看系统范围的函数
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	ViewSystem(func(ResourceScope) error) error

	// ViewTransient 查看临时(DMZ)资源范围。
	// 临时范围负责正在完全建立过程中的资源。
	// 例如,握手前的新连接不属于任何对等点,但仍需要约束,因为这会打开临时资源使用的攻击途径。
	// 同样,尚未协商协议的流受临时范围约束。
	//
	// 参数:
	//   - func(ResourceScope) error: 用于查看临时范围的函数
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	ViewTransient(func(ResourceScope) error) error

	// ViewService 检索特定服务的范围。
	//
	// 参数:
	//   - string: 服务名称
	//   - func(ServiceScope) error: 用于查看服务范围的函数
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	ViewService(string, func(ServiceScope) error) error

	// ViewProtocol 查看特定协议的资源管理范围。
	//
	// 参数:
	//   - protocol.ID: 协议ID
	//   - func(ProtocolScope) error: 用于查看协议范围的函数
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	ViewProtocol(protocol.ID, func(ProtocolScope) error) error

	// ViewPeer 查看特定对等点的资源管理范围。
	//
	// 参数:
	//   - peer.ID: 对等点ID
	//   - func(PeerScope) error: 用于查看对等点范围的函数
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	ViewPeer(peer.ID, func(PeerScope) error) error
}

const (
	// ReservationPriorityLow 是一个预留优先级,表示范围内存使用率在40%或以下时的预留。
	ReservationPriorityLow uint8 = 101
	// ReservationPriorityMedium 是一个预留优先级,表示范围内存使用率在60%或以下时的预留。
	ReservationPriorityMedium uint8 = 152
	// ReservationPriorityHigh 是一个预留优先级,表示范围内存使用率在80%或以下时的预留。
	ReservationPriorityHigh uint8 = 203
	// ReservationPriorityAlways 是一个预留优先级,表示只要有足够的内存就进行预留,不考虑范围使用率。
	ReservationPriorityAlways uint8 = 255
)

// ResourceScope 是所有范围的接口。
type ResourceScope interface {
	// ReserveMemory 在范围内预留内存/缓冲区空间;单位是字节。
	//
	// 如果 ReserveMemory 返回错误,则没有预留内存,调用者应处理失败情况。
	//
	// priority 参数表示内存预留的优先级。
	// 如果可用内存小于范围限制的(1+prio)/256,预留将失败,
	// 提供了一种机制来优雅地处理可能使系统过载的可选预留。
	// 例如,多路复用器增加窗口缓冲区时将使用低优先级,
	// 只有在系统没有内存压力时才增加缓冲区。
	//
	// 有4个预定义的优先级:Low、Medium、High和Always,
	// 捕获常见模式,但用户可以自由使用适用于其情况的任何粒度。
	//
	// 参数:
	//   - size: int 要预留的内存大小(字节)
	//   - prio: uint8 预留优先级
	//
	// 返回值:
	//   - error: 如果预留失败,返回错误信息
	ReserveMemory(size int, prio uint8) error

	// ReleaseMemory 显式释放之前用 ReserveMemory 预留的内存
	//
	// 参数:
	//   - size: int 要释放的内存大小(字节)
	ReleaseMemory(size int)

	// Stat 检索范围的当前资源使用情况。
	//
	// 返回值:
	//   - ScopeStat: 范围统计信息
	Stat() ScopeStat

	// BeginSpan 创建一个新的跨度范围,根植于此范围
	//
	// 返回值:
	//   - ResourceScopeSpan: 新的跨度范围
	//   - error: 如果发生错误,返回错误信息
	BeginSpan() (ResourceScopeSpan, error)
}

// ResourceScopeSpan 是一个带有限定跨度的 ResourceScope。
// 跨度范围是控制流限定的,当程序员调用 Done 时释放所有关联的资源。
//
// 示例:
//
//	s, err := someScope.BeginSpan()
//	if err != nil { ... }
//	defer s.Done()
//
//	if err := s.ReserveMemory(...); err != nil { ... }
//	// ... 使用内存
type ResourceScopeSpan interface {
	ResourceScope
	// Done 结束跨度并释放关联的资源。
	Done()
}

// ServiceScope 是服务资源范围的接口
type ServiceScope interface {
	ResourceScope

	// Name 返回此服务的名称
	//
	// 返回值:
	//   - string: 服务名称
	Name() string
}

// ProtocolScope 是协议资源范围的接口。
type ProtocolScope interface {
	ResourceScope

	// Protocol 返回此范围的协议
	//
	// 返回值:
	//   - protocol.ID: 协议ID
	Protocol() protocol.ID
}

// PeerScope 是对等点资源范围的接口。
type PeerScope interface {
	ResourceScope

	// Peer 返回此范围的对等点ID
	//
	// 返回值:
	//   - peer.ID: 对等点ID
	Peer() peer.ID
}

// ConnManagementScope 是连接资源范围的低级接口。
// 此接口由创建和拥有连接范围跨度的系统低级组件使用。
type ConnManagementScope interface {
	ResourceScopeSpan

	// PeerScope 返回与此连接关联的对等点范围。
	// 如果连接尚未与任何对等点关联,则返回 nil。
	//
	// 返回值:
	//   - PeerScope: 对等点范围
	PeerScope() PeerScope

	// SetPeer 为先前未关联的连接设置对等点
	//
	// 参数:
	//   - peer.ID: 对等点ID
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	SetPeer(peer.ID) error
}

// ConnScope 是连接范围的用户视图
type ConnScope interface {
	ResourceScope
}

// StreamManagementScope 是流资源范围的接口。
// 此接口由创建和拥有流范围跨度的系统低级组件使用。
type StreamManagementScope interface {
	ResourceScopeSpan

	// ProtocolScope 返回与此流关联的协议资源范围。
	// 如果流未与任何协议范围关联,则返回 nil。
	//
	// 返回值:
	//   - ProtocolScope: 协议范围
	ProtocolScope() ProtocolScope

	// SetProtocol 为先前未协商的流设置协议
	//
	// 参数:
	//   - proto: protocol.ID 协议ID
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	SetProtocol(proto protocol.ID) error

	// ServiceScope 返回拥有流的服务(如果有)。
	//
	// 返回值:
	//   - ServiceScope: 服务范围
	ServiceScope() ServiceScope

	// SetService 设置拥有此流的服务。
	//
	// 参数:
	//   - srv: string 服务名称
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	SetService(srv string) error

	// PeerScope 返回与此流关联的对等点资源范围。
	//
	// 返回值:
	//   - PeerScope: 对等点范围
	PeerScope() PeerScope
}

// StreamScope 是 StreamScope 的用户视图。
type StreamScope interface {
	ResourceScope

	// SetService 设置拥有此流的服务。
	//
	// 参数:
	//   - srv: string 服务名称
	//
	// 返回值:
	//   - error: 如果发生错误,返回错误信息
	SetService(srv string) error
}

// ScopeStat 是包含资源记账信息的结构。
type ScopeStat struct {
	// NumStreamsInbound 是入站流的数量
	NumStreamsInbound int
	// NumStreamsOutbound 是出站流的数量
	NumStreamsOutbound int
	// NumConnsInbound 是入站连接的数量
	NumConnsInbound int
	// NumConnsOutbound 是出站连接的数量
	NumConnsOutbound int
	// NumFD 是文件描述符的数量
	NumFD int
	// Memory 是内存使用量(字节)
	Memory int64
}

// NullResourceManager 是用于测试和默认值初始化的存根
type NullResourceManager struct{}

var _ ResourceScope = (*NullScope)(nil)
var _ ResourceScopeSpan = (*NullScope)(nil)
var _ ServiceScope = (*NullScope)(nil)
var _ ProtocolScope = (*NullScope)(nil)
var _ PeerScope = (*NullScope)(nil)
var _ ConnManagementScope = (*NullScope)(nil)
var _ ConnScope = (*NullScope)(nil)
var _ StreamManagementScope = (*NullScope)(nil)
var _ StreamScope = (*NullScope)(nil)

// NullScope 是用于测试和默认值初始化的存根
type NullScope struct{}

// ViewSystem 实现了 ResourceManager 接口的 ViewSystem 方法
func (n *NullResourceManager) ViewSystem(f func(ResourceScope) error) error {
	return f(&NullScope{})
}

// ViewTransient 实现了 ResourceManager 接口的 ViewTransient 方法
func (n *NullResourceManager) ViewTransient(f func(ResourceScope) error) error {
	return f(&NullScope{})
}

// ViewService 实现了 ResourceManager 接口的 ViewService 方法
func (n *NullResourceManager) ViewService(svc string, f func(ServiceScope) error) error {
	return f(&NullScope{})
}

// ViewProtocol 实现了 ResourceManager 接口的 ViewProtocol 方法
func (n *NullResourceManager) ViewProtocol(p protocol.ID, f func(ProtocolScope) error) error {
	return f(&NullScope{})
}

// ViewPeer 实现了 ResourceManager 接口的 ViewPeer 方法
func (n *NullResourceManager) ViewPeer(p peer.ID, f func(PeerScope) error) error {
	return f(&NullScope{})
}

// OpenConnection 实现了 ResourceManager 接口的 OpenConnection 方法
func (n *NullResourceManager) OpenConnection(dir Direction, usefd bool, endpoint multiaddr.Multiaddr) (ConnManagementScope, error) {
	return &NullScope{}, nil
}

// OpenStream 实现了 ResourceManager 接口的 OpenStream 方法
func (n *NullResourceManager) OpenStream(p peer.ID, dir Direction) (StreamManagementScope, error) {
	return &NullScope{}, nil
}

// Close 实现了 ResourceManager 接口的 Close 方法
func (n *NullResourceManager) Close() error {
	return nil
}

// ReserveMemory 实现了 ResourceScope 接口的 ReserveMemory 方法
func (n *NullScope) ReserveMemory(size int, prio uint8) error { return nil }

// ReleaseMemory 实现了 ResourceScope 接口的 ReleaseMemory 方法
func (n *NullScope) ReleaseMemory(size int) {}

// Stat 实现了 ResourceScope 接口的 Stat 方法
func (n *NullScope) Stat() ScopeStat { return ScopeStat{} }

// BeginSpan 实现了 ResourceScope 接口的 BeginSpan 方法
func (n *NullScope) BeginSpan() (ResourceScopeSpan, error) { return &NullScope{}, nil }

// Done 实现了 ResourceScopeSpan 接口的 Done 方法
func (n *NullScope) Done() {}

// Name 实现了 ServiceScope 接口的 Name 方法
func (n *NullScope) Name() string { return "" }

// Protocol 实现了 ProtocolScope 接口的 Protocol 方法
func (n *NullScope) Protocol() protocol.ID { return "" }

// Peer 实现了 PeerScope 接口的 Peer 方法
func (n *NullScope) Peer() peer.ID { return "" }

// PeerScope 实现了 ConnManagementScope 接口的 PeerScope 方法
func (n *NullScope) PeerScope() PeerScope { return &NullScope{} }

// SetPeer 实现了 ConnManagementScope 接口的 SetPeer 方法
func (n *NullScope) SetPeer(peer.ID) error { return nil }

// ProtocolScope 实现了 StreamManagementScope 接口的 ProtocolScope 方法
func (n *NullScope) ProtocolScope() ProtocolScope { return &NullScope{} }

// SetProtocol 实现了 StreamManagementScope 接口的 SetProtocol 方法
func (n *NullScope) SetProtocol(proto protocol.ID) error { return nil }

// ServiceScope 实现了 StreamManagementScope 接口的 ServiceScope 方法
func (n *NullScope) ServiceScope() ServiceScope { return &NullScope{} }

// SetService 实现了 StreamManagementScope 接口的 SetService 方法
func (n *NullScope) SetService(srv string) error { return nil }
