// Package dht 提供分布式哈希表实现
package dht

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              发现器接口
// ============================================================================

// FindPeer 查找节点
//
// Layer1 修复：优先通过 LookupPeerRecord 从 DHT 获取经过签名验证的地址记录，
// 实现"只知道 NodeID 也能找到可拨号地址"的语义。
//
// 查找顺序：
// 1. 尝试从 DHT 查询 sys/peer/<NodeID> 的 SignedPeerRecord（权威、经验证）
// 2. 如果 PeerRecord 查询失败，回退到路由表缓存
// 3. 如果路由表也没有，执行迭代 FIND_NODE 查找
func (d *DHT) FindPeer(ctx context.Context, id types.NodeID) ([]endpoint.Address, error) {
	if atomic.LoadInt32(&d.running) == 0 {
		return nil, ErrDHTClosed
	}

	// 1. 优先尝试 LookupPeerRecord（获取经过签名验证的地址）
	if addrs, err := d.LookupPeerRecord(ctx, id); err == nil && len(addrs) > 0 {
		result := make([]endpoint.Address, len(addrs))
		for i, addr := range addrs {
			result[i] = address.NewAddr(types.Multiaddr(addr))
		}
		log.Debug("FindPeer 通过 PeerRecord 找到地址",
			"nodeID", id.ShortString(),
			"addrs", len(addrs))
		return result, nil
	}

	// 2. 回退：检查本地路由表缓存
	if node := d.routingTable.Find(id); node != nil {
		addrs := make([]endpoint.Address, len(node.Addrs))
		for i, addr := range node.Addrs {
			addrs[i] = address.NewAddr(types.Multiaddr(addr))
		}
		log.Debug("FindPeer 从路由表缓存找到地址",
			"nodeID", id.ShortString(),
			"addrs", len(node.Addrs))
		return addrs, nil
	}

	// 3. 回退：执行迭代 FIND_NODE 查找（可能在过程中发现目标节点）
	peers, err := d.lookupPeers(ctx, id[:])
	if err != nil {
		return nil, err
	}

	// 查找目标节点
	for _, peer := range peers {
		if peer.ID == id {
			addrs := make([]endpoint.Address, len(peer.Addrs))
			for i, addr := range peer.Addrs {
				addrs[i] = address.NewAddr(addr)
			}
			log.Debug("FindPeer 通过迭代查找找到地址",
				"nodeID", id.ShortString(),
				"addrs", len(peer.Addrs))
			return addrs, nil
		}
	}

	// 节点未找到
	return nil, ErrKeyNotFound
}

