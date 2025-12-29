package endpoint

import (
	"context"
	"time"

	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
)

// ============================================================================
//                              拆分接口（可达性优先重构）
// ============================================================================

// CoreEndpoint 核心连接接口（精简）
//
// 包含身份、连接和生命周期管理的最小接口集。
// 用于只需要基础连接功能的场景。
type CoreEndpoint interface {
	// ID 返回节点的唯一标识符
	ID() NodeID

	// PublicKey 返回节点的公钥
	PublicKey() PublicKey

	// Connect 连接到指定节点
	Connect(ctx context.Context, nodeID NodeID) (Connection, error)

	// ConnectWithAddrs 使用指定地址连接到节点
	ConnectWithAddrs(ctx context.Context, nodeID NodeID, addrs []Address) (Connection, error)

	// Disconnect 断开与指定节点的连接
	Disconnect(nodeID NodeID) error

	// Listen 开始监听连接
	Listen(ctx context.Context) error

	// Close 关闭节点
	Close() error
}

// ProtocolRegistry 协议处理接口
//
// 管理协议处理器的注册和查询。
type ProtocolRegistry interface {
	// SetProtocolHandler 注册协议处理器
	SetProtocolHandler(protocolID ProtocolID, handler ProtocolHandler)

	// RemoveProtocolHandler 移除协议处理器
	RemoveProtocolHandler(protocolID ProtocolID)

	// Protocols 返回已注册的协议列表
	Protocols() []ProtocolID
}

// AddressProvider 地址提供接口
//
// 提供节点的监听地址和通告地址。
// 用于发现服务等需要获取节点地址的场景。
type AddressProvider interface {
	// ListenAddrs 返回监听地址列表
	ListenAddrs() []Address

	// AdvertisedAddrs 返回通告地址列表
	// 可达性优先策略：已验证直连 > Relay > 监听地址
	AdvertisedAddrs() []Address

	// VerifiedDirectAddrs 返回已验证的直连地址列表（REQ-ADDR-002 真源）
	//
	// 这是 ShareableAddrs 的真实数据源：
	// - 仅返回通过 dial-back 验证的公网直连地址
	// - 不包含 Relay 地址
	// - 不包含 ListenAddrs 回退
	// - 无验证地址时返回 nil（而非空切片）
	//
	// 用于构建可分享的 Full Address（INV-005）。
	VerifiedDirectAddrs() []Address

	// AddAdvertisedAddr 添加通告地址
	AddAdvertisedAddr(addr Address)
}

// ConnectionManager 连接管理接口
//
// 查询和管理活跃连接。
type ConnectionManager interface {
	// Connections 返回所有活跃连接
	Connections() []Connection

	// Connection 获取指定节点的连接（如存在）
	Connection(nodeID NodeID) (Connection, bool)

	// ConnectionCount 返回当前连接数
	ConnectionCount() int
}

// SubsystemAccessor 子系统访问接口
//
// 提供对子系统的访问。
type SubsystemAccessor interface {
	// Discovery 返回发现服务
	Discovery() DiscoveryService

	// NAT 返回 NAT 服务
	NAT() NATService

	// Relay 返回中继客户端
	Relay() relayif.RelayClient

	// AddressBook 返回地址簿
	AddressBook() AddressBook
}

// ============================================================================
//                              Endpoint 聚合接口
// ============================================================================

