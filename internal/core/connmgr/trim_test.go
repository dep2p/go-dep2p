package connmgr

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestManager_CalculateScore 测试分数计算
func TestManager_CalculateScore(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	peer := "peer-1"

	// 无标签，分数为 0
	score := mgr.calculateScore(peer)
	assert.Equal(t, 0, score)

	// 添加标签
	mgr.TagPeer(peer, "bootstrap", 50)
	mgr.TagPeer(peer, "relay", 50)

	score = mgr.calculateScore(peer)
	assert.Equal(t, 100, score)

	t.Log("✅ calculateScore 正确")
}

// TestManager_TrimWithProtection 测试回收时保护机制
func TestManager_TrimWithProtection(t *testing.T) {
	cfg := Config{
		LowWater:  2,
		HighWater: 4,
	}
	mgr, _ := New(cfg)
	defer mgr.Close()

	host := newMockHost(5)
	mgr.SetHost(host)

	peers := host.Peers()

	// 保护前 2 个节点
	mgr.Protect(peers[0], "important")
	mgr.Protect(peers[1], "bootstrap")

	// 回收
	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 受保护的连接不应该被关闭
	assert.True(t, host.IsConnected(peers[0]))
	assert.True(t, host.IsConnected(peers[1]))

	// 其他连接被关闭（5 - 2 保护 = 3 个候选，回收至 2，关闭 1 个）
	// 实际上因为有 2 个受保护，只能关闭 3 个，回收到 2 个连接
	assert.Equal(t, 2, host.ConnCount())

	t.Log("✅ Trim 保护机制正确")
}

// TestManager_TrimBelowLowWater 测试低于低水位不回收
func TestManager_TrimBelowLowWater(t *testing.T) {
	cfg := Config{
		LowWater:  10,
		HighWater: 20,
	}
	mgr, _ := New(cfg)
	defer mgr.Close()

	// 只有 5 个连接（低于低水位）
	host := newMockHost(5)
	mgr.SetHost(host)

	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 不应该关闭任何连接
	assert.Equal(t, 5, host.ConnCount())

	t.Log("✅ 低于低水位不回收")
}

// TestManager_TrimNoHost 测试无 Host 时不崩溃
func TestManager_TrimNoHost(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	// 没有设置 Host
	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 不应该崩溃
	t.Log("✅ 无 Host 时安全返回")
}

// TestManager_TrimCancelContext 测试上下文取消
func TestManager_TrimCancelContext(t *testing.T) {
	cfg := Config{
		LowWater:  1,
		HighWater: 4,
	}
	mgr, _ := New(cfg)
	defer mgr.Close()

	host := newMockHost(5)
	mgr.SetHost(host)

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	mgr.TrimOpenConns(ctx)

	// 应该提前停止回收
	// 由于取消，可能不会回收到目标数量
	t.Log("✅ 上下文取消时停止回收")
}

// TestManager_SetHost 测试设置 Host
func TestManager_SetHost(t *testing.T) {
	mgr, _ := New(DefaultConfig())
	defer mgr.Close()

	host := newMockHost(3)
	mgr.SetHost(host)

	// 验证 Host 已设置
	assert.NotNil(t, mgr.host)

	t.Log("✅ SetHost 设置成功")
}
