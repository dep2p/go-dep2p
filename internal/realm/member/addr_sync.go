package member

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/core/peerstore"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/addrbook"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// AddrSyncer 地址同步器
//
// 负责将 MemberList 中的成员地址同步到 Peerstore，
// 支持"仅 ID 连接"的地址发现。
type AddrSyncer struct {
	realmID   string
	peerstore pkgif.Peerstore

	// 默认 TTL
	defaultTTL time.Duration
}

// AddrSyncerConfig 同步器配置
type AddrSyncerConfig struct {
	RealmID    string
	Peerstore  pkgif.Peerstore
	DefaultTTL time.Duration
}

// NewAddrSyncer 创建地址同步器
func NewAddrSyncer(config AddrSyncerConfig) *AddrSyncer {
	ttl := config.DefaultTTL
	if ttl <= 0 {
		ttl = peerstore.DefaultTTLForSource(peerstore.SourceMemberList)
	}

	return &AddrSyncer{
		realmID:    config.RealmID,
		peerstore:  config.Peerstore,
		defaultTTL: ttl,
	}
}

// OnMemberJoined 成员加入时同步地址到 Peerstore
func (s *AddrSyncer) OnMemberJoined(member *Member) {
	if s.peerstore == nil || member == nil {
		return
	}

	s.syncMemberAddrs(member)
}

// OnMemberUpdated 成员更新时同步地址
func (s *AddrSyncer) OnMemberUpdated(member *Member) {
	if s.peerstore == nil || member == nil {
		return
	}

	s.syncMemberAddrs(member)
}

// AddrBookWithSource 支持按来源清除地址的接口
type AddrBookWithSource interface {
	ClearAddrsBySource(peerID types.PeerID, source addrbook.AddressSource)
}

// OnMemberLeft 成员离开时清理地址
//
// 只清除来源为 MemberList 的地址，保留其他来源（如 DHT、手动配置）的地址。
// 这确保了成员离开 Realm 后，如果通过其他途径仍可连接，连接不会受影响。
func (s *AddrSyncer) OnMemberLeft(peerID string) {
	if s.peerstore == nil || peerID == "" {
		return
	}

	pid := types.PeerID(peerID)

	// 尝试按来源清除（如果 peerstore 底层支持）
	// 这是一个优雅降级的实现：
	// - 如果底层 AddrBook 支持 ClearAddrsBySource，则只清除 MemberList 来源的地址
	// - 否则不做任何操作，让地址自然过期
	if ps, ok := s.peerstore.(*peerstore.Peerstore); ok {
		if ab := ps.AddrBook(); ab != nil {
			ab.ClearAddrsBySource(pid, addrbook.SourceMemberList)
		}
	}
}

// OnMemberInfoUpdated 从 MemberInfo 更新地址
func (s *AddrSyncer) OnMemberInfoUpdated(info *interfaces.MemberInfo) {
	if s.peerstore == nil || info == nil {
		return
	}

	member := FromMemberInfo(info)
	if member != nil {
		s.syncMemberAddrs(member)
	}
}

// syncMemberAddrs 同步成员地址到 Peerstore
func (s *AddrSyncer) syncMemberAddrs(member *Member) {
	if len(member.Addrs) == 0 {
		return
	}

	peerID := types.PeerID(member.PeerID)

	// 转换地址字符串为 Multiaddr
	addrsWithSource := make([]addrbook.AddressWithSource, 0, len(member.Addrs))
	for _, addrStr := range member.Addrs {
		addr, err := types.NewMultiaddr(addrStr)
		if err != nil {
			continue // 跳过无效地址
		}

		addrsWithSource = append(addrsWithSource, addrbook.AddressWithSource{
			Addr:   addr,
			Source: addrbook.SourceMemberList,
			TTL:    s.defaultTTL,
		})
	}

	if len(addrsWithSource) == 0 {
		return
	}

	// 使用类型断言检查是否支持带来源的地址方法
	if ps, ok := s.peerstore.(*peerstore.Peerstore); ok {
		ps.AddAddrsWithSource(peerID, addrsWithSource)
	} else {
		// 回退到标准方法
		addrs := make([]types.Multiaddr, len(addrsWithSource))
		for i, aws := range addrsWithSource {
			addrs[i] = aws.Addr
		}
		s.peerstore.AddAddrs(peerID, addrs, s.defaultTTL)
	}
}

// SyncFromRelay 从 Relay 地址簿同步地址
//
// 用于将 Relay 查询结果缓存到 Peerstore。
func (s *AddrSyncer) SyncFromRelay(peerID types.PeerID, addrs []types.Multiaddr) {
	if s.peerstore == nil || len(addrs) == 0 {
		return
	}

	ttl := peerstore.DefaultTTLForSource(peerstore.SourceRelay)

	// 使用类型断言检查是否支持带来源的地址方法
	if ps, ok := s.peerstore.(*peerstore.Peerstore); ok {
		addrsWithSource := make([]addrbook.AddressWithSource, len(addrs))
		for i, addr := range addrs {
			addrsWithSource[i] = addrbook.AddressWithSource{
				Addr:   addr,
				Source: addrbook.SourceRelay,
				TTL:    ttl,
			}
		}
		ps.AddAddrsWithSource(peerID, addrsWithSource)
	} else {
		// 回退到标准方法
		s.peerstore.AddAddrs(peerID, addrs, ttl)
	}
}

// SyncFromDHT 从 DHT 同步地址
//
// 用于将 DHT 发现结果缓存到 Peerstore。
func (s *AddrSyncer) SyncFromDHT(peerID types.PeerID, addrs []types.Multiaddr) {
	if s.peerstore == nil || len(addrs) == 0 {
		return
	}

	ttl := peerstore.DefaultTTLForSource(peerstore.SourceDHT)

	// 使用类型断言检查是否支持带来源的地址方法
	if ps, ok := s.peerstore.(*peerstore.Peerstore); ok {
		addrsWithSource := make([]addrbook.AddressWithSource, len(addrs))
		for i, addr := range addrs {
			addrsWithSource[i] = addrbook.AddressWithSource{
				Addr:   addr,
				Source: addrbook.SourceDHT,
				TTL:    ttl,
			}
		}
		ps.AddAddrsWithSource(peerID, addrsWithSource)
	} else {
		// 回退到标准方法
		s.peerstore.AddAddrs(peerID, addrs, ttl)
	}
}
