// Package connmgr 定义连接管理相关接口
package connmgr

import (
	"net"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              ConnectionGater 接口
// ============================================================================

// ConnectionGater 连接门控接口
//
// 提供主动连接控制能力，包括黑名单管理和连接拦截。
// 参考 libp2p 的 ConnectionGater 设计简化版。
//
// ConnectionGater 是主动式的，ConnectionManager 倾向于被动式。
// Gater 在连接建立的各个阶段进行拦截检查。
//
// 使用示例:
//
//	gater := connmgr.NewConnectionGater(nil) // 无持久化
//
//	// 阻止恶意节点
//	gater.BlockPeer(maliciousPeerID)
//
//	// 阻止整个 IP 段
//	_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")
//	gater.BlockSubnet(ipnet)
//
//	// 在连接建立时检查
//	if !gater.InterceptPeerDial(peerID) {
//	    return ErrBlocked
//	}
type ConnectionGater interface {
	// ==================== Peer 级别阻止 ====================

	// BlockPeer 阻止指定节点
	//
	// 将节点加入黑名单，后续与该节点的连接请求将被拒绝。
	// 注意：不会自动关闭与该节点的现有连接。
	BlockPeer(nodeID types.NodeID) error

	// UnblockPeer 解除节点阻止
	//
	// 将节点从黑名单移除。
	UnblockPeer(nodeID types.NodeID) error

	// ListBlockedPeers 列出所有被阻止的节点
	ListBlockedPeers() []types.NodeID

	// IsBlocked 检查节点是否被阻止
	IsBlocked(nodeID types.NodeID) bool

	// ==================== IP 地址级别阻止 ====================

	// BlockAddr 阻止指定 IP 地址
	//
	// 将 IP 地址加入黑名单，来自该地址的连接将被拒绝。
	BlockAddr(ip net.IP) error

	// UnblockAddr 解除 IP 地址阻止
	UnblockAddr(ip net.IP) error

	// ListBlockedAddrs 列出所有被阻止的 IP 地址
	ListBlockedAddrs() []net.IP

	// IsAddrBlocked 检查 IP 地址是否被阻止
	IsAddrBlocked(ip net.IP) bool

	// ==================== 子网级别阻止 ====================

	// BlockSubnet 阻止指定子网
	//
	// 将整个子网加入黑名单，来自该子网的连接将被拒绝。
	BlockSubnet(ipnet *net.IPNet) error

	// UnblockSubnet 解除子网阻止
	UnblockSubnet(ipnet *net.IPNet) error

	// ListBlockedSubnets 列出所有被阻止的子网
	ListBlockedSubnets() []*net.IPNet

	// ==================== 连接拦截点 ====================

	// InterceptPeerDial 拦截出站连接
	//
	// 在拨号连接前调用，检查是否允许连接到该节点。
	// 返回 true 表示允许，false 表示拒绝。
	InterceptPeerDial(nodeID types.NodeID) bool

	// InterceptAccept 拦截入站连接
	//
	// 在接受连接时调用，检查是否允许来自该地址的连接。
	// remoteAddr 是远程地址字符串（如 "192.168.1.100:8080"）。
	// 返回 true 表示允许，false 表示拒绝。
	InterceptAccept(remoteAddr string) bool

	// InterceptSecured 拦截已认证连接
	//
	// 在安全握手完成后调用，此时已知对端身份。
	// 可用于入站连接的节点 ID 检查。
	// 返回 true 表示允许，false 表示拒绝。
	InterceptSecured(direction types.Direction, nodeID types.NodeID) bool

	// ==================== 管理 ====================

	// Clear 清除所有阻止规则
	Clear()

	// Stats 返回阻止统计
	Stats() GaterStats
}

// ============================================================================
//                              GaterStats 统计
// ============================================================================

// GaterStats 门控统计信息
type GaterStats struct {
	// BlockedPeers 被阻止的节点数量
	BlockedPeers int

	// BlockedAddrs 被阻止的 IP 地址数量
	BlockedAddrs int

	// BlockedSubnets 被阻止的子网数量
	BlockedSubnets int

	// InterceptedDials 拦截的出站连接次数
	InterceptedDials int64

	// InterceptedAccepts 拦截的入站连接次数
	InterceptedAccepts int64
}

// ============================================================================
//                              GaterStore 持久化接口
// ============================================================================

// GaterStore 门控规则持久化存储接口
//
// 可选接口，用于持久化阻止规则。
// 如果不提供，规则仅保存在内存中。
type GaterStore interface {
	// SavePeer 保存被阻止的节点
	SavePeer(nodeID types.NodeID) error

	// DeletePeer 删除被阻止的节点
	DeletePeer(nodeID types.NodeID) error

	// LoadPeers 加载所有被阻止的节点
	LoadPeers() ([]types.NodeID, error)

	// SaveAddr 保存被阻止的 IP 地址
	SaveAddr(ip net.IP) error

	// DeleteAddr 删除被阻止的 IP 地址
	DeleteAddr(ip net.IP) error

	// LoadAddrs 加载所有被阻止的 IP 地址
	LoadAddrs() ([]net.IP, error)

	// SaveSubnet 保存被阻止的子网
	SaveSubnet(ipnet *net.IPNet) error

	// DeleteSubnet 删除被阻止的子网
	DeleteSubnet(ipnet *net.IPNet) error

	// LoadSubnets 加载所有被阻止的子网
	LoadSubnets() ([]*net.IPNet, error)
}

// ============================================================================
//                              GaterConfig 配置
// ============================================================================

// GaterConfig 连接门控配置
type GaterConfig struct {
	// Enabled 是否启用门控
	// 默认 true
	Enabled bool

	// AutoCloseBlocked 阻止节点时是否自动关闭现有连接
	// 默认 false
	AutoCloseBlocked bool

	// Store 持久化存储（可选）
	Store GaterStore
}

// DefaultGaterConfig 返回默认配置
func DefaultGaterConfig() GaterConfig {
	return GaterConfig{
		Enabled:          true,
		AutoCloseBlocked: false,
		Store:            nil,
	}
}

// ============================================================================
//                              事件
// ============================================================================

// 门控事件类型
const (
	// EventPeerBlocked 节点被阻止
	EventPeerBlocked = "gater.peer_blocked"

	// EventPeerUnblocked 节点被解除阻止
	EventPeerUnblocked = "gater.peer_unblocked"

	// EventAddrBlocked IP 地址被阻止
	EventAddrBlocked = "gater.addr_blocked"

	// EventAddrUnblocked IP 地址被解除阻止
	EventAddrUnblocked = "gater.addr_unblocked"

	// EventConnectionRejected 连接被拒绝
	EventConnectionRejected = "gater.connection_rejected"
)

// PeerBlockedEvent 节点阻止事件
type PeerBlockedEvent struct {
	NodeID types.NodeID
}

// Type 返回事件类型
func (e PeerBlockedEvent) Type() string {
	return EventPeerBlocked
}

// ConnectionRejectedEvent 连接拒绝事件
type ConnectionRejectedEvent struct {
	NodeID    types.NodeID
	Addr      string
	Reason    string
	Direction types.Direction
}

// Type 返回事件类型
func (e ConnectionRejectedEvent) Type() string {
	return EventConnectionRejected
}

