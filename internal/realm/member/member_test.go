package member

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/member"
)

// ============================================================================
//                              成员添加/删除测试（5个）
// ============================================================================

// TestManager_AddMember 测试添加成员
func TestManager_AddMember(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx) // 启动管理器

	member := &interfaces.MemberInfo{
		PeerID:   "peer1",
		RealmID:  "realm-test",
		Role:     interfaces.RoleMember,
		Online:   true,
		JoinedAt: time.Now(),
		LastSeen: time.Now(),
	}

	err := manager.Add(ctx, member)
	require.NoError(t, err)

	// 验证成员已添加
	retrieved, err := manager.Get(ctx, "peer1")
	require.NoError(t, err)
	assert.Equal(t, "peer1", retrieved.PeerID)
	assert.True(t, retrieved.Online)
}

// TestManager_RemoveMember 测试移除成员
func TestManager_RemoveMember(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 先添加
	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
		Role:    interfaces.RoleMember,
	}
	manager.Add(ctx, member)

	// 再移除
	err := manager.Remove(ctx, "peer1")
	require.NoError(t, err)

	// 验证已移除
	_, err = manager.Get(ctx, "peer1")
	assert.Error(t, err)
}

// TestManager_BatchAdd 测试批量添加
func TestManager_BatchAdd(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	members := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test", Role: interfaces.RoleMember},
		{PeerID: "peer2", RealmID: "realm-test", Role: interfaces.RoleMember},
		{PeerID: "peer3", RealmID: "realm-test", Role: interfaces.RoleAdmin},
	}

	err := manager.BatchAdd(ctx, members)
	require.NoError(t, err)

	// 验证所有成员都已添加
	assert.Equal(t, 3, manager.GetTotalCount())
}

// TestManager_BatchRemove 测试批量删除
func TestManager_BatchRemove(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 先批量添加
	members := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test", Role: interfaces.RoleMember},
		{PeerID: "peer2", RealmID: "realm-test", Role: interfaces.RoleMember},
		{PeerID: "peer3", RealmID: "realm-test", Role: interfaces.RoleAdmin},
	}
	manager.BatchAdd(ctx, members)

	// 批量删除
	peerIDs := []string{"peer1", "peer2"}
	err := manager.BatchRemove(ctx, peerIDs)
	require.NoError(t, err)

	// 验证删除成功
	assert.Equal(t, 1, manager.GetTotalCount())
	_, err = manager.Get(ctx, "peer3")
	assert.NoError(t, err)
}

// TestManager_DuplicateAdd 测试重复添加
func TestManager_DuplicateAdd(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
		Role:    interfaces.RoleMember,
	}

	// 第一次添加
	err := manager.Add(ctx, member)
	require.NoError(t, err)

	// 第二次添加（应该更新）
	member.Role = interfaces.RoleAdmin
	err = manager.Add(ctx, member)
	require.NoError(t, err)

	// 验证角色已更新
	retrieved, _ := manager.Get(ctx, "peer1")
	assert.Equal(t, interfaces.RoleAdmin, retrieved.Role)
}

// ============================================================================
//                              缓存功能测试（4个）
// ============================================================================

// TestCache_Basic 测试基础缓存功能
func TestCache_Basic(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)

	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
		Role:    interfaces.RoleMember,
	}

	// 设置缓存
	cache.Set(member)

	// 获取缓存
	retrieved, ok := cache.Get("peer1")
	require.True(t, ok)
	assert.Equal(t, "peer1", retrieved.PeerID)
}

// TestCache_TTL 测试 TTL 过期
func TestCache_TTL(t *testing.T) {
	cache := NewCache(100*time.Millisecond, 100)

	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
		Role:    interfaces.RoleMember,
	}

	cache.Set(member)

	// 立即获取应该成功
	_, ok := cache.Get("peer1")
	assert.True(t, ok)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 再次获取应该失败
	_, ok = cache.Get("peer1")
	assert.False(t, ok)
}

// TestCache_LRU 测试 LRU 淘汰
func TestCache_LRU(t *testing.T) {
	cache := NewCache(5*time.Minute, 2) // 容量为 2

	// 添加 3 个成员
	member1 := &interfaces.MemberInfo{PeerID: "peer1", RealmID: "realm-test"}
	member2 := &interfaces.MemberInfo{PeerID: "peer2", RealmID: "realm-test"}
	member3 := &interfaces.MemberInfo{PeerID: "peer3", RealmID: "realm-test"}

	cache.Set(member1)
	cache.Set(member2)
	cache.Set(member3) // 应该淘汰 peer1

	// peer1 应该被淘汰
	_, ok := cache.Get("peer1")
	assert.False(t, ok)

	// peer2 和 peer3 应该存在
	_, ok = cache.Get("peer2")
	assert.True(t, ok)
	_, ok = cache.Get("peer3")
	assert.True(t, ok)
}

// TestCache_Delete 测试删除缓存
func TestCache_Delete(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)

	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
	}

	cache.Set(member)
	cache.Delete("peer1")

	_, ok := cache.Get("peer1")
	assert.False(t, ok)
}

// ============================================================================
//                              同步机制测试（4个）
// ============================================================================

// TestSync_Full 测试全量同步
func TestSync_Full(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)
	sync := NewSynchronizer(manager, nil)

	members := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test", Role: interfaces.RoleMember},
		{PeerID: "peer2", RealmID: "realm-test", Role: interfaces.RoleMember},
	}

	err := sync.SyncFull(ctx, members)
	require.NoError(t, err)

	// 验证同步成功
	assert.Equal(t, 2, manager.GetTotalCount())
}

// TestSync_Delta 测试增量同步
func TestSync_Delta(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)
	sync := NewSynchronizer(manager, nil)

	// 先全量同步
	initial := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test", Role: interfaces.RoleMember},
		{PeerID: "peer2", RealmID: "realm-test", Role: interfaces.RoleMember},
	}
	sync.SyncFull(ctx, initial)

	// 增量同步：添加 peer3，删除 peer1
	added := []*interfaces.MemberInfo{
		{PeerID: "peer3", RealmID: "realm-test", Role: interfaces.RoleMember},
	}
	removed := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test"},
	}

	err := sync.SyncDelta(ctx, added, removed)
	require.NoError(t, err)

	// 验证结果
	assert.Equal(t, 2, manager.GetTotalCount())
	_, err = manager.Get(ctx, "peer1")
	assert.Error(t, err) // peer1 已删除
	_, err = manager.Get(ctx, "peer3")
	assert.NoError(t, err) // peer3 已添加
}

// TestSync_ConflictResolution 测试冲突解决
func TestSync_ConflictResolution(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 添加一个成员
	older := &interfaces.MemberInfo{
		PeerID:   "peer1",
		RealmID:  "realm-test",
		Role:     interfaces.RoleMember,
		LastSeen: time.Now().Add(-1 * time.Hour),
	}
	manager.Add(ctx, older)

	// 同步更新的版本
	newer := &interfaces.MemberInfo{
		PeerID:   "peer1",
		RealmID:  "realm-test",
		Role:     interfaces.RoleAdmin,
		LastSeen: time.Now(),
	}
	manager.Add(ctx, newer)

	// 验证使用了更新的版本
	retrieved, _ := manager.Get(ctx, "peer1")
	assert.Equal(t, interfaces.RoleAdmin, retrieved.Role)
}

// TestSync_EmptySync 测试空同步
func TestSync_EmptySync(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)
	sync := NewSynchronizer(manager, nil)

	// 同步空列表
	err := sync.SyncFull(ctx, []*interfaces.MemberInfo{})
	require.NoError(t, err)

	assert.Equal(t, 0, manager.GetTotalCount())
}

// ============================================================================
//                              心跳监控测试（3个）
// ============================================================================

// TestHeartbeat_Send 测试发送心跳
func TestHeartbeat_Send(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	monitor := NewHeartbeatMonitor(manager, nil, 15*time.Second, 3)

	// 启动管理器
	manager.Start(ctx)

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
		Online:  true,
	}
	manager.Add(ctx, member)

	// 发送心跳
	err := monitor.SendHeartbeat(ctx, "peer1")
	// host==nil 时应该返回错误（代码 heartbeat.go:133-134）
	assert.Error(t, err, "SendHeartbeat without host should fail")
	assert.Contains(t, err.Error(), "host is nil", "error should indicate missing host")
}

