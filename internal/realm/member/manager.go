package member

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// member 模块 logger
var logger = log.Logger("realm/member")

// truncateID 安全截断 ID 用于日志显示
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ============================================================================
//                              成员管理器
// ============================================================================

// Manager 成员管理器
type Manager struct {
	mu sync.RWMutex

	// 配置
	realmID string
	config  *Config

	// 组件
	cache    interfaces.MemberCache
	store    interfaces.MemberStore
	eventBus pkgif.EventBus

	// 地址同步（"仅 ID 连接"支持）
	peerstore  pkgif.Peerstore
	addrSyncer *AddrSyncer

	// Host 引用（用于获取本地 PeerID）
	// 注：不再用于连接状态检查，仅用于识别自己
	host pkgif.Host

	// 最近断开的成员保护列表
	// 防止已断开的成员因 PubSub/DHT 消息竞态被重新添加
	recentlyDisconnected map[string]time.Time

	// ★ 主动离开的成员列表
	// 记录通过 MemberLeave 消息主动离开的节点
	// 这些节点只能通过【真正重新连接并认证成功】后才能重新加入
	// 成员同步消息无法覆盖此状态
	gracefullyLeft   map[string]time.Time
	gracefullyLeftMu sync.RWMutex

	// 防误判机制（快速断开检测）
	// 集成：重连宽限期、震荡检测、断开保护期
	antiFalsePositive *AntiFalsePositive

	// 成员数据
	members map[string]*Member

	// 统计
	onlineCount int32
	totalHits   int64
	totalMisses int64

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc

	// 状态
	started atomic.Bool
	closed  atomic.Bool
}

// NewManager 创建成员管理器
func NewManager(
	realmID string,
	cache interfaces.MemberCache,
	store interfaces.MemberStore,
	eventBus pkgif.EventBus,
) *Manager {
	config := DefaultConfig()
	return &Manager{
		realmID:              realmID,
		config:               config,
		cache:                cache,
		store:                store,
		eventBus:             eventBus,
		members:              make(map[string]*Member),
		recentlyDisconnected: make(map[string]time.Time),
		gracefullyLeft:       make(map[string]time.Time),
		antiFalsePositive:    NewAntiFalsePositive(newAntiFalsePositiveConfigFromManagerConfig(config)),
	}
}

// NewManagerWithConfig 创建带配置的管理器
func NewManagerWithConfig(
	realmID string,
	config *Config,
	cache interfaces.MemberCache,
	store interfaces.MemberStore,
	eventBus pkgif.EventBus,
) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	return &Manager{
		realmID:              realmID,
		config:               config,
		cache:                cache,
		store:                store,
		eventBus:             eventBus,
		members:              make(map[string]*Member),
		recentlyDisconnected: make(map[string]time.Time),
		gracefullyLeft:       make(map[string]time.Time),
		antiFalsePositive:    NewAntiFalsePositive(newAntiFalsePositiveConfigFromManagerConfig(config)),
	}
}

// newAntiFalsePositiveConfigFromManagerConfig 从 Manager 配置创建防误判配置
func newAntiFalsePositiveConfigFromManagerConfig(config *Config) *AntiFalsePositiveConfig {
	return &AntiFalsePositiveConfig{
		GracePeriod:        ReconnectGracePeriod,
		FlapWindow:         60 * time.Second,
		FlapThreshold:      3,
		ProtectionDuration: config.DisconnectProtection,
	}
}

// ============================================================================
//                              基础操作
// ============================================================================

