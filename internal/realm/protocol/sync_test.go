package protocol

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              Mock Synchronizer
// ============================================================================

// mockSynchronizer 模拟同步器
type mockSynchronizer struct {
	mu            sync.RWMutex
	fullSyncCalls int
	deltaSyncCalls int
	addedMembers  []*interfaces.MemberInfo
	removedMembers []*interfaces.MemberInfo
}

func newMockSynchronizer() *mockSynchronizer {
	return &mockSynchronizer{
		addedMembers:   make([]*interfaces.MemberInfo, 0),
		removedMembers: make([]*interfaces.MemberInfo, 0),
	}
}

func (m *mockSynchronizer) SyncFull(ctx context.Context, members []*interfaces.MemberInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fullSyncCalls++
	m.addedMembers = append(m.addedMembers, members...)
	return nil
}

func (m *mockSynchronizer) SyncDelta(ctx context.Context, added, removed []*interfaces.MemberInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deltaSyncCalls++
	m.addedMembers = append(m.addedMembers, added...)
	m.removedMembers = append(m.removedMembers, removed...)
	return nil
}

func (m *mockSynchronizer) Start(ctx context.Context) error {
	return nil
}

func (m *mockSynchronizer) Stop(ctx context.Context) error {
	return nil
}

func (m *mockSynchronizer) GetDeltaSyncCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.deltaSyncCalls
}

func (m *mockSynchronizer) GetAddedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.addedMembers)
}

// ============================================================================
//                              Mock MemberManager
// ============================================================================

// mockMemberManager 模拟成员管理器
type mockMemberManager struct {
	mu         sync.RWMutex
	members    map[string]*interfaces.MemberInfo
	onlineCount int
}

func newMockMemberManager() *mockMemberManager {
	return &mockMemberManager{
		members: make(map[string]*interfaces.MemberInfo),
	}
}

func (m *mockMemberManager) Add(ctx context.Context, member *interfaces.MemberInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.members[member.PeerID] = member
	if member.Online {
		m.onlineCount++
	}
	return nil
}

func (m *mockMemberManager) Remove(ctx context.Context, peerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if member, ok := m.members[peerID]; ok {
		if member.Online {
			m.onlineCount--
		}
		delete(m.members, peerID)
	}
	return nil
}

func (m *mockMemberManager) Get(ctx context.Context, peerID string) (*interfaces.MemberInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if member, ok := m.members[peerID]; ok {
		return member, nil
	}
	return nil, fmt.Errorf("member not found: %s", peerID)
}

func (m *mockMemberManager) List(ctx context.Context, opts *interfaces.ListOptions) ([]*interfaces.MemberInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*interfaces.MemberInfo, 0, len(m.members))
	for _, member := range m.members {
		result = append(result, member)
	}
	return result, nil
}

func (m *mockMemberManager) UpdateStatus(ctx context.Context, peerID string, online bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if member, ok := m.members[peerID]; ok {
		if member.Online != online {
			if online {
				m.onlineCount++
			} else {
				m.onlineCount--
			}
		}
		member.Online = online
	}
	return nil
}

func (m *mockMemberManager) UpdateLastSeen(ctx context.Context, peerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if member, ok := m.members[peerID]; ok {
		member.LastSeen = time.Now()
	}
	return nil
}

func (m *mockMemberManager) BatchAdd(ctx context.Context, members []*interfaces.MemberInfo) error {
	for _, member := range members {
		if err := m.Add(ctx, member); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockMemberManager) BatchRemove(ctx context.Context, peerIDs []string) error {
	for _, peerID := range peerIDs {
		if err := m.Remove(ctx, peerID); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockMemberManager) IsMember(ctx context.Context, peerID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.members[peerID]
	return ok
}

func (m *mockMemberManager) GetOnlineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.onlineCount
}

func (m *mockMemberManager) GetTotalCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.members)
}

func (m *mockMemberManager) GetStats() *interfaces.Stats {
	return &interfaces.Stats{
		TotalCount:  m.GetTotalCount(),
		OnlineCount: m.GetOnlineCount(),
	}
}

