package dht

import (
	"context"
	"sort"
	"sync"
	"time"

	corepeerstore "github.com/dep2p/go-dep2p/internal/core/peerstore"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                           迭代查询框架
// ============================================================================

const (
	// MinProvidersForEarlyTermination Provider 查询早期终止阈值
	// v2.0.1: 找到指定数量的 Provider 后提前终止查询，避免不必要的等待
	// 对于 Relay 发现等场景，通常只需要 1-3 个 Provider 即可
	MinProvidersForEarlyTermination = 3
)

// iterativeQuery 迭代查询实现
//
// 实现 Kademlia 迭代查询算法：
//  1. 从本地路由表获取 Alpha 个最近节点
//  2. 并发查询（最多 Alpha 个）
//  3. 收到响应后更新待查询列表（按距离排序）
//  4. 重复直到无更近节点或达到 K 个结果
type iterativeQuery struct {
	dht       *DHT
	target    types.NodeID
	queryType MessageType // FIND_NODE / FIND_VALUE / GET_PROVIDERS
	key       string      // 用于 FIND_VALUE / GET_PROVIDERS

	mu           sync.Mutex
	queried      map[types.NodeID]struct{} // 已查询节点
	pending      []*RoutingNode            // 待查询节点（按距离排序）
	result       []*RoutingNode            // 结果节点
	value        []byte                    // FIND_VALUE 结果
	providers    []types.PeerInfo          // GET_PROVIDERS 结果
	foundValue   bool                      // 是否找到值
	queryRunning int                       // 当前正在运行的查询数
	done         chan struct{}             // 完成信号
	doneOnce     sync.Once                 // v2.0.1: 防止重复关闭 done channel
	queryDone    chan struct{}             // v2.0.1: 单个查询完成通知（替代忙等待）
}

// newIterativeQuery 创建迭代查询
func newIterativeQuery(dht *DHT, target types.NodeID, queryType MessageType, key string) *iterativeQuery {
	return &iterativeQuery{
		dht:       dht,
		target:    target,
		queryType: queryType,
		key:       key,
		queried:   make(map[types.NodeID]struct{}),
		pending:   make([]*RoutingNode, 0),
		result:    make([]*RoutingNode, 0),
		done:      make(chan struct{}),
		queryDone: make(chan struct{}, Alpha*2), // v2.0.1: 缓冲通道，避免阻塞
	}
}

// closeDone 安全关闭 done channel（使用 sync.Once 防止重复关闭）
func (q *iterativeQuery) closeDone() {
	q.doneOnce.Do(func() {
		close(q.done)
	})
}

// notifyQueryDone 通知单个查询完成（非阻塞）
func (q *iterativeQuery) notifyQueryDone() {
	select {
	case q.queryDone <- struct{}{}:
	default:
		// 通道已满，忽略（主循环会处理）
	}
}

// Run 执行迭代查询
func (q *iterativeQuery) Run(ctx context.Context) error {
	// v2.0.1: 添加查询统计日志，用于诊断超时问题
	startTime := time.Now()
	defer func() {
		q.mu.Lock()
		nodesQueried := len(q.queried)
		resultsFound := len(q.result)
		providersFound := len(q.providers)
		pendingRemaining := len(q.pending)
		q.mu.Unlock()

		logger.Debug("DHT 迭代查询完成",
			"queryType", q.queryType.String(),
			"duration", time.Since(startTime),
			"nodesQueried", nodesQueried,
			"resultsFound", resultsFound,
			"providersFound", providersFound,
			"pendingRemaining", pendingRemaining,
			"routingTableSize", q.dht.routingTable.Size(),
		)
	}()

	// 1. 从本地路由表获取初始节点（使用自适应 Alpha）
	adaptiveAlpha := q.getAdaptiveAlpha()
	initialPeers := q.dht.routingTable.NearestPeers(q.target, adaptiveAlpha)
	if len(initialPeers) == 0 {
		q.closeDone() // v2.0.1: 使用安全关闭
		return ErrNoNearbyPeers
	}

	// 初始化待查询列表
	q.pending = initialPeers

	// 2. 启动迭代查询循环
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.done:
			return nil
		default:
		}

		// 3. 获取下一批待查询节点
		nodesToQuery := q.getNextBatch()
		if len(nodesToQuery) == 0 {
			// 没有更多节点可查询，等待正在运行的查询完成
			q.mu.Lock()
			running := q.queryRunning
			q.mu.Unlock()

			if running == 0 {
				q.closeDone() // v2.0.1: 使用安全关闭
				return nil
			}

			// v2.0.1: 使用 channel 通知替代忙等待
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-q.done:
				return nil
			case <-q.queryDone:
				// 有查询完成，继续循环检查
				continue
			case <-time.After(100 * time.Millisecond):
				// 超时保护，避免死锁
				continue
			}
		}

		// 4. 并发查询
		for _, node := range nodesToQuery {
			q.mu.Lock()
			q.queryRunning++
			q.mu.Unlock()

			go q.queryNode(ctx, node)
		}
	}
}

