// Package bootstrap 提供引导节点发现服务
//
// 本文件实现 Random Walk 主动发现，用于发现新节点并填充存储。
package bootstrap

import (
	"context"
	"crypto/rand"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// DiscoveryWalker Random Walk 发现服务
// ════════════════════════════════════════════════════════════════════════════

// DiscoveryWalker 主动发现服务
// 通过 Random Walk 在 DHT 中发现新节点
type DiscoveryWalker struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	// 依赖
	host  pkgif.Host
	store *ExtendedNodeStore
	dht   pkgif.DHT

	// 配置
	interval time.Duration
	walkLen  int

	// 状态
	running atomic.Bool
	lastRun time.Time
	mu      sync.RWMutex

	// 统计
	stats WalkerStats
}

// WalkerStats 发现统计
type WalkerStats struct {
	TotalWalks      int64
	NodesDiscovered int64
	LastWalkTime    time.Time
}

// WalkerOption 发现选项
type WalkerOption func(*DiscoveryWalker)

// WithWalkerInterval 设置发现间隔
func WithWalkerInterval(d time.Duration) WalkerOption {
	return func(w *DiscoveryWalker) {
		w.interval = d
	}
}

// WithWalkLen 设置 Walk 步数
func WithWalkLen(len int) WalkerOption {
	return func(w *DiscoveryWalker) {
		w.walkLen = len
	}
}

// NewDiscoveryWalker 创建发现服务
func NewDiscoveryWalker(host pkgif.Host, store *ExtendedNodeStore, dht pkgif.DHT, opts ...WalkerOption) *DiscoveryWalker {
	defaults := GetDefaults()

	ctx, cancel := context.WithCancel(context.Background())

	w := &DiscoveryWalker{
		ctx:       ctx,
		ctxCancel: cancel,
		host:      host,
		store:     store,
		dht:       dht,
		interval:  defaults.DiscoveryInterval,
		walkLen:   defaults.DiscoveryWalkLen,
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// ════════════════════════════════════════════════════════════════════════════
// 生命周期
// ════════════════════════════════════════════════════════════════════════════

// Start 启动发现服务
func (w *DiscoveryWalker) Start() error {
	if !w.running.CompareAndSwap(false, true) {
		return nil // 已在运行
	}

	go w.runLoop()
	return nil
}

// Stop 停止发现服务
func (w *DiscoveryWalker) Stop() error {
	if !w.running.CompareAndSwap(true, false) {
		return nil // 未在运行
	}

	if w.ctxCancel != nil {
		w.ctxCancel()
	}
	return nil
}

// IsRunning 检查是否在运行
func (w *DiscoveryWalker) IsRunning() bool {
	return w.running.Load()
}

// Stats 返回统计信息
func (w *DiscoveryWalker) Stats() WalkerStats {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.stats
}

// LastRun 返回最后运行时间
func (w *DiscoveryWalker) LastRun() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastRun
}

// ════════════════════════════════════════════════════════════════════════════
// 发现循环
// ════════════════════════════════════════════════════════════════════════════

// runLoop 运行发现循环
func (w *DiscoveryWalker) runLoop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// 立即执行一次
	w.runWalk()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.runWalk()
		}
	}
}

// runWalk 执行一轮 Random Walk
func (w *DiscoveryWalker) runWalk() {
	w.mu.Lock()
	w.lastRun = time.Now()
	w.stats.TotalWalks++
	w.mu.Unlock()

	// 执行多次 FIND_NODE
	for i := 0; i < w.walkLen; i++ {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		// 生成随机目标 ID
		targetID := w.generateRandomID()

		// 通过路由发现最近节点
		peers, err := w.findClosest(w.ctx, targetID)
		if err != nil {
			continue
		}

		// 将发现的节点添加到存储
		for _, peer := range peers {
			w.addToStore(peer)
		}
	}

	w.mu.Lock()
	w.stats.LastWalkTime = time.Now()
	w.mu.Unlock()
}

// generateRandomID 生成随机节点 ID
func (w *DiscoveryWalker) generateRandomID() types.NodeID {
	// 生成 32 字节的随机 ID
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return types.NodeID(id)
}

// findClosest 查找最近的节点
func (w *DiscoveryWalker) findClosest(_ context.Context, target types.NodeID) ([]types.PeerInfo, error) {
	if w.dht == nil {
		return nil, nil
	}

	// 使用 DHT 路由表查找最近节点
	routeTable := w.dht.RoutingTable()
	if routeTable == nil {
		return nil, nil
	}

	// 获取最近的 K 个节点 ID
	peerIDs := routeTable.NearestPeers(string(target), DefaultResponseK)
	if len(peerIDs) == 0 {
		return nil, nil
	}

	// 转换为 PeerInfo（从 Peerstore 获取地址）
	peers := make([]types.PeerInfo, 0, len(peerIDs))
	peerstore := w.host.Peerstore()
	for _, peerID := range peerIDs {
		// 从 Peerstore 获取地址
		addrs := peerstore.Addrs(types.PeerID(peerID))
		if len(addrs) > 0 {
			peers = append(peers, types.PeerInfo{
				ID:    types.NodeID(peerID),
				Addrs: addrs,
			})
		}
	}

	return peers, nil
}

// addToStore 添加节点到存储
func (w *DiscoveryWalker) addToStore(peer types.PeerInfo) {
	if peer.ID == "" || len(peer.Addrs) == 0 {
		return
	}

	// 检查是否已存在
	if _, exists := w.store.Get(types.NodeID(peer.ID)); exists {
		return
	}

	// 转换地址格式
	addrs := make([]string, len(peer.Addrs))
	for i, addr := range peer.Addrs {
		addrs[i] = addr.String()
	}

	// 创建新条目
	entry := &NodeEntry{
		ID:        types.NodeID(peer.ID),
		Addrs:     addrs,
		LastSeen:  time.Now(),
		Status:    NodeStatusUnknown,
		CreatedAt: time.Now(),
	}

	if err := w.store.Put(entry); err == nil {
		w.mu.Lock()
		w.stats.NodesDiscovered++
		w.mu.Unlock()
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 手动触发
// ════════════════════════════════════════════════════════════════════════════

// WalkNow 立即执行一轮发现
func (w *DiscoveryWalker) WalkNow() {
	go w.runWalk()
}

// ════════════════════════════════════════════════════════════════════════════
// 辅助方法
// ════════════════════════════════════════════════════════════════════════════

// AddFromPeers 从 PeerInfo 列表添加节点
// 用于从其他来源（如 DHT）添加发现的节点
func (w *DiscoveryWalker) AddFromPeers(peers []types.PeerInfo) int {
	added := 0
	for _, peer := range peers {
		w.addToStore(peer)
		added++
	}
	return added
}