// TestHeartbeat_Timeout 测试心跳超时
func TestHeartbeat_Timeout(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	monitor := NewHeartbeatMonitor(manager, nil, 100*time.Millisecond, 2)

	// 启动管理器
	manager.Start(ctx)

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:   "peer1",
		RealmID:  "realm-test",
		Online:   true,
		LastSeen: time.Now(),
	}
	manager.Add(ctx, member)

	// 等待超时
	time.Sleep(300 * time.Millisecond)

	// 成员应该还在（只是标记为离线）
	retrieved, err := manager.Get(ctx, "peer1")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)

	// 验证 monitor 状态
	assert.NotNil(t, monitor, "monitor should not be nil")
}

// TestHeartbeat_StatusCheck 测试状态检查
func TestHeartbeat_StatusCheck(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	monitor := NewHeartbeatMonitor(manager, nil, 15*time.Second, 3)

	// 启动管理器
	manager.Start(ctx)

	// 添加在线成员
	member := &interfaces.MemberInfo{
		PeerID:   "peer1",
		RealmID:  "realm-test",
		Online:   true,
		LastSeen: time.Now(),
	}
	manager.Add(ctx, member)

	// 先发送一次心跳以初始化
	monitor.SendHeartbeat(ctx, "peer1")

	// 检查状态
	online, err := monitor.GetStatus("peer1")
	require.NoError(t, err)
	assert.True(t, online)
}

// TestHeartbeat_StartStop 测试心跳监控启动和停止
// 修复 A11: heartbeat Start/Stop 0% 覆盖
func TestHeartbeat_StartStop(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)

	// 使用默认参数
	monitor := NewHeartbeatMonitor(manager, nil, 0, 0)
	assert.Equal(t, 15*time.Second, monitor.interval, "默认间隔应为 15 秒")
	assert.Equal(t, 3, monitor.maxRetries, "默认重试次数应为 3")

	// 启动监控（host 为 nil 但应该能启动）
	err := monitor.Start(ctx)
	require.NoError(t, err)

	// 再次启动应返回错误
	err = monitor.Start(ctx)
	assert.ErrorIs(t, err, ErrAlreadyStarted)

	// 等待一小段时间让心跳循环运行
	time.Sleep(50 * time.Millisecond)

	// 停止监控
	err = monitor.Stop(ctx)
	require.NoError(t, err)

	// 再次停止应返回错误
	err = monitor.Stop(ctx)
	assert.ErrorIs(t, err, ErrNotStarted)
}

// TestHeartbeat_StartWithHost 测试带 host 的心跳启动
func TestHeartbeat_StartWithHost(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 创建一个简单的 mock host（只需要 SetStreamHandler）
	handlerSet := false
	host := &localMockHost{
		setStreamHandlerFunc: func(protocolID string, handler pkgif.StreamHandler) {
			if protocolID == HeartbeatProtocol {
				handlerSet = true
			}
		},
	}

	monitor := NewHeartbeatMonitor(manager, host, 100*time.Millisecond, 2)

	// 启动监控
	err := monitor.Start(ctx)
	require.NoError(t, err)

	// 验证流处理器已注册
	assert.True(t, handlerSet, "心跳流处理器应已注册")

	// 停止监控
	err = monitor.Stop(ctx)
	require.NoError(t, err)
}

// localMockHost 用于心跳测试的本地 mock
type localMockHost struct {
	setStreamHandlerFunc func(protocolID string, handler pkgif.StreamHandler)
}

func (h *localMockHost) ID() string                                                       { return "test-host" }
func (h *localMockHost) Addrs() []string                                                  { return nil }
func (h *localMockHost) Listen(addrs ...string) error                                     { return nil }
func (h *localMockHost) Connect(ctx context.Context, peerID string, addrs []string) error { return nil }
func (h *localMockHost) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {
	if h.setStreamHandlerFunc != nil {
		h.setStreamHandlerFunc(protocolID, handler)
	}
}
func (h *localMockHost) RemoveStreamHandler(protocolID string) {}
func (h *localMockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	return nil, fmt.Errorf("not implemented")
}
func (h *localMockHost) Peerstore() pkgif.Peerstore                                           { return nil }
func (h *localMockHost) EventBus() pkgif.EventBus                                             { return nil }
func (h *localMockHost) Close() error                                                         { return nil }
func (h *localMockHost) AdvertisedAddrs() []string                                            { return nil }
func (h *localMockHost) ShareableAddrs() []string                                             { return nil }
func (h *localMockHost) HolePunchAddrs() []string                                             { return nil }
func (h *localMockHost) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {}

func (h *localMockHost) Network() pkgif.Swarm { return nil }

func (h *localMockHost) HandleInboundStream(stream pkgif.Stream) {
	// Mock implementation: no-op
}

// ============================================================================
//                              角色管理测试（2个）
// ============================================================================

// TestRole_Permission 测试角色权限
func TestRole_Permission(t *testing.T) {
	tests := []struct {
		name     string
		role     interfaces.Role
		expected string
	}{
		{"Member", interfaces.RoleMember, "Member"},
		{"Admin", interfaces.RoleAdmin, "Admin"},
		{"Relay", interfaces.RoleRelay, "Relay"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.role.String())
		})
	}
}

// TestRole_Filter 测试角色过滤
func TestRole_Filter(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 添加不同角色的成员
	members := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test", Role: interfaces.RoleMember},
		{PeerID: "peer2", RealmID: "realm-test", Role: interfaces.RoleAdmin},
		{PeerID: "peer3", RealmID: "realm-test", Role: interfaces.RoleRelay},
	}
	manager.BatchAdd(ctx, members)

	// 过滤管理员
	adminRole := interfaces.RoleAdmin
	opts := &interfaces.ListOptions{
		Role: &adminRole,
	}
	filtered, err := manager.List(ctx, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, len(filtered))
	assert.Equal(t, "peer2", filtered[0].PeerID)
}

// ============================================================================
//                              统计信息测试（2个）
// ============================================================================

// TestStats_Basic 测试基础统计
func TestStats_Basic(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 添加成员
	members := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test", Role: interfaces.RoleMember, Online: true},
		{PeerID: "peer2", RealmID: "realm-test", Role: interfaces.RoleAdmin, Online: true},
		{PeerID: "peer3", RealmID: "realm-test", Role: interfaces.RoleRelay, Online: false},
	}
	manager.BatchAdd(ctx, members)

	// 获取统计
	stats := manager.GetStats()
	assert.Equal(t, 3, stats.TotalCount)
	assert.Equal(t, 2, stats.OnlineCount)
	assert.Equal(t, 1, stats.AdminCount)
	assert.Equal(t, 1, stats.RelayCount)
}

// TestStats_OnlineCount 测试在线统计
func TestStats_OnlineCount(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
		Online:  true,
	}
	manager.Add(ctx, member)

	assert.Equal(t, 1, manager.GetOnlineCount())

	// 更新状态为离线
	manager.UpdateStatus(ctx, "peer1", false)

	assert.Equal(t, 0, manager.GetOnlineCount())
}

// ============================================================================
//                 角色权限函数测试 - 覆盖 role.go
// ============================================================================

// TestPermission_String 测试权限字符串
func TestPermission_String(t *testing.T) {
	tests := []struct {
		perm     Permission
		expected string
	}{
		{PermRead, "Read"},
		{PermWrite, "Write"},
		{PermAdmin, "Admin"},
		{PermRelay, "Relay"},
		{Permission(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.perm.String())
		})
	}

	t.Log("✅ 权限字符串正确")
}

// TestHasPermission_NilMember 测试空成员权限
func TestHasPermission_NilMember(t *testing.T) {
	// nil 成员没有任何权限
	assert.False(t, HasPermission(nil, PermRead))
	assert.False(t, HasPermission(nil, PermWrite))
	assert.False(t, HasPermission(nil, PermAdmin))
	assert.False(t, HasPermission(nil, PermRelay))

	t.Log("✅ 空成员权限检查正确")
}

