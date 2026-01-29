// Package peerstore 实现节点信息存储
package peerstore

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/peerstore/nodedb"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/addrbook"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/keybook"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/metadata"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/protobook"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/peerstore")

// 确保实现了接口
var _ pkgif.Peerstore = (*Peerstore)(nil)

// addrBookInterface 地址簿接口
type addrBookInterface interface {
	AddAddr(types.PeerID, types.Multiaddr, time.Duration)
	AddAddrs(types.PeerID, []types.Multiaddr, time.Duration)
	SetAddr(types.PeerID, types.Multiaddr, time.Duration)
	SetAddrs(types.PeerID, []types.Multiaddr, time.Duration)
	UpdateAddrs(types.PeerID, time.Duration, time.Duration)
	Addrs(types.PeerID) []types.Multiaddr
	AddrStream(context.Context, types.PeerID) <-chan types.Multiaddr
	ClearAddrs(types.PeerID)
	PeersWithAddrs() []types.PeerID
	AddAddrsWithSource(types.PeerID, []addrbook.AddressWithSource)
	SetAddrsWithSource(types.PeerID, []addrbook.AddressWithSource)
	AddrsWithSource(types.PeerID) []addrbook.AddressWithSource
	GetAddrSource(types.PeerID, types.Multiaddr) addrbook.AddressSource
	ResetTemporaryAddrs() int
	GCNow()
	Close() error
	// 
	ReduceAddrsTTL(types.PeerID, time.Duration) int
}

// keyBookInterface 密钥簿接口
type keyBookInterface interface {
	PubKey(types.PeerID) (pkgif.PublicKey, error)
	AddPubKey(types.PeerID, pkgif.PublicKey) error
	PrivKey(types.PeerID) (pkgif.PrivateKey, error)
	AddPrivKey(types.PeerID, pkgif.PrivateKey) error
	PeersWithKeys() []types.PeerID
	RemovePeer(types.PeerID)
}

// protoBookInterface 协议簿接口
type protoBookInterface interface {
	GetProtocols(types.PeerID) ([]types.ProtocolID, error)
	AddProtocols(types.PeerID, ...types.ProtocolID) error
	SetProtocols(types.PeerID, ...types.ProtocolID) error
	RemoveProtocols(types.PeerID, ...types.ProtocolID) error
	SupportsProtocols(types.PeerID, ...types.ProtocolID) ([]types.ProtocolID, error)
	FirstSupportedProtocol(types.PeerID, ...types.ProtocolID) (types.ProtocolID, error)
	RemovePeer(types.PeerID)
}

// metadataInterface 元数据接口
type metadataInterface interface {
	Get(types.PeerID, string) (interface{}, error)
	Put(types.PeerID, string, interface{}) error
	RemovePeer(types.PeerID)
}

// Peerstore 节点信息存储（彻底重构：集成 nodedb）
type Peerstore struct {
	mu sync.RWMutex

	// 使用接口支持内存和持久化两种实现
	addrBook  addrBookInterface
	keyBook   keyBookInterface
	protoBook protoBookInterface
	metadata  metadataInterface

	// 保留对持久化实现的引用（用于特殊操作）
	persistentAddrBook *addrbook.PersistentAddrBook
	persistentKeyBook  *keybook.PersistentKeyBook
	persistentProto    *protobook.PersistentProtoBook
	persistentMeta     *metadata.PersistentMetadataStore

	// 彻底重构：集成 nodedb 节点缓存
	nodeDB nodedb.NodeDB

	closed bool
}

// NewPeerstore 创建新的 Peerstore（彻底重构：集成 nodedb）
//
// 注意：这是基础实现，用于测试或简单场景。
// 生产环境应使用 Fx 模块，它会自动配置 BadgerDB 持久化存储。
func NewPeerstore() *Peerstore {
	return &Peerstore{
		addrBook:  addrbook.New(),
		keyBook:   keybook.New(),
		protoBook: protobook.New(),
		metadata:  metadata.New(),
		nodeDB:    nodedb.NewMemoryDB(nodedb.DefaultConfig()), // 彻底重构：默认启用 nodedb
		closed:    false,
	}
}

// ========== AddrBook 方法 ==========

// AddAddr 添加单个地址
func (ps *Peerstore) AddAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration) {
	ps.addrBook.AddAddr(peerID, addr, ttl)
}

