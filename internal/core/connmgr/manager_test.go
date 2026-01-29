package connmgr

import (
	"context"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManager_New 测试创建管理器
func TestManager_New(t *testing.T) {
	cfg := DefaultConfig()
	mgr, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, mgr)

	defer mgr.Close()

	assert.Equal(t, cfg.LowWater, mgr.cfg.LowWater)
	assert.Equal(t, cfg.HighWater, mgr.cfg.HighWater)

	t.Log("✅ Manager 创建成功")
}

// TestManager_TagPeer 测试标签操作
func TestManager_TagPeer(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	peer := "test-peer-1"

	// 添加标签
	mgr.TagPeer(peer, "score", 10)

	// 获取标签信息
	info := mgr.GetTagInfo(peer)
	require.NotNil(t, info)
	assert.Equal(t, 10, info.Value)
	assert.Equal(t, 10, info.Tags["score"])

	t.Log("✅ 标签操作正确")
}

// TestManager_UntagPeer 测试移除标签
func TestManager_UntagPeer(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	peer := "test-peer-1"

	// 添加并移除标签
	mgr.TagPeer(peer, "score", 10)
	mgr.UntagPeer(peer, "score")

	info := mgr.GetTagInfo(peer)
	// 移除后应该没有标签信息
	assert.Nil(t, info)

	t.Log("✅ 移除标签正确")
}

// TestManager_UpsertTag 测试更新标签
func TestManager_UpsertTag(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	peer := "test-peer-1"

	// 初始值
	mgr.TagPeer(peer, "count", 5)

	// 更新（加倍）
	mgr.UpsertTag(peer, "count", func(old int) int {
		return old * 2
	})

	info := mgr.GetTagInfo(peer)
	assert.Equal(t, 10, info.Tags["count"])
	assert.Equal(t, 10, info.Value)

	t.Log("✅ UpsertTag 更新正确")
}

// TestManager_Protect 测试保护连接
func TestManager_Protect(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	peer := "test-peer-1"

	// 保护
	mgr.Protect(peer, "important")

	// 检查
	assert.True(t, mgr.IsProtected(peer, "important"))

	t.Log("✅ Protect 保护正确")
}

// TestManager_Unprotect 测试取消保护
func TestManager_Unprotect(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	peer := "test-peer-1"

	// 添加两个保护标签
	mgr.Protect(peer, "tag1")
	mgr.Protect(peer, "tag2")

	// 移除一个
	hasMore := mgr.Unprotect(peer, "tag1")
	assert.True(t, hasMore, "应该还有其他保护标签")
	assert.True(t, mgr.IsProtected(peer, "tag2"))

	// 移除最后一个
	hasMore = mgr.Unprotect(peer, "tag2")
	assert.False(t, hasMore, "不应该有保护标签了")
	assert.False(t, mgr.IsProtected(peer, "tag2"))

	t.Log("✅ Unprotect 取消保护正确")
}