// getAdaptiveAlpha 获取自适应并发参数
//
// v2.0.1: 根据路由表大小和待查询节点数动态调整并发度
// - 路由表极小时（<5）：使用更激进的并发（10）加速填充
// - 待查询节点多时（>20）：增加并发（8）加速收敛
// - 其他情况：使用默认 Alpha（5）
func (q *iterativeQuery) getAdaptiveAlpha() int {
	rtSize := q.dht.routingTable.Size()

	q.mu.Lock()
	pendingSize := len(q.pending)
	q.mu.Unlock()

	return q.calculateAdaptiveAlpha(rtSize, pendingSize)
}

// getAdaptiveAlphaLocked 获取自适应并发参数（调用者已持有锁）
func (q *iterativeQuery) getAdaptiveAlphaLocked() int {
	// 防御性检查：避免 nil 指针访问
	if q.dht == nil || q.dht.routingTable == nil {
		return Alpha
	}
	rtSize := q.dht.routingTable.Size()
	pendingSize := len(q.pending)
	return q.calculateAdaptiveAlpha(rtSize, pendingSize)
}

// calculateAdaptiveAlpha 计算自适应 Alpha 值
func (q *iterativeQuery) calculateAdaptiveAlpha(rtSize, pendingSize int) int {
	switch {
	case rtSize < 5:
		// 路由表极小，激进并发加速填充
		return 10
	case pendingSize > 20:
		// 待查询节点多，增加并发加速收敛
		return 8
	default:
		return Alpha
	}
}

// getNextBatch 获取下一批待查询节点
func (q *iterativeQuery) getNextBatch() []*RoutingNode {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 如果已经找到值，不再查询
	if q.foundValue {
		return nil
	}

	// v2.0.1: 使用自适应 Alpha 计算可用槽位
	adaptiveAlpha := q.getAdaptiveAlphaLocked()
	availableSlots := adaptiveAlpha - q.queryRunning
	if availableSlots <= 0 {
		return nil
	}

	// 从 pending 中选择节点
	nodesToQuery := make([]*RoutingNode, 0, availableSlots)
	remaining := make([]*RoutingNode, 0)

	for _, node := range q.pending {
		if len(nodesToQuery) < availableSlots {
			// 检查是否已查询过
			if _, queried := q.queried[node.ID]; !queried {
				nodesToQuery = append(nodesToQuery, node)
				q.queried[node.ID] = struct{}{}
			}
		} else {
			remaining = append(remaining, node)
		}
	}

	q.pending = remaining
	return nodesToQuery
}

// queryNode 查询单个节点
func (q *iterativeQuery) queryNode(ctx context.Context, node *RoutingNode) {
	defer func() {
		q.mu.Lock()
		q.queryRunning--
		q.mu.Unlock()
		q.notifyQueryDone()
	}()

	peerID := types.PeerID(node.ID)

	// 将节点地址存入 Peerstore，确保后续 Connect 能找到地址
	if q.dht.peerstore != nil && len(node.Addrs) > 0 {
		multiaddrs := convertToMultiaddrs(node.Addrs)
		if len(multiaddrs) > 0 {
			q.dht.peerstore.AddAddrs(peerID, multiaddrs, corepeerstore.DiscoveredAddrTTL)
		}
	}

	// 准备发送者信息
	localID := types.NodeID(q.dht.host.ID())
	localAddrs := q.dht.host.AdvertisedAddrs()
	requestID := uint64(time.Now().UnixNano())

	var msg *Message

	// 根据查询类型构造消息
	switch q.queryType {
	case MessageTypeFindNode:
		msg = NewFindNodeRequest(requestID, localID, localAddrs, q.target)
	case MessageTypeFindValue:
		msg = NewFindValueRequest(requestID, localID, localAddrs, q.key)
	case MessageTypeGetProviders:
		msg = NewGetProvidersRequest(requestID, localID, localAddrs, q.key)
	case MessageTypeGetPeerRecord:
		msg = NewGetPeerRecordRequest(requestID, localID, localAddrs, q.key)
	default:
		return
	}

	// 发送查询
	response, err := q.dht.network.SendMessage(ctx, peerID, msg)
	if err != nil {
		// 查询失败，跳过该节点
		return
	}

	// 处理响应
	q.processResponse(node, response)
}