// AddAddrs 添加节点地址
func (ps *Peerstore) AddAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration) {
	ps.addrBook.AddAddrs(peerID, addrs, ttl)
}

// SetAddr 设置单个地址
func (ps *Peerstore) SetAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration) {
	ps.addrBook.SetAddr(peerID, addr, ttl)
}

// SetAddrs 设置节点地址（覆盖现有）
func (ps *Peerstore) SetAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration) {
	ps.addrBook.SetAddrs(peerID, addrs, ttl)
}

// UpdateAddrs 更新地址 TTL
func (ps *Peerstore) UpdateAddrs(peerID types.PeerID, oldTTL time.Duration, newTTL time.Duration) {
	ps.addrBook.UpdateAddrs(peerID, oldTTL, newTTL)
}

// Addrs 获取节点地址
func (ps *Peerstore) Addrs(peerID types.PeerID) []types.Multiaddr {
	return ps.addrBook.Addrs(peerID)
}

// AddrStream 返回节点地址更新的通道
func (ps *Peerstore) AddrStream(ctx context.Context, peerID types.PeerID) <-chan types.Multiaddr {
	return ps.addrBook.AddrStream(ctx, peerID)
}

// ClearAddrs 清除节点地址
func (ps *Peerstore) ClearAddrs(peerID types.PeerID) {
	ps.addrBook.ClearAddrs(peerID)
}

// PeersWithAddrs 返回拥有地址的节点列表
func (ps *Peerstore) PeersWithAddrs() []types.PeerID {
	return ps.addrBook.PeersWithAddrs()
}

// ========== 带来源的地址方法（扩展） ==========

// AddAddrsWithSource 添加带来源的地址
func (ps *Peerstore) AddAddrsWithSource(peerID types.PeerID, addrs []AddressWithSource) {
	ps.addrBook.AddAddrsWithSource(peerID, addrs)
}

// SetAddrsWithSource 设置带来源的地址（覆盖现有）
func (ps *Peerstore) SetAddrsWithSource(peerID types.PeerID, addrs []AddressWithSource) {
	ps.addrBook.SetAddrsWithSource(peerID, addrs)
}

// AddrsWithSource 获取带来源的地址
func (ps *Peerstore) AddrsWithSource(peerID types.PeerID) []AddressWithSource {
	return ps.addrBook.AddrsWithSource(peerID)
}

// GetAddrSource 获取特定地址的来源
func (ps *Peerstore) GetAddrSource(peerID types.PeerID, addr types.Multiaddr) AddressSource {
	return ps.addrBook.GetAddrSource(peerID, addr)
}

// ReduceAddrsTTL 降低所有地址的 TTL
//
// 
// 加速过期地址的清理。
func (ps *Peerstore) ReduceAddrsTTL(peerID types.PeerID, maxTTL time.Duration) int {
	return ps.addrBook.ReduceAddrsTTL(peerID, maxTTL)
}

// ========== KeyBook 方法 ==========

// PubKey 获取节点公钥
func (ps *Peerstore) PubKey(peerID types.PeerID) (pkgif.PublicKey, error) {
	return ps.keyBook.PubKey(peerID)
}

// AddPubKey 添加节点公钥
func (ps *Peerstore) AddPubKey(peerID types.PeerID, pubKey pkgif.PublicKey) error {
	return ps.keyBook.AddPubKey(peerID, pubKey)
}

// PrivKey 获取本地节点私钥
func (ps *Peerstore) PrivKey(peerID types.PeerID) (pkgif.PrivateKey, error) {
	return ps.keyBook.PrivKey(peerID)
}

// AddPrivKey 添加本地节点私钥
func (ps *Peerstore) AddPrivKey(peerID types.PeerID, privKey pkgif.PrivateKey) error {
	return ps.keyBook.AddPrivKey(peerID, privKey)
}

// PeersWithKeys 返回拥有密钥的节点列表
func (ps *Peerstore) PeersWithKeys() []types.PeerID {
	return ps.keyBook.PeersWithKeys()
}

// ========== ProtoBook 方法 ==========

// GetProtocols 获取节点支持的协议
func (ps *Peerstore) GetProtocols(peerID types.PeerID) ([]types.ProtocolID, error) {
	return ps.protoBook.GetProtocols(peerID)
}

