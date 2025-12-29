// Package discovery 定义节点发现相关接口
//
// 发现模块负责 P2P 网络中的节点发现，包括：
// - DHT 分布式发现
// - mDNS 本地发现
// - Bootstrap 引导发现
// - DNS 发现
package discovery

import (
	"context"
	"errors"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              统一发现 API - 类型定义
// ============================================================================

// Scope 发现/注册的作用域
//
// 用于区分系统级服务（sys）与业务级服务（realm）的命名空间。
type Scope int

const (
	// ScopeAuto 自动推断作用域
	//
	// 规则：
	// 1. 如果 Namespace 以 "sys:" 前缀开头，强制使用 ScopeSys
	// 2. 否则若当前已 JoinRealm，默认使用 ScopeRealm
	// 3. 否则默认使用 ScopeSys
	ScopeAuto Scope = iota

	// ScopeSys 系统级作用域
	//
	// 发现/通告不依赖 JoinRealm，用于 relay、bootstrap 等系统服务。
	// Key 格式: dep2p/v1/sys/{namespace}
	ScopeSys

	// ScopeRealm Realm 级作用域
	//
	// 发现/通告要求 JoinRealm（或显式传入 RealmID）。
	// Key 格式: dep2p/v1/realm/{realmID}/{namespace}
	ScopeRealm
)

// String 返回 Scope 的字符串表示
func (s Scope) String() string {
	switch s {
	case ScopeAuto:
		return "auto"
	case ScopeSys:
		return "sys"
	case ScopeRealm:
		return "realm"
	default:
		return "unknown"
	}
}

// Source 发现来源
//
// 用于指定查询哪些发现机制，以及标记结果来源以便可观测。
type Source string

const (
	// SourceProvider DHT Provider 发现
	//
	// 通过 DHT ADD_PROVIDER/GET_PROVIDERS 机制发现节点。
	// 适用于服务发现场景。
	SourceProvider Source = "provider"

	// SourceRendezvous Rendezvous 点发现
	//
	// 通过 Rendezvous 服务点进行注册和发现。
	// 适用于轻量级、低延迟的发现场景。
	SourceRendezvous Source = "rendezvous"

	// SourceLocal 本地邻居
	//
	// 从本地缓存（knownPeers、路由表、已连接节点）获取。
	// 注意：local 不保证覆盖率，仅用于"快速给一点候选"。
	SourceLocal Source = "local"
)

// DefaultSources 默认发现来源顺序
//
// 优先级：provider → rendezvous → local
var DefaultSources = []Source{SourceProvider, SourceRendezvous, SourceLocal}

// DiscoveryQuery 统一发现查询参数
//
// 通过此结构体可以灵活控制发现行为：
//   - 指定命名空间和作用域
//   - 选择发现来源和优先级
//   - 控制超时和返回数量
//
// 示例：
//
//	// 发现 sys 域的 relay 服务
//	query := DiscoveryQuery{
//	    Namespace: "sys:relay",
//	    Scope:     ScopeSys,
//	    Sources:   []Source{SourceProvider},
//	}
//
//	// 发现当前 Realm 的业务服务
//	query := DiscoveryQuery{
//	    Namespace: "myapp/chat",
//	    Scope:     ScopeAuto, // 自动使用当前 Realm
//	    Limit:     10,
//	}
type DiscoveryQuery struct {
	// Namespace 命名空间/主题
	//
	// 格式示例：
	// - 系统服务: "sys:relay", "sys:bootstrap"
	// - 业务主题: "myapp/chat", "blockchain/mainnet"
	Namespace string

	// Scope 作用域（默认 ScopeAuto）
	Scope Scope

	// RealmID 显式指定 RealmID（可选）
	//
	// 当 Scope 为 ScopeRealm 且需要查询非当前 Realm 时使用。
	// 为空时使用当前 JoinRealm 的 RealmID。
	RealmID types.RealmID

	// Limit 最大返回数量（0 表示无限制）
	Limit int

	// Sources 发现来源（默认 DefaultSources）
	//
	// 按数组顺序作为优先级，越靠前优先级越高。
	// 结果会按来源优先级排序后去重。
	Sources []Source

	// IncludeLocal 是否包含本地缓存节点（默认 true）
	//
	// 与 Sources 中是否包含 SourceLocal 配合使用。
	// 设为 false 可强制只返回远程发现结果。
	IncludeLocal bool

	// Timeout 查询超时（默认 30s）
	//
	// 各来源并发查询，在超时前尽量聚合结果。
	Timeout time.Duration
}

// DefaultQuery 返回默认查询参数
func DefaultQuery(namespace string) DiscoveryQuery {
	return DiscoveryQuery{
		Namespace:    namespace,
		Scope:        ScopeAuto,
		Limit:        0,
		Sources:      DefaultSources,
		IncludeLocal: true,
		Timeout:      30 * time.Second,
	}
}

// Registration 统一注册/通告参数
//
// 用于向网络通告本节点提供某服务。
//
// 示例：
//
//	// 注册为 relay 服务提供者
//	reg := Registration{
//	    Namespace: "sys:relay",
//	    Scope:     ScopeSys,
//	    TTL:       2 * time.Hour,
//	    Sources:   []Source{SourceProvider},
//	}
//
//	// 注册到当前 Realm 的业务服务
//	reg := Registration{
//	    Namespace: "myapp/chat",
//	    Scope:     ScopeAuto,
//	}
type Registration struct {
	// Namespace 命名空间/主题
	Namespace string

	// Scope 作用域（默认 ScopeAuto）
	Scope Scope

	// RealmID 显式指定 RealmID（可选）
	//
	// 当 Scope 为 ScopeRealm 且需要注册到非当前 Realm 时使用。
	RealmID types.RealmID

	// TTL 注册有效期（默认 2h）
	//
	// 服务层会自动续约（间隔 = TTL/2）。
	// 上限由 Config.RendezvousMaxTTL 控制（默认 72h）。
	TTL time.Duration

	// Sources 注册到哪些发现机制（默认 [provider, rendezvous]）
	//
	// 为空时同时注册到 provider 和 rendezvous。
	Sources []Source
}

// DefaultRegistration 返回默认注册参数
func DefaultRegistration(namespace string) Registration {
	return Registration{
		Namespace: namespace,
		Scope:     ScopeAuto,
		TTL:       2 * time.Hour,
		Sources:   []Source{SourceProvider, SourceRendezvous},
	}
}

// DiscoveryResult 单个发现结果
//
// 包含节点信息及其来源，便于上层应用做决策。
type DiscoveryResult struct {
	PeerInfo

	// Source 结果来源
	Source Source
}

// ============================================================================
//                              DiscoveryService 接口
// ============================================================================

// DiscoveryService 发现服务接口
//
// 统一管理多种发现机制，提供节点发现和通告能力。
//
// 重要说明：
//   - **不做全网枚举**：DiscoverPeers("")/"广播发现"只代表"局部邻居"，
//     不承诺网络爬取/全网节点列表。
//   - **统一发现来源**：保留 DHT provider 与 Rendezvous 两套机制，
//     按优先级 provider → rendezvous → local 聚合去重。
type DiscoveryService interface {
	Discoverer
	Announcer

	// Start 启动发现服务
	Start(ctx context.Context) error

	// Stop 停止发现服务
	Stop() error

	// =========================================================================
	// 入网状态机 (REQ-DISC-002)
	// =========================================================================

	// State 返回当前入网状态
	//
	// REQ-DISC-002: 入网应存在可解释的状态机
	//
	// 状态语义:
	//   - StateNotStarted: 服务未启动
	//   - StateBootstrapping: 正在连接引导节点
	//   - StateConnected: 已连接至少一个节点（可查询 DHT）
	//   - StateDiscoverable: 已发布到 DHT，可被其他节点发现
	//   - StateFailed: 入网失败
	State() DiscoveryState

	// SetOnStateChanged 注册状态变更回调
	//
	// 当入网状态发生变化时触发回调。
	// 回调在独立 goroutine 中执行，不会阻塞状态转移。
	SetOnStateChanged(callback func(StateChangeEvent))

	// WaitReady 等待服务就绪（阻塞）
	//
	// 返回条件:
	//   - 状态达到 StateConnected 或 StateDiscoverable
	//   - 状态达到 StateFailed（返回错误）
	//   - 上下文取消
	WaitReady(ctx context.Context) error

	// RetryBootstrap 重试引导
	//
	// 当状态为 StateFailed 时，可调用此方法重试引导流程。
	// 如果当前不在 StateFailed 状态，此方法无操作。
	RetryBootstrap()

	// =========================================================================
	// 统一发现 API（推荐使用）
	// =========================================================================

	// Discover 统一发现入口
	//
	// 根据 DiscoveryQuery 参数从多个来源（provider、rendezvous、local）
	// 并发查询并聚合去重结果。
	//
	// 示例：
	//   ch, _ := svc.Discover(ctx, DefaultQuery("myapp/chat"))
	//   for peer := range ch {
	//       fmt.Printf("发现节点: %s (来源: %s)\n", peer.ID, peer.Source)
	//   }
	Discover(ctx context.Context, query DiscoveryQuery) (<-chan DiscoveryResult, error)

	// RegisterService 统一注册入口
	//
	// 根据 Registration 参数向指定来源（provider、rendezvous）注册。
	// 服务层会自动续约直到调用 UnregisterService。
	//
	// 示例：
	//   err := svc.RegisterService(ctx, DefaultRegistration("myapp/chat"))
	RegisterService(ctx context.Context, reg Registration) error

	// UnregisterService 统一注销入口
	//
	// 取消注册并停止自动续约。
	UnregisterService(ctx context.Context, reg Registration) error

	// =========================================================================
	// 发现器管理
	// =========================================================================

	// RegisterDiscoverer 注册完整发现器
	// Deprecated: 使用 RegisterPeerFinder、RegisterNamespaceDiscoverer 等细粒度方法
	RegisterDiscoverer(name string, discoverer Discoverer)

	// RegisterPeerFinder 注册按 NodeID 查找的发现器
	RegisterPeerFinder(name string, finder PeerFinder)

	// RegisterClosestPeerFinder 注册 DHT 风格的发现器
	RegisterClosestPeerFinder(name string, finder ClosestPeerFinder)

	// RegisterNamespaceDiscoverer 注册基于命名空间的发现器
	RegisterNamespaceDiscoverer(name string, discoverer NamespaceDiscoverer)

	// RegisterAnnouncer 注册通告器
	RegisterAnnouncer(name string, announcer Announcer)

	// OnPeerDiscovered 注册节点发现回调
	//
	// 当通过 mDNS、DHT 或其他发现机制发现新节点时触发回调。
	// 应用可以通过此回调主动连接到发现的节点。
	//
	// 注意：发现机制会自动将 peer.Addrs 写入 AddressBook，
	// 因此推荐使用 DialByNodeID（Connect）而非直接传入地址。
	//
	// 示例（推荐，DialByNodeID 语义）:
	//
	//   discovery.OnPeerDiscovered(func(peer PeerInfo) {
	//       // AddressBook 已缓存 peer.Addrs，直接按 NodeID 连接
	//       conn, _ := endpoint.Connect(ctx, peer.ID)
	//       // 使用连接...
	//   })
	//
	// 高级用法（DialByNodeIDWithDialAddrs，受控环境/运维场景）:
	//
	//   discovery.OnPeerDiscovered(func(peer PeerInfo) {
	//       // 显式指定地址（跳过 AddressBook 查找）
	//       conn, _ := endpoint.ConnectWithAddrs(ctx, peer.ID, peer.Addrs)
	//       // 使用连接...
	//   })
	OnPeerDiscovered(callback func(PeerInfo))
}

// ============================================================================
//                              Discoverer 接口（细粒度设计）
// ============================================================================

// PeerFinder 按 NodeID 查找节点接口
//
// 由 DHT 等支持按 NodeID 查找的发现器实现。
// Rendezvous、mDNS 等不需要实现此接口。
type PeerFinder interface {
	// FindPeer 查找指定节点的地址
	//
	// 返回节点的已知地址列表。
	FindPeer(ctx context.Context, id types.NodeID) ([]endpoint.Address, error)

	// FindPeers 批量查找节点
	FindPeers(ctx context.Context, ids []types.NodeID) (map[types.NodeID][]endpoint.Address, error)
}

// ClosestPeerFinder DHT 风格的距离查找接口
//
// 由 DHT 实现，用于 Kademlia 风格的最近节点查找。
type ClosestPeerFinder interface {
	// FindClosestPeers 查找最接近指定 key 的节点
	//
	// 用于 DHT 场景，返回距离目标最近的节点。
	FindClosestPeers(ctx context.Context, key []byte, count int) ([]types.NodeID, error)
}

// NamespaceDiscoverer 基于命名空间的发现接口
//
// 通用的发现接口，支持按命名空间/主题发现节点。
// DHT、Rendezvous、mDNS 都可以实现此接口。
type NamespaceDiscoverer interface {
	// DiscoverPeers 发现新节点
	//
	// 返回发现的节点通道，持续发现直到上下文取消。
	DiscoverPeers(ctx context.Context, namespace string) (<-chan PeerInfo, error)
}

// Discoverer 完整发现器接口（组合接口）
//
// 完整实现所有发现能力的发现器（如 DHT）应实现此接口。
// 对于只支持部分能力的发现器（如 Rendezvous），应实现对应的细粒度接口。
//
// Deprecated: 建议使用细粒度接口 PeerFinder、ClosestPeerFinder、NamespaceDiscoverer
type Discoverer interface {
	PeerFinder
	ClosestPeerFinder
	NamespaceDiscoverer
}

// AddressUpdater 地址更新接口
//
// 发现器可选实现此接口以支持动态地址更新。
// 当节点的通告地址变化时（如 NAT 发现公网地址），服务层会调用此接口更新发现器的本地地址。
type AddressUpdater interface {
	// UpdateLocalAddrs 更新本地通告地址
	//
	// addrs: 新的地址列表（字符串格式）
	UpdateLocalAddrs(addrs []string)
}

// ============================================================================
//                              Announcer 接口
// ============================================================================

// Announcer 通告器接口
//
// 负责向网络通告本节点的存在。
type Announcer interface {
	// Announce 通告本节点
	//
	// 向指定命名空间通告本节点，使其他节点可以发现。
	Announce(ctx context.Context, namespace string) error

	// AnnounceWithTTL 带 TTL 的通告
	AnnounceWithTTL(ctx context.Context, namespace string, ttl time.Duration) error

	// StopAnnounce 停止通告
	StopAnnounce(namespace string) error
}

// ============================================================================
//                              PeerInfo 类型别名
// ============================================================================

// PeerInfo 节点信息（类型别名，实际定义在 types 包）
// 注意：Addrs 字段使用 []string 格式
type PeerInfo = types.PeerInfo

// ============================================================================
//                              DHT 接口
// ============================================================================

// DHT 分布式哈希表接口
type DHT interface {
	Discoverer

	// PutValue 存储值（使用默认 TTL）
	//
	// 使用 DHT 配置中的 MaxRecordAge 作为 TTL。
	PutValue(ctx context.Context, key string, value []byte) error

	// PutValueWithTTL 存储值（指定 TTL）
	//
	// 根据设计文档 STORE 操作：{ Key: bytes, Value: bytes, TTL: uint32 }
	// 值将在 TTL 到期后自动过期。
	PutValueWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// GetValue 获取值
	//
	// 如果值已过期，返回 nil。
	GetValue(ctx context.Context, key string) ([]byte, error)

	// Bootstrap 执行引导
	Bootstrap(ctx context.Context) error

	// RoutingTable 返回路由表
	RoutingTable() RoutingTable

	// Mode 返回 DHT 模式
	Mode() DHTMode

	// SetMode 设置 DHT 模式
	SetMode(mode DHTMode)
}

// DHTMode DHT 运行模式
type DHTMode int

const (
	// DHTModeAuto 自动模式
	DHTModeAuto DHTMode = iota
	// DHTModeServer 服务器模式（完全参与）
	DHTModeServer
	// DHTModeClient 客户端模式（仅查询）
	DHTModeClient
)

// RoutingTable 路由表接口
type RoutingTable interface {
	// Size 返回路由表大小
	Size() int

	// Peers 返回路由表中的所有节点
	Peers() []types.NodeID

	// NearestPeers 返回最近的节点
	NearestPeers(key []byte, count int) []types.NodeID

	// Update 更新节点
	Update(id types.NodeID) error

	// Remove 移除节点
	Remove(id types.NodeID)
}

// ============================================================================
//                              mDNS 接口
// ============================================================================

// MDNS mDNS 发现接口
type MDNS interface {
	// Start 启动 mDNS 服务
	Start(ctx context.Context) error

	// Stop 停止 mDNS 服务
	Stop() error

	// Peers 返回发现的本地节点
	Peers() []PeerInfo
}

// ============================================================================
//                              Bootstrap 接口
// ============================================================================

// Bootstrap 引导服务接口
type Bootstrap interface {
	// GetBootstrapPeers 获取引导节点
	GetBootstrapPeers(ctx context.Context) ([]PeerInfo, error)

	// AddBootstrapPeer 添加引导节点
	AddBootstrapPeer(peer PeerInfo)

	// RemoveBootstrapPeer 移除引导节点
	RemoveBootstrapPeer(id types.NodeID)
}

// ============================================================================
//                              Rendezvous 接口
// ============================================================================

// Rendezvous 基于主题的节点发现接口
//
// Rendezvous 提供轻量级的基于命名空间（主题）的节点发现机制，
// 适用于不需要完整 DHT 的场景。
//
// 命名空间格式示例：
//   - 业务主题: "blockchain/mainnet/peers"
//   - Realm 内: "<RealmID>/topic/<name>"
//   - 服务发现: "service/<service-type>"
type Rendezvous interface {
	// Register 注册到命名空间
	//
	// 将本节点注册到指定命名空间，使其他节点可以发现。
	// TTL 指定注册有效期，到期后需要续约。
	Register(ctx context.Context, namespace string, ttl time.Duration) error

	// Unregister 取消注册
	//
	// 从指定命名空间取消注册。
	Unregister(ctx context.Context, namespace string) error

	// Discover 发现命名空间中的节点
	//
	// 返回在指定命名空间注册的节点列表。
	// limit 指定最大返回数量，0 表示无限制。
	Discover(ctx context.Context, namespace string, limit int) ([]PeerInfo, error)

	// DiscoverAsync 异步发现节点（流式）
	//
	// 返回发现的节点通道，持续发现直到上下文取消。
	DiscoverAsync(ctx context.Context, namespace string) (<-chan PeerInfo, error)
}

// RendezvousPoint Rendezvous 服务点接口
//
// RendezvousPoint 是 Rendezvous 服务的服务端，
// 负责存储注册信息和响应发现请求。
type RendezvousPoint interface {
	// Start 启动 Rendezvous 服务
	Start(ctx context.Context) error

	// Stop 停止 Rendezvous 服务
	Stop() error

	// HandleRegister 处理注册请求
	//
	// 由协议层调用，处理来自其他节点的注册请求。
	HandleRegister(from types.NodeID, namespace string, info PeerInfo, ttl time.Duration) error

	// HandleDiscover 处理发现请求
	//
	// 由协议层调用，处理来自其他节点的发现请求。
	HandleDiscover(from types.NodeID, namespace string, limit int, cookie []byte) ([]PeerInfo, []byte, error)

	// HandleUnregister 处理取消注册请求
	HandleUnregister(from types.NodeID, namespace string) error

	// Stats 返回统计信息
	Stats() RendezvousStats

	// Namespaces 返回所有命名空间
	Namespaces() []string

	// PeersInNamespace 返回命名空间中的节点数
	PeersInNamespace(namespace string) int
}

// RendezvousStats Rendezvous 统计信息
type RendezvousStats struct {
	// TotalRegistrations 总注册数
	TotalRegistrations int

	// TotalNamespaces 总命名空间数
	TotalNamespaces int

	// RegistersReceived 收到的注册请求数
	RegistersReceived uint64

	// DiscoversReceived 收到的发现请求数
	DiscoversReceived uint64

	// RegistrationsExpired 过期的注册数
	RegistrationsExpired uint64
}

// ============================================================================
//                              DNS 发现接口
// ============================================================================

// DNSDiscoverer DNS 发现接口
//
// 通过 DNS TXT 记录发现节点，遵循 libp2p dnsaddr 规范。
//
// DNS TXT 记录格式：
//
//	_dnsaddr.<domain> TXT "dnsaddr=/ip4/<ip>/tcp/<port>/p2p/<nodeID>"
//
// 示例：
//
//	_dnsaddr.bootstrap.dep2p.network TXT "dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/5Q2STWvB..."
type DNSDiscoverer interface {
	NamespaceDiscoverer

	// Resolve 解析 DNS 域名获取节点信息
	//
	// 解析指定域名的 TXT 记录，返回发现的节点列表。
	Resolve(ctx context.Context, domain string) ([]PeerInfo, error)

	// ResolveWithDepth 递归解析（支持 dnsaddr 嵌套）
	//
	// 支持嵌套的 dnsaddr 记录，最大递归深度由 maxDepth 控制。
	ResolveWithDepth(ctx context.Context, domain string, maxDepth int) ([]PeerInfo, error)

	// Domains 返回配置的 DNS 域名列表
	Domains() []string
}

// ============================================================================
//                              入网状态机 (REQ-DISC-002)
// ============================================================================

// DiscoveryState 发现服务状态
//
// REQ-DISC-002: 入网应存在可解释的状态机
//
// 状态转移图:
//
//	StateNotStarted → StateBootstrapping → StateConnected → StateDiscoverable
//	                          ↓                  ↓                  ↓
//	                    StateFailed ←──────────────────────────────────
//
// 状态说明:
//   - StateNotStarted: 服务未启动
//   - StateBootstrapping: 正在连接引导节点
//   - StateConnected: 已连接至少一个节点（可查询 DHT）
//   - StateDiscoverable: 已发布到 DHT，可被其他节点发现
//   - StateFailed: 入网失败（无引导节点/全部连接失败）
type DiscoveryState int

const (
	// StateNotStarted 服务未启动
	StateNotStarted DiscoveryState = iota

	// StateBootstrapping 正在连接引导节点
	//
	// 触发条件: Start() 被调用
	// 退出条件: 连接成功 → StateConnected, 全部失败 → StateFailed
	StateBootstrapping

	// StateConnected 已连接至少一个节点
	//
	// 语义: 可以执行 DHT 查询（FindPeer/DiscoverPeers）
	// 触发条件: 首个引导节点连接成功
	// 退出条件: 路由表发布成功 → StateDiscoverable
	StateConnected

	// StateDiscoverable 可被其他节点发现
	//
	// 语义: 已向 DHT 发布本节点信息，其他节点可通过 FindPeer 找到本节点
	// 触发条件: DHT 路由表刷新成功且本地 PeerRecord 已发布
	StateDiscoverable

	// StateFailed 入网失败
	//
	// 语义: 无法建立网络连接
	// 触发条件:
	//   - 无配置引导节点且无法通过其他方式发现节点
	//   - 所有引导节点连接失败
	//   - DHT 不可用
	// 恢复: 调用 RetryBootstrap() 或添加新引导节点后重试
	StateFailed
)

// String 返回状态的字符串表示
func (s DiscoveryState) String() string {
	switch s {
	case StateNotStarted:
		return "not_started"
	case StateBootstrapping:
		return "bootstrapping"
	case StateConnected:
		return "connected"
	case StateDiscoverable:
		return "discoverable"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// IsReady 是否已就绪（可执行发现操作）
func (s DiscoveryState) IsReady() bool {
	return s == StateConnected || s == StateDiscoverable
}

// StateChangeEvent 状态变更事件
type StateChangeEvent struct {
	// OldState 旧状态
	OldState DiscoveryState
	// NewState 新状态
	NewState DiscoveryState
	// Reason 变更原因（可选）
	Reason string
}

// Type 返回事件类型
func (e StateChangeEvent) Type() string {
	return EventStateChanged
}

// ============================================================================
//                              事件
// ============================================================================

// 发现事件类型
const (
	// EventPeerDiscovered 发现新节点
	EventPeerDiscovered = "discovery.peer_discovered"
	// EventPeerExpired 节点过期
	EventPeerExpired = "discovery.peer_expired"
	// EventRendezvousRegistered 节点在 Rendezvous 注册
	EventRendezvousRegistered = "discovery.rendezvous_registered"
	// EventRendezvousUnregistered 节点从 Rendezvous 取消注册
	EventRendezvousUnregistered = "discovery.rendezvous_unregistered"
	// EventStateChanged 状态变更 (REQ-DISC-002)
	EventStateChanged = "discovery.state_changed"
)

// PeerDiscoveredEvent 节点发现事件
type PeerDiscoveredEvent struct {
	PeerInfo
}

// Type 返回事件类型
func (e PeerDiscoveredEvent) Type() string {
	return EventPeerDiscovered
}

// PeerExpiredEvent 节点过期事件
type PeerExpiredEvent struct {
	ID types.NodeID
}

// Type 返回事件类型
func (e PeerExpiredEvent) Type() string {
	return EventPeerExpired
}

// ============================================================================
//                              配置
// ============================================================================

// Config 发现模块配置
type Config struct {
	// EnableDHT 启用 DHT
	EnableDHT bool

	// EnableMDNS 启用 mDNS
	EnableMDNS bool

	// EnableBootstrap 启用引导
	EnableBootstrap bool

	// ServeBootstrap 是否作为 Bootstrap 服务器
	// 如果为 true，本节点会注册到 DHT 的 sys/bootstrap，可被动态发现
	ServeBootstrap bool

	// DHTMode DHT 模式
	DHTMode DHTMode

	// BootstrapPeers 引导节点列表
	BootstrapPeers []PeerInfo

	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration

	// MDNSServiceTag mDNS 服务标签
	MDNSServiceTag string

	// DiscoveryNamespace 默认发现命名空间
	DiscoveryNamespace string

	// EnableRealmIsolation 启用 Realm 隔离
	// 启用后，发现服务只返回同一 Realm 的节点
	EnableRealmIsolation bool

	// TargetPeers 目标节点数
	// 用于动态发现间隔算法
	TargetPeers int

	// MinInterval 最小发现间隔
	MinInterval time.Duration

	// MaxInterval 最大发现间隔
	MaxInterval time.Duration

	// EnableRendezvous 启用 Rendezvous 发现
	EnableRendezvous bool

	// RendezvousPoints 已知的 Rendezvous 服务点
	// 如果为空，将通过 DHT 发现 Rendezvous 点
	RendezvousPoints []PeerInfo

	// RendezvousTTL 默认注册 TTL
	RendezvousTTL time.Duration

	// ServeRendezvous 是否作为 Rendezvous 服务点
	// 启用后，本节点将接受其他节点的注册请求
	ServeRendezvous bool

	// RendezvousMaxRegistrations Rendezvous 服务点最大注册数
	RendezvousMaxRegistrations int

	// RendezvousMaxNamespaces Rendezvous 服务点最大命名空间数
	RendezvousMaxNamespaces int

	// RendezvousMaxTTL Rendezvous 最大 TTL
	RendezvousMaxTTL time.Duration

	// RendezvousCleanupInterval 过期注册清理间隔
	RendezvousCleanupInterval time.Duration

	// EnableDNS 启用 DNS 发现
	EnableDNS bool

	// DNSDomains DNS 域名列表
	// 格式: _dnsaddr.<domain> 或直接使用 <domain>（自动添加 _dnsaddr. 前缀）
	DNSDomains []string

	// DNSResolver 自定义 DNS 解析器地址（可选）
	// 格式: <ip>:<port>，例如 "8.8.8.8:53"
	// 为空则使用系统默认解析器
	DNSResolver string

	// DNSTimeout DNS 查询超时
	DNSTimeout time.Duration

	// DNSMaxDepth 最大递归深度
	// 用于嵌套的 dnsaddr 记录
	DNSMaxDepth int

	// DNSCacheTTL DNS 结果缓存时间
	DNSCacheTTL time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		EnableDHT:                  true,
		EnableMDNS:                 true,
		EnableBootstrap:            true,
		DHTMode:                    DHTModeAuto,
		RefreshInterval:            10 * time.Minute,
		MDNSServiceTag:             "_dep2p._udp",
		DiscoveryNamespace:         "dep2p",
		EnableRealmIsolation:       false,
		TargetPeers:                50,
		MinInterval:                5 * time.Second,
		MaxInterval:                5 * time.Minute,
		EnableRendezvous:           true,
		RendezvousPoints:           nil,
		RendezvousTTL:              2 * time.Hour,
		ServeRendezvous:            false,
		RendezvousMaxRegistrations: 10000,
		RendezvousMaxNamespaces:    1000,
		RendezvousMaxTTL:           72 * time.Hour,
		RendezvousCleanupInterval:  5 * time.Minute,
		EnableDNS:                  false,
		DNSDomains:                 nil,
		DNSResolver:                "",
		DNSTimeout:                 10 * time.Second,
		DNSMaxDepth:                3,
		DNSCacheTTL:                5 * time.Minute,
	}
}

// RendezvousRegisteredEvent Rendezvous 注册事件
type RendezvousRegisteredEvent struct {
	Namespace string
	PeerInfo
}

// Type 返回事件类型
func (e RendezvousRegisteredEvent) Type() string {
	return EventRendezvousRegistered
}

// RendezvousUnregisteredEvent Rendezvous 取消注册事件
type RendezvousUnregisteredEvent struct {
	Namespace string
	ID        types.NodeID
}

// Type 返回事件类型
func (e RendezvousUnregisteredEvent) Type() string {
	return EventRendezvousUnregistered
}

// ============================================================================
//                              错误定义
// ============================================================================

// ErrBootstrapFailed 引导失败
var ErrBootstrapFailed = errors.New("bootstrap failed: unable to connect to any bootstrap peer")

// ErrNoBootstrapPeers 无引导节点
var ErrNoBootstrapPeers = errors.New("no bootstrap peers configured")

// ErrNotReady 服务未就绪
var ErrNotReady = errors.New("discovery service not ready")

// ErrPeerNotFound 节点未找到
var ErrPeerNotFound = errors.New("peer not found")

// ErrRecursiveDiscovery 递归发现错误（REQ-DISC-006）
// 当检测到发现流程中的自递归时返回
var ErrRecursiveDiscovery = errors.New("recursive discovery detected: aborting to prevent infinite loop")