// Endpoint 是 dep2p 的核心入口点
//
// Endpoint 代表一个 P2P 网络节点，融合了 iroh 的简洁性和 libp2p 的功能性。
// 它负责管理节点身份、连接、协议处理和各种网络服务。
//
// 此接口聚合了多个细粒度接口，保持向后兼容：
// - CoreEndpoint: 核心连接功能
// - ProtocolRegistry: 协议处理
// - AddressProvider: 地址管理
// - ConnectionManager: 连接查询
// - SubsystemAccessor: 子系统访问
//
// 基本使用示例:
//
//	endpoint, _ := dep2p.NewEndpoint(
//	    dep2p.WithIdentity(key),
//	    dep2p.WithListenPort(8000),
//	)
//	defer endpoint.Close()
//
//	// 连接到远程节点
//	conn, _ := endpoint.Connect(ctx, remoteNodeID)
//
//	// 打开流
//	stream, _ := conn.OpenStream(ctx, "/my-protocol/1.0")
type Endpoint interface {
	// ==================== 身份信息 ====================

	// ID 返回节点的唯一标识符
	// NodeID 由公钥派生，是节点在网络中的唯一身份
	ID() NodeID

	// PublicKey 返回节点的公钥
	// 可用于验证节点身份和加密通信
	PublicKey() PublicKey

	// ==================== 连接管理 ====================

	// Connect 连接到指定节点
	//
	// 只需提供 NodeID，Endpoint 会自动：
	// 1. 通过发现服务查找节点地址
	// 2. 选择最优连接路径（直连 → 打洞 → 中继）
	// 3. 建立安全连接
	//
	// 如果已有到该节点的连接，返回现有连接。
	Connect(ctx context.Context, nodeID NodeID) (Connection, error)

	// ConnectWithAddrs 使用指定地址连接到节点
	//
	// 跳过发现服务，直接使用提供的地址尝试连接。
	// 适用于已知节点地址的场景。
	ConnectWithAddrs(ctx context.Context, nodeID NodeID, addrs []Address) (Connection, error)

	// Disconnect 断开与指定节点的连接
	Disconnect(nodeID NodeID) error

	// Connections 返回所有活跃连接
	Connections() []Connection

	// Connection 获取指定节点的连接（如存在）
	// 返回连接和是否存在的布尔值
	Connection(nodeID NodeID) (Connection, bool)

	// ConnectionCount 返回当前连接数
	ConnectionCount() int

	// ==================== 监听与接受 ====================

	// Listen 开始监听连接
	//
	// 调用此方法后，Endpoint 开始接受入站连接。
	// 应该在设置协议处理器之后调用。
	Listen(ctx context.Context) error

	// 注意：Accept(ctx) 方法已删除（v1.1 清理）。
	// 原因：仅测试使用，生产路径通过 SetProtocolHandler + handleInbound 自动处理。
	// 测试替代：直接使用 internal/core/endpoint.Endpoint.Accept()

	// ==================== 协议处理 ====================

	// SetProtocolHandler 注册协议处理器
	//
	// 当收到指定协议的流时，会调用对应的处理器。
	// 示例:
	//   endpoint.SetProtocolHandler("/echo/1.0", func(s Stream) {
	//       io.Copy(s, s) // echo back
	//   })
	SetProtocolHandler(protocolID ProtocolID, handler ProtocolHandler)

	// RemoveProtocolHandler 移除协议处理器
	RemoveProtocolHandler(protocolID ProtocolID)

	// Protocols 返回已注册的协议列表
	Protocols() []ProtocolID

	// ==================== 地址管理 ====================

	// ListenAddrs 返回监听地址列表
	// 这些是节点实际绑定的本地地址
	ListenAddrs() []Address

	// AdvertisedAddrs 返回通告地址列表
	// 可达性优先策略：已验证直连地址 > Relay 地址 > 监听地址
	AdvertisedAddrs() []Address

	// VerifiedDirectAddrs 返回已验证的直连地址列表（REQ-ADDR-002 真源）
	//
	// 这是 ShareableAddrs 的真实数据源：
	// - 仅返回通过 dial-back 验证的公网直连地址
	// - 不包含 Relay 地址
	// - 不包含 ListenAddrs 回退
	// - 无验证地址时返回 nil（而非空切片）
	//
	// 用于构建可分享的 Full Address（INV-005）。
	VerifiedDirectAddrs() []Address

	// AddAdvertisedAddr 添加通告地址
	// 手动添加一个公网可达地址
	AddAdvertisedAddr(addr Address)

	// ==================== 子系统访问 ====================

	// Discovery 返回发现服务
	// 如果未启用发现服务，返回 nil
	Discovery() DiscoveryService

	// NAT 返回 NAT 服务
	// 如果未启用 NAT 服务，返回 nil
	NAT() NATService

	// Relay 返回中继客户端
	// 如果未启用中继服务，返回 nil
	Relay() relayif.RelayClient

	// AddressBook 返回地址簿
	// 用于管理已知节点的地址
	AddressBook() AddressBook

	// ==================== 连接通知 ====================

	// RegisterConnectionCallback 注册连接建立回调
	//
	// 当新连接建立时，回调会被调用。
	// 这用于通知 GossipSub 等模块新 peer 的连接。
	//
	// 参数:
	//   - callback: 回调函数，接收 nodeID 和 outbound 标志（true=出站，false=入站）
	//
	// 示例:
	//   endpoint.RegisterConnectionCallback(func(nodeID NodeID, outbound bool) {
	//       gossipRouter.AddPeer(nodeID, outbound)
	//   })
	RegisterConnectionCallback(callback func(nodeID NodeID, outbound bool))

	// RegisterConnectionEventCallback 注册连接事件回调
	//
	// 当连接状态变化时（建立、关闭、失败），回调会被调用。
	// 事件类型包括：ConnectionOpenedEvent、ConnectionClosedEvent、ConnectionFailedEvent
	//
	// 与 RegisterConnectionCallback 的区别：
	//   - ConnectionCallback 仅在连接建立时触发
	//   - ConnectionEventCallback 在所有连接生命周期事件时触发，包含更多上下文信息
	//
	// 参数:
	//   - callback: 回调函数，接收事件对象（需类型断言判断具体事件类型）
	//
	// 示例:
	//   endpoint.RegisterConnectionEventCallback(func(event interface{}) {
	//       switch e := event.(type) {
	//       case ConnectionOpenedEvent:
	//           log.Info("连接建立", "node", e.Connection.RemoteID())
	//       case ConnectionClosedEvent:
	//           log.Info("连接关闭", "node", e.Connection.RemoteID(), "reason", e.Reason)
	//           if e.IsRelayConn {
	//               // 处理 relay 连接断开
	//           }
	//       case ConnectionFailedEvent:
	//           log.Warn("连接失败", "node", e.NodeID, "err", e.Error)
	//       }
	//   })
	RegisterConnectionEventCallback(callback func(event interface{}))

	// ==================== 带重试的连接 ====================

	// ConnectWithRetry 带自动重试的连接方法
	//
	// 当连接失败时，使用指数退避策略自动重试。
	// 适用于需要高可靠性连接的场景。
	//
	// 参数:
	//   - ctx: 上下文，用于取消重试
	//   - nodeID: 目标节点 ID
	//   - config: 重试配置，nil 时使用默认配置（5次重试，100ms初始退避，30s最大退避）
	//
	// 返回:
	//   - Connection: 成功建立的连接
	//   - error: 所有重试都失败时返回错误
	//
	// 示例:
	//   conn, err := endpoint.ConnectWithRetry(ctx, nodeID, &RetryConfig{
	//       MaxRetries:     3,
	//       InitialBackoff: 200 * time.Millisecond,
	//       MaxBackoff:     10 * time.Second,
	//   })
	ConnectWithRetry(ctx context.Context, nodeID NodeID, config *RetryConfig) (Connection, error)

	// ==================== 生命周期 ====================

	// Close 关闭节点
	//
	// 关闭所有连接和服务，释放资源。
	// 调用后节点不可再使用。
	Close() error

	// ==================== 诊断 (REQ-OPS-001) ====================

	// DiagnosticReport 生成诊断报告
	//
	// REQ-OPS-001: 关键状态可观测且有统一诊断入口
	//
	// 聚合所有子系统的状态信息，返回统一格式的诊断报告。
	// 可用于运维监控、故障排查、健康检查等场景。
	//
	// 示例:
	//   report := endpoint.DiagnosticReport()
	//   fmt.Printf("连接数: %d\n", report.Connections.Total)
	//   fmt.Printf("入网状态: %s\n", report.Discovery.State)
	DiagnosticReport() DiagnosticReport
}

