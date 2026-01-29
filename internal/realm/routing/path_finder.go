package routing

import (
	"container/heap"
	"context"
	"math"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              路径查找器
// ============================================================================

// PathFinder 路径查找器
type PathFinder struct {
	mu sync.RWMutex

	// 组件
	table  interfaces.RouteTable
	prober interfaces.LatencyProber

	// 路径缓存
	pathCache map[string]*interfaces.Path
}

// NewPathFinder 创建路径查找器
func NewPathFinder(table interfaces.RouteTable, prober interfaces.LatencyProber) *PathFinder {
	return &PathFinder{
		table:     table,
		prober:    prober,
		pathCache: make(map[string]*interfaces.Path),
	}
}

// ============================================================================
//                              Dijkstra 最短路径
// ============================================================================

// FindShortestPath 查找最短路径
func (pf *PathFinder) FindShortestPath(_ context.Context, source, target string) (*interfaces.Path, error) {
	// 1. 检查缓存
	if path, ok := pf.getCachedPath(target); ok && path.Valid {
		return path, nil
	}

	// 2. 检查是否可以直连
	if source == target {
		return &interfaces.Path{
			Nodes:        []string{source},
			TotalLatency: 0,
			Hops:         0,
			Valid:        true,
		}, nil
	}

	// 3. Dijkstra 算法
	path, err := pf.dijkstra(source, target)
	if err != nil {
		return nil, err
	}

	// 4. 缓存路径
	pf.cachePath(target, path)

	return path, nil
}

// dijkstra Dijkstra 算法实现
func (pf *PathFinder) dijkstra(source, target string) (*interfaces.Path, error) {
	// 初始化
	dist := make(map[string]time.Duration)
	prev := make(map[string]string)
	visited := make(map[string]bool)

	// 获取所有节点
	allNodes := pf.table.GetAllNodes()
	if len(allNodes) == 0 {
		return nil, ErrPathNotFound
	}

	// 初始化距离
	for _, node := range allNodes {
		dist[node.PeerID] = time.Duration(math.MaxInt64)
	}
	dist[source] = 0

	// 优先队列
	pq := make(PriorityQueue, 0)
	heap.Init(&pq)
	heap.Push(&pq, &Item{
		peerID:   source,
		priority: 0,
	})

	// Dijkstra 主循环
	for pq.Len() > 0 {
		current := heap.Pop(&pq).(*Item)
		currentID := current.peerID

		if visited[currentID] {
			continue
		}

		visited[currentID] = true

		// 找到目标
		if currentID == target {
			break
		}

		// 遍历邻居节点
		neighbors := pf.table.NearestPeers(currentID, 20)
		for _, neighbor := range neighbors {
			if visited[neighbor.PeerID] {
				continue
			}

			// 计算新距离
			edgeWeight := neighbor.Latency
			if edgeWeight == 0 {
				edgeWeight = 10 * time.Millisecond // 默认延迟
			}

			newDist := dist[currentID] + edgeWeight

			if newDist < dist[neighbor.PeerID] {
				dist[neighbor.PeerID] = newDist
				prev[neighbor.PeerID] = currentID

				heap.Push(&pq, &Item{
					peerID:   neighbor.PeerID,
					priority: int64(newDist),
				})
			}
		}
	}

	// 重构路径
	if _, ok := prev[target]; !ok && target != source {
		return nil, ErrPathNotFound
	}

	path := make([]string, 0)
	current := target
	for current != source {
		path = append([]string{current}, path...)
		current = prev[current]
	}
	path = append([]string{source}, path...)

	return &interfaces.Path{
		Nodes:        path,
		TotalLatency: dist[target],
		Hops:         len(path) - 1,
		Valid:        true,
	}, nil
}

// ============================================================================
//                              多路径查找
// ============================================================================

// FindMultiplePaths 查找多条路径（Yen's K-shortest paths 算法）
//
// 实现 Yen 算法查找 K 条最短路径：
//  1. 首先用 Dijkstra 找到最短路径 A[1]
//  2. 对于 k = 2 到 K：
//     a. 对于 A[k-1] 中的每个偏离点 i：
//        - 移除偏离点之后的边
//        - 移除与已有路径冲突的边
//        - 计算从偏离点到目标的最短路径
//        - 组合成候选路径
//     b. 从候选路径中选择最短的作为 A[k]
func (pf *PathFinder) FindMultiplePaths(ctx context.Context, source, target string, count int) ([]*interfaces.Path, error) {
	if count <= 0 {
		return nil, nil
	}

	// 结果路径集合
	paths := make([]*interfaces.Path, 0, count)

	// 1. 查找第一条最短路径
	firstPath, err := pf.FindShortestPath(ctx, source, target)
	if err != nil {
		return nil, err
	}
	paths = append(paths, firstPath)

	if count == 1 {
		return paths, nil
	}

	// 候选路径集合（使用 map 去重）
	candidates := make(map[string]*interfaces.Path)

	// 2. 迭代查找剩余路径
	for k := 1; k < count; k++ {
		// 上一条路径
		prevPath := paths[k-1]

		// 遍历偏离点
		for i := 0; i < len(prevPath.Nodes)-1; i++ {
			// 偏离点
			spurNode := prevPath.Nodes[i]

			// 根路径（从源到偏离点）
			rootPath := prevPath.Nodes[:i+1]
			rootLatency := time.Duration(0)
			if i > 0 {
				// 计算根路径延迟（简化：均分）
				rootLatency = prevPath.TotalLatency * time.Duration(i) / time.Duration(len(prevPath.Nodes)-1)
			}

			// 创建临时图（移除冲突边）
			removedEdges := pf.collectRemovedEdges(paths, rootPath)

			// 从偏离点查找到目标的路径（使用修改后的图）
			spurPath, err := pf.dijkstraWithRemovedEdges(spurNode, target, removedEdges)
			if err != nil {
				continue // 无法找到偏离路径
			}

			// 组合完整路径
			fullNodes := make([]string, 0, len(rootPath)+len(spurPath.Nodes)-1)
			fullNodes = append(fullNodes, rootPath...)
			if len(spurPath.Nodes) > 1 {
				fullNodes = append(fullNodes, spurPath.Nodes[1:]...)
			}

			totalLatency := rootLatency + spurPath.TotalLatency

			candidatePath := &interfaces.Path{
				Nodes:        fullNodes,
				TotalLatency: totalLatency,
				Hops:         len(fullNodes) - 1,
				Valid:        true,
			}

			// 添加到候选集合（使用路径字符串作为 key 去重）
			pathKey := pathToKey(fullNodes)
			if _, exists := candidates[pathKey]; !exists {
				candidates[pathKey] = candidatePath
			}
		}

		// 从候选路径中选择最短的
		var bestCandidate *interfaces.Path
		var bestKey string
		for key, candidate := range candidates {
			if bestCandidate == nil || candidate.TotalLatency < bestCandidate.TotalLatency {
				bestCandidate = candidate
				bestKey = key
			}
		}

		if bestCandidate == nil {
			break // 没有更多候选路径
		}

		// 添加到结果集
		paths = append(paths, bestCandidate)
		delete(candidates, bestKey)
	}

	return paths, nil
}

// collectRemovedEdges 收集需要移除的边
func (pf *PathFinder) collectRemovedEdges(existingPaths []*interfaces.Path, rootPath []string) map[string]map[string]bool {
	removed := make(map[string]map[string]bool)

	for _, path := range existingPaths {
		// 检查是否有相同的根路径
		if len(path.Nodes) >= len(rootPath) {
			match := true
			for i := 0; i < len(rootPath); i++ {
				if path.Nodes[i] != rootPath[i] {
					match = false
					break
				}
			}
			if match && len(rootPath) < len(path.Nodes) {
				// 移除根路径最后节点到下一节点的边
				from := rootPath[len(rootPath)-1]
				to := path.Nodes[len(rootPath)]
				if removed[from] == nil {
					removed[from] = make(map[string]bool)
				}
				removed[from][to] = true
			}
		}
	}

	return removed
}

// dijkstraWithRemovedEdges Dijkstra 算法（排除某些边）
func (pf *PathFinder) dijkstraWithRemovedEdges(source, target string, removedEdges map[string]map[string]bool) (*interfaces.Path, error) {
	dist := make(map[string]time.Duration)
	prev := make(map[string]string)
	visited := make(map[string]bool)

	allNodes := pf.table.GetAllNodes()
	if len(allNodes) == 0 {
		return nil, ErrPathNotFound
	}

	for _, node := range allNodes {
		dist[node.PeerID] = time.Duration(math.MaxInt64)
	}
	dist[source] = 0

	pq := make(PriorityQueue, 0)
	heap.Init(&pq)
	heap.Push(&pq, &Item{peerID: source, priority: 0})

	for pq.Len() > 0 {
		current := heap.Pop(&pq).(*Item)
		currentID := current.peerID

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		if currentID == target {
			break
		}

		neighbors := pf.table.NearestPeers(currentID, 20)
		for _, neighbor := range neighbors {
			if visited[neighbor.PeerID] {
				continue
			}

			// 检查边是否被移除
			if removedEdges[currentID] != nil && removedEdges[currentID][neighbor.PeerID] {
				continue
			}

			edgeWeight := neighbor.Latency
			if edgeWeight == 0 {
				edgeWeight = 10 * time.Millisecond
			}

			newDist := dist[currentID] + edgeWeight
			if newDist < dist[neighbor.PeerID] {
				dist[neighbor.PeerID] = newDist
				prev[neighbor.PeerID] = currentID
				heap.Push(&pq, &Item{peerID: neighbor.PeerID, priority: int64(newDist)})
			}
		}
	}

	if _, ok := prev[target]; !ok && target != source {
		return nil, ErrPathNotFound
	}

	path := make([]string, 0)
	current := target
	for current != source {
		path = append([]string{current}, path...)
		current = prev[current]
	}
	path = append([]string{source}, path...)

	return &interfaces.Path{
		Nodes:        path,
		TotalLatency: dist[target],
		Hops:         len(path) - 1,
		Valid:        true,
	}, nil
}

// pathToKey 将路径转换为字符串 key（用于去重）
func pathToKey(nodes []string) string {
	key := ""
	for i, node := range nodes {
		if i > 0 {
			key += "->"
		}
		key += node
	}
	return key
}

// ============================================================================
//                              路径评分
// ============================================================================

// ScorePath 评分路径
func (pf *PathFinder) ScorePath(path *interfaces.Path) float64 {
	if path == nil || !path.Valid {
		return 0
	}

	// 延迟评分
	latencyScore := 1.0 / (1.0 + float64(path.TotalLatency.Milliseconds())/100.0)

	// 跳数评分
	hopsScore := 1.0 / (1.0 + float64(path.Hops))

	// 综合评分（权重：延迟 0.6, 跳数 0.4）
	score := latencyScore*0.6 + hopsScore*0.4

	return score
}

// ============================================================================
//                              路径缓存
// ============================================================================

// CachePath 缓存路径
func (pf *PathFinder) CachePath(path *interfaces.Path) {
	if path == nil || len(path.Nodes) < 2 {
		return
	}

	target := path.Nodes[len(path.Nodes)-1]
	pf.cachePath(target, path)
}

// InvalidatePath 使路径失效
func (pf *PathFinder) InvalidatePath(target string) {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	if path, ok := pf.pathCache[target]; ok {
		path.Valid = false
	}
}

// getCachedPath 获取缓存路径
func (pf *PathFinder) getCachedPath(target string) (*interfaces.Path, bool) {
	pf.mu.RLock()
	defer pf.mu.RUnlock()

	path, ok := pf.pathCache[target]
	return path, ok
}

// cachePath 缓存路径
func (pf *PathFinder) cachePath(target string, path *interfaces.Path) {
	pf.mu.Lock()
	defer pf.mu.Unlock()

	pf.pathCache[target] = path
}

// ============================================================================
//                              优先队列（Dijkstra 用）
// ============================================================================

// Item 优先队列项
type Item struct {
	peerID   string
	priority int64
	index    int
}

// PriorityQueue 优先队列
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].priority < pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// 确保实现接口
var _ interfaces.PathFinder = (*PathFinder)(nil)
