package connector

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/peerstore"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/addrbook"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// AddressSource 地址来源（复用 peerstore 定义）
type AddressSource = peerstore.AddressSource

// 地址来源常量
const (
	SourceUnknown    = peerstore.SourceUnknown
	SourceManual     = peerstore.SourceManual
	SourcePeerstore  = peerstore.SourcePeerstore
	SourceMemberList = peerstore.SourceMemberList
	SourceDHT        = peerstore.SourceDHT
	SourceRelay      = peerstore.SourceRelay
)

// ResolveResult 解析结果
type ResolveResult struct {
	// Addrs 解析到的地址列表
	Addrs []types.Multiaddr

	// Source 地址来源
	Source AddressSource

	// Cached 是否来自缓存
	Cached bool
}

// HasAddrs 是否有地址
func (r *ResolveResult) HasAddrs() bool {
	return len(r.Addrs) > 0
}

// AddressResolver 地址解析器
//
// v2.0 统一 Relay 架构：地址发现优先级
//
// 地址发现：
//  1. Peerstore 本地缓存（仅本地缓存来源）
//  2. MemberList（Gossip 同步）
//  3. DHT（先缓存，再回退查询）
//  4. Relay 地址簿回退（先缓存，再回退查询）
//  5. 返回空结果（不是错误，上层可通过节点级 Relay 保底）
type AddressResolver struct {
	peerstore  pkgif.Peerstore
	addrSyncer AddrSyncerInterface

	// v2.0 新增：DHT 回退
	dht               DHTInterface
	enableDHTFallback bool

	// v2.0 新增：Relay 地址簿回退
	addressBookClient     AddressBookClientInterface
	enableRelayFallback   bool
	relayPeerID           string // 当前使用的 Relay PeerID

	// 配置
	queryTimeout time.Duration
	cacheTTL     time.Duration
}

// DHTInterface DHT 接口（用于回退查询）
type DHTInterface interface {
	// FindPeer 查找节点地址
	FindPeer(ctx context.Context, id types.PeerID) (types.PeerInfo, error)
}

// AddrSyncerInterface 地址同步器接口
type AddrSyncerInterface interface {
	SyncFromRelay(peerID types.PeerID, addrs []types.Multiaddr)
	SyncFromDHT(peerID types.PeerID, addrs []types.Multiaddr)
}

// AddressBookClientInterface Relay 地址簿客户端接口
//
// v2.0 新增：用于从 Relay 地址簿查询节点地址
type AddressBookClientInterface interface {
	// Query 查询单个节点的地址信息
	// relayPeerID: Relay 节点 ID
	// targetID: 目标节点 ID
	Query(ctx context.Context, relayPeerID, targetID string) ([]types.Multiaddr, error)
}

// ResolverConfig 解析器配置
type ResolverConfig struct {
	Peerstore    pkgif.Peerstore
	AddrSyncer   AddrSyncerInterface
	QueryTimeout time.Duration
	CacheTTL     time.Duration

	// v2.0 新增：DHT 回退配置
	DHT               DHTInterface
	EnableDHTFallback bool // 默认 true

	// v2.0 新增：Relay 地址簿回退配置
	AddressBookClient   AddressBookClientInterface
	EnableRelayFallback bool   // 默认 false（需要显式启用）
	RelayPeerID         string // 当前使用的 Relay PeerID
}

// DefaultResolverConfig 返回默认配置
func DefaultResolverConfig() ResolverConfig {
	return ResolverConfig{
		QueryTimeout:        5 * time.Second,
		CacheTTL:            15 * time.Minute,
		EnableDHTFallback:   true,  // v2.0 默认启用 DHT 回退
		EnableRelayFallback: false, // Relay 地址簿回退默认禁用
	}
}

// NewAddressResolver 创建地址解析器
func NewAddressResolver(config ResolverConfig) *AddressResolver {
	if config.QueryTimeout <= 0 {
		config.QueryTimeout = 5 * time.Second
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = 15 * time.Minute
	}

	return &AddressResolver{
		peerstore:           config.Peerstore,
		addrSyncer:          config.AddrSyncer,
		dht:                 config.DHT,
		enableDHTFallback:   config.EnableDHTFallback,
		addressBookClient:   config.AddressBookClient,
		enableRelayFallback: config.EnableRelayFallback,
		relayPeerID:         config.RelayPeerID,
		queryTimeout:        config.QueryTimeout,
		cacheTTL:            config.CacheTTL,
	}
}