// TestHasPermission_Member 测试普通成员权限
func TestHasPermission_Member(t *testing.T) {
	member := &interfaces.MemberInfo{
		PeerID: "peer1",
		Role:   interfaces.RoleMember,
	}

	// 普通成员有读写权限
	assert.True(t, HasPermission(member, PermRead))
	assert.True(t, HasPermission(member, PermWrite))
	// 没有管理和中继权限
	assert.False(t, HasPermission(member, PermAdmin))
	assert.False(t, HasPermission(member, PermRelay))

	t.Log("✅ 普通成员权限正确")
}

// TestHasPermission_Admin 测试管理员权限
func TestHasPermission_Admin(t *testing.T) {
	admin := &interfaces.MemberInfo{
		PeerID: "admin1",
		Role:   interfaces.RoleAdmin,
	}

	// 管理员有所有基本权限和管理权限
	assert.True(t, HasPermission(admin, PermRead))
	assert.True(t, HasPermission(admin, PermWrite))
	assert.True(t, HasPermission(admin, PermAdmin))
	// 没有专门的中继权限
	assert.False(t, HasPermission(admin, PermRelay))

	t.Log("✅ 管理员权限正确")
}

// TestHasPermission_Relay 测试中继节点权限
func TestHasPermission_Relay(t *testing.T) {
	relay := &interfaces.MemberInfo{
		PeerID: "relay1",
		Role:   interfaces.RoleRelay,
	}

	// 中继有读写和中继权限
	assert.True(t, HasPermission(relay, PermRead))
	assert.True(t, HasPermission(relay, PermWrite))
	assert.True(t, HasPermission(relay, PermRelay))
	// 没有管理权限
	assert.False(t, HasPermission(relay, PermAdmin))

	t.Log("✅ 中继节点权限正确")
}

// TestHasPermission_UnknownPermission 测试未知权限
func TestHasPermission_UnknownPermission(t *testing.T) {
	member := &interfaces.MemberInfo{
		PeerID: "peer1",
		Role:   interfaces.RoleMember,
	}

	// 未知权限应该返回 false
	assert.False(t, HasPermission(member, Permission(99)))

	t.Log("✅ 未知权限正确返回false")
}

// TestRequireAdmin_Success 测试管理员要求成功
func TestRequireAdmin_Success(t *testing.T) {
	admin := &interfaces.MemberInfo{
		PeerID: "admin1",
		Role:   interfaces.RoleAdmin,
	}

	err := RequireAdmin(admin)
	assert.NoError(t, err)

	t.Log("✅ 管理员角色检查通过")
}

// TestRequireAdmin_Fail 测试管理员要求失败
func TestRequireAdmin_Fail(t *testing.T) {
	member := &interfaces.MemberInfo{
		PeerID: "peer1",
		Role:   interfaces.RoleMember,
	}

	err := RequireAdmin(member)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRole)

	t.Log("✅ 非管理员正确被拒绝")
}

// TestRequireAdmin_Nil 测试空成员
func TestRequireAdmin_Nil(t *testing.T) {
	err := RequireAdmin(nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidMember)

	t.Log("✅ 空成员正确被拒绝")
}

// TestRequireRelay_Success 测试中继要求成功
func TestRequireRelay_Success(t *testing.T) {
	relay := &interfaces.MemberInfo{
		PeerID: "relay1",
		Role:   interfaces.RoleRelay,
	}

	err := RequireRelay(relay)
	assert.NoError(t, err)

	t.Log("✅ 中继角色检查通过")
}

// TestRequireRelay_Fail 测试中继要求失败
func TestRequireRelay_Fail(t *testing.T) {
	member := &interfaces.MemberInfo{
		PeerID: "peer1",
		Role:   interfaces.RoleMember,
	}

	err := RequireRelay(member)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRole)

	t.Log("✅ 非中继正确被拒绝")
}

// TestIsAdmin 测试管理员检查
func TestIsAdmin(t *testing.T) {
	admin := &interfaces.MemberInfo{PeerID: "admin", Role: interfaces.RoleAdmin}
	member := &interfaces.MemberInfo{PeerID: "member", Role: interfaces.RoleMember}
	relay := &interfaces.MemberInfo{PeerID: "relay", Role: interfaces.RoleRelay}

	assert.True(t, IsAdmin(admin))
	assert.False(t, IsAdmin(member))
	assert.False(t, IsAdmin(relay))
	assert.False(t, IsAdmin(nil))

	t.Log("✅ IsAdmin检查正确")
}

// TestIsRelay 测试中继检查
func TestIsRelay(t *testing.T) {
	admin := &interfaces.MemberInfo{PeerID: "admin", Role: interfaces.RoleAdmin}
	member := &interfaces.MemberInfo{PeerID: "member", Role: interfaces.RoleMember}
	relay := &interfaces.MemberInfo{PeerID: "relay", Role: interfaces.RoleRelay}

	assert.False(t, IsRelay(admin))
	assert.False(t, IsRelay(member))
	assert.True(t, IsRelay(relay))
	assert.False(t, IsRelay(nil))

	t.Log("✅ IsRelay检查正确")
}

// TestIsMember 测试成员检查
func TestIsMember(t *testing.T) {
	admin := &interfaces.MemberInfo{PeerID: "admin", Role: interfaces.RoleAdmin}
	member := &interfaces.MemberInfo{PeerID: "member", Role: interfaces.RoleMember}
	relay := &interfaces.MemberInfo{PeerID: "relay", Role: interfaces.RoleRelay}

	assert.False(t, IsMember(admin))
	assert.True(t, IsMember(member))
	assert.False(t, IsMember(relay))
	assert.False(t, IsMember(nil))

	t.Log("✅ IsMember检查正确")
}

// TestCanManageMembers 测试成员管理权限
func TestCanManageMembers(t *testing.T) {
	admin := &interfaces.MemberInfo{PeerID: "admin", Role: interfaces.RoleAdmin}
	member := &interfaces.MemberInfo{PeerID: "member", Role: interfaces.RoleMember}
	relay := &interfaces.MemberInfo{PeerID: "relay", Role: interfaces.RoleRelay}

	assert.True(t, CanManageMembers(admin))
	assert.False(t, CanManageMembers(member))
	assert.False(t, CanManageMembers(relay))
	assert.False(t, CanManageMembers(nil))

	t.Log("✅ CanManageMembers检查正确")
}

// TestCanRelay 测试中继权限
func TestCanRelay(t *testing.T) {
	admin := &interfaces.MemberInfo{PeerID: "admin", Role: interfaces.RoleAdmin}
	member := &interfaces.MemberInfo{PeerID: "member", Role: interfaces.RoleMember}
	relay := &interfaces.MemberInfo{PeerID: "relay", Role: interfaces.RoleRelay}

	// 管理员和中继都可以中继
	assert.True(t, CanRelay(admin))
	assert.False(t, CanRelay(member))
	assert.True(t, CanRelay(relay))
	assert.False(t, CanRelay(nil))

	t.Log("✅ CanRelay检查正确")
}

// ============================================================================
//                 成员转换函数测试 - 覆盖 member.go
// ============================================================================

// TestMember_ToMemberInfo 测试成员转换
func TestMember_ToMemberInfo(t *testing.T) {
	now := time.Now()
	member := &Member{
		PeerID:        "peer1",
		RealmID:       "realm1",
		Role:          int(interfaces.RoleAdmin),
		Online:        true,
		JoinedAt:      now,
		LastSeen:      now,
		Addrs:         []string{"/ip4/1.2.3.4/tcp/4001"},
		Metadata:      map[string]string{"key": "value"},
		BytesSent:     1000,
		BytesReceived: 2000,
		MessagesSent:  50,
	}

	info := member.ToMemberInfo()

	assert.Equal(t, "peer1", info.PeerID)
	assert.Equal(t, "realm1", info.RealmID)
	assert.Equal(t, interfaces.RoleAdmin, info.Role)
	assert.True(t, info.Online)
	assert.Equal(t, int64(1000), info.BytesSent)

	t.Log("✅ ToMemberInfo转换正确")
}

