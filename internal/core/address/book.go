// Package address 提供地址管理模块的实现
package address

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              AddressBook 实现
// ============================================================================

// AddressBook 地址簿实现
//
// 支持：
// - 基础地址存储与查询
// - 签名地址记录验证
// - 地址优先级排序
// - 多源地址合并
type AddressBook struct {
	// 基础地址存储
	addrs map[types.NodeID][]addressEntry

	// 签名记录存储
	signedRecords map[types.NodeID]*AddressRecord

	// 优先级地址存储
	prioritizedAddrs map[types.NodeID][]*PrioritizedAddress

	mu sync.RWMutex
}

type addressEntry struct {
	addr    endpoint.Address
	ttl     time.Duration
	addedAt time.Time
	source  AddressSource
}

// NewAddressBook 创建地址簿
func NewAddressBook() *AddressBook {
	return &AddressBook{
		addrs:            make(map[types.NodeID][]addressEntry),
		signedRecords:    make(map[types.NodeID]*AddressRecord),
		prioritizedAddrs: make(map[types.NodeID][]*PrioritizedAddress),
	}
}

// ============================================================================
//                              基础地址操作
// ============================================================================

// AddAddrs 添加地址
func (b *AddressBook) AddAddrs(nodeID types.NodeID, addrs []endpoint.Address, ttl time.Duration) {
	b.AddAddrsWithSource(nodeID, addrs, ttl, AddressSourceManual)
}

// AddAddrsWithSource 添加地址（指定来源）
func (b *AddressBook) AddAddrsWithSource(nodeID types.NodeID, addrs []endpoint.Address, ttl time.Duration, source AddressSource) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries := make([]addressEntry, len(addrs))
	now := time.Now()
	for i, addr := range addrs {
		entries[i] = addressEntry{
			addr:    addr,
			ttl:     ttl,
			addedAt: now,
			source:  source,
		}

		// 同时添加到优先级地址
		addrType := DetectAddressType(addr)
		pa := NewPrioritizedAddress(addr, addrType, source)
		pa.TTL = ttl
		b.addPrioritizedAddr(nodeID, pa)
	}
	b.addrs[nodeID] = b.mergeEntries(b.addrs[nodeID], entries)
}

// mergeEntries 合并地址条目（去重）
func (b *AddressBook) mergeEntries(existing, incoming []addressEntry) []addressEntry {
	seen := make(map[string]struct{})
	result := make([]addressEntry, 0, len(existing)+len(incoming))

	// 新条目优先
	for _, entry := range incoming {
		key := entry.addr.String()
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, entry)
		}
	}

	// 保留未覆盖的旧条目
	for _, entry := range existing {
		key := entry.addr.String()
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, entry)
		}
	}

	return result
}

// Addrs 获取地址
func (b *AddressBook) Addrs(nodeID types.NodeID) []endpoint.Address {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entries, ok := b.addrs[nodeID]
	if !ok {
		return nil
	}

	now := time.Now()
	var result []endpoint.Address
	for _, entry := range entries {
		if now.Sub(entry.addedAt) < entry.ttl {
			result = append(result, entry.addr)
		}
	}
	return result
}

// AddrsSorted 获取按优先级排序的地址
func (b *AddressBook) AddrsSorted(nodeID types.NodeID) []endpoint.Address {
	b.mu.RLock()
	defer b.mu.RUnlock()

	pas, ok := b.prioritizedAddrs[nodeID]
	if !ok {
		return nil
	}

	// 过滤可用地址并排序
	usable := FilterUsableAddresses(pas)
	SortAddresses(usable)

	result := make([]endpoint.Address, len(usable))
	for i, pa := range usable {
		result[i] = pa.Address
	}
	return result
}

// BestAddr 获取最佳地址
func (b *AddressBook) BestAddr(nodeID types.NodeID) endpoint.Address {
	b.mu.RLock()
	defer b.mu.RUnlock()

	pas, ok := b.prioritizedAddrs[nodeID]
	if !ok {
		return nil
	}

	best := SelectBestAddress(pas)
	if best == nil {
		return nil
	}
	return best.Address
}

// ClearAddrs 清除地址
func (b *AddressBook) ClearAddrs(nodeID types.NodeID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.addrs, nodeID)
	delete(b.prioritizedAddrs, nodeID)
	delete(b.signedRecords, nodeID)
}

// Peers 获取所有节点
func (b *AddressBook) Peers() []types.NodeID {
	b.mu.RLock()
	defer b.mu.RUnlock()

	peers := make([]types.NodeID, 0, len(b.addrs))
	for id := range b.addrs {
		peers = append(peers, id)
	}
	return peers
}