// Add 添加成员
//
// 添加连接状态检查和断开保护期机制
// - 检查是否在断开保护期内（防止竞态重新添加）
// - 检查是否有活跃连接（自己除外）
func (m *Manager) Add(_ context.Context, memberInfo *interfaces.MemberInfo) error {
	if !m.started.Load() {
		logger.Debug("MemberManager 未启动，无法添加成员")
		return ErrNotStarted
	}

	if m.closed.Load() {
		logger.Debug("MemberManager 已关闭，无法添加成员")
		return ErrManagerClosed
	}

	if memberInfo == nil {
		logger.Warn("尝试添加 nil 成员")
		return ErrInvalidMember
	}

	if memberInfo.PeerID == "" {
		logger.Warn("尝试添加空 PeerID 成员")
		return ErrInvalidPeerID
	}

	// ★ 检查是否为主动离开的成员
	// 主动离开的成员只能通过重新连接并认证后才能加入
	// 成员同步消息无法覆盖此状态
	m.gracefullyLeftMu.RLock()
	if leftTime, ok := m.gracefullyLeft[memberInfo.PeerID]; ok {
		m.gracefullyLeftMu.RUnlock()
		logger.Debug("拒绝添加主动离开的成员",
			"peerID", truncateID(memberInfo.PeerID),
			"leftAt", leftTime,
			"reason", "成员已主动发送 MemberLeave，需重新连接认证")
		return nil // 静默忽略，不返回错误
	}
	m.gracefullyLeftMu.RUnlock()

	// ★ 防误判机制检查（快速断开检测）
	// 检查是否应该拒绝添加（断开保护期、震荡状态）
	if m.antiFalsePositive != nil {
		if reject, reason := m.antiFalsePositive.ShouldRejectAdd(memberInfo.PeerID); reject {
			logger.Debug("防误判机制: 拒绝添加成员",
				"peerID", truncateID(memberInfo.PeerID),
				"reason", reason)
			return nil // 静默忽略，不返回错误
		}
	}

	// ★ 检查是否在断开保护期内（兼容旧逻辑）
	// 防止已断开的成员因 PubSub/DHT 消息竞态被重新添加
	if m.config.DisconnectProtection > 0 {
		m.mu.RLock()
		if disconnectTime, ok := m.recentlyDisconnected[memberInfo.PeerID]; ok {
			if time.Since(disconnectTime) < m.config.DisconnectProtection {
				m.mu.RUnlock()
				logger.Debug("拒绝添加最近断开的成员（保护期内）",
					"peerID", truncateID(memberInfo.PeerID),
					"disconnectedAt", disconnectTime,
					"protection", m.config.DisconnectProtection)
				return nil // 静默忽略，不返回错误
			}
		}
		m.mu.RUnlock()
	}

	// ★ 移除"必须有活跃连接"检查
	// 原设计过于严格，阻止了正常的成员同步：
	//   - 成员同步通过 PubSub 广播传递（A 通过 B 收到 C 的信息）
	//   - A 与 C 可能没有直接连接，但 C 是合法成员
	//   - 断开保护期 + 防误判机制已足够防止竞态重新添加
	//
	// 成员添加现在依赖以下保护机制：
	//   1. 断开保护期（DisconnectProtection）- 已断开的成员在保护期内不会被重新添加
	//   2. 防误判机制（AntiFalsePositive）- 震荡检测 + 宽限期
	//   3. PubSub 消息验证 - 只接受来自已知成员的广播

	logger.Debug("添加成员", "peerID", truncateID(memberInfo.PeerID), "realmID", m.realmID)

	// 转换为内部 Member
	member := FromMemberInfo(memberInfo)

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	isNewMember := false
	if existing, ok := m.members[member.PeerID]; ok {
		// 更新现有成员
		logger.Debug("更新现有成员", "peerID", truncateID(member.PeerID))
		existing.Role = member.Role
		existing.Online = member.Online
		existing.LastSeen = member.LastSeen
		existing.Addrs = member.Addrs
		existing.Metadata = member.Metadata
		member = existing
	} else {
		// 添加新成员
		isNewMember = true
		logger.Info("添加新成员", "peerID", truncateID(member.PeerID), "realmID", m.realmID, "totalMembers", len(m.members)+1)
		m.members[member.PeerID] = member
	}

	// 更新在线计数（仅对新成员）
	// 修复：避免更新已存在成员时重复计数
	if isNewMember && member.Online {
		atomic.AddInt32(&m.onlineCount, 1)
	}

	// 更新缓存
	if m.cache != nil {
		m.cache.Set(member.ToMemberInfo())
	}

	// 保存到存储
	if m.store != nil {
		if err := m.store.Save(member.ToMemberInfo()); err != nil {
			logger.Warn("保存成员到存储失败", "peerID", truncateID(member.PeerID), "error", err)
		}
	}

	// 同步地址到 Peerstore（"仅 ID 连接"支持）
	if m.addrSyncer != nil {
		m.addrSyncer.OnMemberJoined(member)
	}

	// 仅对新成员发布事件（避免重复触发回调）
	if isNewMember {
		m.publishMemberJoined(member.PeerID)
	}

	return nil
}

