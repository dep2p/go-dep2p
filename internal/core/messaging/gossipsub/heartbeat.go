// Package gossipsub 实现 GossipSub v1.1 协议
package gossipsub

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              心跳管理器
// ============================================================================

// Heartbeat 心跳管理器
//
// 负责定期执行 GossipSub 的维护任务：
// - Mesh 维护（GRAFT/PRUNE）
// - Fanout 清理
// - Gossip 传播（IHAVE）
// - 评分衰减
type Heartbeat struct {
	mu sync.RWMutex

	// config 配置
	config *Config

	// mesh mesh 管理器
	mesh *MeshManager

	// cache 消息缓存
	cache *MessageCache

	// scorer 评分器
	scorer *PeerScorer

	// iwantTracker IWANT 追踪器
	iwantTracker *IWantTracker

	// sendRPC 发送 RPC 回调
	sendRPC SendRPCFunc

	// running 是否正在运行
	running bool

	// stopCh 停止通道
	stopCh chan struct{}

	// tickCount 心跳计数
	tickCount uint64

	// lastHeartbeat 最后心跳时间
	lastHeartbeat time.Time
}

// SendRPCFunc 发送 RPC 函数类型
type SendRPCFunc func(peer types.NodeID, rpc *RPC) error

// HeartbeatResult 心跳结果
type HeartbeatResult struct {
	// Grafts 需要发送 GRAFT 的 peers
	Grafts map[string][]types.NodeID

	// Prunes 需要发送 PRUNE 的 peers
	Prunes map[string][]types.NodeID

	// IHaves 需要发送 IHAVE 的 peers 和消息
	IHaves map[types.NodeID][]ControlIHaveMessage

	// Duration 心跳耗时
	Duration time.Duration
}

// NewHeartbeat 创建新的心跳管理器
func NewHeartbeat(
	config *Config,
	mesh *MeshManager,
	cache *MessageCache,
	scorer *PeerScorer,
) *Heartbeat {
	if config == nil {
		config = DefaultConfig()
	}

	return &Heartbeat{
		config:        config,
		mesh:          mesh,
		cache:         cache,
		scorer:        scorer,
		iwantTracker:  NewIWantTracker(config.IWantFollowupTime),
		stopCh:        make(chan struct{}),
		lastHeartbeat: time.Now(),
	}
}

// SetSendRPC 设置发送 RPC 回调
func (h *Heartbeat) SetSendRPC(fn SendRPCFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sendRPC = fn
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动心跳
func (h *Heartbeat) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return nil
	}
	h.running = true
	h.stopCh = make(chan struct{})
	h.mu.Unlock()

	log.Info("心跳管理器启动中",
		"interval", h.config.HeartbeatInterval)

	go h.heartbeatLoop(ctx)

	return nil
}

// Stop 停止心跳
func (h *Heartbeat) Stop() error {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return nil
	}
	h.running = false
	close(h.stopCh)
	h.mu.Unlock()

	log.Info("心跳管理器已停止")
	return nil
}

// heartbeatLoop 心跳循环
func (h *Heartbeat) heartbeatLoop(ctx context.Context) {
	// 初始延迟
	select {
	case <-time.After(h.config.HeartbeatInitialDelay):
	case <-ctx.Done():
		return
	case <-h.stopCh:
		return
	}

	ticker := time.NewTicker(h.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.doHeartbeat()
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		}
	}
}

// ============================================================================
//                              心跳执行
// ============================================================================

// doHeartbeat 执行一次心跳
func (h *Heartbeat) doHeartbeat() {
	start := time.Now()

	h.mu.Lock()
	h.tickCount++
	tickCount := h.tickCount
	h.lastHeartbeat = start
	h.mu.Unlock()

	// 1. 执行评分衰减
	if h.scorer != nil {
		h.scorer.Decay()
	}

	// 2. Mesh 维护
	grafts, prunes := h.mesh.HeartbeatMaintenance()

	// 3. 发送 GRAFT 消息
	for topic, peers := range grafts {
		for _, peer := range peers {
			h.sendGraft(peer, topic)
		}
	}

	// 4. 发送 PRUNE 消息
	for topic, peers := range prunes {
		for _, peer := range peers {
			pxPeers := h.mesh.GetPXPeers(topic, peer, 10)
			h.sendPrune(peer, topic, pxPeers)
		}
	}

	// 5. Fanout 清理
	h.mesh.CleanupFanout()

	// 6. 缓存移动窗口
	if h.cache != nil {
		h.cache.Shift()
	}

	// 7. Gossip 传播（IHAVE）
	h.emitGossip()

	// 8. 处理未履行的 IWANT
	h.handleBrokenPromises()

	// 9. 机会性 GRAFT
	if tickCount%uint64(h.config.OpportunisticGraftTicks) == 0 {
		h.opportunisticGraft()
	}

	duration := time.Since(start)
	if duration > h.config.SlowHeartbeatWarning {
		log.Warn("心跳执行过慢",
			"duration", duration,
			"tick", tickCount)
	}

	log.Debug("心跳完成",
		"tick", tickCount,
		"duration", duration,
		"grafts", countMapValues(grafts),
		"prunes", countMapValues(prunes))
}