func (m *mockMemberManager) SyncMembers(ctx context.Context) error {
	return nil
}

func (m *mockMemberManager) Start(ctx context.Context) error {
	return nil
}

func (m *mockMemberManager) Stop(ctx context.Context) error {
	return nil
}

func (m *mockMemberManager) Close() error {
	return nil
}

// ============================================================================
//                              单元测试
// ============================================================================

func TestSyncHandler_Creation(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	synchronizer := newMockSynchronizer()
	memberManager := newMockMemberManager()

	handler := NewSyncHandler(host, realmID, synchronizer, memberManager)

	require.NotNil(t, handler)
	require.Equal(t, realmID, handler.realmID)
	require.Equal(t, uint64(1), handler.GetVersion())
	require.Equal(t, DefaultSyncInterval, handler.syncInterval)
	require.Equal(t, DefaultSyncPeerCount, handler.syncPeerCount)
}

func TestSyncHandler_StartStop(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	synchronizer := newMockSynchronizer()
	memberManager := newMockMemberManager()

	handler := NewSyncHandler(host, realmID, synchronizer, memberManager)

	ctx := context.Background()

	// 测试启动
	err := handler.Start(ctx)
	require.NoError(t, err)
	require.True(t, handler.started)
	require.NotNil(t, handler.syncTicker)

	// 验证协议已注册
	protocolID := "/dep2p/realm/test-realm/sync/1.0.0"
	require.NotNil(t, host.getHandler(protocolID))

	// 测试重复启动
	err = handler.Start(ctx)
	require.Error(t, err)

	// 测试停止
	err = handler.Stop(ctx)
	require.NoError(t, err)
	require.False(t, handler.started)

	// 验证协议已注销
	require.Nil(t, host.getHandler(protocolID))
}

func TestSyncHandler_MessageEncoding(t *testing.T) {
	// 测试 SyncRequest 编解码
	req := &SyncRequest{
		RealmID:      "test-realm-id",
		LocalVersion: 123,
	}

	data := encodeSyncRequest(req)
	decoded, err := decodeSyncRequest(data)
	require.NoError(t, err)
	require.Equal(t, req.RealmID, decoded.RealmID)
	require.Equal(t, req.LocalVersion, decoded.LocalVersion)
}

func TestSyncHandler_SyncResponseEncoding(t *testing.T) {
	// 测试 SyncResponse 编解码
	resp := &SyncResponse{
		Version: 456,
		Added: []*interfaces.MemberInfo{
			{
				PeerID:  "peer1",
				RealmID: "realm1",
				Role:    interfaces.RoleMember,
				Online:  true,
			},
			{
				PeerID:  "peer2",
				RealmID: "realm1",
				Role:    interfaces.RoleAdmin,
				Online:  false,
			},
		},
		Removed: []string{"peer3", "peer4"},
	}

	data := encodeSyncResponse(resp)
	decoded, err := decodeSyncResponse(data)
	require.NoError(t, err)
	require.Equal(t, resp.Version, decoded.Version)
	require.Equal(t, len(resp.Added), len(decoded.Added))
	require.Equal(t, resp.Added[0].PeerID, decoded.Added[0].PeerID)
	require.Equal(t, resp.Added[1].Role, decoded.Added[1].Role)
	require.Equal(t, resp.Removed, decoded.Removed)
}

func TestSyncHandler_EmptySyncResponse(t *testing.T) {
	// 测试空响应编解码
	resp := &SyncResponse{
		Version: 100,
		Added:   []*interfaces.MemberInfo{},
		Removed: []string{},
	}

	data := encodeSyncResponse(resp)
	decoded, err := decodeSyncResponse(data)
	require.NoError(t, err)
	require.Equal(t, resp.Version, decoded.Version)
	require.Equal(t, 0, len(decoded.Added))
	require.Equal(t, 0, len(decoded.Removed))
}