// Remove 移除成员
func (m *Manager) Remove(_ context.Context, peerID string) error {
	if !m.started.Load() {
		return ErrNotStarted
	}

	if m.closed.Load() {
		return ErrManagerClosed
	}

	if peerID == "" {
		return ErrInvalidPeerID
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	member, ok := m.members[peerID]
	if !ok {
		return ErrMemberNotFound
	}

	// 删除成员
	delete(m.members, peerID)

	// 更新在线计数
	if member.Online {
		atomic.AddInt32(&m.onlineCount, -1)
	}

	// 从缓存删除
	if m.cache != nil {
		m.cache.Delete(peerID)
	}

	// 从存储删除
	if m.store != nil {
		if err := m.store.Delete(peerID); err != nil {
			// 记录错误但不返回
			_ = err
		}
	}

	// 通知地址同步器成员离开（清理 Peerstore 中的地址）
	if m.addrSyncer != nil {
		m.addrSyncer.OnMemberLeft(peerID)
	}

	// 发布事件
	m.publishMemberLeft(peerID)

	return nil
}

// Get 获取成员
func (m *Manager) Get(_ context.Context, peerID string) (*interfaces.MemberInfo, error) {
	if m.closed.Load() {
		return nil, ErrManagerClosed
	}

	// 1. 尝试从缓存获取
	if m.cache != nil {
		if member, ok := m.cache.Get(peerID); ok {
			atomic.AddInt64(&m.totalHits, 1)
			return member, nil
		}
		atomic.AddInt64(&m.totalMisses, 1)
	}

	// 2. 从内存获取
	m.mu.RLock()
	member, ok := m.members[peerID]
	m.mu.RUnlock()

	if ok {
		memberInfo := member.ToMemberInfo()

		// 更新缓存
		if m.cache != nil {
			m.cache.Set(memberInfo)
		}

		return memberInfo, nil
	}

	// 3. 从存储加载
	if m.store != nil {
		memberInfo, err := m.store.Load(peerID)
		if err == nil {
			// 更新到内存和缓存
			member := FromMemberInfo(memberInfo)
			m.mu.Lock()
			m.members[peerID] = member
			m.mu.Unlock()

			if m.cache != nil {
				m.cache.Set(memberInfo)
			}

			return memberInfo, nil
		}
	}

	return nil, ErrMemberNotFound
}

// List 列出成员
func (m *Manager) List(_ context.Context, opts *interfaces.ListOptions) ([]*interfaces.MemberInfo, error) {
	if m.closed.Load() {
		return nil, ErrManagerClosed
	}

	if opts == nil {
		opts = interfaces.DefaultListOptions()
	}

	// 修复 B27 数据竞争：持有读锁直到所有字段访问完成
	m.mu.RLock()
	defer m.mu.RUnlock()

	members := make([]*Member, 0, len(m.members))
	for _, member := range m.members {
		members = append(members, member)
	}

	// 过滤
	filtered := make([]*Member, 0, len(members))
	for _, member := range members {
		// 在线过滤
		if opts.OnlineOnly && !member.Online {
			continue
		}

		// 角色过滤
		if opts.Role != nil && member.Role != int(*opts.Role) {
			continue
		}

		filtered = append(filtered, member)
	}

	// 排序
	switch opts.SortBy {
	case "joined_at":
		sort.Slice(filtered, func(i, j int) bool {
			if opts.Descending {
				return filtered[i].JoinedAt.After(filtered[j].JoinedAt)
			}
			return filtered[i].JoinedAt.Before(filtered[j].JoinedAt)
		})
	case "last_seen":
		sort.Slice(filtered, func(i, j int) bool {
			if opts.Descending {
				return filtered[i].LastSeen.After(filtered[j].LastSeen)
			}
			return filtered[i].LastSeen.Before(filtered[j].LastSeen)
		})
	}

	// 限制数量
	if opts.Limit > 0 && len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}

	// 转换为 MemberInfo
	result := make([]*interfaces.MemberInfo, len(filtered))
	for i, member := range filtered {
		result[i] = member.ToMemberInfo()
	}

	return result, nil
}

