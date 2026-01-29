package connmgr

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/connmgr/msgrate"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

var logger = log.Logger("core/connmgr")

// truncateID 安全截取 ID 用于日志显示
func truncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// connInfo 连接信息
type connInfo struct {
	peerID    string
	direction pkgif.Direction
	createdAt time.Time
	lastActive time.Time
}

// Manager 连接管理器（彻底重构：集成 msgrate）
type Manager struct {
	cfg      Config
	tags     *tagStore
	protects *protectStore
	jitter   *JitterTolerance // 抖动容忍器

	// 彻底重构：集成 msgrate 消息速率追踪
	rateTrackers msgrate.Trackers

	mu     sync.RWMutex
	closed bool

	// 连接信息存储
	connInfos map[string]*connInfo // peerID -> connInfo

	// host 用于获取连接列表（可选，用于实际回收）
	host Host

	// 裁剪触发通道
	trimCh chan struct{}
}

// Host 定义获取连接的最小接口
type Host interface {
	// Connections 返回所有连接
	Connections() []Connection
	// CloseConnection 关闭指定节点的连接
	CloseConnection(peer string) error
}

// Connection 定义连接的最小接口
type Connection interface {
	// RemotePeer 返回远端节点 ID
	RemotePeer() string
	// Close 关闭连接
	Close() error
}

var _ pkgif.ConnManager = (*Manager)(nil)

// New 创建连接管理器（彻底重构：集成 msgrate）
func New(cfg Config) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 创建抖动容忍器
	jitterCfg := DefaultJitterConfig()
	jitter := NewJitterTolerance(jitterCfg)

	// 如果EmergencyWater未设置，使用HighWater * 1.5
	if cfg.EmergencyWater == 0 {
		cfg.EmergencyWater = cfg.HighWater * 3 / 2
	}

	// 彻底重构：初始化消息速率追踪器
	rateTrackers := msgrate.NewTrackers(msgrate.DefaultConfig())

	return &Manager{
		cfg:          cfg,
		tags:         newTagStore(),
		protects:     newProtectStore(),
		jitter:       jitter,
		rateTrackers: rateTrackers,
		closed:       false,
		connInfos:    make(map[string]*connInfo),
		trimCh:       make(chan struct{}, 1),
	}, nil
}

// TagPeer 为节点添加标签
func (m *Manager) TagPeer(peerID string, tag string, weight int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return
	}

	logger.Debug("为节点添加标签", "peerID", truncateID(peerID, 8), "tag", tag, "weight", weight)
	m.tags.Set(peerID, tag, weight)
}

// UntagPeer 移除节点标签
func (m *Manager) UntagPeer(peerID string, tag string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return
	}

	m.tags.Delete(peerID, tag)
}

// UpsertTag 更新或插入节点标签
func (m *Manager) UpsertTag(peerID string, tag string, upsert func(int) int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return
	}

	m.tags.Upsert(peerID, tag, upsert)
}

// GetTagInfo 获取节点的标签信息
func (m *Manager) GetTagInfo(peerID string) *pkgif.TagInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil
	}

	return m.tags.GetInfo(peerID)
}

// Protect 保护节点连接不被裁剪
func (m *Manager) Protect(peerID string, tag string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return
	}

	logger.Debug("保护节点", "peerID", truncateID(peerID, 8), "tag", tag)
	m.protects.Protect(peerID, tag)
}

// Unprotect 取消节点保护
// 返回 true 表示还有其他保护标签
func (m *Manager) Unprotect(peerID string, tag string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return false
	}

	return m.protects.Unprotect(peerID, tag)
}

// IsProtected 检查节点是否受保护
func (m *Manager) IsProtected(peerID string, tag string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return false
	}

	return m.protects.IsProtected(peerID, tag)
}

// TrimOpenConns 裁剪连接到目标数量
func (m *Manager) TrimOpenConns(ctx context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return
	}

	if m.host == nil {
		// 没有 Host，无法回收（仅标签管理模式）
		return
	}

	logger.Debug("裁剪连接", "target", m.cfg.LowWater)
	m.trimToTarget(ctx, m.cfg.LowWater)
}

// Notifee 返回连接通知接口
//
// 返回一个实现 SwarmNotifier 接口的对象，用于接收 Swarm 连接事件。
func (m *Manager) Notifee() pkgif.SwarmNotifier {
	return &managerNotifee{mgr: m}
}

// managerNotifee ConnManager 的 Swarm 通知器实现
type managerNotifee struct {
	mgr *Manager
}

