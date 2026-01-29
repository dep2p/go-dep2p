package tls

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              测试辅助函数
// ============================================================================

// testPeerIDs 测试用 PeerID 列表
var testPeerIDs = []types.PeerID{
	types.PeerID("peer-alice"),
	types.PeerID("peer-bob"),
	types.PeerID("peer-charlie"),
	types.PeerID("peer-dave"),
	types.PeerID("peer-eve"),
}

// createTestAccessControl 创建测试用访问控制器
func createTestAccessControl(mode AccessMode) *AccessControl {
	return NewAccessControl(AccessControlConfig{
		Mode: mode,
	})
}

// ============================================================================
//                              构造函数测试
// ============================================================================

// TestNewAccessControl 测试创建访问控制器
func TestNewAccessControl(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		ac := NewAccessControl(DefaultAccessControlConfig())
		assert.NotNil(t, ac)
		assert.Equal(t, AccessModeAllowAll, ac.GetMode())
		assert.Equal(t, 0, ac.WhitelistSize())
		assert.Equal(t, 0, ac.BlacklistSize())
	})

	t.Run("with whitelist", func(t *testing.T) {
		ac := NewAccessControl(AccessControlConfig{
			Mode:      AccessModeWhitelist,
			Whitelist: testPeerIDs[:2],
		})
		assert.Equal(t, AccessModeWhitelist, ac.GetMode())
		assert.Equal(t, 2, ac.WhitelistSize())
	})

	t.Run("with blacklist", func(t *testing.T) {
		ac := NewAccessControl(AccessControlConfig{
			Mode:      AccessModeBlacklist,
			Blacklist: testPeerIDs[:3],
		})
		assert.Equal(t, AccessModeBlacklist, ac.GetMode())
		assert.Equal(t, 3, ac.BlacklistSize())
	})
}

// ============================================================================
//                              AllowAll 模式测试
// ============================================================================

// TestAccessControl_AllowAll 测试全放行模式
func TestAccessControl_AllowAll(t *testing.T) {
	ac := createTestAccessControl(AccessModeAllowAll)

	// 所有节点都应该被允许
	for _, peerID := range testPeerIDs {
		err := ac.Check(peerID)
		assert.NoError(t, err, "peer %s should be allowed", peerID)
	}

	// 即使添加到黑名单，AllowAll 模式也不检查
	ac.AddToBlacklist(testPeerIDs[0])
	err := ac.Check(testPeerIDs[0])
	assert.NoError(t, err)
}

// ============================================================================
//                              Whitelist 模式测试
// ============================================================================

// TestAccessControl_Whitelist 测试白名单模式
func TestAccessControl_Whitelist(t *testing.T) {
	ac := createTestAccessControl(AccessModeWhitelist)

	// 白名单为空时，所有节点都被拒绝
	err := ac.Check(testPeerIDs[0])
	assert.ErrorIs(t, err, ErrNotInWhitelist)

	// 添加到白名单后应该被允许
	ac.AddToWhitelist(testPeerIDs[0], testPeerIDs[1])
	err = ac.Check(testPeerIDs[0])
	assert.NoError(t, err)
	err = ac.Check(testPeerIDs[1])
	assert.NoError(t, err)

	// 不在白名单中的节点被拒绝
	err = ac.Check(testPeerIDs[2])
	assert.ErrorIs(t, err, ErrNotInWhitelist)
}

// TestAccessControl_WhitelistOperations 测试白名单操作
func TestAccessControl_WhitelistOperations(t *testing.T) {
	ac := createTestAccessControl(AccessModeWhitelist)

	// 添加
	ac.AddToWhitelist(testPeerIDs[0], testPeerIDs[1])
	assert.Equal(t, 2, ac.WhitelistSize())
	assert.True(t, ac.IsInWhitelist(testPeerIDs[0]))
	assert.True(t, ac.IsInWhitelist(testPeerIDs[1]))

	// 移除
	ac.RemoveFromWhitelist(testPeerIDs[0])
	assert.Equal(t, 1, ac.WhitelistSize())
	assert.False(t, ac.IsInWhitelist(testPeerIDs[0]))
	assert.True(t, ac.IsInWhitelist(testPeerIDs[1]))

	// 获取列表
	list := ac.GetWhitelist()
	assert.Len(t, list, 1)
	assert.Contains(t, list, testPeerIDs[1])

	// 清空
	ac.ClearWhitelist()
	assert.Equal(t, 0, ac.WhitelistSize())
}

// ============================================================================
//                              Blacklist 模式测试
// ============================================================================

