// Package relay 定义中继服务相关接口
//
// 中继模块负责通过第三方服务器转发流量，包括：
// - 中继客户端
// - 中继服务器
// - 预留管理
package relay

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/netaddr"
	"github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Dialer 接口（最小依赖）
// ============================================================================

// Dialer 提供 relay client 需要的最小拨号能力
//
// 设计目的：避免 relay ↔ endpoint 的循环依赖。
// RelayClient 只需要连接到 relay server 并开流的能力，不需要完整的 Endpoint 接口。
type Dialer interface {
	// Connect 连接到目标节点
	//
	// 返回的 Connection 支持 OpenStream 用于协议通信。
	Connect(ctx context.Context, nodeID types.NodeID) (Connection, error)

	// ID 返回本地节点 ID
	ID() types.NodeID

	// Discovery 返回发现服务（可选，用于 FindRelays）
	// 如果不支持发现，返回 nil
	Discovery() DiscoveryService
}

// DiscoveryService 发现服务接口（最小版本）
type DiscoveryService interface {
	// DiscoverPeers 发现指定命名空间的节点
	DiscoverPeers(ctx context.Context, namespace string) (<-chan PeerInfo, error)
}

// PeerInfo 发现的节点信息
type PeerInfo struct {
	ID    types.NodeID
	Addrs []string
}

// Connection 表示与远程节点的连接（Dialer 使用的最小接口）
type Connection interface {
	// RemoteID 返回远程节点 ID
	RemoteID() types.NodeID

	// OpenStream 打开一个新流用于协议通信
	OpenStream(ctx context.Context, protocolID string) (Stream, error)

	// Close 关闭连接
	Close() error
}

// Stream 表示连接中的流（Dialer 使用的最小接口）
type Stream interface {
	// Read 读取数据
	Read(p []byte) (n int, err error)

	// Write 写入数据
	Write(p []byte) (n int, err error)

	// Close 关闭流
	Close() error

	// SetDeadline 设置读写超时
	SetDeadline(t time.Time) error

	// SetReadDeadline 设置读超时
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline 设置写超时
	SetWriteDeadline(t time.Time) error

	// IsClosed 检查流是否已关闭
	IsClosed() bool
}

// ============================================================================
//                              RelayClient 接口
// ============================================================================

// RelayClient 中继客户端接口
//
// 用于通过中继服务器连接到其他节点。
type RelayClient interface {
	// Reserve 在中继服务器预留资源
	//
	// 预留成功后，其他节点可以通过中继连接到本节点。
	Reserve(ctx context.Context, relay types.NodeID) (Reservation, error)

	// Connect 通过中继连接到目标节点
	//
	// 建立透明双向隧道，不进行协议白名单检查。
	// 如需协议预检查，请使用 ConnectWithProtocol。
	//
	// 参数:
	//   - relay: 中继服务器节点 ID
	//   - dest: 目标节点 ID
	Connect(ctx context.Context, relay types.NodeID, dest types.NodeID) (transport.Conn, error)

	// ConnectWithProtocol 通过中继连接到目标节点（支持协议预检查）
	//
	// IMPL-1227 协议白名单：
	//   - protocol 非空时，Server 会检查该协议是否在白名单中
	//   - System Relay：只允许 /dep2p/sys/* 协议
	//   - Realm Relay：只允许本 Realm 协议
	//   - protocol 为空时，等同于 Connect（跳过检查）
	//
	// 参数:
	//   - relay: 中继服务器节点 ID
	//   - dest: 目标节点 ID
	//   - protocol: 目标协议（空表示不检查）
	ConnectWithProtocol(ctx context.Context, relay types.NodeID, dest types.NodeID, protocol types.ProtocolID) (transport.Conn, error)

	// Relays 返回已知的中继服务器
	Relays() []types.NodeID

	// AddRelay 添加中继服务器
	AddRelay(relay types.NodeID, addrs []netaddr.Address)

	// RemoveRelay 移除中继服务器
	RemoveRelay(relay types.NodeID)

	// FindRelays 发现中继服务器
	FindRelays(ctx context.Context) ([]types.NodeID, error)
}

// ============================================================================
//                              Reservation 接口
// ============================================================================

