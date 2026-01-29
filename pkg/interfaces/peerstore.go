// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Peerstore 接口，管理节点信息存储。
package interfaces

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// Peerstore 定义节点信息存储接口
//
// Peerstore 存储节点的地址、密钥、协议支持和元数据。
type Peerstore interface {
	AddrBook
	KeyBook
	ProtoBook
	PeerMetadata

	// Peers 返回所有已知节点 ID
	Peers() []types.PeerID

	// PeerInfo 返回指定节点的完整信息
	PeerInfo(peerID types.PeerID) types.PeerInfo

	// RemovePeer 移除节点信息（除地址外）
	RemovePeer(peerID types.PeerID)

	// ==================== nodedb 集成（P3 修复完成）====================

	// QuerySeeds 查询种子节点（用于重启后恢复连接）
	//
	// 参数:
	//   - count: 返回的最大节点数
	//   - maxAge: 节点的最大年龄（超过此时间的节点不返回）
	//
	// 返回:
	//   - 种子节点记录列表
	QuerySeeds(count int, maxAge time.Duration) []*NodeRecord

	// UpdateDialAttempt 更新拨号尝试结果
	UpdateDialAttempt(peerID types.PeerID, success bool) error

	// NodeDBSize 返回节点缓存大小
	NodeDBSize() int

	// Close 关闭存储
	Close() error
}

// NodeRecord 节点记录
type NodeRecord struct {
	// ID 节点 ID
	ID string

	// Addrs 节点地址列表
	Addrs []string

	// LastSeen 最后活跃时间
	LastSeen time.Time

	// LastPong 最后 Pong 时间
	LastPong time.Time

	// FailedDials 连续拨号失败次数
	FailedDials int
}

// AddrBook 定义地址簿接口
type AddrBook interface {
	// AddAddr 添加单个地址
	AddAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration)

	// AddAddrs 添加节点地址
	AddAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration)

	// SetAddr 设置单个地址（覆盖现有）
	SetAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration)

	// SetAddrs 设置节点地址（覆盖现有）
	SetAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration)

	// UpdateAddrs 更新地址 TTL
	UpdateAddrs(peerID types.PeerID, oldTTL time.Duration, newTTL time.Duration)

	// Addrs 获取节点地址
	Addrs(peerID types.PeerID) []types.Multiaddr

	// AddrStream 返回节点地址更新的通道
	AddrStream(ctx context.Context, peerID types.PeerID) <-chan types.Multiaddr

	// ClearAddrs 清除节点地址
	ClearAddrs(peerID types.PeerID)

	// PeersWithAddrs 返回拥有地址的节点列表
	PeersWithAddrs() []types.PeerID
}

// KeyBook 定义密钥簿接口
type KeyBook interface {
	// PubKey 获取节点公钥
	PubKey(peerID types.PeerID) (PublicKey, error)

	// AddPubKey 添加节点公钥
	AddPubKey(peerID types.PeerID, pubKey PublicKey) error

	// PrivKey 获取本地节点私钥
	PrivKey(peerID types.PeerID) (PrivateKey, error)

	// AddPrivKey 添加本地节点私钥
	AddPrivKey(peerID types.PeerID, privKey PrivateKey) error

	// PeersWithKeys 返回拥有密钥的节点列表
	PeersWithKeys() []types.PeerID

	// RemovePeer 移除节点密钥
	RemovePeer(peerID types.PeerID)
}

// ProtoBook 定义协议簿接口
type ProtoBook interface {
	// GetProtocols 获取节点支持的协议
	GetProtocols(peerID types.PeerID) ([]types.ProtocolID, error)

	// AddProtocols 添加节点支持的协议
	AddProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error

	// SetProtocols 设置节点支持的协议（覆盖）
	SetProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error

	// RemoveProtocols 移除节点支持的协议
	RemoveProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error

	// SupportsProtocols 检查节点是否支持指定协议
	SupportsProtocols(peerID types.PeerID, protocols ...types.ProtocolID) ([]types.ProtocolID, error)

	// FirstSupportedProtocol 返回首个支持的协议
	FirstSupportedProtocol(peerID types.PeerID, protocols ...types.ProtocolID) (types.ProtocolID, error)

	// RemovePeer 移除节点协议
	RemovePeer(peerID types.PeerID)
}

// PeerMetadata 定义节点元数据存储接口
type PeerMetadata interface {
	// Get 获取元数据
	Get(peerID types.PeerID, key string) (interface{}, error)

	// Put 存储元数据
	Put(peerID types.PeerID, key string, val interface{}) error

	// RemovePeer 移除节点元数据
	RemovePeer(peerID types.PeerID)
}