// Resolve 解析目标节点地址
//
// v2.0 统一 Relay 架构：
//  1. Peerstore 本地缓存（仅本地缓存来源）
//  2. MemberList（Gossip 同步）
//  3. DHT（先缓存，再回退查询）
//  4. Relay 地址簿回退（先缓存，再回退查询）
//  5. 返回空结果（不是错误，上层可通过节点级 Relay 保底）
func (r *AddressResolver) Resolve(ctx context.Context, target string) (*ResolveResult, error) {
	if target == "" {
		return nil, ErrInvalidTarget
	}

	peerID := types.PeerID(target)

	// 1. Peerstore 本地缓存（不含 MemberList/DHT/Relay 来源）
	if addrs := r.getPeerstoreAddrsBySource(peerID, SourcePeerstore, SourceManual, SourceUnknown); len(addrs) > 0 {
		return &ResolveResult{
			Addrs:  addrs,
			Source: SourcePeerstore,
			Cached: true,
		}, nil
	}
	if !r.hasPeerstoreSourceAccess() && r.peerstore != nil {
		if addrs := r.peerstore.Addrs(peerID); len(addrs) > 0 {
			return &ResolveResult{
				Addrs:  addrs,
				Source: SourcePeerstore,
				Cached: true,
			}, nil
		}
	}

	// 2. MemberList（Gossip 同步）
	if addrs := r.getPeerstoreAddrsBySource(peerID, SourceMemberList); len(addrs) > 0 {
		return &ResolveResult{
			Addrs:  addrs,
			Source: SourceMemberList,
			Cached: true,
		}, nil
	}

	// 3. DHT（优先读取 DHT 缓存，其次回退查询）
	if addrs := r.getPeerstoreAddrsBySource(peerID, SourceDHT); len(addrs) > 0 {
		return &ResolveResult{
			Addrs:  addrs,
			Source: SourceDHT,
			Cached: true,
		}, nil
	}
	if r.enableDHTFallback && r.dht != nil {
		result, err := r.queryDHT(ctx, peerID)
		if err == nil && result.HasAddrs() {
			// 将 DHT 查询结果缓存到 Peerstore
			r.cacheAddrs(peerID, result.Addrs, SourceDHT)
			return result, nil
		}
	}

	// 4. Relay 地址簿回退（仅在 DHT 失败时）
	if addrs := r.getPeerstoreAddrsBySource(peerID, SourceRelay); len(addrs) > 0 {
		return &ResolveResult{
			Addrs:  addrs,
			Source: SourceRelay,
			Cached: true,
		}, nil
	}
	if r.enableRelayFallback && r.addressBookClient != nil && r.relayPeerID != "" {
		result, err := r.queryRelayAddressBook(ctx, peerID)
		if err == nil && result.HasAddrs() {
			// 将 Relay 地址簿查询结果缓存到 Peerstore
			r.cacheAddrs(peerID, result.Addrs, SourceRelay)
			return result, nil
		}
	}

	// 无地址，返回空结果（不是错误）
	return &ResolveResult{
		Addrs:  nil,
		Source: SourceUnknown,
		Cached: false,
	}, nil
}

// queryRelayAddressBook 通过 Relay 地址簿查询节点地址
//
// v2.0 新增：DHT 为空时的回退机制
func (r *AddressResolver) queryRelayAddressBook(ctx context.Context, peerID types.PeerID) (*ResolveResult, error) {
	// 创建超时上下文
	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	// 查询 Relay 地址簿
	addrs, err := r.addressBookClient.Query(queryCtx, r.relayPeerID, string(peerID))
	if err != nil {
		return nil, err
	}

	if len(addrs) == 0 {
		return &ResolveResult{
			Addrs:  nil,
			Source: SourceRelay,
			Cached: false,
		}, nil
	}

	return &ResolveResult{
		Addrs:  addrs,
		Source: SourceRelay,
		Cached: false,
	}, nil
}