// TestAccessControl_Blacklist 测试黑名单模式
func TestAccessControl_Blacklist(t *testing.T) {
	ac := createTestAccessControl(AccessModeBlacklist)

	// 黑名单为空时，所有节点都被允许
	err := ac.Check(testPeerIDs[0])
	assert.NoError(t, err)

	// 添加到黑名单后应该被拒绝
	ac.AddToBlacklist(testPeerIDs[0], testPeerIDs[1])
	err = ac.Check(testPeerIDs[0])
	assert.ErrorIs(t, err, ErrInBlacklist)
	err = ac.Check(testPeerIDs[1])
	assert.ErrorIs(t, err, ErrInBlacklist)

	// 不在黑名单中的节点被允许
	err = ac.Check(testPeerIDs[2])
	assert.NoError(t, err)
}

// TestAccessControl_BlacklistOperations 测试黑名单操作
func TestAccessControl_BlacklistOperations(t *testing.T) {
	ac := createTestAccessControl(AccessModeBlacklist)

	// 添加
	ac.AddToBlacklist(testPeerIDs[0], testPeerIDs[1])
	assert.Equal(t, 2, ac.BlacklistSize())
	assert.True(t, ac.IsInBlacklist(testPeerIDs[0]))
	assert.True(t, ac.IsInBlacklist(testPeerIDs[1]))

	// 移除
	ac.RemoveFromBlacklist(testPeerIDs[0])
	assert.Equal(t, 1, ac.BlacklistSize())
	assert.False(t, ac.IsInBlacklist(testPeerIDs[0]))
	assert.True(t, ac.IsInBlacklist(testPeerIDs[1]))

	// 获取列表
	list := ac.GetBlacklist()
	assert.Len(t, list, 1)
	assert.Contains(t, list, testPeerIDs[1])

	// 清空
	ac.ClearBlacklist()
	assert.Equal(t, 0, ac.BlacklistSize())
}

// ============================================================================
//                              Mixed 模式测试
// ============================================================================

// TestAccessControl_Mixed 测试混合模式
func TestAccessControl_Mixed(t *testing.T) {
	ac := createTestAccessControl(AccessModeMixed)

	// 白名单优先
	ac.AddToWhitelist(testPeerIDs[0])
	ac.AddToBlacklist(testPeerIDs[0]) // 同时在白名单和黑名单
	ac.AddToBlacklist(testPeerIDs[1]) // 只在黑名单

	// 白名单中的节点被允许（即使也在黑名单）
	err := ac.Check(testPeerIDs[0])
	assert.NoError(t, err)

	// 只在黑名单中的节点被拒绝
	err = ac.Check(testPeerIDs[1])
	assert.ErrorIs(t, err, ErrInBlacklist)

	// 既不在白名单也不在黑名单的节点被允许
	err = ac.Check(testPeerIDs[2])
	assert.NoError(t, err)
}

// ============================================================================
//                              模式管理测试
// ============================================================================

// TestAccessControl_ModeSwitch 测试模式切换
func TestAccessControl_ModeSwitch(t *testing.T) {
	ac := createTestAccessControl(AccessModeAllowAll)

	// 切换到白名单模式
	ac.SetMode(AccessModeWhitelist)
	assert.Equal(t, AccessModeWhitelist, ac.GetMode())

	// 切换到黑名单模式
	ac.SetMode(AccessModeBlacklist)
	assert.Equal(t, AccessModeBlacklist, ac.GetMode())

	// 切换到混合模式
	ac.SetMode(AccessModeMixed)
	assert.Equal(t, AccessModeMixed, ac.GetMode())

	// 切换回全放行模式
	ac.SetMode(AccessModeAllowAll)
	assert.Equal(t, AccessModeAllowAll, ac.GetMode())
}

// TestAccessMode_String 测试模式字符串表示
func TestAccessMode_String(t *testing.T) {
	assert.Equal(t, "allow_all", AccessModeAllowAll.String())
	assert.Equal(t, "whitelist", AccessModeWhitelist.String())
	assert.Equal(t, "blacklist", AccessModeBlacklist.String())
	assert.Equal(t, "mixed", AccessModeMixed.String())
	assert.Equal(t, "unknown", AccessMode(99).String())
}

// ============================================================================
//                              统计信息测试
// ============================================================================

