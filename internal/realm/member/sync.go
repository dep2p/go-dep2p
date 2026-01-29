package member

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              成员同步器
// ============================================================================

// Synchronizer 成员同步器
type Synchronizer struct {
	mu sync.RWMutex

	// 配置
	manager *Manager
	dht     pkgif.DHT

	// 同步状态
	version      atomic.Uint64
	lastSyncTime time.Time
	syncInterval time.Duration

	// 控制
	ctx        context.Context
	cancel     context.CancelFunc
	started    atomic.Bool
	syncTicker *time.Ticker
}

// NewSynchronizer 创建同步器
func NewSynchronizer(manager *Manager, dht pkgif.DHT) *Synchronizer {
	return &Synchronizer{
		manager:      manager,
		dht:          dht,
		syncInterval: 30 * time.Second,
	}
}

// ============================================================================
//                              全量同步
// ============================================================================

// SyncFull 全量同步
func (s *Synchronizer) SyncFull(ctx context.Context, members []*interfaces.MemberInfo) error {
	if s.manager == nil {
		return fmt.Errorf("manager is nil")
	}

	// 清除现有成员（如果需要）
	// s.manager.mu.Lock()
	// s.manager.members = make(map[string]*Member)
	// s.manager.mu.Unlock()

	// 添加所有成员
	for _, memberInfo := range members {
		if err := s.manager.Add(ctx, memberInfo); err != nil {
			return fmt.Errorf("failed to add member %s: %w", memberInfo.PeerID, err)
		}
	}

	// 更新版本号
	s.version.Add(1)
	s.mu.Lock()
	s.lastSyncTime = time.Now()
	s.mu.Unlock()

	return nil
}

// ============================================================================
//                              增量同步
// ============================================================================

// SyncDelta 增量同步
func (s *Synchronizer) SyncDelta(ctx context.Context, added, removed []*interfaces.MemberInfo) error {
	if s.manager == nil {
		return fmt.Errorf("manager is nil")
	}

	// 添加新成员
	for _, memberInfo := range added {
		if err := s.manager.Add(ctx, memberInfo); err != nil {
			return fmt.Errorf("failed to add member %s: %w", memberInfo.PeerID, err)
		}
	}

	// 移除成员
	for _, memberInfo := range removed {
		if err := s.manager.Remove(ctx, memberInfo.PeerID); err != nil && err != ErrMemberNotFound {
			return fmt.Errorf("failed to remove member %s: %w", memberInfo.PeerID, err)
		}
	}

	// 更新版本号
	s.version.Add(1)
	s.mu.Lock()
	s.lastSyncTime = time.Now()
	s.mu.Unlock()

	return nil
}

// ============================================================================
//                              自动同步
// ============================================================================

// Start 启动同步器
func (s *Synchronizer) Start(_ context.Context) error {
	if s.started.Load() {
		return ErrAlreadyStarted
	}

	// 使用 context.Background() 保证后台循环不受上层 ctx 取消的影响
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.started.Store(true)

	// 启动定期同步
	s.syncTicker = time.NewTicker(s.syncInterval)
	go s.syncLoop()

	return nil
}

// Stop 停止同步器
func (s *Synchronizer) Stop(_ context.Context) error {
	if !s.started.Load() {
		return ErrNotStarted
	}

	s.started.Store(false)

	if s.cancel != nil {
		s.cancel()
	}

	if s.syncTicker != nil {
		s.syncTicker.Stop()
	}

	return nil
}

// syncLoop 同步循环
func (s *Synchronizer) syncLoop() {
	for {
		select {
		case <-s.syncTicker.C:
			// 执行增量同步
			ctx := context.Background()
			s.performSync(ctx)

		case <-s.ctx.Done():
			return
		}
	}
}

// performSync 执行同步
//
// 同步流程：
//  1. 通过 DHT FindRealmMembers 发现 Realm 成员
//  2. 对比本地成员列表，执行增量同步
//  3. 标记失联成员为离线
//
// 使用 DHT.FindRealmMembers 替代 Discovery.FindPeers
// 原因：Discovery.FindPeers 使用 RealmProviderKey，而成员声明使用 RealmMembersKey
// 两个 Key 类型不一致导致成员发现永远返回空
func (s *Synchronizer) performSync(ctx context.Context) error {
	if s.dht == nil {
		return nil
	}

	// 1. 通过 DHT 查找 Realm 成员（使用正确的 RealmMembersKey）
	peerIDCh, err := s.dht.FindRealmMembers(ctx, types.RealmID(s.manager.realmID))
	if err != nil {
		return fmt.Errorf("find realm members failed: %w", err)
	}

	// 2. 收集发现的成员
	discoveredSet := make(map[string]bool)
	discovered := make([]*interfaces.MemberInfo, 0)

	for peerID := range peerIDCh {
		peerIDStr := string(peerID)
		discoveredSet[peerIDStr] = true

		// 通过 DHT 查询完整的 PeerInfo（包含地址）
		var addrs []string
		peerInfo, findErr := s.dht.FindPeer(ctx, peerIDStr)
		if findErr == nil {
			addrs = make([]string, 0, len(peerInfo.Addrs))
			for _, addr := range peerInfo.Addrs {
				if addr != nil {
					addrs = append(addrs, addr.String())
				}
			}
		}

		memberInfo := &interfaces.MemberInfo{
			PeerID:   peerIDStr,
			RealmID:  s.manager.realmID,
			Role:     interfaces.RoleMember,
			Online:   true,
			LastSeen: time.Now(),
			Addrs:    addrs,
		}
		discovered = append(discovered, memberInfo)
	}

	// 3. 增量同步：添加新成员
	for _, member := range discovered {
		if err := s.manager.Add(ctx, member); err != nil {
			// 忽略已存在的成员错误，只更新其状态
			s.manager.UpdateStatus(ctx, member.PeerID, true)
			s.manager.UpdateLastSeen(ctx, member.PeerID)
		}
	}

	// 4. 标记未发现的成员为离线（超时检测）
	existingMembers, err := s.manager.List(ctx, &interfaces.ListOptions{OnlineOnly: true})
	if err == nil {
		for _, existing := range existingMembers {
			if !discoveredSet[existing.PeerID] {
				// 检查是否超过离线阈值（3个同步周期）
				if time.Since(existing.LastSeen) > 3*s.syncInterval {
					s.manager.UpdateStatus(ctx, existing.PeerID, false)
				}
			}
		}
	}

	// 5. 更新同步状态
	s.version.Add(1)
	s.mu.Lock()
	s.lastSyncTime = time.Now()
	s.mu.Unlock()

	return nil
}

// GetVersion 获取当前版本号
func (s *Synchronizer) GetVersion() uint64 {
	return s.version.Load()
}

// GetLastSyncTime 获取最后同步时间
func (s *Synchronizer) GetLastSyncTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncTime
}

// 确保实现接口
var _ interfaces.MemberSynchronizer = (*Synchronizer)(nil)