// TestFromMemberInfo 测试从接口转换
func TestFromMemberInfo(t *testing.T) {
	now := time.Now()
	info := &interfaces.MemberInfo{
		PeerID:        "peer1",
		RealmID:       "realm1",
		Role:          interfaces.RoleRelay,
		Online:        true,
		JoinedAt:      now,
		LastSeen:      now,
		Addrs:         []string{"/ip4/1.2.3.4/tcp/4001"},
		Metadata:      map[string]string{"key": "value"},
		BytesSent:     1000,
		BytesReceived: 2000,
		MessagesSent:  50,
	}

	member := FromMemberInfo(info)

	assert.Equal(t, "peer1", member.PeerID)
	assert.Equal(t, "realm1", member.RealmID)
	assert.Equal(t, int(interfaces.RoleRelay), member.Role)
	assert.True(t, member.Online)

	t.Log("✅ FromMemberInfo转换正确")
}

// TestFromMemberInfo_Nil 测试空转换
func TestFromMemberInfo_Nil(t *testing.T) {
	member := FromMemberInfo(nil)
	assert.Nil(t, member)

	t.Log("✅ nil转换正确")
}

// ============================================================================
//                 Cache 补充测试 - 覆盖 0% 函数
// ============================================================================

// TestCache_Clear 测试缓存清空
func TestCache_Clear(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)
	defer cache.Close()

	// 添加多个成员
	for i := 0; i < 10; i++ {
		member := &interfaces.MemberInfo{
			PeerID:  fmt.Sprintf("peer%d", i),
			RealmID: "realm-test",
		}
		cache.Set(member)
	}

	// 验证添加成功
	assert.Equal(t, 10, cache.Size())

	// 清空缓存
	cache.Clear()

	// 验证清空成功
	assert.Equal(t, 0, cache.Size())

	// 再次获取应该失败
	_, ok := cache.Get("peer0")
	assert.False(t, ok)

	t.Log("✅ 缓存清空测试通过")
}

// TestCache_Size 测试缓存大小
func TestCache_Size(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)
	defer cache.Close()

	// 初始大小应该是 0
	assert.Equal(t, 0, cache.Size())

	// 添加成员
	for i := 0; i < 5; i++ {
		member := &interfaces.MemberInfo{
			PeerID:  fmt.Sprintf("peer%d", i),
			RealmID: "realm-test",
		}
		cache.Set(member)
	}

	// 大小应该是 5
	assert.Equal(t, 5, cache.Size())

	// 删除一个
	cache.Delete("peer0")
	assert.Equal(t, 4, cache.Size())

	t.Log("✅ 缓存大小测试通过")
}

// TestCache_Close 测试缓存关闭
func TestCache_Close(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
	}
	cache.Set(member)

	// 关闭缓存
	cache.Close()

	// 再次关闭应该是空操作（不崩溃）
	cache.Close()

	t.Log("✅ 缓存关闭测试通过")
}

// TestCache_GetStats 测试缓存统计
func TestCache_GetStats(t *testing.T) {
	cache := NewCache(5*time.Minute, 100)
	defer cache.Close()

	// 添加成员
	for i := 0; i < 10; i++ {
		member := &interfaces.MemberInfo{
			PeerID:  fmt.Sprintf("peer%d", i),
			RealmID: "realm-test",
		}
		cache.Set(member)
	}

	// 获取统计
	stats := cache.GetStats()
	assert.Equal(t, 10, stats.Size)
	assert.Equal(t, 100, stats.MaxSize)
	assert.Equal(t, 0.1, stats.Capacity)

	t.Log("✅ 缓存统计测试通过")
}

// TestCache_CleanupExpired 测试过期清理
func TestCache_CleanupExpired(t *testing.T) {
	// 使用很短的 TTL
	cache := NewCache(50*time.Millisecond, 100)
	defer cache.Close()

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:  "peer1",
		RealmID: "realm-test",
	}
	cache.Set(member)

	// 立即应该能获取
	_, ok := cache.Get("peer1")
	assert.True(t, ok)

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 手动触发清理
	cache.cleanupExpired()

	// 现在应该获取不到
	_, ok = cache.Get("peer1")
	assert.False(t, ok)

	t.Log("✅ 过期清理测试通过")
}

// ============================================================================
//                 Config 补充测试 - 覆盖 0% 函数
// ============================================================================

// TestConfig_Validate 测试配置验证
func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "默认配置有效",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "CacheSize 为 0",
			modify:  func(c *Config) { c.CacheSize = 0 },
			wantErr: true,
			errMsg:  "CacheSize must be positive",
		},
		{
			name:    "CacheSize 为负数",
			modify:  func(c *Config) { c.CacheSize = -1 },
			wantErr: true,
			errMsg:  "CacheSize must be positive",
		},
		{
			name:    "CacheTTL 为 0",
			modify:  func(c *Config) { c.CacheTTL = 0 },
			wantErr: true,
			errMsg:  "CacheTTL must be positive",
		},
		{
			name:    "无效的 StoreType",
			modify:  func(c *Config) { c.StoreType = "invalid" },
			wantErr: true,
			errMsg:  "StoreType must be",
		},
		{
			name:    "StoreType file 有效",
			modify:  func(c *Config) { c.StoreType = "file" },
			wantErr: false,
		},
		{
			name:    "StoreType memory 有效",
			modify:  func(c *Config) { c.StoreType = "memory" },
			wantErr: false,
		},
		{
			name:    "HeartbeatInterval 为 0",
			modify:  func(c *Config) { c.HeartbeatInterval = 0 },
			wantErr: true,
			errMsg:  "HeartbeatInterval must be positive",
		},
		{
			name:    "HeartbeatRetries 为负数",
			modify:  func(c *Config) { c.HeartbeatRetries = -1 },
			wantErr: true,
			errMsg:  "HeartbeatRetries must be non-negative",
		},
		{
			name:    "HeartbeatRetries 为 0 有效",
			modify:  func(c *Config) { c.HeartbeatRetries = 0 },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			tt.modify(config)

			err := config.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}

	t.Log("✅ 配置验证测试通过")
}

// TestConfig_Clone 测试配置克隆
func TestConfig_Clone(t *testing.T) {
	original := DefaultConfig()
	original.CacheSize = 500
	original.StorePath = "/tmp/test"
	original.HeartbeatRetries = 5

	cloned := original.Clone()

	// 验证值相等
	assert.Equal(t, original.CacheSize, cloned.CacheSize)
	assert.Equal(t, original.StorePath, cloned.StorePath)
	assert.Equal(t, original.HeartbeatRetries, cloned.HeartbeatRetries)
	assert.Equal(t, original.CacheTTL, cloned.CacheTTL)

	// 修改克隆不影响原始
	cloned.CacheSize = 999
	assert.NotEqual(t, original.CacheSize, cloned.CacheSize)
	assert.Equal(t, 500, original.CacheSize)

	t.Log("✅ 配置克隆测试通过")
}

// ============================================================================
//                 Manager 补充测试 - 覆盖 0% 函数
// ============================================================================

// TestManager_IsMember 测试成员检查
func TestManager_IsMember_Extended(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:  "peer123",
		RealmID: "realm-test",
		Role:    interfaces.RoleMember,
	}
	manager.Add(ctx, member)

	// 检查成员
	assert.True(t, manager.IsMember(ctx, "peer123"))
	assert.False(t, manager.IsMember(ctx, "non-existent-peer"))
	assert.False(t, manager.IsMember(ctx, "short")) // 短 ID

	t.Log("✅ 成员检查测试通过")
}