// queryDHT 通过 DHT 查询节点地址
//
// v2.0 新增：DHT 回退查询
func (r *AddressResolver) queryDHT(ctx context.Context, peerID types.PeerID) (*ResolveResult, error) {
	// 创建超时上下文
	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	// 查询 DHT
	peerInfo, err := r.dht.FindPeer(queryCtx, peerID)
	if err != nil {
		return nil, err
	}

	if len(peerInfo.Addrs) == 0 {
		return &ResolveResult{
			Addrs:  nil,
			Source: SourceDHT,
			Cached: false,
		}, nil
	}

	return &ResolveResult{
		Addrs:  peerInfo.Addrs,
		Source: SourceDHT,
		Cached: false,
	}, nil
}

// cacheAddrs 缓存地址到 Peerstore
func (r *AddressResolver) cacheAddrs(peerID types.PeerID, addrs []types.Multiaddr, source AddressSource) {
	// 优先使用 AddrSyncer（带来源标记）
	if r.addrSyncer != nil {
		switch source {
		case SourceDHT:
			r.addrSyncer.SyncFromDHT(peerID, addrs)
		case SourceRelay:
			r.addrSyncer.SyncFromRelay(peerID, addrs)
		default:
			r.addrSyncer.SyncFromRelay(peerID, addrs)
		}
		return
	}

	// 回退到直接添加
	if r.peerstore != nil {
		// 尝试使用带来源的方法
		if ps, ok := r.peerstore.(*peerstore.Peerstore); ok {
			addrsWithSource := make([]addrbook.AddressWithSource, len(addrs))
			for i, addr := range addrs {
				addrsWithSource[i] = addrbook.AddressWithSource{
					Addr:   addr,
					Source: addrbook.AddressSource(source),
					TTL:    r.cacheTTL,
				}
			}
			ps.AddAddrsWithSource(peerID, addrsWithSource)
		} else {
			r.peerstore.AddAddrs(peerID, addrs, r.cacheTTL)
		}
	}
}

func (r *AddressResolver) hasPeerstoreSourceAccess() bool {
	if r.peerstore == nil {
		return false
	}
	_, ok := r.peerstore.(*peerstore.Peerstore)
	return ok
}

func (r *AddressResolver) getPeerstoreAddrsBySource(peerID types.PeerID, sources ...AddressSource) []types.Multiaddr {
	if r.peerstore == nil {
		return nil
	}

	sourceSet := make(map[AddressSource]struct{}, len(sources))
	for _, source := range sources {
		sourceSet[source] = struct{}{}
	}

	if ps, ok := r.peerstore.(*peerstore.Peerstore); ok {
		addrsWithSource := ps.AddrsWithSource(peerID)
		if len(addrsWithSource) == 0 {
			return nil
		}
		var addrs []types.Multiaddr
		for _, entry := range addrsWithSource {
			if _, ok := sourceSet[entry.Source]; ok {
				addrs = append(addrs, entry.Addr)
			}
		}
		return addrs
	}

	return nil
}

// SetPeerstore 设置 Peerstore
func (r *AddressResolver) SetPeerstore(peerstore pkgif.Peerstore) {
	r.peerstore = peerstore
}

// SetAddrSyncer 设置地址同步器
func (r *AddressResolver) SetAddrSyncer(syncer AddrSyncerInterface) {
	r.addrSyncer = syncer
}

// SetDHT 设置 DHT（用于回退查询）
//
// v2.0 新增：通过 DHT 查询地址
func (r *AddressResolver) SetDHT(dht DHTInterface) {
	r.dht = dht
}

// SetEnableDHTFallback 设置是否启用 DHT 回退
func (r *AddressResolver) SetEnableDHTFallback(enable bool) {
	r.enableDHTFallback = enable
}

// SetAddressBookClient 设置 Relay 地址簿客户端
//
// v2.0 新增：用于 Relay 地址簿回退查询
func (r *AddressResolver) SetAddressBookClient(client AddressBookClientInterface) {
	r.addressBookClient = client
}

// SetEnableRelayFallback 设置是否启用 Relay 地址簿回退
//
// v2.0 新增：当 DHT 也查不到时，从 Relay 地址簿查询
func (r *AddressResolver) SetEnableRelayFallback(enable bool) {
	r.enableRelayFallback = enable
}

// SetRelayPeerID 设置当前使用的 Relay PeerID
//
// v2.0 新增：用于 Relay 地址簿查询
func (r *AddressResolver) SetRelayPeerID(relayPeerID string) {
	r.relayPeerID = relayPeerID
}