// ============================================================================
//                              状态管理
// ============================================================================

// UpdateStatus 更新成员状态
func (m *Manager) UpdateStatus(_ context.Context, peerID string, online bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	member, ok := m.members[peerID]
	if !ok {
		return ErrMemberNotFound
	}

	oldOnline := member.Online
	member.SetOnline(online)

	// 更新在线计数
	if online && !oldOnline {
		atomic.AddInt32(&m.onlineCount, 1)
	} else if !online && oldOnline {
		atomic.AddInt32(&m.onlineCount, -1)
	}

	// 更新缓存
	if m.cache != nil {
		m.cache.Set(member.ToMemberInfo())
	}

	return nil
}

// UpdateLastSeen 更新最后活跃时间
func (m *Manager) UpdateLastSeen(_ context.Context, peerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	member, ok := m.members[peerID]
	if !ok {
		return ErrMemberNotFound
	}

	member.UpdateLastSeen()

	// 更新缓存
	if m.cache != nil {
		m.cache.Set(member.ToMemberInfo())
	}

	return nil
}

// ============================================================================
//                              批量操作
// ============================================================================

// BatchAdd 批量添加成员
func (m *Manager) BatchAdd(ctx context.Context, members []*interfaces.MemberInfo) error {
	for _, member := range members {
		if err := m.Add(ctx, member); err != nil {
			return err
		}
	}
	return nil
}

// BatchRemove 批量删除成员
func (m *Manager) BatchRemove(ctx context.Context, peerIDs []string) error {
	for _, peerID := range peerIDs {
		if err := m.Remove(ctx, peerID); err != nil && err != ErrMemberNotFound {
			return err
		}
	}
	return nil
}

// ============================================================================
//                              查询操作
// ============================================================================

// IsMember 检查是否为成员
func (m *Manager) IsMember(_ context.Context, peerID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.members[peerID]
	// 移除 DEBUG 日志：IsMember 被频繁调用（如 PubSub 验证器），
	// 每次检查非成员都记录日志会导致日志文件过大
	// 如果需要调试，可以在调用方记录日志
	return ok
}

// GetOnlineCount 获取在线成员数
func (m *Manager) GetOnlineCount() int {
	return int(atomic.LoadInt32(&m.onlineCount))
}

// GetTotalCount 获取总成员数
func (m *Manager) GetTotalCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.members)
}

// GetStats 获取统计信息
func (m *Manager) GetStats() *interfaces.Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &interfaces.Stats{
		TotalCount:  len(m.members),
		OnlineCount: int(atomic.LoadInt32(&m.onlineCount)),
	}

	// 统计不同角色的成员
	for _, member := range m.members {
		if member.Role == int(interfaces.RoleAdmin) {
			stats.AdminCount++
		} else if member.Role == int(interfaces.RoleRelay) {
			stats.RelayCount++
		}
	}

	// 计算缓存命中率
	totalAccess := atomic.LoadInt64(&m.totalHits) + atomic.LoadInt64(&m.totalMisses)
	if totalAccess > 0 {
		stats.CacheHitRate = float64(atomic.LoadInt64(&m.totalHits)) / float64(totalAccess)
	}

	return stats
}

// ============================================================================
//                              同步操作
// ============================================================================

// SyncMembers 同步成员
func (m *Manager) SyncMembers(_ context.Context) error {
	if !m.started.Load() {
		return ErrNotStarted
	}

	// 从存储加载所有成员
	if m.store != nil {
		members, err := m.store.LoadAll()
		if err != nil {
			return fmt.Errorf("failed to load members: %w", err)
		}

		// 更新到内存
		m.mu.Lock()
		for _, memberInfo := range members {
			member := FromMemberInfo(memberInfo)
			m.members[member.PeerID] = member

			// 更新在线计数
			if member.Online {
				atomic.AddInt32(&m.onlineCount, 1)
			}
		}
		m.mu.Unlock()
	}

	return nil
}