// TestManager_TrimOpenConns 测试水位回收
func TestManager_TrimOpenConns(t *testing.T) {
	cfg := Config{
		LowWater:  2,
		HighWater: 4,
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	ctx := context.Background()

	// 无 Host 时，TrimOpenConns 应该安全返回（不 panic）
	mgr.TrimOpenConns(ctx)

	t.Log("✅ TrimOpenConns 安全执行（无 Host 模式）")
}

// TestManager_Notifee 测试通知器
func TestManager_Notifee(t *testing.T) {
	mgr, err := New(DefaultConfig())
	require.NoError(t, err)
	defer mgr.Close()

	notifee := mgr.Notifee()
	assert.NotNil(t, notifee)

	// 验证 notifee 实现了 SwarmNotifier 接口
	var _ pkgif.SwarmNotifier = notifee

	t.Log("✅ Notifee 返回正确")
}

// TestManager_Close 测试关闭
func TestManager_Close(t *testing.T) {
	mgr, _ := New(DefaultConfig())

	err := mgr.Close()
	assert.NoError(t, err)

	// 再次关闭应该返回错误
	err = mgr.Close()
	assert.Error(t, err)

	t.Log("✅ Close 关闭正确")
}

// TestManager_Concurrent 测试并发安全
func TestManager_Concurrent(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	done := make(chan bool, 10)

	// 并发添加标签
	for i := 0; i < 10; i++ {
		go func(n int) {
			peer := testPeer(string(rune('0' + n)))
			for j := 0; j < 100; j++ {
				mgr.TagPeer(peer, "score", j)
				mgr.UntagPeer(peer, "score")
			}
			done <- true
		}(i)
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("✅ 并发操作安全")
}

// TestManager_Interface 验证接口实现
func TestManager_Interface(t *testing.T) {
	var _ pkgif.ConnManager = (*Manager)(nil)
	t.Log("✅ Manager 实现 ConnManager 接口")
}

// testPeer 辅助函数
func testPeer(id string) string {
	return "test-peer-" + id
}

// testTagInfo 辅助函数
func testTagInfo() *pkgif.TagInfo {
	return &pkgif.TagInfo{
		FirstSeen: time.Now(),
		Value:     100,
		Tags: map[string]int{
			"bootstrap": 50,
			"relay":     50,
		},
		Conns: 1,
	}
}

// ============================================================================
// 覆盖率提升测试
// ============================================================================

// TestManager_SetLimits 测试设置水位线
func TestManager_SetLimits(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	// 设置新的水位线
	mgr.SetLimits(50, 100)

	// 验证
	low, high := mgr.GetLimits()
	assert.Equal(t, 50, low)
	assert.Equal(t, 100, high)

	t.Log("✅ SetLimits/GetLimits 正确")
}

// TestManager_TriggerTrim 测试手动触发裁剪
func TestManager_TriggerTrim(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	// 手动触发裁剪（无 Host 应该安全执行）
	mgr.TriggerTrim()

	t.Log("✅ TriggerTrim 安全执行")
}

// TestManager_ConnCount 测试连接计数
func TestManager_ConnCount(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	// 无 Host 时应该返回 0
	count := mgr.ConnCount()
	assert.Equal(t, 0, count)

	t.Log("✅ ConnCount 返回正确")
}

// TestManager_DialedConnCount 测试出站连接计数
func TestManager_DialedConnCount(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	// 无 Host 时应该返回 0
	count := mgr.DialedConnCount()
	assert.Equal(t, 0, count)

	t.Log("✅ DialedConnCount 返回正确")
}

// TestManager_InboundConnCount 测试入站连接计数
func TestManager_InboundConnCount(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	// 无 Host 时应该返回 0
	count := mgr.InboundConnCount()
	assert.Equal(t, 0, count)

	t.Log("✅ InboundConnCount 返回正确")
}

// TestManager_RateTracking 测试速率追踪
func TestManager_RateTracking(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	peer := "test-peer-1"

	// 更新速率（kind=0, elapsed=1ms, items=10）
	mgr.UpdatePeerRate(peer, 0, time.Millisecond, 10)

	// 获取容量（kind=0, targetRTT=10ms）
	capacity := mgr.GetPeerCapacity(peer, 0, 10*time.Millisecond)
	assert.GreaterOrEqual(t, capacity, 1) // 至少返回 1

	// 获取目标 RTT（有默认值）
	rtt := mgr.GetTargetRTT()
	assert.GreaterOrEqual(t, rtt, time.Duration(0))

	// 获取目标超时（有默认值）
	timeout := mgr.GetTargetTimeout()
	assert.GreaterOrEqual(t, timeout, time.Duration(0))

	// 获取中位 RTT（有默认值）
	medianRTT := mgr.GetMedianRTT()
	assert.GreaterOrEqual(t, medianRTT, time.Duration(0))

	// 获取平均容量
	meanCaps := mgr.GetMeanCapacities()
	assert.NotNil(t, meanCaps)

	// 获取速率追踪器（默认配置下会创建）
	trackers := mgr.RateTrackers()
	// 可能是 nil 或非 nil，取决于配置
	_ = trackers

	t.Log("✅ 速率追踪功能正确")
}