// Connected 当连接建立时调用
func (n *managerNotifee) Connected(conn pkgif.Connection) {
	if n.mgr == nil {
		return
	}
	
	peerID := string(conn.RemotePeer())
	
	// 获取连接方向
	stat := conn.Stat()
	direction := stat.Direction
	
	// 记录连接信息
	n.mgr.mu.Lock()
	now := time.Now()
	n.mgr.connInfos[peerID] = &connInfo{
		peerID:     peerID,
		direction:  direction,
		createdAt:  now,
		lastActive: now,
	}
	connCount := len(n.mgr.connInfos)
	n.mgr.mu.Unlock()
	
	logger.Debug("连接管理器：连接建立", "peerID", truncateID(peerID, 8), "direction", direction, "total", connCount)
	
	// 标记节点为已连接（提高优先级）
	n.mgr.TagPeer(peerID, "connected", 10)
	
	// 彻底重构：为新连接创建速率追踪器
	if n.mgr.rateTrackers != nil {
		tracker := msgrate.NewTracker(msgrate.DefaultConfig(), nil, msgrate.DefaultConfig().RTTMaxEstimate)
		if err := n.mgr.rateTrackers.Track(peerID, tracker); err != nil {
			logger.Debug("添加速率追踪器失败", "peerID", truncateID(peerID, 8), "error", err)
		}
	}
	
	// 通知抖动容忍器节点重连成功
	if n.mgr.jitter != nil {
		n.mgr.jitter.NotifyReconnected(peerID)
	}
	
	// 检查是否需要裁剪
	if connCount >= n.mgr.cfg.HighWater {
		n.mgr.TriggerTrim()
	}
}

// Disconnected 当连接断开时调用
func (n *managerNotifee) Disconnected(conn pkgif.Connection) {
	if n.mgr == nil {
		return
	}
	
	peerID := string(conn.RemotePeer())
	
	// 移除连接信息
	n.mgr.mu.Lock()
	delete(n.mgr.connInfos, peerID)
	connCount := len(n.mgr.connInfos)
	n.mgr.mu.Unlock()
	
	logger.Debug("连接管理器：连接断开", "peerID", truncateID(peerID, 8), "total", connCount)
	
	// 彻底重构：移除速率追踪器
	if n.mgr.rateTrackers != nil {
		if err := n.mgr.rateTrackers.Untrack(peerID); err != nil {
			logger.Debug("移除速率追踪器失败", "peerID", truncateID(peerID, 8), "error", err)
		}
	}
	
	// 通知抖动容忍器节点断连
	// 如果返回 true，表示应该立即移除节点
	if n.mgr.jitter != nil && !n.mgr.jitter.NotifyDisconnected(peerID) {
		// 进入抖动容错，暂不移除连接标签
		logger.Debug("节点进入抖动容错", "peerID", peerID)
		return
	}
	
	// 移除连接标签
	n.mgr.UntagPeer(peerID, "connected")
}

// ============================================================================
//             彻底重构：msgrate 集成 - 消息速率追踪 API
// ============================================================================

// UpdatePeerRate 更新节点消息速率测量结果
//
// 参数:
//   - peerID: 节点 ID
//   - kind: 消息类型（可自定义，如 0=普通消息, 1=区块, 2=交易等）
//   - elapsed: 请求耗时
//   - items: 处理的消息数量
func (m *Manager) UpdatePeerRate(peerID string, kind uint64, elapsed time.Duration, items int) {
	if m.rateTrackers == nil {
		return
	}
	m.rateTrackers.Update(peerID, kind, elapsed, items)
}

// GetPeerCapacity 获取节点在目标 RTT 内可处理的消息数量
//
// 参数:
//   - peerID: 节点 ID
//   - kind: 消息类型
//   - targetRTT: 目标往返时间
//
// 返回值:
//   - 节点可处理的消息数量估计
func (m *Manager) GetPeerCapacity(peerID string, kind uint64, targetRTT time.Duration) int {
	if m.rateTrackers == nil {
		return 1
	}
	return m.rateTrackers.Capacity(peerID, kind, targetRTT)
}

// GetTargetRTT 获取当前目标 RTT
func (m *Manager) GetTargetRTT() time.Duration {
	if m.rateTrackers == nil {
		return time.Second
	}
	return m.rateTrackers.TargetRoundTrip()
}

// GetTargetTimeout 获取基于 RTT 的超时时间
func (m *Manager) GetTargetTimeout() time.Duration {
	if m.rateTrackers == nil {
		return 30 * time.Second
	}
	return m.rateTrackers.TargetTimeout()
}

// GetMedianRTT 获取所有追踪节点的中位数 RTT
func (m *Manager) GetMedianRTT() time.Duration {
	if m.rateTrackers == nil {
		return time.Second
	}
	return m.rateTrackers.MedianRoundTrip()
}

