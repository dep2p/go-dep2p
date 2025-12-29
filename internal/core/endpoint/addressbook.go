// Package endpoint 提供 Endpoint 聚合模块的实现
package endpoint

import (
	"sync"
	"time"

	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// addressEntry 地址簿条目
type addressEntry struct {
	addrs     []coreif.Address
	updatedAt time.Time
	ttl       time.Duration
}

// AddressBook 实现了 coreif.AddressBook 接口
// 管理已知节点的地址信息
type AddressBook struct {
	entries map[types.NodeID]*addressEntry
	mu      sync.RWMutex

	// 默认 TTL
	defaultTTL time.Duration
}

// 确保实现接口
var _ coreif.AddressBook = (*AddressBook)(nil)

// NewAddressBook 创建地址簿
func NewAddressBook() *AddressBook {
	return &AddressBook{
		entries:    make(map[types.NodeID]*addressEntry),
		defaultTTL: 24 * time.Hour, // 默认 24 小时 TTL
	}
}

// Add 添加地址
func (ab *AddressBook) Add(nodeID coreif.NodeID, addrs ...coreif.Address) {
	if len(addrs) == 0 {
		return
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	entry, exists := ab.entries[nodeID]
	if !exists {
		entry = &addressEntry{
			addrs:     make([]coreif.Address, 0),
			updatedAt: time.Now(),
			ttl:       ab.defaultTTL,
		}
		ab.entries[nodeID] = entry
	}

	// 合并地址，去重
	addrSet := make(map[string]coreif.Address)
	for _, addr := range entry.addrs {
		addrSet[addr.String()] = addr
	}
	for _, addr := range addrs {
		addrSet[addr.String()] = addr
	}

	entry.addrs = make([]coreif.Address, 0, len(addrSet))
	for _, addr := range addrSet {
		entry.addrs = append(entry.addrs, addr)
	}
	entry.updatedAt = time.Now()

	log.Debug("添加节点地址",
		"nodeID", nodeID.ShortString(),
		"addrCount", len(entry.addrs))
}

// Get 获取地址
func (ab *AddressBook) Get(nodeID coreif.NodeID) []coreif.Address {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	entry, exists := ab.entries[nodeID]
	if !exists {
		return nil
	}

	// 检查 TTL
	if time.Since(entry.updatedAt) > entry.ttl {
		return nil
	}

	// 返回副本
	addrs := make([]coreif.Address, len(entry.addrs))
	copy(addrs, entry.addrs)
	return addrs
}

// Remove 移除节点的所有地址
func (ab *AddressBook) Remove(nodeID coreif.NodeID) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	delete(ab.entries, nodeID)

	log.Debug("移除节点地址", "nodeID", nodeID.ShortString())
}

// Peers 返回所有已知节点
func (ab *AddressBook) Peers() []coreif.NodeID {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	peers := make([]coreif.NodeID, 0, len(ab.entries))
	now := time.Now()

	for nodeID, entry := range ab.entries {
		// 跳过过期条目
		if now.Sub(entry.updatedAt) > entry.ttl {
			continue
		}
		peers = append(peers, nodeID)
	}

	return peers
}

// SetTTL 设置节点地址的 TTL
func (ab *AddressBook) SetTTL(nodeID coreif.NodeID, ttl time.Duration) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if entry, exists := ab.entries[nodeID]; exists {
		entry.ttl = ttl
	}
}

// Clear 清空地址簿
func (ab *AddressBook) Clear() {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	ab.entries = make(map[types.NodeID]*addressEntry)
	log.Debug("地址簿已清空")
}

// Count 返回节点数量
func (ab *AddressBook) Count() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return len(ab.entries)
}

// GC 清理过期条目
func (ab *AddressBook) GC() int {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	now := time.Now()
	removed := 0

	for nodeID, entry := range ab.entries {
		if now.Sub(entry.updatedAt) > entry.ttl {
			delete(ab.entries, nodeID)
			removed++
		}
	}

	if removed > 0 {
		log.Debug("清理过期地址条目", "removed", removed)
	}

	return removed
}