// ============================================================================
//                              生命周期管理
// ============================================================================

// Start 启动管理器
func (m *Manager) Start(ctx context.Context) error {
	if m.started.Load() {
		return ErrAlreadyStarted
	}

	if m.closed.Load() {
		return ErrManagerClosed
	}

	// 创建内部 context
	m.ctx, m.cancel = context.WithCancel(context.Background())

	m.started.Store(true)

	// 配置防误判机制的回调
	if m.antiFalsePositive != nil {
		m.antiFalsePositive.SetOnMemberRemove(func(peerID string) {
			// 宽限期超时后移除成员
			logger.Info("宽限期超时，移除成员",
				"peerID", truncateID(peerID),
				"realmID", truncateID(m.realmID))

			// 记录到 34 保护列表
			if m.config.DisconnectProtection > 0 {
				m.mu.Lock()
				m.recentlyDisconnected[peerID] = time.Now()
				m.mu.Unlock()
			}

			m.Remove(m.ctx, peerID)
		})
	}

	// 从存储加载成员
	if err := m.SyncMembers(ctx); err != nil {
		return err
	}

	// 订阅连接断开事件，自动移除离线成员
	go m.watchDisconnections()

	// 快速断开检测：监听重连事件（宽限期恢复）
	go m.watchReconnections()

	// 启动定期清理过期成员的循环
	go m.cleanupLoop()

	// 启动断开保护记录清理循环
	go m.cleanupDisconnectProtectionLoop()

	// 启动防误判机制清理循环
	go m.cleanupAntiFalsePositiveLoop()

	return nil
}

// watchDisconnections 监听连接断开事件
//
// 快速断开检测：使用防误判机制处理断开事件
//   - 进入宽限期而非立即移除
//   - 震荡检测抑制频繁断开
//   - 断开保护期防止竞态重新添加
func (m *Manager) watchDisconnections() {
	if m.eventBus == nil {
		return
	}

	// 订阅断开事件（需要传入指针类型）
	sub, err := m.eventBus.Subscribe(new(types.EvtPeerDisconnected))
	if err != nil {
		logger.Warn("订阅断开事件失败", "error", err)
		return
	}
	defer sub.Close()

	ch := sub.Out()
	for {
		select {
		case <-m.ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			// 处理断开事件（事件以指针形式传递）
			disconnected, ok := evt.(*types.EvtPeerDisconnected)
			if !ok {
				continue
			}
			peerID := string(disconnected.PeerID)

			// 检查是否是 Realm 成员
			if !m.IsMember(m.ctx, peerID) {
				continue
			}

			logger.Debug("检测到成员断开连接",
				"peerID", truncateID(peerID),
				"realmID", truncateID(m.realmID))

			// ★ 使用防误判机制处理断开
			if m.antiFalsePositive != nil {
				shouldRemove, inGracePeriod := m.antiFalsePositive.OnPeerDisconnected(peerID, m.realmID)

				if inGracePeriod {
					logger.Debug("成员进入宽限期",
						"peerID", truncateID(peerID),
						"gracePeriod", ReconnectGracePeriod)
					// 宽限期内不移除，等待超时或重连
					continue
				}

				if !shouldRemove {
					// 被震荡检测抑制
					logger.Debug("断开被防误判机制抑制",
						"peerID", truncateID(peerID))
					continue
				}
			}

			// 如果没有防误判机制或需要立即移除
			logger.Info("移除断开的成员",
				"peerID", truncateID(peerID),
				"realmID", truncateID(m.realmID))

			// ★ 记录断开时间（用于保护期）
			if m.config.DisconnectProtection > 0 {
				m.mu.Lock()
				m.recentlyDisconnected[peerID] = time.Now()
				m.mu.Unlock()
			}

			m.Remove(m.ctx, peerID)
		}
	}
}