// TestAccessControl_Stats 测试统计信息
func TestAccessControl_Stats(t *testing.T) {
	ac := createTestAccessControl(AccessModeWhitelist)

	// 初始统计
	stats := ac.Stats()
	assert.Equal(t, uint64(0), stats.AllowedCount)
	assert.Equal(t, uint64(0), stats.DeniedCount)

	// 添加白名单
	ac.AddToWhitelist(testPeerIDs[0])

	// 执行检查
	_ = ac.Check(testPeerIDs[0]) // 允许
	_ = ac.Check(testPeerIDs[1]) // 拒绝
	_ = ac.Check(testPeerIDs[2]) // 拒绝

	stats = ac.Stats()
	assert.Equal(t, uint64(1), stats.AllowedCount)
	assert.Equal(t, uint64(2), stats.DeniedCount)
	assert.Equal(t, 1, stats.WhitelistSize)
	assert.Equal(t, 0, stats.BlacklistSize)
	assert.Equal(t, AccessModeWhitelist, stats.Mode)

	// 重置统计
	ac.ResetStats()
	stats = ac.Stats()
	assert.Equal(t, uint64(0), stats.AllowedCount)
	assert.Equal(t, uint64(0), stats.DeniedCount)
}

// ============================================================================
//                              ConnectionGater 接口测试
// ============================================================================

// TestAccessControl_ConnectionGater 测试 ConnectionGater 接口
func TestAccessControl_ConnectionGater(t *testing.T) {
	ac := createTestAccessControl(AccessModeWhitelist)
	ac.AddToWhitelist(testPeerIDs[0])

	// 验证实现了接口
	var _ ConnectionGater = ac

	t.Run("InterceptPeerDial", func(t *testing.T) {
		assert.True(t, ac.InterceptPeerDial(testPeerIDs[0]))
		assert.False(t, ac.InterceptPeerDial(testPeerIDs[1]))
	})

	t.Run("InterceptAccept", func(t *testing.T) {
		assert.True(t, ac.InterceptAccept(testPeerIDs[0]))
		assert.False(t, ac.InterceptAccept(testPeerIDs[1]))
	})

	t.Run("InterceptSecured", func(t *testing.T) {
		assert.True(t, ac.InterceptSecured(testPeerIDs[0]))
		assert.False(t, ac.InterceptSecured(testPeerIDs[1]))
	})
}

// ============================================================================
//                              并发测试
// ============================================================================

// TestAccessControl_Concurrent 测试并发安全性
func TestAccessControl_Concurrent(t *testing.T) {
	ac := createTestAccessControl(AccessModeWhitelist)

	done := make(chan bool)

	// 使用固定长度的 PeerID 以避免切片越界
	makePeerID := func(i int) types.PeerID {
		return types.PeerID(fmt.Sprintf("peer-%08d", i))
	}

	// 并发添加
	go func() {
		for i := 0; i < 100; i++ {
			ac.AddToWhitelist(makePeerID(i))
		}
		done <- true
	}()

	// 并发移除
	go func() {
		for i := 0; i < 100; i++ {
			ac.RemoveFromWhitelist(makePeerID(i))
		}
		done <- true
	}()

	// 并发检查
	go func() {
		for i := 0; i < 100; i++ {
			_ = ac.Check(makePeerID(i))
		}
		done <- true
	}()

	// 并发读取
	go func() {
		for i := 0; i < 100; i++ {
			_ = ac.Stats()
			_ = ac.GetWhitelist()
			_ = ac.WhitelistSize()
		}
		done <- true
	}()

	// 等待所有 goroutine 完成
	for i := 0; i < 4; i++ {
		<-done
	}

	// 如果没有 panic，测试通过
}

// ============================================================================
//                              边界条件测试
// ============================================================================

// TestAccessControl_EmptyPeerID 测试空 PeerID
func TestAccessControl_EmptyPeerID(t *testing.T) {
	ac := createTestAccessControl(AccessModeWhitelist)

	// 空 PeerID 应该被正确处理
	err := ac.Check(types.PeerID(""))
	assert.ErrorIs(t, err, ErrNotInWhitelist)

	// 可以添加空 PeerID 到白名单
	ac.AddToWhitelist(types.PeerID(""))
	err = ac.Check(types.PeerID(""))
	assert.NoError(t, err)
}

// TestAccessControl_DuplicateAdd 测试重复添加
func TestAccessControl_DuplicateAdd(t *testing.T) {
	ac := createTestAccessControl(AccessModeWhitelist)

	// 重复添加不应该增加计数
	ac.AddToWhitelist(testPeerIDs[0])
	ac.AddToWhitelist(testPeerIDs[0])
	ac.AddToWhitelist(testPeerIDs[0])

	assert.Equal(t, 1, ac.WhitelistSize())
}

// TestDefaultAccessControlConfig 测试默认配置
func TestDefaultAccessControlConfig(t *testing.T) {
	config := DefaultAccessControlConfig()
	require.Equal(t, AccessModeAllowAll, config.Mode)
	require.Nil(t, config.Whitelist)
	require.Nil(t, config.Blacklist)
}