func TestSyncHandler_VersionControl(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	synchronizer := newMockSynchronizer()
	memberManager := newMockMemberManager()

	handler := NewSyncHandler(host, realmID, synchronizer, memberManager)

	// 初始版本为 1
	require.Equal(t, uint64(1), handler.GetVersion())

	// 递增版本
	newVersion := handler.IncrementVersion()
	require.Equal(t, uint64(2), newVersion)
	require.Equal(t, uint64(2), handler.GetVersion())

	// 设置版本
	handler.SetVersion(100)
	require.Equal(t, uint64(100), handler.GetVersion())
}

func TestSyncHandler_Configuration(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	synchronizer := newMockSynchronizer()
	memberManager := newMockMemberManager()

	handler := NewSyncHandler(host, realmID, synchronizer, memberManager)

	// 测试设置同步间隔
	handler.SetSyncInterval(60 * time.Second)
	require.Equal(t, 60*time.Second, handler.syncInterval)

	// 测试设置同步节点数
	handler.SetSyncPeerCount(5)
	require.Equal(t, 5, handler.syncPeerCount)
}

func TestSyncHandler_Callbacks(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	synchronizer := newMockSynchronizer()
	memberManager := newMockMemberManager()

	handler := NewSyncHandler(host, realmID, synchronizer, memberManager)

	// 设置成功回调
	handler.SetOnSyncSuccess(func(peerID string, added, removed int) {
		// Callback set
	})

	// 设置失败回调
	handler.SetOnSyncFailed(func(peerID string, err error) {
		// Callback set
	})

	require.NotNil(t, handler.onSyncSuccess)
	require.NotNil(t, handler.onSyncFailed)
}

func TestSyncHandler_Close(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	synchronizer := newMockSynchronizer()
	memberManager := newMockMemberManager()

	handler := NewSyncHandler(host, realmID, synchronizer, memberManager)

	ctx := context.Background()

	// 启动
	err := handler.Start(ctx)
	require.NoError(t, err)

	// 关闭
	err = handler.Close()
	require.NoError(t, err)
	require.True(t, handler.closed)
	require.False(t, handler.started)

	// 重复关闭应该无错误
	err = handler.Close()
	require.NoError(t, err)
}

func TestSyncHandler_SelectRandomPeers(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	synchronizer := newMockSynchronizer()
	memberManager := newMockMemberManager()

	handler := NewSyncHandler(host, realmID, synchronizer, memberManager)

	// 创建测试成员
	members := []*interfaces.MemberInfo{
		{PeerID: "peer1"},
		{PeerID: "peer2"},
		{PeerID: "peer3"},
		{PeerID: "peer4"},
		{PeerID: "peer5"},
	}

	// 测试选择 3 个
	selected := handler.selectRandomPeers(members, 3)
	require.Equal(t, 3, len(selected))

	// 测试选择超过总数
	selected = handler.selectRandomPeers(members, 10)
	require.Equal(t, 5, len(selected))

	// 测试空列表
	selected = handler.selectRandomPeers([]*interfaces.MemberInfo{}, 3)
	require.Equal(t, 0, len(selected))
}

// ============================================================================
//                              集成测试
// ============================================================================