// PeerCount 获取节点数量
func (b *AddressBook) PeerCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.addrs)
}

// ============================================================================
//                              签名记录操作
// ============================================================================

// AddSignedRecord 添加签名地址记录
//
// 验证签名后添加记录，如果记录序列号更新则替换旧记录。
func (b *AddressBook) AddSignedRecord(record *AddressRecord, _ interface{}) error {
	if record == nil {
		return ErrEmptyAddresses
	}

	// 验证基本有效性
	if err := record.Validate(); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查序列号
	existing := b.signedRecords[record.NodeID]
	if existing != nil && !record.IsNewerThan(existing) {
		return ErrStaleSequence
	}

	// 存储记录
	b.signedRecords[record.NodeID] = record

	// 同步到地址簿
	for _, addr := range record.Addresses {
		entry := addressEntry{
			addr:    addr,
			ttl:     record.TTL,
			addedAt: record.Timestamp,
			source:  AddressSourceDHT, // 签名记录通常来自 DHT
		}

		// 合并到地址簿
		entries := b.addrs[record.NodeID]
		b.addrs[record.NodeID] = b.mergeEntries(entries, []addressEntry{entry})

		// 添加到优先级地址
		addrType := DetectAddressType(addr)
		pa := NewPrioritizedAddress(addr, addrType, AddressSourceDHT)
		pa.TTL = record.TTL
		b.addPrioritizedAddr(record.NodeID, pa)
	}

	return nil
}

// GetSignedRecord 获取签名记录
func (b *AddressBook) GetSignedRecord(nodeID types.NodeID) *AddressRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.signedRecords[nodeID]
}

// ============================================================================
//                              优先级地址操作
// ============================================================================

// addPrioritizedAddr 添加优先级地址（内部方法，需持有锁）
func (b *AddressBook) addPrioritizedAddr(nodeID types.NodeID, pa *PrioritizedAddress) {
	existing := b.prioritizedAddrs[nodeID]

	// 检查是否已存在相同地址
	addrStr := pa.Address.String()
	for i, p := range existing {
		if p.Address.String() == addrStr {
			// 更新现有条目
			if pa.Source.Priority() >= p.Source.Priority() {
				existing[i] = pa
			}
			return
		}
	}

	// 添加新条目
	b.prioritizedAddrs[nodeID] = append(existing, pa)
}

// RecordSuccess 记录地址连接成功
func (b *AddressBook) RecordSuccess(nodeID types.NodeID, addr endpoint.Address, rtt time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	pas, ok := b.prioritizedAddrs[nodeID]
	if !ok {
		return
	}

	addrStr := addr.String()
	for _, pa := range pas {
		if pa.Address.String() == addrStr {
			pa.RecordSuccess(rtt)
			return
		}
	}
}

// RecordFail 记录地址连接失败
func (b *AddressBook) RecordFail(nodeID types.NodeID, addr endpoint.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()

	pas, ok := b.prioritizedAddrs[nodeID]
	if !ok {
		return
	}

	addrStr := addr.String()
	for _, pa := range pas {
		if pa.Address.String() == addrStr {
			pa.RecordFail()
			return
		}
	}
}

// GetPrioritizedAddrs 获取优先级地址列表
func (b *AddressBook) GetPrioritizedAddrs(nodeID types.NodeID) []*PrioritizedAddress {
	b.mu.RLock()
	defer b.mu.RUnlock()

	pas, ok := b.prioritizedAddrs[nodeID]
	if !ok {
		return nil
	}

	// 返回副本
	result := make([]*PrioritizedAddress, len(pas))
	copy(result, pas)
	return result
}

// ============================================================================
//                              清理与维护
// ============================================================================

// Cleanup 清理过期地址
func (b *AddressBook) Cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// 清理基础地址
	for nodeID, entries := range b.addrs {
		valid := entries[:0]
		for _, entry := range entries {
			if now.Sub(entry.addedAt) < entry.ttl {
				valid = append(valid, entry)
			}
		}
		if len(valid) == 0 {
			delete(b.addrs, nodeID)
		} else {
			b.addrs[nodeID] = valid
		}
	}

	// 清理签名记录
	for nodeID, record := range b.signedRecords {
		if record.IsExpired() {
			delete(b.signedRecords, nodeID)
		}
	}

	// 清理优先级地址
	for nodeID, pas := range b.prioritizedAddrs {
		valid := pas[:0]
		for _, pa := range pas {
			if !pa.IsExpired() {
				valid = append(valid, pa)
			}
		}
		if len(valid) == 0 {
			delete(b.prioritizedAddrs, nodeID)
		} else {
			b.prioritizedAddrs[nodeID] = valid
		}
	}
}

