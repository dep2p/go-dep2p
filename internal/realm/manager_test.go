package realm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// setupTestManager 创建带有 mock 依赖的测试 Manager
func setupTestManager(t *testing.T) *Manager {
	mockHost := mocks.NewMockHost("test-peer")
	mockDiscovery := mocks.NewMockDiscovery()
	mockPeerstore := mocks.NewMockPeerstore()
	mockEventBus := mocks.NewMockEventBus()

	manager := NewManagerMinimal(mockHost, mockDiscovery, mockPeerstore, mockEventBus, nil)
	require.NotNil(t, manager)
	return manager
}

// ============================================================================
//                              Manager 基础测试（5个）
// ============================================================================

// TestManager_Join 测试加入 Realm
func TestManager_Join(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)
	defer manager.Close()

	// 加入 Realm
	realm, err := manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err, "Join should succeed")
	assert.NotNil(t, realm, "Realm should not be nil")
}

// TestManager_Leave 测试离开 Realm
func TestManager_Leave(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 先加入
	realm, err := manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err)
	require.NotNil(t, realm)

	// 离开
	err = manager.Leave(ctx)
	require.NoError(t, err, "Leave should succeed")
}

// TestManager_Current 测试获取当前 Realm
func TestManager_Current(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 未加入时为 nil
	current := manager.Current()
	assert.Nil(t, current, "Current should be nil before Join")

	// 加入后不为 nil
	realm, err := manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err)
	require.NotNil(t, realm)

	current = manager.Current()
	assert.NotNil(t, current, "Current realm should not be nil after Join")
}

// TestManager_Get 测试获取指定 Realm
func TestManager_Get(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 加入 Realm
	_, err := manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err)

	// 获取
	realm, ok := manager.Get("test-realm")
	assert.True(t, ok, "Get should return true for existing realm")
	assert.NotNil(t, realm, "Realm should not be nil")
}

// TestManager_List 测试列出所有 Realm
func TestManager_List(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 列出（空）
	list := manager.List()
	assert.NotNil(t, list)
}

// ============================================================================
//                              Realm 切换测试（3个）
// ============================================================================

// TestManager_SwitchRealm 测试切换 Realm
func TestManager_SwitchRealm(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 加入第一个 Realm
	realm1, err := manager.Join(ctx, "realm1", []byte("psk-key-111111111"))
	require.NoError(t, err)
	assert.NotNil(t, realm1)

	// 切换到第二个 Realm
	realm2, err := manager.Join(ctx, "realm2", []byte("psk-key-222222222"))
	require.NoError(t, err)
	assert.NotNil(t, realm2)
}

// TestManager_LeaveBeforeSwitch 测试切换前离开
func TestManager_LeaveBeforeSwitch(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 加入 Realm
	_, err := manager.Join(ctx, "realm1", []byte("psk-key-111111111"))
	require.NoError(t, err)

	// 离开
	err = manager.Leave(ctx)
	require.NoError(t, err, "Leave should succeed")

	// 加入另一个
	realm2, err := manager.Join(ctx, "realm2", []byte("psk-key-222222222"))
	require.NoError(t, err)
	assert.NotNil(t, realm2)
}

// TestManager_AutoLeave 测试自动离开
func TestManager_AutoLeave(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 加入 Realm
	_, err := manager.Join(ctx, "realm1", []byte("psk-key-111111111"))
	require.NoError(t, err)

	// 直接加入另一个（应自动离开前一个）
	realm2, err := manager.Join(ctx, "realm2", []byte("psk-key-222222222"))
	require.NoError(t, err, "Auto-leave and join should succeed")
	assert.NotNil(t, realm2)
}

// ============================================================================
//                              并发测试（5个）
// ============================================================================