func TestSyncHandler_Integration_VersionHigher(t *testing.T) {
	// 创建两个节点
	host1 := newMockHost("peer1")
	host2 := newMockHost("peer2")

	realmID := "test-realm"

	// 创建同步器和成员管理器
	synchronizer1 := newMockSynchronizer()
	synchronizer2 := newMockSynchronizer()
	memberManager1 := newMockMemberManager()
	memberManager2 := newMockMemberManager()

	ctx := context.Background()
	memberManager1.Start(ctx)
	memberManager2.Start(ctx)

	// 创建处理器
	handler1 := NewSyncHandler(host1, realmID, synchronizer1, memberManager1)
	handler2 := NewSyncHandler(host2, realmID, synchronizer2, memberManager2)

	// 设置 peer2 的版本更高
	handler2.SetVersion(10)

	// 在 peer2 添加一些成员
	for i := 1; i <= 3; i++ {
		member := &interfaces.MemberInfo{
			PeerID:  fmt.Sprintf("peer%d", i),
			RealmID: realmID,
			Role:    interfaces.RoleMember,
			Online:  true,
		}
		memberManager2.Add(ctx, member)
	}

	// 启动处理器
	err := handler1.Start(ctx)
	require.NoError(t, err)
	defer handler1.Stop(ctx)

	err = handler2.Start(ctx)
	require.NoError(t, err)
	defer handler2.Stop(ctx)

	// 设置成功回调
	var syncSuccess bool
	var addedCount, removedCount int
	handler1.SetOnSyncSuccess(func(peerID string, added, removed int) {
		syncSuccess = true
		addedCount = added
		removedCount = removed
	})

	// peer1 向 peer2 发起同步请求
	go func() {
		stream := <-host1.streams
		protocolID := "/dep2p/realm/test-realm/sync/1.0.0"
		handler := host2.getHandler(protocolID)
		if handler != nil {
			handler(stream)
		}
	}()

	// 执行同步
	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = handler1.RequestSync(syncCtx, "peer2")
	require.NoError(t, err)

	// 等待回调
	time.Sleep(100 * time.Millisecond)
	require.True(t, syncSuccess, "sync should succeed")
	require.Equal(t, 3, addedCount, "should receive 3 members")
	require.Equal(t, 0, removedCount, "should have no removed members")

	// 验证同步器被调用
	require.Equal(t, 1, synchronizer1.GetDeltaSyncCalls())
	require.Equal(t, 3, synchronizer1.GetAddedCount())

	// 验证版本号已更新
	require.Equal(t, uint64(10), handler1.GetVersion())
}

func TestSyncHandler_Integration_VersionLower(t *testing.T) {
	// 创建两个节点
	host1 := newMockHost("peer1")
	host2 := newMockHost("peer2")

	realmID := "test-realm"

	synchronizer1 := newMockSynchronizer()
	synchronizer2 := newMockSynchronizer()
	memberManager1 := newMockMemberManager()
	memberManager2 := newMockMemberManager()

	ctx := context.Background()
	memberManager1.Start(ctx)
	memberManager2.Start(ctx)

	handler1 := NewSyncHandler(host1, realmID, synchronizer1, memberManager1)
	handler2 := NewSyncHandler(host2, realmID, synchronizer2, memberManager2)

	// 设置 peer1 的版本更高
	handler1.SetVersion(10)
	handler2.SetVersion(5)

	err := handler1.Start(ctx)
	require.NoError(t, err)
	defer handler1.Stop(ctx)

	err = handler2.Start(ctx)
	require.NoError(t, err)
	defer handler2.Stop(ctx)

	// peer1 向 peer2 发起同步请求（版本更低，不应该更新）
	go func() {
		stream := <-host1.streams
		protocolID := "/dep2p/realm/test-realm/sync/1.0.0"
		handler := host2.getHandler(protocolID)
		if handler != nil {
			handler(stream)
		}
	}()

	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = handler1.RequestSync(syncCtx, "peer2")
	require.NoError(t, err)

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	// 验证同步器未被调用（因为远程版本更低）
	require.Equal(t, 0, synchronizer1.GetDeltaSyncCalls())

	// 验证版本号未更新
	require.Equal(t, uint64(10), handler1.GetVersion())
}

func TestSyncHandler_Integration_VersionEqual(t *testing.T) {
	// 创建两个节点
	host1 := newMockHost("peer1")
	host2 := newMockHost("peer2")

	realmID := "test-realm"

	synchronizer1 := newMockSynchronizer()
	synchronizer2 := newMockSynchronizer()
	memberManager1 := newMockMemberManager()
	memberManager2 := newMockMemberManager()

	ctx := context.Background()
	memberManager1.Start(ctx)
	memberManager2.Start(ctx)

	handler1 := NewSyncHandler(host1, realmID, synchronizer1, memberManager1)
	handler2 := NewSyncHandler(host2, realmID, synchronizer2, memberManager2)

	// 设置相同版本
	handler1.SetVersion(10)
	handler2.SetVersion(10)

	err := handler1.Start(ctx)
	require.NoError(t, err)
	defer handler1.Stop(ctx)

	err = handler2.Start(ctx)
	require.NoError(t, err)
	defer handler2.Stop(ctx)

	// peer1 向 peer2 发起同步请求（版本相同，不应该更新）
	go func() {
		stream := <-host1.streams
		protocolID := "/dep2p/realm/test-realm/sync/1.0.0"
		handler := host2.getHandler(protocolID)
		if handler != nil {
			handler(stream)
		}
	}()

	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = handler1.RequestSync(syncCtx, "peer2")
	require.NoError(t, err)

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	// 验证同步器未被调用
	require.Equal(t, 0, synchronizer1.GetDeltaSyncCalls())
}