// watchReconnections 监听连接重连事件
//
// 快速断开检测：在宽限期内检测到重连时恢复成员状态
func (m *Manager) watchReconnections() {
	if m.eventBus == nil {
		return
	}

	// 订阅连接事件
	sub, err := m.eventBus.Subscribe(new(types.EvtPeerConnected))
	if err != nil {
		logger.Warn("订阅连接事件失败", "error", err)
		return
	}
	defer sub.Close()

	ch := sub.Out()
	for {
		select {
		case <-m.ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			connected, ok := evt.(*types.EvtPeerConnected)
			if !ok {
				continue
			}
			peerID := string(connected.PeerID)

			// 检查防误判机制
			if m.antiFalsePositive == nil {
				continue
			}

			// 检查是否在宽限期内
			if !m.antiFalsePositive.IsInGracePeriod(peerID) {
				continue
			}

			// 尝试在宽限期内恢复
			recovered, suppressed := m.antiFalsePositive.OnPeerReconnected(peerID)
			if recovered {
				logger.Info("成员宽限期内重连成功",
					"peerID", truncateID(peerID),
					"realmID", truncateID(m.realmID))
			} else if suppressed {
				logger.Debug("重连被震荡检测抑制",
					"peerID", truncateID(peerID))
			}
		}
	}
}

// cleanupLoop 定期清理过期成员
//
// 实现基于 TTL 的成员清理
// 解决成员列表不一致问题 - 当连接断开事件未能正确触发时，
// 通过定期检查 LastSeen 来清理过期成员
func (m *Manager) cleanupLoop() {
	if m.config.CleanupInterval <= 0 {
		logger.Debug("成员清理已禁用", "realmID", truncateID(m.realmID))
		return
	}

	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	logger.Info("成员清理循环已启动",
		"realmID", truncateID(m.realmID),
		"interval", m.config.CleanupInterval,
		"maxOffline", m.config.MaxOfflineDuration)

	for {
		select {
		case <-m.ctx.Done():
			logger.Debug("成员清理循环已停止", "realmID", truncateID(m.realmID))
			return
		case <-ticker.C:
			cleaned := m.cleanupStaleMembers()
			if cleaned > 0 {
				logger.Info("清理过期成员完成",
					"realmID", truncateID(m.realmID),
					"cleanedCount", cleaned)
			}
		}
	}
}

// cleanupStaleMembers 清理过期成员
//
// 返回清理的成员数量
func (m *Manager) cleanupStaleMembers() int {
	if m.config.MaxOfflineDuration <= 0 {
		return 0
	}

	now := time.Now()
	threshold := now.Add(-m.config.MaxOfflineDuration)

	m.mu.RLock()
	// 收集需要清理的成员
	var toRemove []string
	for peerID, member := range m.members {
		// 跳过在线成员
		if member.Online {
			continue
		}
		// 检查 LastSeen 是否超过阈值
		if member.LastSeen.Before(threshold) {
			toRemove = append(toRemove, peerID)
			logger.Debug("检测到过期成员",
				"peerID", truncateID(peerID),
				"lastSeen", member.LastSeen,
				"threshold", threshold)
		}
	}
	m.mu.RUnlock()

	// 移除过期成员
	for _, peerID := range toRemove {
		logger.Info("移除过期成员",
			"peerID", truncateID(peerID),
			"realmID", truncateID(m.realmID))
		m.Remove(m.ctx, peerID)
	}

	return len(toRemove)
}

// cleanupDisconnectProtectionLoop 清理过期的断开保护记录
//
// 定期清理 recentlyDisconnected 中过期的记录
// 避免内存泄漏
func (m *Manager) cleanupDisconnectProtectionLoop() {
	if m.config.DisconnectProtection <= 0 {
		logger.Debug("34: 断开保护已禁用", "realmID", truncateID(m.realmID))
		return
	}

	// 清理间隔 = 保护期的 2 倍
	cleanupInterval := m.config.DisconnectProtection * 2
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	logger.Debug("34: 断开保护清理循环已启动",
		"realmID", truncateID(m.realmID),
		"cleanupInterval", cleanupInterval,
		"protection", m.config.DisconnectProtection)

	for {
		select {
		case <-m.ctx.Done():
			logger.Debug("34: 断开保护清理循环已停止", "realmID", truncateID(m.realmID))
			return
		case <-ticker.C:
			m.cleanupDisconnectProtection()
		}
	}
}