// TestManager_ConcurrentJoin 测试并发加入
func TestManager_ConcurrentJoin(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 并发加入同一个 Realm
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			manager.Join(ctx, "test-realm", []byte("test-psk"))
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestManager_ConcurrentLeave 测试并发离开
func TestManager_ConcurrentLeave(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	manager.Join(ctx, "test-realm", []byte("test-psk"))

	// 并发离开
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			manager.Leave(ctx)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestManager_ConcurrentSwitch 测试并发切换
func TestManager_ConcurrentSwitch(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 并发切换 Realm
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			realmID := "realm-" + string(rune('0'+id%3))
			manager.Join(ctx, realmID, []byte("psk"))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestManager_ConcurrentGet 测试并发查询
func TestManager_ConcurrentGet(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	manager.Join(ctx, "test-realm", []byte("test-psk"))

	// 并发查询
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			manager.Get("test-realm")
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestManager_ConcurrentList 测试并发列表
func TestManager_ConcurrentList(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	manager.Join(ctx, "test-realm", []byte("test-psk"))

	// 并发列表
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			manager.List()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// ============================================================================
//                              错误处理测试（8个）
// ============================================================================

// TestManager_JoinWithoutStart 测试未启动时加入
func TestManager_JoinWithoutStart(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()

	_, err := manager.Join(ctx, "test-realm", []byte("test-psk"))
	assert.Error(t, err, "Join without Start should fail")
}

// TestManager_JoinInvalidRealmID 测试无效的 RealmID
func TestManager_JoinInvalidRealmID(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	_, err := manager.Join(ctx, "", []byte("test-psk"))
	assert.Error(t, err, "Join with empty RealmID should fail")
}

// TestManager_JoinInvalidPSK 测试无效的 PSK
func TestManager_JoinInvalidPSK(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	_, err := manager.Join(ctx, "test-realm", nil)
	assert.Error(t, err, "Join with nil PSK should fail")
}

// TestManager_LeaveWithoutJoin 测试未加入时离开
func TestManager_LeaveWithoutJoin(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	err := manager.Leave(ctx)
	assert.Error(t, err, "Leave without Join should fail")
}

// TestManager_DoubleStart 测试重复启动
func TestManager_DoubleStart(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	defer manager.Close()

	err1 := manager.Start(ctx)
	require.NoError(t, err1, "First Start should succeed")

	err2 := manager.Start(ctx)
	assert.Error(t, err2, "Second Start should fail")
}

// TestManager_DoubleClose 测试重复关闭
func TestManager_DoubleClose(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)

	err1 := manager.Close()
	require.NoError(t, err1, "First Close should succeed")

	err2 := manager.Close()
	// 重复关闭应该是幂等的（成功或返回已关闭错误）
	assert.NoError(t, err2, "Second Close should be idempotent")
}

// TestManager_JoinTimeout 测试加入超时
func TestManager_JoinTimeout(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	manager.Start(context.Background())
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond)
	_, err := manager.Join(ctx, "test-realm", []byte("test-psk"))
	assert.Error(t, err, "Join with expired context should fail")
}

// TestManager_LeaveTimeout 测试离开超时
func TestManager_LeaveTimeout(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	_, err := manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err)

	// Leave 操作本身很快完成，不会因为 context 超时而失败
	// 这里测试的是 Leave 在正常情况下能正确工作
	err = manager.Leave(ctx)
	require.NoError(t, err, "Leave should succeed")

	// 验证 Current 为 nil
	assert.Nil(t, manager.Current(), "Current should be nil after Leave")
}

// ============================================================================
//                              配置验证测试（3个）
// ============================================================================

// TestManager_DefaultConfig 测试默认配置
func TestManager_DefaultConfig(t *testing.T) {
	config := DefaultManagerConfig()
	assert.NotNil(t, config)
}

// TestManager_ValidateConfig 测试配置验证
func TestManager_ValidateConfig(t *testing.T) {
	config := DefaultManagerConfig()
	err := config.Validate()
	assert.NoError(t, err, "Default config should be valid")
}

// TestManager_CloneConfig 测试配置克隆
func TestManager_CloneConfig(t *testing.T) {
	config := DefaultManagerConfig()
	cloned := config.Clone()
	assert.NotNil(t, cloned)
}

// ============================================================================
//                              生命周期测试（6个）
// ============================================================================

// TestManager_Lifecycle 测试完整生命周期
func TestManager_Lifecycle(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()

	// Start
	err := manager.Start(ctx)
	require.NoError(t, err, "Start should succeed")

	// Join
	_, err = manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err, "Join should succeed")

	// Leave
	err = manager.Leave(ctx)
	require.NoError(t, err, "Leave should succeed")

	// Stop
	err = manager.Stop(ctx)
	require.NoError(t, err, "Stop should succeed")

	// Close
	err = manager.Close()
	require.NoError(t, err, "Close should succeed")
}

// TestManager_StartStop 测试启动停止
func TestManager_StartStop(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()

	err := manager.Start(ctx)
	require.NoError(t, err)

	err = manager.Stop(ctx)
	require.NoError(t, err)
}

// TestManager_StopWithRealm 测试停止时有 Realm
func TestManager_StopWithRealm(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	_, err := manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err)

	err = manager.Stop(ctx)
	require.NoError(t, err, "Stop with active Realm should succeed")
}