func TestSyncHandler_Integration_RealmMismatch(t *testing.T) {
	// 创建两个节点，使用不同的 RealmID
	host1 := newMockHost("peer1")
	host2 := newMockHost("peer2")

	realmID1 := "realm-1"
	realmID2 := "realm-2"

	synchronizer1 := newMockSynchronizer()
	synchronizer2 := newMockSynchronizer()
	memberManager1 := newMockMemberManager()
	memberManager2 := newMockMemberManager()

	ctx := context.Background()
	memberManager1.Start(ctx)
	memberManager2.Start(ctx)

	handler1 := NewSyncHandler(host1, realmID1, synchronizer1, memberManager1)
	handler2 := NewSyncHandler(host2, realmID2, synchronizer2, memberManager2)

	err := handler1.Start(ctx)
	require.NoError(t, err)
	defer handler1.Stop(ctx)

	err = handler2.Start(ctx)
	require.NoError(t, err)
	defer handler2.Stop(ctx)

	// peer1 向 peer2 发起同步请求（RealmID 不匹配）
	go func() {
		stream := <-host1.streams
		protocolID := "/dep2p/realm/realm-2/sync/1.0.0"
		handler := host2.getHandler(protocolID)
		if handler != nil {
			handler(stream)
		}
	}()

	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 应该失败（因为打开的是 realm-2 的协议，但 handler1 的 request 携带 realm-1）
	_ = handler1.RequestSync(syncCtx, "peer2")

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	// 验证同步器未被调用
	require.Equal(t, 0, synchronizer1.GetDeltaSyncCalls())
}

func TestSyncHandler_Integration_WithRemovedMembers(t *testing.T) {
	// 测试包含移除成员的同步
	host1 := newMockHost("peer1")
	host2 := newMockHost("peer2")

	realmID := "test-realm"

	synchronizer1 := newMockSynchronizer()
	synchronizer2 := newMockSynchronizer()
	memberManager1 := newMockMemberManager()
	memberManager2 := newMockMemberManager()

	ctx := context.Background()
	memberManager1.Start(ctx)
	memberManager2.Start(ctx)

	handler1 := NewSyncHandler(host1, realmID, synchronizer1, memberManager1)
	handler2 := NewSyncHandler(host2, realmID, synchronizer2, memberManager2)

	// 设置版本
	handler1.SetVersion(1)
	handler2.SetVersion(5)

	err := handler1.Start(ctx)
	require.NoError(t, err)
	defer handler1.Stop(ctx)

	err = handler2.Start(ctx)
	require.NoError(t, err)
	defer handler2.Stop(ctx)

	// 手动模拟带有移除成员的响应
	go func() {
		stream := <-host1.streams
		
		// 读取请求
		typeBuf := make([]byte, 1)
		stream.Read(typeBuf)
		reqData, _ := readMessage(stream)
		_, _ = decodeSyncRequest(reqData)

		// 发送响应（包含移除成员）
		resp := &SyncResponse{
			Version: 5,
			Added: []*interfaces.MemberInfo{
				{PeerID: "peer3", RealmID: realmID, Role: interfaces.RoleMember, Online: true},
			},
			Removed: []string{"peer4", "peer5"},
		}

		stream.Write([]byte{MsgTypeSyncResponse})
		writeMessage(stream, encodeSyncResponse(resp))
	}()

	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = handler1.RequestSync(syncCtx, "peer2")
	require.NoError(t, err)

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	// 验证同步器被调用
	require.Equal(t, 1, synchronizer1.GetDeltaSyncCalls())
}