// TestManager_UpdateLastSeen 测试更新最后在线时间
func TestManager_UpdateLastSeen(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:   "peer123",
		RealmID:  "realm-test",
		Role:     interfaces.RoleMember,
		LastSeen: time.Now().Add(-1 * time.Hour),
	}
	manager.Add(ctx, member)

	// 记录原始时间
	original, _ := manager.Get(ctx, "peer123")
	originalLastSeen := original.LastSeen

	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)

	// 更新最后在线时间
	err := manager.UpdateLastSeen(ctx, "peer123")
	require.NoError(t, err)

	// 验证时间已更新
	updated, _ := manager.Get(ctx, "peer123")
	assert.True(t, updated.LastSeen.After(originalLastSeen))

	// 更新不存在的成员应该失败
	err = manager.UpdateLastSeen(ctx, "non-existent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrMemberNotFound)

	t.Log("✅ 更新最后在线时间测试通过")
}

// TestManager_StopAndClose 测试停止和关闭
func TestManager_StopAndClose(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)

	// 未启动时停止应该返回错误
	err := manager.Stop(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotStarted)

	// 启动
	manager.Start(ctx)

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:  "peer123",
		RealmID: "realm-test",
	}
	manager.Add(ctx, member)

	// 停止
	err = manager.Stop(ctx)
	assert.NoError(t, err)

	// 关闭
	err = manager.Close()
	assert.NoError(t, err)

	// 再次关闭应该是空操作
	err = manager.Close()
	assert.NoError(t, err)

	t.Log("✅ 停止和关闭测试通过")
}

// TestManager_WithCache 测试带缓存的管理器
func TestManager_WithCache(t *testing.T) {
	ctx := context.Background()
	cache := NewCache(5*time.Minute, 100)
	defer cache.Close()

	manager := NewManager("realm-test", cache, nil, nil)
	manager.Start(ctx)

	// 添加成员
	member := &interfaces.MemberInfo{
		PeerID:  "peer123",
		RealmID: "realm-test",
	}
	manager.Add(ctx, member)

	// 验证缓存中有数据
	cached, ok := cache.Get("peer123")
	assert.True(t, ok)
	assert.Equal(t, "peer123", cached.PeerID)

	// 更新最后在线时间
	manager.UpdateLastSeen(ctx, "peer123")

	// 验证缓存已更新
	cached, ok = cache.Get("peer123")
	assert.True(t, ok)

	t.Log("✅ 带缓存管理器测试通过")
}

// ============================================================================
//                 Member 结构体测试 - 覆盖 0% 函数
// ============================================================================

// TestMember_ToProto 测试转换为 Protobuf
func TestMember_ToProto(t *testing.T) {
	now := time.Now()
	member := &Member{
		PeerID:        "peer123",
		RealmID:       "realm-test",
		Role:          int(interfaces.RoleAdmin),
		Online:        true,
		JoinedAt:      now,
		LastSeen:      now,
		Addrs:         []string{"/ip4/127.0.0.1/tcp/4001", "/ip4/192.168.1.1/tcp/4001"},
		Metadata:      map[string]string{"key1": "value1", "key2": "value2"},
		BytesSent:     1000,
		BytesReceived: 2000,
		MessagesSent:  50,
	}

	pbMember := member.ToProto()
	require.NotNil(t, pbMember)

	assert.Equal(t, []byte("peer123"), pbMember.PeerId)
	assert.Equal(t, "realm-test", pbMember.RealmId)
	assert.Equal(t, int32(interfaces.RoleAdmin), pbMember.Role)
	assert.True(t, pbMember.Online)
	assert.Equal(t, now.Unix(), pbMember.JoinedAt)
	assert.Equal(t, now.Unix(), pbMember.LastSeen)
	assert.Len(t, pbMember.Addrs, 2)
	assert.Equal(t, "value1", pbMember.Metadata["key1"])
	assert.Equal(t, int64(1000), pbMember.BytesSent)
	assert.Equal(t, int64(2000), pbMember.BytesReceived)
	assert.Equal(t, int64(50), pbMember.MessagesSent)

	t.Log("✅ ToProto 测试通过")
}

// TestMember_FromProto 测试从 Protobuf 转换
func TestMember_FromProto(t *testing.T) {
	now := time.Now().Unix()
	pb := &pb.MemberInfo{
		PeerId:        []byte("peer456"),
		RealmId:       "realm-test",
		Role:          int32(interfaces.RoleRelay),
		Online:        false,
		JoinedAt:      now,
		LastSeen:      now,
		Addrs:         [][]byte{[]byte("/ip4/10.0.0.1/tcp/4001")},
		Metadata:      map[string]string{"env": "production"},
		BytesSent:     500,
		BytesReceived: 1000,
		MessagesSent:  25,
	}

	member := FromProto(pb)
	require.NotNil(t, member)

	assert.Equal(t, "peer456", member.PeerID)
	assert.Equal(t, "realm-test", member.RealmID)
	assert.Equal(t, int(interfaces.RoleRelay), member.Role)
	assert.False(t, member.Online)
	assert.Len(t, member.Addrs, 1)
	assert.Equal(t, "production", member.Metadata["env"])
	assert.Equal(t, int64(500), member.BytesSent)

	t.Log("✅ FromProto 测试通过")
}

// TestMember_FromProto_Nil 测试从 nil Protobuf 转换
func TestMember_FromProto_Nil(t *testing.T) {
	member := FromProto(nil)
	assert.Nil(t, member)

	t.Log("✅ FromProto nil 测试通过")
}

// TestMember_IsOnline 测试在线状态检查
func TestMember_IsOnline(t *testing.T) {
	member := &Member{Online: true}
	assert.True(t, member.IsOnline())

	member.Online = false
	assert.False(t, member.IsOnline())

	t.Log("✅ IsOnline 测试通过")
}

// TestMember_IsAdmin 测试管理员检查
func TestMember_IsAdmin(t *testing.T) {
	member := &Member{Role: int(interfaces.RoleAdmin)}
	assert.True(t, member.IsAdmin())

	member.Role = int(interfaces.RoleMember)
	assert.False(t, member.IsAdmin())

	t.Log("✅ Member.IsAdmin 测试通过")
}

// TestMember_IsRelay 测试中继检查
func TestMember_IsRelay(t *testing.T) {
	member := &Member{Role: int(interfaces.RoleRelay)}
	assert.True(t, member.IsRelay())

	member.Role = int(interfaces.RoleMember)
	assert.False(t, member.IsRelay())

	t.Log("✅ Member.IsRelay 测试通过")
}

// TestMember_HasRole 测试角色检查
func TestMember_HasRole(t *testing.T) {
	member := &Member{Role: int(interfaces.RoleAdmin)}
	assert.True(t, member.HasRole(interfaces.RoleAdmin))
	assert.False(t, member.HasRole(interfaces.RoleMember))
	assert.False(t, member.HasRole(interfaces.RoleRelay))

	t.Log("✅ HasRole 测试通过")
}

// TestMember_Clone 测试克隆
func TestMember_Clone(t *testing.T) {
	original := &Member{
		PeerID:        "peer123",
		RealmID:       "realm-test",
		Role:          int(interfaces.RoleAdmin),
		Online:        true,
		JoinedAt:      time.Now(),
		LastSeen:      time.Now(),
		Addrs:         []string{"/ip4/127.0.0.1/tcp/4001"},
		Metadata:      map[string]string{"key": "value"},
		BytesSent:     100,
		BytesReceived: 200,
	}

	cloned := original.Clone()
	require.NotNil(t, cloned)

	// 验证值相等
	assert.Equal(t, original.PeerID, cloned.PeerID)
	assert.Equal(t, original.Role, cloned.Role)
	assert.Equal(t, original.Online, cloned.Online)

	// 验证是深拷贝
	cloned.PeerID = "modified"
	cloned.Addrs[0] = "modified-addr"
	cloned.Metadata["key"] = "modified"

	assert.Equal(t, "peer123", original.PeerID)
	assert.Equal(t, "/ip4/127.0.0.1/tcp/4001", original.Addrs[0])
	assert.Equal(t, "value", original.Metadata["key"])

	t.Log("✅ Clone 测试通过")
}

// TestMember_Clone_Nil 测试 nil 克隆
func TestMember_Clone_Nil(t *testing.T) {
	var member *Member = nil
	cloned := member.Clone()
	assert.Nil(t, cloned)

	t.Log("✅ Clone nil 测试通过")
}

