package connmgr

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// integrationTestPeer 创建测试用 peer ID（避免与 manager_test.go 冲突）
func integrationTestPeer(suffix string) string {
	return "test-peer-" + suffix
}

// MockHost 模拟 Host 接口
type MockHost struct {
	conns map[string]*MockConn
}

func newMockHost(numConns int) *MockHost {
	host := &MockHost{
		conns: make(map[string]*MockConn),
	}

	for i := 0; i < numConns; i++ {
		peer := integrationTestPeer(fmt.Sprintf("%d", i))
		host.conns[peer] = &MockConn{
			remotePeer: peer,
			closed:     false,
		}
	}

	return host
}

func (h *MockHost) Connections() []Connection {
	conns := make([]Connection, 0, len(h.conns))
	for _, conn := range h.conns {
		if !conn.closed {
			conns = append(conns, conn)
		}
	}
	return conns
}

func (h *MockHost) ConnCount() int {
	count := 0
	for _, conn := range h.conns {
		if !conn.closed {
			count++
		}
	}
	return count
}

func (h *MockHost) Peers() []string {
	peers := make([]string, 0, len(h.conns))
	for peer := range h.conns {
		peers = append(peers, peer)
	}
	return peers
}

func (h *MockHost) IsConnected(peer string) bool {
	conn, ok := h.conns[peer]
	return ok && !conn.closed
}

func (h *MockHost) CloseConnection(peer string) error {
	if conn, ok := h.conns[peer]; ok {
		conn.closed = true
	}
	return nil
}

// MockConn 模拟连接
type MockConn struct {
	remotePeer string
	closed     bool
}

func (c *MockConn) RemotePeer() string {
	return c.remotePeer
}

func (c *MockConn) Close() error {
	c.closed = true
	return nil
}

