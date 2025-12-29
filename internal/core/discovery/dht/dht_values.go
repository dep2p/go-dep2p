// Package dht 提供分布式哈希表实现
package dht

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              类型定义
// ============================================================================

// storedValue 存储的值
type storedValue struct {
	Value     []byte
	Provider  types.NodeID
	Timestamp time.Time
	TTL       time.Duration
}

// ============================================================================
//                              值存储
// ============================================================================

// PutValue 存储值（使用默认 TTL）
//
// 使用 DHT 配置中的 MaxRecordAge 作为 TTL。
func (d *DHT) PutValue(ctx context.Context, key string, value []byte) error {
	return d.PutValueWithTTL(ctx, key, value, d.config.MaxRecordAge)
}

// PutValueWithTTL 存储值（指定 TTL）
//
// 根据设计文档 STORE 操作：{ Key: bytes, Value: bytes, TTL: uint32 }
// 值将在 TTL 到期后自动过期。
//
// T2 修复：存储和距离计算使用 SHA256(key)，网络消息传输原始 key。
func (d *DHT) PutValueWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if atomic.LoadInt32(&d.running) == 0 {
		return ErrDHTClosed
	}

	// 验证 TTL
	if ttl <= 0 {
		ttl = d.config.MaxRecordAge
	}

	// T2: 使用哈希后的 key 作为存储 key
	hashedKey := HashKeyString(key)

	// 本地存储
	d.storeMu.Lock()
	d.store[hashedKey] = storedValue{
		Value:     value,
		Provider:  d.localID,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
	d.storeMu.Unlock()

	log.Debug("本地存储值",
		"key", key,
		"hashedKey", hashedKey[:16]+"...",
		"size", len(value),
		"ttl", ttl)

	// T2: 距离计算使用哈希后的 key bytes
	keyBytes := HashKey(key)
	closestPeers := d.routingTable.NearestPeers(keyBytes, d.config.ReplicationFactor)

	var wg sync.WaitGroup
	for _, peer := range closestPeers {
		wg.Add(1)
		go func(nodeID types.NodeID) {
			defer wg.Done()

			if d.network != nil {
				// 网络消息传输原始 key（对端自行哈希）
				if err := d.network.SendStore(ctx, nodeID, key, value, ttl); err != nil {
					log.Debug("存储到节点失败",
						"peer", nodeID.ShortString(),
						"err", err)
				}
			}
		}(peer)
	}

	wg.Wait()
	return nil
}

// GetValue 获取值
//
// T2 修复：存储和距离计算使用 SHA256(key)，网络消息传输原始 key。
// T4 修复：实现迭代查询（alpha=3，最多10轮）
func (d *DHT) GetValue(ctx context.Context, key string) ([]byte, error) {
	if atomic.LoadInt32(&d.running) == 0 {
		return nil, ErrDHTClosed
	}

	// T2: 使用哈希后的 key 作为存储 key
	hashedKey := HashKeyString(key)

	// 先检查本地
	d.storeMu.RLock()
	if stored, ok := d.store[hashedKey]; ok {
		if time.Since(stored.Timestamp) < stored.TTL {
			d.storeMu.RUnlock()
			return stored.Value, nil
		}
	}
	d.storeMu.RUnlock()

	// 检查网络层
	d.networkMu.RLock()
	network := d.network
	d.networkMu.RUnlock()

	if network == nil {
		return nil, ErrKeyNotFound
	}

	// T4: 迭代查询
	value, err := d.iterativeFindValue(ctx, key)
	if err != nil {
		return nil, err
	}

	// T2: 本地缓存使用哈希后的 key
	d.storeMu.Lock()
	d.store[hashedKey] = storedValue{
		Value:     value,
		Timestamp: time.Now(),
		TTL:       d.config.MaxRecordAge,
	}
	d.storeMu.Unlock()

	return value, nil
}

// iterativeFindValue 迭代查找值
//
// T4 修复：实现 Kademlia 迭代查询算法
// - alpha=3 并发
// - 最多 10 轮迭代
// - 使用 closer peers 继续查询
func (d *DHT) iterativeFindValue(ctx context.Context, key string) ([]byte, error) {
	const maxIterations = 10

	// T2: 距离计算使用哈希后的 key bytes
	keyBytes := HashKey(key)

	// 候选节点集合（按距离排序）
	queried := make(map[types.NodeID]struct{})
	candidates := d.routingTable.NearestPeers(keyBytes, d.config.BucketSize)

	for iteration := 0; iteration < maxIterations; iteration++ {
		// 选择下一批查询节点（排除已查询过的）
		var toQuery []types.NodeID
		for _, id := range candidates {
			if _, ok := queried[id]; !ok {
				toQuery = append(toQuery, id)
				if len(toQuery) >= d.config.Alpha {
					break
				}
			}
		}

		if len(toQuery) == 0 {
			// 没有新的候选节点
			break
		}

		log.Debug("迭代查询 FIND_VALUE",
			"iteration", iteration,
			"key", key[:min(20, len(key))]+"...",
			"toQuery", len(toQuery))

		// 并发查询
		type queryResult struct {
			value       []byte
			closerPeers []types.NodeID
		}
		resultCh := make(chan queryResult, len(toQuery))

		for _, nodeID := range toQuery {
			queried[nodeID] = struct{}{}

			go func(id types.NodeID) {
				d.networkMu.RLock()
				network := d.network
				d.networkMu.RUnlock()

				if network == nil {
					resultCh <- queryResult{}
					return
				}

				value, closerPeers, err := network.SendFindValue(ctx, id, key)
				if err != nil {
					log.Debug("FIND_VALUE 失败",
						"peer", id.ShortString(),
						"err", err)
					resultCh <- queryResult{}
					return
				}

				var closerIDs []types.NodeID
				for _, p := range closerPeers {
					closerIDs = append(closerIDs, p.ID)
				}

				resultCh <- queryResult{
					value:       value,
					closerPeers: closerIDs,
				}
			}(nodeID)
		}

		// 收集结果
		var newCandidates []types.NodeID
		for i := 0; i < len(toQuery); i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case result := <-resultCh:
				if result.value != nil {
					// 找到值
					return result.value, nil
				}
				newCandidates = append(newCandidates, result.closerPeers...)
			}
		}

		// 合并新候选并按距离排序
		if len(newCandidates) > 0 {
			candidates = d.mergeCandidates(candidates, newCandidates, keyBytes, queried)
		}
	}

	return nil, ErrKeyNotFound
}

// mergeCandidates 合并候选节点并按距离排序
func (d *DHT) mergeCandidates(existing, incoming []types.NodeID, targetKey []byte, exclude map[types.NodeID]struct{}) []types.NodeID {
	seen := make(map[types.NodeID]struct{})
	var merged []types.NodeID

	// 添加现有候选
	for _, id := range existing {
		if _, ok := seen[id]; !ok {
			if _, excluded := exclude[id]; !excluded {
				seen[id] = struct{}{}
				merged = append(merged, id)
			}
		}
	}

	// 添加新候选
	for _, id := range incoming {
		if _, ok := seen[id]; !ok {
			if _, excluded := exclude[id]; !excluded {
				seen[id] = struct{}{}
				merged = append(merged, id)
			}
		}
	}

	// 按距离排序
	sort.Slice(merged, func(i, j int) bool {
		distI := XORDistance(merged[i][:], targetKey)
		distJ := XORDistance(merged[j][:], targetKey)
		return bytes.Compare(distI, distJ) < 0
	})

	// 限制数量
	if len(merged) > d.config.BucketSize {
		merged = merged[:d.config.BucketSize]
	}

	return merged
}