// AddProtocols 添加节点支持的协议
func (ps *Peerstore) AddProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	return ps.protoBook.AddProtocols(peerID, protocols...)
}

// SetProtocols 设置节点支持的协议（覆盖）
func (ps *Peerstore) SetProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	return ps.protoBook.SetProtocols(peerID, protocols...)
}

// RemoveProtocols 移除节点支持的协议
func (ps *Peerstore) RemoveProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	return ps.protoBook.RemoveProtocols(peerID, protocols...)
}

// SupportsProtocols 检查节点是否支持指定协议
func (ps *Peerstore) SupportsProtocols(peerID types.PeerID, protocols ...types.ProtocolID) ([]types.ProtocolID, error) {
	return ps.protoBook.SupportsProtocols(peerID, protocols...)
}

// FirstSupportedProtocol 返回首个支持的协议
func (ps *Peerstore) FirstSupportedProtocol(peerID types.PeerID, protocols ...types.ProtocolID) (types.ProtocolID, error) {
	return ps.protoBook.FirstSupportedProtocol(peerID, protocols...)
}

// ========== PeerMetadata 方法 ==========

// Get 获取元数据
func (ps *Peerstore) Get(peerID types.PeerID, key string) (interface{}, error) {
	return ps.metadata.Get(peerID, key)
}

// Put 存储元数据
func (ps *Peerstore) Put(peerID types.PeerID, key string, val interface{}) error {
	return ps.metadata.Put(peerID, key, val)
}

// ========== Peerstore 方法 ==========

// Peers 返回所有已知节点 ID
func (ps *Peerstore) Peers() []types.PeerID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// 合并所有子簿的节点
	peerSet := make(map[types.PeerID]struct{})

	for _, p := range ps.addrBook.PeersWithAddrs() {
		peerSet[p] = struct{}{}
	}
	for _, p := range ps.keyBook.PeersWithKeys() {
		peerSet[p] = struct{}{}
	}

	peers := make([]types.PeerID, 0, len(peerSet))
	for p := range peerSet {
		peers = append(peers, p)
	}

	return peers
}

// PeerInfo 返回指定节点的完整信息
func (ps *Peerstore) PeerInfo(peerID types.PeerID) types.PeerInfo {
	return types.PeerInfo{
		ID:    peerID,
		Addrs: ps.Addrs(peerID),
	}
}

// RemovePeer 移除节点信息（除地址外）
func (ps *Peerstore) RemovePeer(peerID types.PeerID) {
	ps.keyBook.RemovePeer(peerID)
	ps.protoBook.RemovePeer(peerID)
	ps.metadata.RemovePeer(peerID)
}

// ResetStates 重置端点状态
//
// 参考 iroh-main: 网络变化时清除已缓存的最佳路径信息，
// 触发下次连接时重新探测。
//
// 实现：清除临时地址并执行 GC，保留永久地址。
func (ps *Peerstore) ResetStates(_ context.Context) error {
	ps.mu.RLock()
	if ps.closed {
		ps.mu.RUnlock()
		return ErrClosed
	}
	ps.mu.RUnlock()

	logger.Debug("重置端点状态")
	
	// 清除临时地址（短 TTL）
	removed := ps.addrBook.ResetTemporaryAddrs()
	
	// 立即执行 GC
	ps.addrBook.GCNow()
	
	logger.Info("端点状态已重置", "removedAddrs", removed)
	return nil
}

// ============================================================================
//             彻底重构：nodedb 集成 - 节点缓存 API
// ============================================================================

// UpdateNodeRecord 更新节点记录到缓存
//
// 用于持久化节点信息，包括 IP、端口、最后活跃时间等。
// 重启后可通过 QuerySeeds 恢复已知节点。
func (ps *Peerstore) UpdateNodeRecord(peerID types.PeerID, addrs []string) error {
	if ps.nodeDB == nil {
		return nil
	}
	
	record := &nodedb.NodeRecord{
		ID:       string(peerID),
		Addrs:    addrs,
		LastSeen: time.Now(),
	}
	return ps.nodeDB.UpdateNode(record)
}