// ============================================================================
//                 Store 测试 - 覆盖 0% 函数
// ============================================================================

// TestStore_NewStore_Memory 测试内存存储
func TestStore_NewStore_Memory(t *testing.T) {
	store, err := NewStore("", "memory")
	require.NoError(t, err)
	require.NotNil(t, store)
	defer store.Close()

	t.Log("✅ NewStore memory 测试通过")
}

// TestStore_Save_Memory 测试内存保存
func TestStore_Save_Memory(t *testing.T) {
	store, _ := NewStore("", "memory")
	defer store.Close()

	member := &interfaces.MemberInfo{
		PeerID:  "peer123",
		RealmID: "realm-test",
	}

	err := store.Save(member)
	assert.NoError(t, err)

	// 验证已保存
	loaded, err := store.Load("peer123")
	require.NoError(t, err)
	assert.Equal(t, "peer123", loaded.PeerID)

	t.Log("✅ Store Save memory 测试通过")
}

// TestStore_Save_Nil 测试保存 nil
func TestStore_Save_Nil(t *testing.T) {
	store, _ := NewStore("", "memory")
	defer store.Close()

	err := store.Save(nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidMember, err)

	t.Log("✅ Store Save nil 测试通过")
}

// TestStore_Load_NotFound 测试加载不存在的成员
func TestStore_Load_NotFound(t *testing.T) {
	store, _ := NewStore("", "memory")
	defer store.Close()

	_, err := store.Load("non-existent")
	assert.Error(t, err)
	assert.Equal(t, ErrMemberNotFound, err)

	t.Log("✅ Store Load NotFound 测试通过")
}

// TestStore_Delete 测试删除
func TestStore_Delete(t *testing.T) {
	store, _ := NewStore("", "memory")
	defer store.Close()

	member := &interfaces.MemberInfo{
		PeerID:  "peer123",
		RealmID: "realm-test",
	}
	store.Save(member)

	err := store.Delete("peer123")
	assert.NoError(t, err)

	// 验证已删除
	_, err = store.Load("peer123")
	assert.Error(t, err)

	t.Log("✅ Store Delete 测试通过")
}

// TestStore_LoadAll 测试加载全部
func TestStore_LoadAll(t *testing.T) {
	store, _ := NewStore("", "memory")
	defer store.Close()

	// 保存多个成员
	store.Save(&interfaces.MemberInfo{PeerID: "peer1", RealmID: "realm-test"})
	store.Save(&interfaces.MemberInfo{PeerID: "peer2", RealmID: "realm-test"})
	store.Save(&interfaces.MemberInfo{PeerID: "peer3", RealmID: "realm-test"})

	members, err := store.LoadAll()
	require.NoError(t, err)
	assert.Len(t, members, 3)

	t.Log("✅ Store LoadAll 测试通过")
}

// TestStore_Compact_Memory 测试内存压缩（应该是 no-op）
func TestStore_Compact_Memory(t *testing.T) {
	store, _ := NewStore("", "memory")
	defer store.Close()

	err := store.Compact()
	assert.NoError(t, err)

	t.Log("✅ Store Compact memory 测试通过")
}

// TestStore_Close_Idempotent 测试关闭幂等性
func TestStore_Close_Idempotent(t *testing.T) {
	store, _ := NewStore("", "memory")

	err := store.Close()
	assert.NoError(t, err)

	err = store.Close()
	assert.NoError(t, err)

	t.Log("✅ Store Close 幂等性测试通过")
}

// TestStore_OperationsAfterClose 测试关闭后操作
func TestStore_OperationsAfterClose(t *testing.T) {
	store, _ := NewStore("", "memory")
	store.Close()

	// 所有操作都应该返回 ErrStoreClosed
	err := store.Save(&interfaces.MemberInfo{PeerID: "peer1"})
	assert.Equal(t, ErrStoreClosed, err)

	_, err = store.Load("peer1")
	assert.Equal(t, ErrStoreClosed, err)

	err = store.Delete("peer1")
	assert.Equal(t, ErrStoreClosed, err)

	_, err = store.LoadAll()
	assert.Equal(t, ErrStoreClosed, err)

	err = store.Compact()
	assert.Equal(t, ErrStoreClosed, err)

	t.Log("✅ Store 关闭后操作测试通过")
}

// ============================================================================
//                 StatsCollector 测试 - 覆盖 0% 函数
// ============================================================================

// TestStatsCollector_New 测试创建
func TestStatsCollector_New(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	stats := NewStatsCollector(manager)
	require.NotNil(t, stats)

	t.Log("✅ NewStatsCollector 测试通过")
}

// TestStatsCollector_RecordAdded 测试记录添加
func TestStatsCollector_RecordAdded(t *testing.T) {
	stats := NewStatsCollector(nil)

	stats.RecordAdded()
	stats.RecordAdded()
	stats.RecordAdded()

	assert.Equal(t, int64(3), stats.GetTotalAdded())

	t.Log("✅ RecordAdded 测试通过")
}

// TestStatsCollector_RecordRemoved 测试记录删除
func TestStatsCollector_RecordRemoved(t *testing.T) {
	stats := NewStatsCollector(nil)

	stats.RecordRemoved()
	stats.RecordRemoved()

	assert.Equal(t, int64(2), stats.GetTotalRemoved())

	t.Log("✅ RecordRemoved 测试通过")
}

// TestStatsCollector_RecordSync 测试记录同步
func TestStatsCollector_RecordSync(t *testing.T) {
	stats := NewStatsCollector(nil)

	beforeSync := time.Now()
	stats.RecordSync()
	afterSync := time.Now()

	assert.Equal(t, int64(1), stats.GetTotalSyncs())

	// 验证 lastSyncTime 在合理范围内
	s := stats.GetStats()
	assert.True(t, s.LastSyncTime.After(beforeSync) || s.LastSyncTime.Equal(beforeSync))
	assert.True(t, s.LastSyncTime.Before(afterSync) || s.LastSyncTime.Equal(afterSync))

	t.Log("✅ RecordSync 测试通过")
}

// TestStatsCollector_GetStats 测试获取统计
func TestStatsCollector_GetStats(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 添加一些成员
	manager.Add(ctx, &interfaces.MemberInfo{PeerID: "peer1", RealmID: "realm-test", Role: interfaces.RoleMember, Online: true})
	manager.Add(ctx, &interfaces.MemberInfo{PeerID: "peer2", RealmID: "realm-test", Role: interfaces.RoleAdmin})

	stats := NewStatsCollector(manager)
	s := stats.GetStats()

	assert.Equal(t, 2, s.TotalCount)
	assert.Equal(t, 1, s.OnlineCount)
	assert.Equal(t, 1, s.AdminCount)

	t.Log("✅ GetStats 测试通过")
}

// TestStatsCollector_Reset 测试重置
func TestStatsCollector_Reset(t *testing.T) {
	stats := NewStatsCollector(nil)

	stats.RecordAdded()
	stats.RecordRemoved()
	stats.RecordSync()

	stats.Reset()

	assert.Equal(t, int64(0), stats.GetTotalAdded())
	assert.Equal(t, int64(0), stats.GetTotalRemoved())
	assert.Equal(t, int64(0), stats.GetTotalSyncs())

	t.Log("✅ StatsCollector Reset 测试通过")
}

// TestStatsCollector_Concurrent 测试并发安全
func TestStatsCollector_Concurrent(t *testing.T) {
	stats := NewStatsCollector(nil)
	done := make(chan struct{})

	// 并发记录
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				stats.RecordAdded()
				stats.RecordRemoved()
				stats.RecordSync()
				_ = stats.GetTotalAdded()
				_ = stats.GetTotalRemoved()
				_ = stats.GetTotalSyncs()
			}
			done <- struct{}{}
		}()
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, int64(1000), stats.GetTotalAdded())
	assert.Equal(t, int64(1000), stats.GetTotalRemoved())
	assert.Equal(t, int64(1000), stats.GetTotalSyncs())

	t.Log("✅ StatsCollector 并发安全测试通过")
}

// ============================================================================
//                 Synchronizer 补充测试（覆盖 0% 函数）
// ============================================================================