// cleanupDisconnectProtection 清理过期的断开保护记录
func (m *Manager) cleanupDisconnectProtection() {
	now := time.Now()
	// 保护期过后的记录可以清理（额外保留 1 倍保护期作为缓冲）
	threshold := m.config.DisconnectProtection * 2

	m.mu.Lock()
	defer m.mu.Unlock()

	var cleaned int
	for peerID, disconnectTime := range m.recentlyDisconnected {
		if now.Sub(disconnectTime) > threshold {
			delete(m.recentlyDisconnected, peerID)
			cleaned++
		}
	}

	if cleaned > 0 {
		logger.Debug("34: 已清理过期的断开保护记录",
			"realmID", truncateID(m.realmID),
			"cleanedCount", cleaned,
			"remainingCount", len(m.recentlyDisconnected))
	}
}

// cleanupAntiFalsePositiveLoop 清理防误判机制的过期数据
func (m *Manager) cleanupAntiFalsePositiveLoop() {
	if m.antiFalsePositive == nil {
		return
	}

	// 每分钟清理一次
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.antiFalsePositive.Cleanup()
		}
	}
}

// Stop 停止管理器
func (m *Manager) Stop(_ context.Context) error {
	if !m.started.Load() {
		return ErrNotStarted
	}

	m.started.Store(false)

	// 取消内部 context（停止事件监听）
	if m.cancel != nil {
		m.cancel()
	}

	// 保存所有成员到存储
	if m.store != nil {
		m.mu.RLock()
		for _, member := range m.members {
			m.store.Save(member.ToMemberInfo())
		}
		m.mu.RUnlock()

		// 压缩存储
		if err := m.store.Compact(); err != nil {
			return err
		}
	}

	return nil
}

// Close 关闭管理器
func (m *Manager) Close() error {
	if m.closed.Load() {
		return nil
	}

	m.closed.Store(true)

	// 停止（如果还在运行）
	if m.started.Load() {
		ctx := context.Background()
		m.Stop(ctx)
	}

	// 关闭防误判机制
	if m.antiFalsePositive != nil {
		m.antiFalsePositive.Close()
	}

	// 关闭缓存
	if m.cache != nil {
		m.cache.Clear()
	}

	// 关闭存储
	if m.store != nil {
		m.store.Close()
	}

	m.mu.Lock()
	m.members = nil
	m.mu.Unlock()

	return nil
}

// ============================================================================
//                              依赖注入
// ============================================================================

// SetHost 设置 Host 引用
//
// 用于获取本地 PeerID，识别"添加自己"的情况。
// 注：不再用于连接状态检查。
func (m *Manager) SetHost(host pkgif.Host) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.host = host
	if host != nil {
		logger.Debug("Host 已注入到 MemberManager", "realmID", truncateID(m.realmID))
	}
}

// ============================================================================
//                              地址同步（"仅 ID 连接"支持）
// ============================================================================

// SetPeerstore 设置 Peerstore 并创建地址同步器
//
// 用于将 MemberList 中的成员地址同步到 Peerstore，
// 支持"仅 ID 连接"的地址发现。
func (m *Manager) SetPeerstore(peerstore pkgif.Peerstore) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.peerstore = peerstore
	if peerstore != nil {
		m.addrSyncer = NewAddrSyncer(AddrSyncerConfig{
			RealmID:   m.realmID,
			Peerstore: peerstore,
		})
	} else {
		m.addrSyncer = nil
	}
}

// AddrSyncer 返回地址同步器
func (m *Manager) AddrSyncer() *AddrSyncer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.addrSyncer
}

// ============================================================================
//                              事件发布
// ============================================================================

// publishMemberJoined 发布成员加入事件
func (m *Manager) publishMemberJoined(peerID string) {
	if m.eventBus == nil {
		return
	}

	// 通过 Emitter 模式发布事件
	emitter, err := m.eventBus.Emitter(&types.EvtRealmMemberJoined{})
	if err != nil {
		return
	}
	defer emitter.Close()

	evt := &types.EvtRealmMemberJoined{
		BaseEvent: types.NewBaseEvent(types.EventTypeRealmMemberJoined),
		RealmID:   types.RealmID(m.realmID),
		MemberID:  types.PeerID(peerID),
	}
	emitter.Emit(evt)
}