// emitGossip 发送 gossip（IHAVE 消息）
func (h *Heartbeat) emitGossip() {
	if h.cache == nil {
		return
	}

	topics := h.mesh.Topics()

	for _, topic := range topics {
		// 获取最近的消息 ID
		msgIDs := h.cache.GetGossipIDs(topic)
		if len(msgIDs) == 0 {
			continue
		}

		// 限制 IHAVE 消息大小
		if len(msgIDs) > h.config.MaxIHaveLength {
			msgIDs = msgIDs[:h.config.MaxIHaveLength]
		}

		// 选择 gossip 目标
		gossipPeers := h.mesh.SelectGossipPeers(topic)
		if len(gossipPeers) == 0 {
			continue
		}

		// 发送 IHAVE
		for _, peer := range gossipPeers {
			h.sendIHave(peer, topic, msgIDs)
		}
	}
}

// handleBrokenPromises 处理未履行的 IWANT 请求
func (h *Heartbeat) handleBrokenPromises() {
	broken := h.iwantTracker.GetBrokenPromises()
	for peerStr, count := range broken {
		// 解析 peer ID
		peer, err := types.ParseNodeID(peerStr)
		if err != nil || peer == (types.NodeID{}) {
			continue
		}

		// 记录惩罚
		if h.scorer != nil {
			for i := 0; i < count; i++ {
				h.scorer.BrokenPromise(peer)
			}
		}

		log.Debug("Peer 未履行 IWANT 请求",
			"peer", peerStr,
			"count", count)
	}
}

// opportunisticGraft 机会性 GRAFT
func (h *Heartbeat) opportunisticGraft() {
	if h.scorer == nil {
		return
	}

	topics := h.mesh.Topics()

	for _, topic := range topics {
		// 检查 mesh 中位数评分
		meshPeers := h.mesh.MeshPeers(topic)
		if len(meshPeers) == 0 {
			continue
		}

		// 计算中位数评分
		scores := make([]float64, len(meshPeers))
		for i, peer := range meshPeers {
			scores[i] = h.scorer.Score(peer)
		}
		medianScore := median(scores)

		// 如果中位数低于阈值，尝试 GRAFT 高分 peer
		if medianScore < h.config.OpportunisticGraftThreshold {
			gossipPeers := h.mesh.SelectGossipPeers(topic)
			for _, peer := range gossipPeers {
				if h.scorer.Score(peer) > medianScore {
					h.mesh.Graft(peer, topic)
					h.sendGraft(peer, topic)
					log.Debug("机会性 GRAFT",
						"topic", topic,
						"peer", peer.String())
					break // 每次只 GRAFT 一个
				}
			}
		}
	}
}

// ============================================================================
//                              消息发送
// ============================================================================

// sendGraft 发送 GRAFT 消息
func (h *Heartbeat) sendGraft(peer types.NodeID, topic string) {
	h.mu.RLock()
	sendRPC := h.sendRPC
	h.mu.RUnlock()

	if sendRPC == nil {
		return
	}

	rpc := &RPC{
		Control: &ControlMessage{
			Graft: []ControlGraftMessage{{Topic: topic}},
		},
	}

	if err := sendRPC(peer, rpc); err != nil {
		log.Debug("发送 GRAFT 失败",
			"peer", peer.String(),
			"topic", topic,
			"err", err)
	}
}

// sendPrune 发送 PRUNE 消息
func (h *Heartbeat) sendPrune(peer types.NodeID, topic string, pxPeers []PeerInfo) {
	h.mu.RLock()
	sendRPC := h.sendRPC
	h.mu.RUnlock()

	if sendRPC == nil {
		return
	}

	rpc := &RPC{
		Control: &ControlMessage{
			Prune: []ControlPruneMessage{{
				Topic:   topic,
				Peers:   pxPeers,
				Backoff: uint64(h.config.PruneBackoff.Seconds()),
			}},
		},
	}

	if err := sendRPC(peer, rpc); err != nil {
		log.Debug("发送 PRUNE 失败",
			"peer", peer.String(),
			"topic", topic,
			"err", err)
	}
}

// sendIHave 发送 IHAVE 消息
func (h *Heartbeat) sendIHave(peer types.NodeID, topic string, msgIDs [][]byte) {
	h.mu.RLock()
	sendRPC := h.sendRPC
	h.mu.RUnlock()

	if sendRPC == nil {
		return
	}

	rpc := &RPC{
		Control: &ControlMessage{
			IHave: []ControlIHaveMessage{{
				Topic:      topic,
				MessageIDs: msgIDs,
			}},
		},
	}

	if err := sendRPC(peer, rpc); err != nil {
		log.Debug("发送 IHAVE 失败",
			"peer", peer.String(),
			"topic", topic,
			"err", err)
	}
}

// ============================================================================
//                              IWANT 追踪
// ============================================================================

// TrackIWant 追踪发送的 IWANT
func (h *Heartbeat) TrackIWant(msgID []byte, peer types.NodeID) {
	h.iwantTracker.Track(msgID, peer.String())
}

// FulfillIWant 消息已收到
func (h *Heartbeat) FulfillIWant(msgID []byte) {
	h.iwantTracker.Fulfill(msgID)
}

// ============================================================================
//                              查询方法
// ============================================================================

// IsRunning 检查是否正在运行
func (h *Heartbeat) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

// TickCount 返回心跳计数
func (h *Heartbeat) TickCount() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.tickCount
}

// LastHeartbeat 返回最后心跳时间
func (h *Heartbeat) LastHeartbeat() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastHeartbeat
}

// ============================================================================
//                              辅助函数
// ============================================================================

// countMapValues 计算 map 中所有值的总数
func countMapValues(m map[string][]types.NodeID) int {
	count := 0
	for _, v := range m {
		count += len(v)
	}
	return count
}

// median 计算中位数
// 使用 O(n log n) 的标准库排序算法替代原有 O(n²) 冒泡排序
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// 复制并排序（O(n log n)）
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