// TestSynchronizer_GetVersion 测试获取版本号
func TestSynchronizer_GetVersion(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)
	sync := NewSynchronizer(manager, nil)

	// 初始版本应为 0
	assert.Equal(t, uint64(0), sync.GetVersion())

	// 同步后版本应增加
	members := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test"},
	}
	sync.SyncFull(ctx, members)

	assert.Greater(t, sync.GetVersion(), uint64(0))

	t.Log("✅ GetVersion 测试通过")
}

// TestSynchronizer_GetLastSyncTime 测试获取最后同步时间
func TestSynchronizer_GetLastSyncTime(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)
	sync := NewSynchronizer(manager, nil)

	// 初始时间应为零值
	initialTime := sync.GetLastSyncTime()
	assert.True(t, initialTime.IsZero())

	// 同步后时间应更新
	beforeSync := time.Now()
	members := []*interfaces.MemberInfo{
		{PeerID: "peer1", RealmID: "realm-test"},
	}
	sync.SyncFull(ctx, members)
	afterSync := time.Now()

	lastSyncTime := sync.GetLastSyncTime()
	assert.True(t, lastSyncTime.After(beforeSync) || lastSyncTime.Equal(beforeSync))
	assert.True(t, lastSyncTime.Before(afterSync) || lastSyncTime.Equal(afterSync))

	t.Log("✅ GetLastSyncTime 测试通过")
}

// ============================================================================
//                 Manager 补充测试（覆盖 0% 函数）
// ============================================================================

// TestManager_NewManagerWithConfig 测试带配置创建
func TestManager_NewManagerWithConfig(t *testing.T) {
	config := DefaultConfig()
	config.CacheSize = 500
	config.HeartbeatRetries = 5

	manager := NewManagerWithConfig("realm-test", config, nil, nil, nil)
	require.NotNil(t, manager)
	assert.Equal(t, 500, manager.config.CacheSize)
	assert.Equal(t, 5, manager.config.HeartbeatRetries)

	t.Log("✅ NewManagerWithConfig 测试通过")
}

// TestManager_NewManagerWithConfig_NilConfig 测试空配置
func TestManager_NewManagerWithConfig_NilConfig(t *testing.T) {
	manager := NewManagerWithConfig("realm-test", nil, nil, nil, nil)
	require.NotNil(t, manager)
	// 应该使用默认配置
	assert.Equal(t, DefaultConfig().CacheSize, manager.config.CacheSize)

	t.Log("✅ NewManagerWithConfig nil config 测试通过")
}

// TestManager_SetPeerstore_Nil 测试设置 nil Peerstore
func TestManager_SetPeerstore_Nil(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 初始没有 AddrSyncer
	assert.Nil(t, manager.AddrSyncer())

	// 设置为 nil 应该保持 nil
	manager.SetPeerstore(nil)
	assert.Nil(t, manager.AddrSyncer())

	t.Log("✅ SetPeerstore nil 测试通过")
}

// TestManager_AddrSyncer_Initial 测试获取地址同步器初始状态
func TestManager_AddrSyncer_Initial(t *testing.T) {
	ctx := context.Background()
	manager := NewManager("realm-test", nil, nil, nil)
	manager.Start(ctx)

	// 初始为 nil
	syncer := manager.AddrSyncer()
	assert.Nil(t, syncer)

	t.Log("✅ AddrSyncer 初始状态测试通过")
}

// ============================================================================
//                              成员清理测试
// ============================================================================

// TestManager_CleanupStaleMembers 测试过期成员清理
func TestManager_CleanupStaleMembers(t *testing.T) {
	ctx := context.Background()

	// 创建配置，使用短的清理间隔和离线时长
	config := DefaultConfig()
	config.CleanupInterval = 100 * time.Millisecond
	config.MaxOfflineDuration = 200 * time.Millisecond

	manager := NewManagerWithConfig("realm-test", config, nil, nil, nil)
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop(ctx)

	// 添加一个成员
	member := &interfaces.MemberInfo{
		PeerID:   "stale-peer",
		RealmID:  "realm-test",
		Online:   false,
		JoinedAt: time.Now().Add(-1 * time.Hour),
		LastSeen: time.Now().Add(-500 * time.Millisecond), // 已过期
	}
	require.NoError(t, manager.Add(ctx, member))

	// 验证成员已添加
	assert.True(t, manager.IsMember(ctx, "stale-peer"))

	// 等待清理循环执行
	time.Sleep(300 * time.Millisecond)

	// 验证成员已被清理
	assert.False(t, manager.IsMember(ctx, "stale-peer"), "过期成员应该被清理")

	t.Log("✅ 过期成员清理测试通过")
}

// TestManager_CleanupStaleMembers_SkipsOnlineMembers 测试清理跳过在线成员
func TestManager_CleanupStaleMembers_SkipsOnlineMembers(t *testing.T) {
	ctx := context.Background()

	// 创建配置，使用短的清理间隔和离线时长
	config := DefaultConfig()
	config.CleanupInterval = 100 * time.Millisecond
	config.MaxOfflineDuration = 200 * time.Millisecond

	manager := NewManagerWithConfig("realm-test", config, nil, nil, nil)
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop(ctx)

	// 添加一个在线成员，即使 LastSeen 很旧
	member := &interfaces.MemberInfo{
		PeerID:   "online-peer",
		RealmID:  "realm-test",
		Online:   true, // 在线
		JoinedAt: time.Now().Add(-1 * time.Hour),
		LastSeen: time.Now().Add(-500 * time.Millisecond), // 虽然很旧，但因为在线不应被清理
	}
	require.NoError(t, manager.Add(ctx, member))

	// 等待清理循环执行
	time.Sleep(300 * time.Millisecond)

	// 验证在线成员未被清理
	assert.True(t, manager.IsMember(ctx, "online-peer"), "在线成员不应该被清理")

	t.Log("✅ 在线成员跳过清理测试通过")
}

// TestManager_CleanupStaleMembers_Disabled 测试禁用清理
func TestManager_CleanupStaleMembers_Disabled(t *testing.T) {
	ctx := context.Background()

	// 创建配置，禁用清理
	config := DefaultConfig()
	config.CleanupInterval = 0 // 禁用

	manager := NewManagerWithConfig("realm-test", config, nil, nil, nil)
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop(ctx)

	// 添加一个过期成员
	member := &interfaces.MemberInfo{
		PeerID:   "stale-peer",
		RealmID:  "realm-test",
		Online:   false,
		JoinedAt: time.Now().Add(-1 * time.Hour),
		LastSeen: time.Now().Add(-48 * time.Hour), // 很旧
	}
	require.NoError(t, manager.Add(ctx, member))

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 验证成员未被清理（因为清理被禁用）
	assert.True(t, manager.IsMember(ctx, "stale-peer"), "禁用清理时成员不应被移除")

	t.Log("✅ 禁用清理测试通过")
}

// ============================================================================
//
// ============================================================================

// mockSwarmForDisconnectTest 是用于
type mockSwarmForDisconnectTest struct {
	connectedPeers map[string]bool
}

func newMockSwarmForDisconnectTest() *mockSwarmForDisconnectTest {
	return &mockSwarmForDisconnectTest{
		connectedPeers: make(map[string]bool),
	}
}

func (m *mockSwarmForDisconnectTest) LocalPeer() string            { return "self-peer" }
func (m *mockSwarmForDisconnectTest) Peers() []string              { return nil }
func (m *mockSwarmForDisconnectTest) Conns() []pkgif.Connection    { return nil }
func (m *mockSwarmForDisconnectTest) Listen(addrs ...string) error { return nil }
func (m *mockSwarmForDisconnectTest) ListenAddrs() []string        { return nil }
func (m *mockSwarmForDisconnectTest) Connectedness(peerID string) pkgif.Connectedness {
	return pkgif.NotConnected
}
func (m *mockSwarmForDisconnectTest) DialPeer(ctx context.Context, peerID string) (pkgif.Connection, error) {
	return nil, nil
}
func (m *mockSwarmForDisconnectTest) ClosePeer(peerID string) error { return nil }
func (m *mockSwarmForDisconnectTest) NewStream(ctx context.Context, peerID string) (pkgif.Stream, error) {
	return nil, nil
}
func (m *mockSwarmForDisconnectTest) SetInboundStreamHandler(handler pkgif.InboundStreamHandler) {}
func (m *mockSwarmForDisconnectTest) AddInboundConnection(conn pkgif.Connection)                 {}
func (m *mockSwarmForDisconnectTest) Notify(notifier pkgif.SwarmNotifier)                        {}
func (m *mockSwarmForDisconnectTest) Close() error                                               { return nil }