// publishMemberLeft 发布成员离开事件
func (m *Manager) publishMemberLeft(peerID string) {
	if m.eventBus == nil {
		return
	}

	// 通过 Emitter 模式发布事件
	emitter, err := m.eventBus.Emitter(&types.EvtRealmMemberLeft{})
	if err != nil {
		return
	}
	defer emitter.Close()

	evt := &types.EvtRealmMemberLeft{
		BaseEvent: types.NewBaseEvent(types.EventTypeRealmMemberLeft),
		RealmID:   types.RealmID(m.realmID),
		MemberID:  types.PeerID(peerID),
	}
	emitter.Emit(evt)
}

// ============================================================================
//                              防误判机制 API
// ============================================================================

// GetAntiFalsePositiveStats 获取防误判机制统计信息
//
// 返回当前处于宽限期、震荡状态、保护期的成员信息。
func (m *Manager) GetAntiFalsePositiveStats() *AntiFalsePositiveStats {
	if m.antiFalsePositive == nil {
		return &AntiFalsePositiveStats{}
	}
	return m.antiFalsePositive.GetStats()
}

// IsInGracePeriod 检查成员是否在宽限期内
func (m *Manager) IsInGracePeriod(peerID string) bool {
	if m.antiFalsePositive == nil {
		return false
	}
	return m.antiFalsePositive.IsInGracePeriod(peerID)
}

// IsFlapping 检查成员是否处于震荡状态
func (m *Manager) IsFlapping(peerID string) bool {
	if m.antiFalsePositive == nil {
		return false
	}
	return m.antiFalsePositive.IsFlapping(peerID)
}

// ClearProtection 清除成员的保护状态
//
// 用于管理员强制允许重新添加被保护的成员。
func (m *Manager) ClearProtection(peerID string) {
	if m.antiFalsePositive != nil {
		m.antiFalsePositive.ClearProtection(peerID)
	}

	// 同时清除 34 保护列表
	m.mu.Lock()
	delete(m.recentlyDisconnected, peerID)
	m.mu.Unlock()
}

// OnMemberCommunication 通知收到成员通信
//
// 用于在宽限期内延长超时时间。
// 应在收到成员消息时调用（如 PubSub 消息、Ping 等）。
func (m *Manager) OnMemberCommunication(peerID string) {
	if m.antiFalsePositive != nil {
		m.antiFalsePositive.OnCommunication(peerID)
	}
}

// ============================================================================
//                              主动离开成员管理
// ============================================================================

// MarkGracefullyLeft 标记成员为主动离开
//
// 当收到成员的 MemberLeave 消息（GRACEFUL/KICKED）时调用。
// 标记后的成员只能通过【真正重新连接并认证成功】后才能重新加入。
// 成员同步消息无法覆盖此状态。
func (m *Manager) MarkGracefullyLeft(peerID string) {
	m.gracefullyLeftMu.Lock()
	defer m.gracefullyLeftMu.Unlock()

	m.gracefullyLeft[peerID] = time.Now()
	logger.Info("标记成员为主动离开",
		"peerID", truncateID(peerID),
		"realmID", truncateID(m.realmID))
}

// ClearGracefullyLeft 清除成员的主动离开标记
//
// 当成员【真正重新连接并认证成功】后调用。
// 清除后成员可以正常加入。
func (m *Manager) ClearGracefullyLeft(peerID string) {
	m.gracefullyLeftMu.Lock()
	defer m.gracefullyLeftMu.Unlock()

	if _, ok := m.gracefullyLeft[peerID]; ok {
		delete(m.gracefullyLeft, peerID)
		logger.Info("清除成员主动离开标记（已重新连接认证）",
			"peerID", truncateID(peerID),
			"realmID", truncateID(m.realmID))
	}
}

// IsGracefullyLeft 检查成员是否已主动离开
func (m *Manager) IsGracefullyLeft(peerID string) bool {
	m.gracefullyLeftMu.RLock()
	defer m.gracefullyLeftMu.RUnlock()
	_, ok := m.gracefullyLeft[peerID]
	return ok
}

// 确保实现接口
var _ interfaces.MemberManager = (*Manager)(nil)