// Reservation 中继预留接口
//
// 代表在中继服务器上的资源预留。
type Reservation interface {
	// Relay 返回中继服务器 ID
	Relay() types.NodeID

	// Addrs 返回中继地址
	//
	// 其他节点可以使用这些地址通过中继连接到本节点。
	Addrs() []netaddr.Address

	// Expiry 返回预留过期时间
	Expiry() time.Time

	// Refresh 刷新预留
	Refresh(ctx context.Context) error

	// Cancel 取消预留
	Cancel() error
}

// ============================================================================
//                              RelayServer 接口
// ============================================================================

// RelayServer 中继服务器接口
//
// 提供中继服务，帮助 NAT 后的节点互相连接。
type RelayServer interface {
	// Start 启动中继服务
	Start(ctx context.Context) error

	// Stop 停止中继服务
	Stop() error

	// Stats 返回统计信息
	Stats() RelayStats

	// Reservations 返回当前所有预留
	Reservations() []ReservationInfo

	// Config 返回配置
	Config() ServerConfig
}

// RelayStats 中继统计（类型别名，实际定义在 types 包）
type RelayStats = types.RelayStats

// ReservationInfo 预留信息（类型别名，实际定义在 types 包）
// 注意：Addrs 字段使用 []string 格式
type ReservationInfo = types.ReservationInfo

// ============================================================================
//                              RelayDiscovery 接口
// ============================================================================

// RelayDiscovery 中继发现接口
type RelayDiscovery interface {
	// FindRelays 发现中继服务器
	FindRelays(ctx context.Context, count int) ([]types.NodeID, error)

	// Advertise 通告本节点为中继服务器
	Advertise(ctx context.Context) error
}

// ============================================================================
//                              AutoRelay 接口
// ============================================================================

// AutoRelay 自动中继接口
//
// 自动管理中继连接，确保节点可达性。
type AutoRelay interface {
	// Enable 启用自动中继
	Enable()

	// Disable 禁用自动中继
	Disable()

	// IsEnabled 是否已启用
	IsEnabled() bool

	// Relays 返回当前使用的中继
	Relays() []types.NodeID

	// Status 返回状态
	Status() AutoRelayStatus
}

// AutoRelayStatus 自动中继状态（类型别名，实际定义在 types 包）
type AutoRelayStatus = types.AutoRelayStatus

// ============================================================================
//                              配置
// ============================================================================

// Config 中继客户端配置
type Config struct {
	// EnableClient 启用中继客户端
	EnableClient bool

	// EnableServer 启用中继服务器
	EnableServer bool

	// EnableAutoRelay 启用自动中继
	EnableAutoRelay bool

	// MaxReservations 最大预留数（服务器）
	MaxReservations int

	// MaxCircuits 最大同时连接数（服务器）
	MaxCircuits int

	// ReservationTTL 预留有效期
	ReservationTTL time.Duration

	// ConnectionTimeout 连接超时
	ConnectionTimeout time.Duration

	// StaticRelays 静态中继服务器列表
	StaticRelays []types.NodeID
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		EnableClient:      true,
		EnableServer:      false,
		EnableAutoRelay:   true,
		MaxReservations:   128,
		MaxCircuits:       16,
		ReservationTTL:    time.Hour,
		ConnectionTimeout: 30 * time.Second,
	}
}

// ServerConfig 中继服务器配置
type ServerConfig struct {
	// MaxReservations 最大预留数
	MaxReservations int

	// MaxCircuits 最大同时连接数
	MaxCircuits int

	// MaxCircuitsPerPeer 每节点最大连接数
	MaxCircuitsPerPeer int

	// ReservationTTL 预留有效期
	ReservationTTL time.Duration

	// MaxDataRate 最大数据速率（字节/秒）
	MaxDataRate int64

	// MaxDuration 最大连接时长
	MaxDuration time.Duration
}

// DefaultServerConfig 返回默认服务器配置
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		MaxReservations:    128,
		MaxCircuits:        16,
		MaxCircuitsPerPeer: 4,
		ReservationTTL:     time.Hour,
		MaxDataRate:        1024 * 1024, // 1 MB/s
		MaxDuration:        2 * time.Minute,
	}
}
