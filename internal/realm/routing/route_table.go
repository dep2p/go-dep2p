package routing

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              路由表实现
// ============================================================================

// RouteTable 路由表（基于 DHT 路由表）
type RouteTable struct {
	mu sync.RWMutex

	// 本地节点 ID
	localPeerID string

	// 节点映射
	nodes map[string]*interfaces.RouteNode

	// 配置
	maxSize    int
	expireTime time.Duration
}

// NewRouteTable 创建路由表
func NewRouteTable(localPeerID string) *RouteTable {
	return &RouteTable{
		localPeerID: localPeerID,
		nodes:       make(map[string]*interfaces.RouteNode),
		maxSize:     1000,
		expireTime:  30 * time.Minute,
	}
}

// AddNode 添加节点
func (rt *RouteTable) AddNode(node *interfaces.RouteNode) error {
	if node == nil {
		return ErrInvalidNode
	}

	if node.PeerID == "" {
		return ErrInvalidNode
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// 检查容量
	if len(rt.nodes) >= rt.maxSize && rt.nodes[node.PeerID] == nil {
		return ErrTableFull
	}

	// 添加或更新节点
	rt.nodes[node.PeerID] = node

	return nil
}

// RemoveNode 移除节点
func (rt *RouteTable) RemoveNode(peerID string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if _, ok := rt.nodes[peerID]; !ok {
		return ErrNodeNotFound
	}

	delete(rt.nodes, peerID)
	return nil
}

// GetNode 获取节点
func (rt *RouteTable) GetNode(peerID string) (*interfaces.RouteNode, error) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	node, ok := rt.nodes[peerID]
	if !ok {
		return nil, ErrNodeNotFound
	}

	return node, nil
}

// NearestPeers 返回距离目标最近的 K 个节点
//
// 使用 Kademlia XOR 距离度量，找到与目标 ID 距离最近的节点。
// 这是 DHT 查找的核心操作。
func (rt *RouteTable) NearestPeers(targetID string, count int) []*interfaces.RouteNode {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// 收集所有可达节点及其到目标的距离
	type nodeWithDistance struct {
		node     *interfaces.RouteNode
		distance *big.Int
	}

	candidates := make([]nodeWithDistance, 0, len(rt.nodes))
	for _, node := range rt.nodes {
		if node.IsReachable {
			dist := rt.XORDistance(node.PeerID, targetID)
			candidates = append(candidates, nodeWithDistance{
				node:     node,
				distance: dist,
			})
		}
	}

	// 按 XOR 距离排序（从近到远）
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].distance.Cmp(candidates[j].distance) < 0
	})

	// 返回最近的 K 个
	result := make([]*interfaces.RouteNode, 0, count)
	for i := 0; i < len(candidates) && i < count; i++ {
		result = append(result, candidates[i].node)
	}

	return result
}

// Size 返回路由表大小
func (rt *RouteTable) Size() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	return len(rt.nodes)
}

// Update 更新节点信息
func (rt *RouteTable) Update(peerID string, latency time.Duration) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	node, ok := rt.nodes[peerID]
	if !ok {
		return ErrNodeNotFound
	}

	node.Latency = latency
	node.LastSeen = time.Now()

	return nil
}

// GetAllNodes 获取所有节点
func (rt *RouteTable) GetAllNodes() []*interfaces.RouteNode {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	nodes := make([]*interfaces.RouteNode, 0, len(rt.nodes))
	for _, node := range rt.nodes {
		nodes = append(nodes, node)
	}

	return nodes
}

// CleanupExpired 清理过期节点
func (rt *RouteTable) CleanupExpired() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := time.Now()
	removed := 0

	for peerID, node := range rt.nodes {
		if now.Sub(node.LastSeen) > rt.expireTime {
			delete(rt.nodes, peerID)
			removed++
		}
	}

	return removed
}

// CalculateDistance 计算节点距离（Kademlia XOR 距离）
//
// XOR 距离是 Kademlia DHT 的核心度量：
//   - 将节点 ID 视为大整数
//   - 两个 ID 的距离 = ID1 XOR ID2
//   - 返回 XOR 结果的位数（前导零的数量决定距离的"桶"）
//
// 距离性质：
//   - d(x, x) = 0
//   - d(x, y) = d(y, x)
//   - d(x, z) <= d(x, y) + d(y, z)
func (rt *RouteTable) CalculateDistance(peerID1, peerID2 string) int {
	// 将 PeerID 转换为哈希（确保长度一致）
	hash1 := peerIDToHash(peerID1)
	hash2 := peerIDToHash(peerID2)

	// 计算 XOR
	xorResult := xorBytes(hash1, hash2)

	// 计算距离（前导零的数量 = 256 - 有效位数）
	// 返回有效位数作为距离度量
	return countLeadingZeros(xorResult)
}

// XORDistance 返回完整的 XOR 距离（用于精确比较）
func (rt *RouteTable) XORDistance(peerID1, peerID2 string) *big.Int {
	hash1 := peerIDToHash(peerID1)
	hash2 := peerIDToHash(peerID2)

	xorResult := xorBytes(hash1, hash2)

	return new(big.Int).SetBytes(xorResult)
}

// peerIDToHash 将 PeerID 转换为固定长度哈希
func peerIDToHash(peerID string) []byte {
	// 尝试解码 hex 格式的 PeerID
	if decoded, err := hex.DecodeString(peerID); err == nil && len(decoded) == 32 {
		return decoded
	}

	// 否则使用 SHA256 哈希
	hash := sha256.Sum256([]byte(peerID))
	return hash[:]
}

// xorBytes 对两个字节数组进行 XOR
func xorBytes(a, b []byte) []byte {
	// 确保长度一致
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	result := make([]byte, minLen)
	for i := 0; i < minLen; i++ {
		result[i] = a[i] ^ b[i]
	}

	return result
}

// countLeadingZeros 计算前导零位数
func countLeadingZeros(data []byte) int {
	zeros := 0
	for _, b := range data {
		if b == 0 {
			zeros += 8
			continue
		}
		// 计算这个字节的前导零
		for i := 7; i >= 0; i-- {
			if (b>>i)&1 == 0 {
				zeros++
			} else {
				return zeros
			}
		}
	}
	return zeros
}

// GetReachableNodes 获取所有可达节点
func (rt *RouteTable) GetReachableNodes() []*interfaces.RouteNode {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	reachable := make([]*interfaces.RouteNode, 0, len(rt.nodes))
	for _, node := range rt.nodes {
		if node.IsReachable {
			reachable = append(reachable, node)
		}
	}

	return reachable
}

// MarkUnreachable 标记节点不可达
func (rt *RouteTable) MarkUnreachable(peerID string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	node, ok := rt.nodes[peerID]
	if !ok {
		return ErrNodeNotFound
	}

	node.IsReachable = false
	return nil
}

// MarkReachable 标记节点可达
func (rt *RouteTable) MarkReachable(peerID string) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	node, ok := rt.nodes[peerID]
	if !ok {
		return ErrNodeNotFound
	}

	node.IsReachable = true
	node.LastSeen = time.Now()
	return nil
}

// GetStats 获取路由表统计
func (rt *RouteTable) GetStats() RouteTableStats {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	stats := RouteTableStats{
		TotalNodes: len(rt.nodes),
	}

	for _, node := range rt.nodes {
		if node.IsReachable {
			stats.ReachableNodes++
		}
	}

	return stats
}

// RouteTableStats 路由表统计
type RouteTableStats struct {
	TotalNodes     int
	ReachableNodes int
}

// 确保实现接口
var _ interfaces.RouteTable = (*RouteTable)(nil)