// ============================================================================
//                              协议处理器
// ============================================================================

// ProtocolHandler 协议处理函数
//
// 当收到指定协议的入站流时被调用。
// 处理器负责读取和写入流，完成后应该关闭流。
type ProtocolHandler func(Stream)

// ============================================================================
//                              子系统接口（简化版，详细定义在各自包中）
// ============================================================================

// DiscoveryService 发现服务接口
// 详细定义见 pkg/interfaces/discovery/
type DiscoveryService interface {
	// FindPeer 查找节点地址
	FindPeer(ctx context.Context, id NodeID) ([]Address, error)

	// Announce 通告本节点
	Announce(ctx context.Context, namespace string) error

	// RefreshAnnounce 刷新地址发布
	// 当节点地址变化时（如发现公网地址）调用此方法重新发布到网络
	// addrs: 当前要通告的地址列表
	RefreshAnnounce(addrs []Address)

	// OnPeerDiscovered 注册节点发现回调
	// 当发现新节点时触发，应用可用于主动连接
	OnPeerDiscovered(callback func(PeerInfo))

	// DiscoverPeers 发现指定命名空间中的节点
	// 用于 relay 发现等场景，通过 DHT GET_PROVIDERS 获取候选节点
	// 返回一个 channel，调用方需遍历消费；channel 会在发现结束后关闭
	DiscoverPeers(ctx context.Context, namespace string) (<-chan PeerInfo, error)

	// GetBootstrapPeers 获取当前配置的引导节点列表
	GetBootstrapPeers(ctx context.Context) ([]PeerInfo, error)

	// AddBootstrapPeer 运行时添加引导节点
	AddBootstrapPeer(peer PeerInfo)

	// RemoveBootstrapPeer 运行时移除引导节点
	RemoveBootstrapPeer(id NodeID)

	// Start 启动发现服务
	Start(ctx context.Context) error

	// Stop 停止发现服务
	Stop() error

	// =========================================================================
	// 入网状态机 (REQ-DISC-002)
	// =========================================================================

	// State 返回当前入网状态
	State() DiscoveryState

	// SetOnStateChanged 注册状态变更回调
	SetOnStateChanged(callback func(StateChangeEvent))

	// WaitReady 等待服务就绪（阻塞）
	WaitReady(ctx context.Context) error

	// =========================================================================
	// 连接通知（DHT 引导重试支持）
	// =========================================================================

	// NotifyPeerConnected 通知新连接建立
	//
	// 当 Endpoint 新连接建立时调用，用于：
	// - 将连接的节点加入 DHT 路由表
	// - 触发 seed 节点的 DHT 引导重试
	NotifyPeerConnected(nodeID NodeID, addrs []string)

	// NotifyPeerDisconnected 通知连接断开
	NotifyPeerDisconnected(nodeID NodeID)
}