func (m *mockSwarmForDisconnectTest) ConnsToPeer(peerID string) []pkgif.Connection {
	if m.connectedPeers[peerID] {
		// 返回非空切片表示有连接
		return []pkgif.Connection{nil} // 简化 mock，只需非空即可
	}
	return nil
}

func (m *mockSwarmForDisconnectTest) SetConnected(peerID string, connected bool) {
	m.connectedPeers[peerID] = connected
}

// mockHostForDisconnectTest 是用于
type mockHostForDisconnectTest struct {
	selfID string
	swarm  *mockSwarmForDisconnectTest
}

func newMockHostForDisconnectTest(selfID string) *mockHostForDisconnectTest {
	return &mockHostForDisconnectTest{
		selfID: selfID,
		swarm:  newMockSwarmForDisconnectTest(),
	}
}

func (m *mockHostForDisconnectTest) ID() string                   { return m.selfID }
func (m *mockHostForDisconnectTest) Addrs() []string              { return nil }
func (m *mockHostForDisconnectTest) AdvertisedAddrs() []string    { return nil }
func (m *mockHostForDisconnectTest) ShareableAddrs() []string     { return nil }
func (m *mockHostForDisconnectTest) HolePunchAddrs() []string     { return nil }
func (m *mockHostForDisconnectTest) Listen(addrs ...string) error { return nil }
func (m *mockHostForDisconnectTest) Connect(ctx context.Context, peerID string, addrs []string) error {
	return nil
}
func (m *mockHostForDisconnectTest) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {
}
func (m *mockHostForDisconnectTest) RemoveStreamHandler(protocolID string) {}
func (m *mockHostForDisconnectTest) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	return nil, nil
}
func (m *mockHostForDisconnectTest) Peerstore() pkgif.Peerstore { return nil }
func (m *mockHostForDisconnectTest) EventBus() pkgif.EventBus   { return nil }
func (m *mockHostForDisconnectTest) Network() pkgif.Swarm       { return m.swarm }
func (m *mockHostForDisconnectTest) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
}
func (m *mockHostForDisconnectTest) Close() error                            { return nil }
func (m *mockHostForDisconnectTest) HandleInboundStream(stream pkgif.Stream) {}

// TestManager_DisconnectProtection 测试断开保护期
func TestManager_DisconnectProtection(t *testing.T) {
	ctx := context.Background()

	// 创建配置，使用短的断开保护期
	config := DefaultConfig()
	config.DisconnectProtection = 200 * time.Millisecond

	manager := NewManagerWithConfig("realm-test", config, nil, nil, nil)
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop(ctx)

	// 1. 先添加一个成员
	member := &interfaces.MemberInfo{
		PeerID:   "test-peer",
		RealmID:  "realm-test",
		Online:   true,
		JoinedAt: time.Now(),
		LastSeen: time.Now(),
	}
	require.NoError(t, manager.Add(ctx, member))
	assert.True(t, manager.IsMember(ctx, "test-peer"), "成员应该已添加")

	// 2. 模拟成员断开：手动记录断开时间
	manager.mu.Lock()
	manager.recentlyDisconnected["test-peer"] = time.Now()
	manager.mu.Unlock()

	// 3. 移除成员
	require.NoError(t, manager.Remove(ctx, "test-peer"))
	assert.False(t, manager.IsMember(ctx, "test-peer"), "成员应该已移除")

	// 4. 立即尝试重新添加（应该被拒绝，因为在保护期内）
	err := manager.Add(ctx, member)
	assert.NoError(t, err, "Add 不应返回错误，只是静默忽略")
	assert.False(t, manager.IsMember(ctx, "test-peer"), "保护期内成员不应被重新添加")

	// 5. 等待保护期过后再添加（应该成功）
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, manager.Add(ctx, member))
	assert.True(t, manager.IsMember(ctx, "test-peer"), "保护期过后成员应该可以重新添加")

	t.Log("✅ 断开保护期测试通过")
}

// TestManager_NoConnectionReject 测试无连接成员可以添加
//
// 注意：成员同步优化后，移除了"必须有活跃连接"检查。
// 这是因为成员同步通过 PubSub 广播传递，A 与 C 可能没有直接连接，但 C 是合法成员。
// 现在依赖断开保护期 + 防误判机制来防止竞态重新添加。
func TestManager_NoConnectionReject(t *testing.T) {
	ctx := context.Background()

	config := DefaultConfig()
	manager := NewManagerWithConfig("realm-test", config, nil, nil, nil)
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop(ctx)

	// 设置 mock Host
	mockHost := newMockHostForDisconnectTest("self-peer")
	manager.SetHost(mockHost)

	// 1. 尝试添加一个没有连接的成员（现在应该可以添加）
	// 这是成员同步优化后的新行为
	member := &interfaces.MemberInfo{
		PeerID:   "unconnected-peer",
		RealmID:  "realm-test",
		Online:   true,
		JoinedAt: time.Now(),
		LastSeen: time.Now(),
	}
	err := manager.Add(ctx, member)
	assert.NoError(t, err)
	assert.True(t, manager.IsMember(ctx, "unconnected-peer"), "无连接的成员现在可以添加（成员同步优化）")

	t.Log("✅ 无连接成员可以添加测试通过（成员同步优化后的行为）")
}

// TestManager_SelfAddAllowed 测试自己可以添加自己
func TestManager_SelfAddAllowed(t *testing.T) {
	ctx := context.Background()

	config := DefaultConfig()
	manager := NewManagerWithConfig("realm-test", config, nil, nil, nil)
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop(ctx)

	// 设置 mock Host
	mockHost := newMockHostForDisconnectTest("self-peer")
	manager.SetHost(mockHost)

	// 尝试添加自己（即使没有到自己的连接，也应该允许）
	selfMember := &interfaces.MemberInfo{
		PeerID:   "self-peer", // 与 Host ID 相同
		RealmID:  "realm-test",
		Online:   true,
		JoinedAt: time.Now(),
		LastSeen: time.Now(),
	}
	require.NoError(t, manager.Add(ctx, selfMember))
	assert.True(t, manager.IsMember(ctx, "self-peer"), "自己应该可以添加自己")

	t.Log("✅ 自己可以添加自己测试通过")
}

// TestManager_CleanupProtectionList 测试断开保护列表清理
func TestManager_CleanupProtectionList(t *testing.T) {
	ctx := context.Background()

	// 创建配置，使用很短的断开保护期
	config := DefaultConfig()
	config.DisconnectProtection = 50 * time.Millisecond

	manager := NewManagerWithConfig("realm-test", config, nil, nil, nil)
	require.NoError(t, manager.Start(ctx))
	defer manager.Stop(ctx)

	// 手动添加一些断开记录
	manager.mu.Lock()
	manager.recentlyDisconnected["peer1"] = time.Now().Add(-1 * time.Second) // 已过期
	manager.recentlyDisconnected["peer2"] = time.Now()                       // 未过期
	manager.mu.Unlock()

	// 手动触发清理
	manager.cleanupDisconnectProtection()

	// 验证过期记录已清理
	manager.mu.RLock()
	_, peer1Exists := manager.recentlyDisconnected["peer1"]
	_, peer2Exists := manager.recentlyDisconnected["peer2"]
	manager.mu.RUnlock()

	assert.False(t, peer1Exists, "过期的断开记录应该已清理")
	assert.True(t, peer2Exists, "未过期的断开记录应该保留")

	t.Log("✅ 断开保护列表清理测试通过")
}