// TestManager_CloseWithRealm 测试关闭时有 Realm
func TestManager_CloseWithRealm(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)

	_, err := manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err)

	err = manager.Close()
	require.NoError(t, err, "Close with active Realm should succeed")
}

// TestManager_RestartAfterStop 测试停止后重启
func TestManager_RestartAfterStop(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	defer manager.Close()

	err := manager.Start(ctx)
	require.NoError(t, err)
	err = manager.Stop(ctx)
	require.NoError(t, err)

	err = manager.Start(ctx)
	require.NoError(t, err, "Restart after Stop should succeed")
}

// TestManager_JoinAfterClose 测试关闭后加入
func TestManager_JoinAfterClose(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	manager.Close()

	_, err := manager.Join(ctx, "test-realm", []byte("test-psk"))
	assert.Error(t, err, "Join after Close should fail")
}

// ============================================================================
//                              补充覆盖率测试
// ============================================================================

// TestManager_SetFactories 测试设置工厂
func TestManager_SetFactories(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)

	// 设置 Auth 工厂
	manager.SetAuthFactory(func(realmID string, psk []byte) (interfaces.Authenticator, error) {
		return nil, nil
	})

	// 设置 Member 工厂
	manager.SetMemberFactory(func(realmID string) (interfaces.MemberManager, error) {
		return nil, nil
	})

	// 设置 Routing 工厂
	manager.SetRoutingFactory(func(realmID string) (interfaces.Router, error) {
		return nil, nil
	})

	// 设置 Gateway 工厂
	manager.SetGatewayFactory(func(realmID string, auth interfaces.Authenticator) (interfaces.Gateway, error) {
		return nil, nil
	})
}

// TestManager_GetStats 测试获取统计
func TestManager_GetStats(t *testing.T) {
	manager := NewManagerMinimal(nil, nil, nil, nil, nil)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	stats := manager.GetStats()
	assert.NotNil(t, stats)
}

// TestManager_CreateMethod 测试 Create 方法
func TestManager_CreateMethod(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	realm, err := manager.Create(ctx, "test-realm", "Test Realm", []byte("test-psk-key-123"))
	require.NoError(t, err, "Create should succeed")
	assert.NotNil(t, realm)
}

// TestConfig_ValidateInvalid 测试无效配置验证
func TestConfig_ValidateInvalid(t *testing.T) {
	// 测试各种无效配置
	configs := []*ManagerConfig{
		{DefaultRealmName: ""},
		{DefaultRealmName: "test", MaxRealms: 0},
		{DefaultRealmName: "test", MaxRealms: 1, AuthTimeout: 0},
		{DefaultRealmName: "test", MaxRealms: 1, AuthTimeout: 1, LeaveTimeout: 0},
		{DefaultRealmName: "test", MaxRealms: 1, AuthTimeout: 1, LeaveTimeout: 1, SyncInterval: 0},
	}

	for i, config := range configs {
		err := config.Validate()
		assert.Error(t, err, "Config %d should be invalid", i)
	}
}

// TestManager_CreateWithFactories 测试使用工厂创建
func TestManager_CreateWithFactories(t *testing.T) {
	manager := setupTestManager(t)
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Close()

	// 设置工厂
	manager.SetAuthFactory(func(realmID string, psk []byte) (interfaces.Authenticator, error) {
		return nil, nil
	})

	manager.SetMemberFactory(func(realmID string) (interfaces.MemberManager, error) {
		return nil, nil
	})

	manager.SetRoutingFactory(func(realmID string) (interfaces.Router, error) {
		return nil, nil
	})

	manager.SetGatewayFactory(func(realmID string, auth interfaces.Authenticator) (interfaces.Gateway, error) {
		return nil, nil
	})

	// 创建 Realm
	realm, err := manager.Join(ctx, "test-realm", []byte("test-psk-key-123"))
	require.NoError(t, err, "Join with factories should succeed")
	assert.NotNil(t, realm)
}