// PeerInfo 节点信息（用于发现回调）
//
// 注意: 此类型是 types.PeerInfo 的副本，用于 endpoint 包内部使用
// 以避免 endpoint 包对 types 包的直接依赖（保持接口包的独立性）
type PeerInfo struct {
	// ID 节点唯一标识符
	ID NodeID
	// Addrs 节点地址列表（Multiaddr 字符串格式）
	Addrs []string
}

// DiscoveryState 发现服务状态
// REQ-DISC-002: 入网应存在可解释的状态机
type DiscoveryState int

const (
	// StateNotStarted 服务未启动
	StateNotStarted DiscoveryState = iota
	// StateBootstrapping 正在连接引导节点
	StateBootstrapping
	// StateConnected 已连接至少一个节点（可查询 DHT）
	StateConnected
	// StateDiscoverable 可被其他节点发现
	StateDiscoverable
	// StateFailed 入网失败
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

// NATService NAT 服务接口
// 详细定义见 pkg/interfaces/nat/
type NATService interface {
	// GetExternalAddress 获取外部地址
	GetExternalAddress() (Address, error)

	// NATType 返回 NAT 类型
	NATType() NATType

	// MapPort 映射端口
	MapPort(protocol string, internalPort, externalPort int) error
}

// RelayClient 中继客户端接口（类型别名）
//
// 指向 relayif.RelayClient，统一全仓只使用 pkg/interfaces/relay 定义的接口。
// Connect 返回 transport.Conn（非 endpoint.Connection），这是传输层语义。
type RelayClient = relayif.RelayClient

// Reservation 中继预留接口（类型别名）
//
// 指向 relayif.Reservation，统一全仓只使用 pkg/interfaces/relay 定义的接口。
type Reservation = relayif.Reservation

// AddressBook 地址簿接口
// 详细定义见 pkg/interfaces/address/
type AddressBook interface {
	// Add 添加地址
	Add(nodeID NodeID, addrs ...Address)

	// Get 获取地址
	Get(nodeID NodeID) []Address

	// Remove 移除节点的所有地址
	Remove(nodeID NodeID)

	// Peers 返回所有已知节点
	Peers() []NodeID
}

// 注意：EventBus 已于 2025-12-20 删除
// 原因：从未实现，当前系统使用回调函数模式（如 OnUpgraded callback）
// 如需事件系统，建议使用回调注册或 channel 机制

// ============================================================================
//                              诊断报告 (REQ-OPS-001)
// ============================================================================

// DiagnosticReport 统一诊断报告
//
// REQ-OPS-001: 关键状态可观测且有统一诊断入口
//
// 聚合所有子系统的状态信息，便于运维诊断和调试。
type DiagnosticReport struct {
	// Timestamp 报告生成时间
	Timestamp time.Time `json:"timestamp"`

	// Node 节点基本信息
	Node NodeDiagnostics `json:"node"`

	// Connections 连接状态
	Connections ConnectionDiagnostics `json:"connections"`

	// Addresses 地址状态
	Addresses AddressDiagnostics `json:"addresses"`

	// Discovery 发现服务状态
	Discovery DiscoveryDiagnostics `json:"discovery"`

	// NAT NAT 状态
	NAT NATDiagnostics `json:"nat"`

	// Relay 中继状态
	Relay RelayDiagnostics `json:"relay"`

	// Realm Realm 状态
	Realm RealmDiagnostics `json:"realm"`
}

// NodeDiagnostics 节点诊断信息
type NodeDiagnostics struct {
	// ID 节点 ID
	ID string `json:"id"`
	// IDShort 节点 ID 短格式
	IDShort string `json:"id_short"`
	// PublicKeyType 公钥类型
	PublicKeyType string `json:"public_key_type"`
	// Uptime 运行时长
	Uptime time.Duration `json:"uptime"`
	// StartedAt 启动时间
	StartedAt time.Time `json:"started_at"`
}

// ConnectionDiagnostics 连接诊断信息
type ConnectionDiagnostics struct {
	// Total 总连接数
	Total int `json:"total"`
	// Inbound 入站连接数
	Inbound int `json:"inbound"`
	// Outbound 出站连接数
	Outbound int `json:"outbound"`
	// Peers 已连接的节点 ID 列表
	Peers []string `json:"peers"`
	// PathStats 连接路径统计（直连/打洞/中继）
	PathStats ConnectionPathStats `json:"path_stats"`
	// FailureStats 连接失败原因分类统计
	FailureStats map[string]int64 `json:"failure_stats,omitempty"`
}

// ConnectionPathStats 连接路径统计
type ConnectionPathStats struct {
	// Direct 直连数量
	Direct int `json:"direct"`
	// HolePunched 打洞成功数量
	HolePunched int `json:"hole_punched"`
	// Relayed 通过中继数量
	Relayed int `json:"relayed"`
}

// AddressDiagnostics 地址诊断信息
type AddressDiagnostics struct {
	// ListenAddrs 监听地址
	ListenAddrs []string `json:"listen_addrs"`
	// AdvertisedAddrs 通告地址
	AdvertisedAddrs []string `json:"advertised_addrs"`
	// VerifiedDirectAddrs 已验证直连地址
	VerifiedDirectAddrs []string `json:"verified_direct_addrs"`
	// ShareableAddrs 可分享地址（Full Address）
	ShareableAddrs []string `json:"shareable_addrs"`
}

// DiscoveryDiagnostics 发现服务诊断信息
type DiscoveryDiagnostics struct {
	// State 入网状态
	State string `json:"state"`
	// StateReady 是否就绪
	StateReady bool `json:"state_ready"`
	// BootstrapPeers 引导节点数量
	BootstrapPeers int `json:"bootstrap_peers"`
	// KnownPeers 已知节点数量
	KnownPeers int `json:"known_peers"`
	// DHTMode DHT 模式
	DHTMode string `json:"dht_mode,omitempty"`
	// DHTRoutingTableSize DHT 路由表大小
	DHTRoutingTableSize int `json:"dht_routing_table_size,omitempty"`
}

// NATDiagnostics NAT 诊断信息
type NATDiagnostics struct {
	// Type NAT 类型
	Type string `json:"type"`
	// ExternalAddr 外部地址
	ExternalAddr string `json:"external_addr,omitempty"`
	// PortMappingAvailable 是否可用端口映射
	PortMappingAvailable bool `json:"port_mapping_available"`
}

// RelayDiagnostics 中继诊断信息
type RelayDiagnostics struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// ReservedRelays 已预留的中继数量
	ReservedRelays int `json:"reserved_relays"`
	// RelayAddrs 中继地址列表
	RelayAddrs []string `json:"relay_addrs,omitempty"`
	// ServerStats 中继服务器统计（仅当作为 Relay Server 时）
	ServerStats *RelayServerStats `json:"server_stats,omitempty"`
}