// FindPeers 批量查找节点
func (d *DHT) FindPeers(ctx context.Context, ids []types.NodeID) (map[types.NodeID][]endpoint.Address, error) {
	result := make(map[types.NodeID][]endpoint.Address)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, id := range ids {
		wg.Add(1)
		go func(nodeID types.NodeID) {
			defer wg.Done()

			addrs, err := d.FindPeer(ctx, nodeID)
			if err != nil || len(addrs) == 0 {
				return
			}

			mu.Lock()
			result[nodeID] = addrs
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	return result, nil
}

// FindClosestPeers 查找最近的节点
func (d *DHT) FindClosestPeers(ctx context.Context, key []byte, count int) ([]types.NodeID, error) {
	if atomic.LoadInt32(&d.running) == 0 {
		return nil, ErrDHTClosed
	}

	// 计算 Realm 感知的 Key
	// 使用输入 key 直接进行查找，但在 Realm 隔离模式下会过滤非本 Realm 的节点
	searchKey := key

	peers, err := d.lookupPeers(ctx, searchKey)
	if err != nil {
		return nil, err
	}

	// 按距离排序
	type nodeDistance struct {
		id       types.NodeID
		distance []byte
	}

	var candidates []nodeDistance
	for _, peer := range peers {
		distance := XORDistance(key, peer.ID[:])
		candidates = append(candidates, nodeDistance{
			id:       peer.ID,
			distance: distance,
		})
	}

	// 排序
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if compareBytes(candidates[i].distance, candidates[j].distance) > 0 {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// 返回最近的 count 个
	result := make([]types.NodeID, 0, count)
	for i := 0; i < count && i < len(candidates); i++ {
		result = append(result, candidates[i].id)
	}

	return result, nil
}

// DiscoverPeers 发现节点
//
// # DiscoverPeers 发现节点
//
// 修正后的语义：
// 1. 如果 namespace 非空，从 DHT 获取该 namespace 的 providers
// 2. 补充本地路由表中的节点
func (d *DHT) DiscoverPeers(ctx context.Context, namespace string) (<-chan discoveryif.PeerInfo, error) {
	if atomic.LoadInt32(&d.running) == 0 {
		return nil, ErrDHTClosed
	}

	ch := make(chan discoveryif.PeerInfo, 20)

	go func() {
		defer close(ch)

		seen := make(map[types.NodeID]struct{})

		// 1. 如果有 namespace，从 DHT 获取 providers
		if namespace != "" {
			providers, err := d.GetProviders(ctx, namespace)
			if err == nil {
				for _, p := range providers {
					if _, exists := seen[p.ID]; exists {
						continue
					}
					seen[p.ID] = struct{}{}

					select {
					case <-ctx.Done():
						return
					case ch <- p:
					}
				}
			}
		}

		// 2. 补充本地路由表中的节点
		var peers []types.NodeID
		if namespace != "" {
			// 使用 namespace 作为 key 查找最近的节点
			namespaceKey := RealmAwareValueKey(d.realmID, "namespace", []byte(namespace))
			peers = d.routingTable.NearestPeers(namespaceKey, d.config.BucketSize)
		} else {
			// 无 namespace 时返回所有节点
			peers = d.routingTable.Peers()
		}

		for _, id := range peers {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}

			node := d.routingTable.Find(id)
			if node == nil {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case ch <- discoveryif.PeerInfo{
				ID:    node.ID,
				Addrs: types.StringsToMultiaddrs(node.Addrs),
			}:
			}
		}
	}()

	return ch, nil
}

// ============================================================================
//                              内部查询
// ============================================================================

// lookupPeers 迭代查找节点
func (d *DHT) lookupPeers(ctx context.Context, key []byte) ([]discoveryif.PeerInfo, error) {
	// 获取初始候选节点
	closest := d.routingTable.NearestPeers(key, d.config.Alpha)
	if len(closest) == 0 {
		return nil, ErrNoNodes
	}

	// 已查询和待查询的节点
	queried := make(map[types.NodeID]bool)
	var allPeers []discoveryif.PeerInfo

	// 迭代查询
	for round := 0; round < 10; round++ {
		var wg sync.WaitGroup
		var mu sync.Mutex
		var newPeers []discoveryif.PeerInfo

		for _, nodeID := range closest {
			if queried[nodeID] {
				continue
			}
			queried[nodeID] = true

			wg.Add(1)
			go func(id types.NodeID) {
				defer wg.Done()

				var targetID types.NodeID
				copy(targetID[:], key)

				if d.network != nil {
					peers, err := d.network.SendFindNode(ctx, id, targetID)
					if err != nil {
						log.Debug("FIND_NODE 失败",
							"peer", id.ShortString(),
							"err", err)
						return
					}

					mu.Lock()
					newPeers = append(newPeers, peers...)
					mu.Unlock()
				}
			}(nodeID)
		}

		wg.Wait()

		// 更新路由表并合并结果
		// 同时将地址写入 AddressBook（Peerstore 类底座）
		for _, peer := range newPeers {
			if peer.ID != d.localID {
				addrStrs := types.MultiaddrsToStrings(peer.Addrs)
				node := &RoutingNode{
					ID:       peer.ID,
					Addrs:    addrStrs,
					LastSeen: time.Now(),
					RealmID:  d.realmID,
				}
				_ = d.routingTable.Update(node) // 忽略 bucket full 错误
				allPeers = append(allPeers, peer)

				// 写入 AddressBook（支撑 Endpoint.Connect() 地址查找）
				d.writeAddrsToAddressBook(peer.ID, addrStrs)
			}
		}

		// 如果没有新节点，停止迭代
		if len(newPeers) == 0 {
			break
		}

		// 更新候选节点
		closest = d.routingTable.NearestPeers(key, d.config.Alpha)
	}

	return allPeers, nil
}