// GetNodeRecord 获取节点缓存记录
func (ps *Peerstore) GetNodeRecord(peerID types.PeerID) *nodedb.NodeRecord {
	if ps.nodeDB == nil {
		return nil
	}
	return ps.nodeDB.GetNode(string(peerID))
}

// RemoveNodeRecord 删除节点缓存记录
func (ps *Peerstore) RemoveNodeRecord(peerID types.PeerID) error {
	if ps.nodeDB == nil {
		return nil
	}
	return ps.nodeDB.RemoveNode(string(peerID))
}

// QuerySeeds 查询种子节点
//
// 返回最近活跃的节点列表，用于重启后恢复网络连接。
//
// 参数:
//   - count: 返回的最大节点数
//   - maxAge: 节点的最大年龄（超过此时间的节点不返回）
func (ps *Peerstore) QuerySeeds(count int, maxAge time.Duration) []*pkgif.NodeRecord {
	if ps.nodeDB == nil {
		return nil
	}
	seeds := ps.nodeDB.QuerySeeds(count, maxAge)
	if seeds == nil {
		return nil
	}
	
	// 转换为接口类型
	result := make([]*pkgif.NodeRecord, len(seeds))
	for i, s := range seeds {
		result[i] = &pkgif.NodeRecord{
			ID:          s.ID,
			Addrs:       s.Addrs,
			LastSeen:    s.LastSeen,
			LastPong:    s.LastPong,
			FailedDials: s.FailedDials,
		}
	}
	return result
}

// UpdateDialAttempt 更新拨号尝试结果
//
// 跟踪节点的连接成功/失败状态，用于优化连接策略。
func (ps *Peerstore) UpdateDialAttempt(peerID types.PeerID, success bool) error {
	if ps.nodeDB == nil {
		return nil
	}
	return ps.nodeDB.UpdateDialAttempt(string(peerID), success)
}

// UpdateLastPong 更新最后 Pong 时间
//
// 记录节点的最后响应时间，用于判断节点活跃度。
func (ps *Peerstore) UpdateLastPong(peerID types.PeerID, t time.Time) error {
	if ps.nodeDB == nil {
		return nil
	}
	return ps.nodeDB.UpdateLastPong(string(peerID), t)
}

// LastPongReceived 获取节点最后 Pong 时间
func (ps *Peerstore) LastPongReceived(peerID types.PeerID) time.Time {
	if ps.nodeDB == nil {
		return time.Time{}
	}
	return ps.nodeDB.LastPongReceived(string(peerID))
}

// NodeDBSize 返回节点缓存大小
func (ps *Peerstore) NodeDBSize() int {
	if ps.nodeDB == nil {
		return 0
	}
	return ps.nodeDB.Size()
}

// NodeDB 返回底层节点数据库（高级用法）
func (ps *Peerstore) NodeDB() nodedb.NodeDB {
	return ps.nodeDB
}

// SetNodeDB 设置节点数据库（用于依赖注入）
func (ps *Peerstore) SetNodeDB(db nodedb.NodeDB) {
	ps.nodeDB = db
}

// AddrBook 返回底层地址簿
//
// 用于访问 AddrBook 的扩展功能（如按来源清除地址）。
// 注意：返回的是内部实现，可能是 *addrbook.AddrBook 或 *addrbook.PersistentAddrBook。
func (ps *Peerstore) AddrBook() *addrbook.AddrBook {
	// 尝试类型断言获取 *addrbook.AddrBook
	if ab, ok := ps.addrBook.(*addrbook.AddrBook); ok {
		return ab
	}
	// 如果是 PersistentAddrBook，返回 nil（PersistentAddrBook 有单独的处理）
	return nil
}

// Close 关闭存储（彻底重构：关闭 nodeDB）
func (ps *Peerstore) Close() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.closed {
		return ErrClosed
	}

	logger.Info("正在关闭 Peerstore")
	ps.closed = true

	// 关闭 AddrBook（包括 GC）
	if err := ps.addrBook.Close(); err != nil {
		logger.Warn("关闭 AddrBook 失败", "error", err)
		return err
	}

	// 彻底重构：关闭 nodeDB
	if ps.nodeDB != nil {
		if err := ps.nodeDB.Close(); err != nil {
			logger.Warn("关闭 NodeDB 失败", "error", err)
		}
	}

	logger.Info("Peerstore 已关闭")
	return nil
}