// RelayServerStats 中继服务器统计信息
type RelayServerStats struct {
	// BytesRelayed 已中继字节数
	BytesRelayed int64 `json:"bytes_relayed"`
	// ActiveCircuits 活跃电路数
	ActiveCircuits int `json:"active_circuits"`
	// ActiveReservations 活跃预留数
	ActiveReservations int `json:"active_reservations"`
	// TotalCircuits 总电路数
	TotalCircuits int64 `json:"total_circuits"`
	// TotalReservations 总预留数
	TotalReservations int64 `json:"total_reservations"`
	// RateLimitHits 限流命中次数
	RateLimitHits int64 `json:"rate_limit_hits"`
	// RejectionStats 拒绝原因统计
	RejectionStats map[string]int64 `json:"rejection_stats,omitempty"`
}

// RealmDiagnostics Realm 诊断信息
type RealmDiagnostics struct {
	// CurrentRealm 当前 Realm
	CurrentRealm string `json:"current_realm,omitempty"`
	// IsMember 是否已加入 Realm
	IsMember bool `json:"is_member"`
	// MemberCount 已知成员数量
	MemberCount int `json:"member_count,omitempty"`
	// PSKStats PSK 验证统计
	PSKStats *PSKStats `json:"psk_stats,omitempty"`
	// TopicStats Topic 统计
	TopicStats []TopicStats `json:"topic_stats,omitempty"`
	// RelayReservations Realm Relay 预留数
	RelayReservations int `json:"relay_reservations,omitempty"`
}