// processResponse 处理查询响应
func (q *iterativeQuery) processResponse(node *RoutingNode, response *Message) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 添加到结果集
	if !q.containsNode(q.result, node.ID) {
		q.result = append(q.result, node)
	}

	// 处理不同类型的响应
	switch response.Type {
	case MessageTypeFindNodeResponse:
		// FIND_NODE 响应：添加更近的节点到 pending
		for _, peer := range response.CloserPeers {
			peerNode := &RoutingNode{
				ID:       peer.ID,
				Addrs:    peer.Addrs,
				LastSeen: time.Now(),
			}
			if _, queried := q.queried[peer.ID]; !queried {
				q.addToPending(peerNode)
			}
		}

	case MessageTypeFindValueResponse:
		if len(response.Value) > 0 {
			// 找到值
			q.value = response.Value
			q.foundValue = true
		} else {
			// 返回更近的节点
			for _, peer := range response.CloserPeers {
				peerNode := &RoutingNode{
					ID:       peer.ID,
					Addrs:    peer.Addrs,
					LastSeen: time.Now(),
				}
				if _, queried := q.queried[peer.ID]; !queried {
					q.addToPending(peerNode)
				}
			}
		}

	case MessageTypeGetProvidersResponse:
		// 收集 providers
		for _, p := range response.Providers {
			info := types.PeerInfo{
				ID:    types.PeerID(p.ID),
				Addrs: convertToMultiaddrs(p.Addrs),
			}
			q.providers = append(q.providers, info)
		}

		// v2.0.1: Provider 查询早期终止 - 找到足够数量的 Provider 后提前返回
		// 对于 Relay 发现等场景，无需等待收集满 BucketSize 个结果
		if len(q.providers) >= MinProvidersForEarlyTermination {
			q.foundValue = true // 标记为已找到，触发早期终止
		}

		// 添加更近的节点（即使已有足够 Provider，继续收集以便缓存）
		for _, peer := range response.CloserPeers {
			peerNode := &RoutingNode{
				ID:       peer.ID,
				Addrs:    peer.Addrs,
				LastSeen: time.Now(),
			}
			if _, queried := q.queried[peer.ID]; !queried {
				q.addToPending(peerNode)
			}
		}

	case MessageTypeGetPeerRecordResponse:
		// v2.0 新增：处理 GET_PEER_RECORD 响应
		if len(response.SignedRecord) > 0 {
			// 找到 PeerRecord
			q.value = response.SignedRecord
			q.foundValue = true
		} else {
			// 返回更近的节点
			for _, peer := range response.CloserPeers {
				peerNode := &RoutingNode{
					ID:       peer.ID,
					Addrs:    peer.Addrs,
					LastSeen: time.Now(),
				}
				if _, queried := q.queried[peer.ID]; !queried {
					q.addToPending(peerNode)
				}
			}
		}
	}

	// 检查是否达到终止条件
	// v2.0.1: 使用 closeDone() 确保只关闭一次，避免 panic
	if q.foundValue || len(q.result) >= BucketSize {
		q.closeDone()
	}
}

// addToPending 添加节点到待查询列表（保持按距离排序）
func (q *iterativeQuery) addToPending(node *RoutingNode) {
	// 检查是否已存在
	for _, n := range q.pending {
		if n.ID == node.ID {
			return
		}
	}

	q.pending = append(q.pending, node)

	// 按照与目标的距离排序
	sort.Slice(q.pending, func(i, j int) bool {
		return CompareDistance(q.pending[i].ID, q.pending[j].ID, q.target) < 0
	})

	// 保持 pending 列表在合理大小
	if len(q.pending) > BucketSize*2 {
		q.pending = q.pending[:BucketSize*2]
	}
}

// containsNode 检查节点是否在列表中
func (q *iterativeQuery) containsNode(nodes []*RoutingNode, id types.NodeID) bool {
	for _, n := range nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

// GetResult 获取查询结果
func (q *iterativeQuery) GetResult() []*RoutingNode {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 按距离排序
	sort.Slice(q.result, func(i, j int) bool {
		return CompareDistance(q.result[i].ID, q.result[j].ID, q.target) < 0
	})

	// 返回最近的 K 个
	if len(q.result) > BucketSize {
		return q.result[:BucketSize]
	}
	return q.result
}

// GetValue 获取查询到的值
func (q *iterativeQuery) GetValue() []byte {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.value
}

// GetProviders 获取查询到的 providers
func (q *iterativeQuery) GetProviders() []types.PeerInfo {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.providers
}