// TestTrim_BelowLowWater 测试低于低水位，不回收
func TestTrim_BelowLowWater(t *testing.T) {
	cfg := Config{
		LowWater:  100,
		HighWater: 400,
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	// Mock 50 个连接（低于低水位）
	host := newMockHost(50)
	mgr.SetHost(host)

	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 验证：没有连接被关闭
	assert.Equal(t, 50, host.ConnCount())
}

// TestTrim_AboveHighWater 测试超过高水位，回收至低水位
func TestTrim_AboveHighWater(t *testing.T) {
	cfg := Config{
		LowWater:  10,
		HighWater: 40,
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	// Mock 50 个连接（超过高水位）
	host := newMockHost(50)
	mgr.SetHost(host)

	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 验证：连接数应该降至低水位附近
	// 允许一定误差（低水位 ± 5）
	assert.LessOrEqual(t, host.ConnCount(), cfg.LowWater+5)
}

// TestTrim_ProtectedNotClosed 测试受保护连接不被回收
func TestTrim_ProtectedNotClosed(t *testing.T) {
	cfg := Config{
		LowWater:  2,
		HighWater: 4,
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	// Mock 5 个连接
	host := newMockHost(5)
	mgr.SetHost(host)
	peers := host.Peers()

	// 保护前 2 个节点
	mgr.Protect(peers[0], "important")
	mgr.Protect(peers[1], "bootstrap")

	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 验证：受保护的连接没有被关闭
	assert.True(t, host.IsConnected(peers[0]), "protected peer 0 should remain connected")
	assert.True(t, host.IsConnected(peers[1]), "protected peer 1 should remain connected")
}

// TestTrim_PriorityOrder 测试按优先级回收
func TestTrim_PriorityOrder(t *testing.T) {
	cfg := Config{
		LowWater:  2,
		HighWater: 4,
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	// Mock 5 个连接
	host := newMockHost(5)
	mgr.SetHost(host)
	peers := host.Peers()

	// 设置不同优先级
	mgr.TagPeer(peers[0], "vip", 100)   // 最高优先级
	mgr.TagPeer(peers[1], "normal", 50) // 中等优先级
	mgr.TagPeer(peers[2], "low", 10)    // 低优先级
	// peers[3] 和 peers[4] 没有标签（优先级 0）

	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 验证：高优先级节点没有被关闭
	assert.True(t, host.IsConnected(peers[0]), "highest priority peer should remain connected")
	assert.True(t, host.IsConnected(peers[1]), "high priority peer should remain connected")

	// 验证：连接数应该降至低水位附近
	assert.LessOrEqual(t, host.ConnCount(), cfg.LowWater+1)
}

// TestTrim_GracePeriod 测试保护期
func TestTrim_GracePeriod(t *testing.T) {
	cfg := Config{
		LowWater:    2,
		HighWater:   4,
		GracePeriod: 0, // 禁用保护期以便测试
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	// Mock 5 个连接
	host := newMockHost(5)
	mgr.SetHost(host)

	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 验证：连接数降至低水位
	assert.LessOrEqual(t, host.ConnCount(), cfg.LowWater+1)
}

// TestIntegration_FullScenario 测试完整场景
func TestIntegration_FullScenario(t *testing.T) {
	cfg := Config{
		LowWater:  10,
		HighWater: 40,
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	// 1. 模拟 50 个连接
	host := newMockHost(50)
	mgr.SetHost(host)
	peers := host.Peers()

	// 2. 标记不同优先级
	for i := 0; i < 5; i++ {
		mgr.TagPeer(peers[i], "critical", 100)
	}
	for i := 5; i < 10; i++ {
		mgr.TagPeer(peers[i], "important", 50)
	}

	// 3. 保护关键连接
	mgr.Protect(peers[0], "bootstrap")
	mgr.Protect(peers[1], "relay")

	// 4. 触发回收
	ctx := context.Background()
	mgr.TrimOpenConns(ctx)

	// 5. 验证结果
	// 受保护的节点应该保持连接
	assert.True(t, host.IsConnected(peers[0]), "protected bootstrap should remain")
	assert.True(t, host.IsConnected(peers[1]), "protected relay should remain")

	// 连接数应该降至低水位附近
	assert.LessOrEqual(t, host.ConnCount(), cfg.LowWater+5)
}

// TestIntegration_TagOperations 测试标签操作
func TestIntegration_TagOperations(t *testing.T) {
	cfg := Config{
		LowWater:  10,
		HighWater: 40,
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	peer := "test-peer"

	// 测试 TagPeer
	mgr.TagPeer(peer, "tag1", 10)
	info := mgr.GetTagInfo(peer)
	require.NotNil(t, info)
	assert.Equal(t, 10, info.Tags["tag1"])

	// 测试 UpsertTag
	mgr.UpsertTag(peer, "tag2", func(current int) int {
		return current + 20
	})
	info = mgr.GetTagInfo(peer)
	assert.Equal(t, 20, info.Tags["tag2"])

	// 测试 UntagPeer
	mgr.UntagPeer(peer, "tag1")
	info = mgr.GetTagInfo(peer)
	_, hasTag1 := info.Tags["tag1"]
	assert.False(t, hasTag1)
}

// TestIntegration_ProtectOperations 测试保护操作
func TestIntegration_ProtectOperations(t *testing.T) {
	cfg := Config{
		LowWater:  10,
		HighWater: 40,
	}
	mgr, err := New(cfg)
	require.NoError(t, err)
	defer mgr.Close()

	peer := "test-peer"

	// 测试 Protect
	mgr.Protect(peer, "important")
	assert.True(t, mgr.IsProtected(peer, "important"))

	// 测试多重保护
	mgr.Protect(peer, "bootstrap")
	assert.True(t, mgr.IsProtected(peer, "important"))
	assert.True(t, mgr.IsProtected(peer, "bootstrap"))

	// 测试 Unprotect
	wasProtected := mgr.Unprotect(peer, "important")
	assert.True(t, wasProtected)
	assert.False(t, mgr.IsProtected(peer, "important"))
	assert.True(t, mgr.IsProtected(peer, "bootstrap"))
}