// GetMeanCapacities 获取所有消息类型的平均容量
func (m *Manager) GetMeanCapacities() map[uint64]float64 {
	if m.rateTrackers == nil {
		return make(map[uint64]float64)
	}
	return m.rateTrackers.MeanCapacities()
}

// RateTrackers 返回底层速率追踪器（高级用法）
func (m *Manager) RateTrackers() msgrate.Trackers {
	return m.rateTrackers
}

// Close 关闭连接管理器
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return ErrManagerClosed
	}

	logger.Info("正在关闭连接管理器")
	m.closed = true
	
	// 关闭抖动容忍器
	if m.jitter != nil {
		m.jitter.Stop()
	}
	
	m.tags.Clear()
	m.protects.Clear()
	m.connInfos = make(map[string]*connInfo)

	logger.Info("连接管理器已关闭")
	return nil
}

// SetHost 设置 Host（用于测试和集成）
func (m *Manager) SetHost(host Host) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.host = host
}

// calculateScore 计算节点优先级分数
//
// 评分规则：
//  1. 标签权重累加（基础分数）
//  2. 出站连接加分（主动拨号的节点更重要）
//  3. 活跃流加分（有数据传输的连接更重要）
func (m *Manager) calculateScore(peer string) int {
	score := 0

	// 1. 标签权重累加
	score += m.tags.Sum(peer)

	// 2. 方向加分（从 host 获取连接信息）
	if m.host != nil {
		// 检查是否有出站连接
		// 注意：需要 host 提供 Network() 接口访问连接详情
		// 当前简化：有连接标签就认为是重要节点
		if m.tags.Get(peer, "connected") > 0 {
			score += 10
		}
	}

	// 3. 流计数加分（从 peerstore 或 swarm 获取）
	// 注意：完整实现需要从 swarm.ConnsToPeer() 获取流统计
	// 当前简化：保护的节点额外加分
	if m.protects.HasAnyProtection(peer) {
		score += 20
	}

	return score
}

// ==================== 查询方法 ====================

// ConnCount 返回当前连接数
func (m *Manager) ConnCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connInfos)
}

// DialedConnCount 返回当前出站连接数
func (m *Manager) DialedConnCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, info := range m.connInfos {
		if info.direction == pkgif.DirOutbound {
			count++
		}
	}
	return count
}

// InboundConnCount 返回当前入站连接数
func (m *Manager) InboundConnCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, info := range m.connInfos {
		if info.direction == pkgif.DirInbound {
			count++
		}
	}
	return count
}

// ==================== 水位线方法 ====================

// SetLimits 设置水位线限制
func (m *Manager) SetLimits(low, high int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}

	m.cfg.LowWater = low
	m.cfg.HighWater = high
	// 自动设置EmergencyWater
	if m.cfg.EmergencyWater == 0 || m.cfg.EmergencyWater <= high {
		m.cfg.EmergencyWater = high * 3 / 2
	}

	logger.Info("水位线已更新", "low", low, "high", high, "emergency", m.cfg.EmergencyWater)
}

// GetLimits 获取水位线限制
func (m *Manager) GetLimits() (low, high int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.LowWater, m.cfg.HighWater
}

// ==================== 裁剪触发 ====================

// TriggerTrim 手动触发裁剪
func (m *Manager) TriggerTrim() {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return
	}
	m.mu.RUnlock()

	select {
	case m.trimCh <- struct{}{}:
		logger.Debug("裁剪已触发")
	default:
		// 已有裁剪任务在队列中
	}
}

// startTrimLoop 启动后台裁剪循环（由Fx生命周期调用）
func (m *Manager) startTrimLoop(ctx context.Context) {
	if m.cfg.TrimInterval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(m.cfg.TrimInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.checkAndTrim(ctx)
			case <-m.trimCh:
				m.checkAndTrim(ctx)
			}
		}
	}()
}

// checkAndTrim 检查并执行裁剪
func (m *Manager) checkAndTrim(ctx context.Context) {
	m.mu.RLock()
	connCount := len(m.connInfos)
	m.mu.RUnlock()

	// 只有超过高水位线才裁剪
	if connCount <= m.cfg.HighWater {
		return
	}

	// 计算需要裁剪的数量
	targetCount := m.cfg.LowWater
	trimCount := connCount - targetCount

	if trimCount <= 0 {
		return
	}

	logger.Info("开始裁剪", "current", connCount, "target", targetCount, "toTrim", trimCount)

	if m.host == nil {
		logger.Debug("没有 Host，无法执行实际裁剪")
		return
	}

	m.trimToTarget(ctx, targetCount)
}