// PSKStats PSK 验证统计
type PSKStats struct {
	// VerifySuccess 验证成功次数
	VerifySuccess int64 `json:"verify_success"`
	// VerifyFailure 验证失败次数
	VerifyFailure int64 `json:"verify_failure"`
}

// TopicStats Topic 统计信息
type TopicStats struct {
	// Topic Topic 名称
	Topic string `json:"topic"`
	// Peers 订阅该 Topic 的节点数
	Peers int `json:"peers"`
	// MessagesPublished 已发布消息数
	MessagesPublished int64 `json:"messages_published"`
	// MessagesReceived 已接收消息数
	MessagesReceived int64 `json:"messages_received"`
}

// Diagnostics 诊断接口
//
// REQ-OPS-001: 关键状态可观测且有统一诊断入口
type Diagnostics interface {
	// DiagnosticReport 生成诊断报告
	//
	// 聚合所有子系统的状态信息，返回统一格式的诊断报告。
	// 可用于运维监控、故障排查、健康检查等场景。
	//
	// 示例:
	//   report := endpoint.DiagnosticReport()
	//   fmt.Printf("连接数: %d\n", report.Connections.Total)
	//   fmt.Printf("入网状态: %s\n", report.Discovery.State)
	DiagnosticReport() DiagnosticReport
}

// ============================================================================
//                              重试配置
// ============================================================================

// RetryConfig 连接重试配置
//
// 用于 ConnectWithRetry 方法的重试策略配置。
// 采用指数退避算法，每次重试等待时间翻倍，直到达到最大退避时间。
type RetryConfig struct {
	// MaxRetries 最大重试次数
	// 0 表示无限重试（直到 context 取消）
	MaxRetries int

	// InitialBackoff 初始退避时间
	// 第一次重试前的等待时间
	InitialBackoff time.Duration

	// MaxBackoff 最大退避时间
	// 退避时间不会超过此值
	MaxBackoff time.Duration
}

// DefaultRetryConfig 返回默认重试配置
//
// 默认值：
//   - MaxRetries: 5
//   - InitialBackoff: 100ms
//   - MaxBackoff: 30s
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
	}
}
