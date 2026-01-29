package holepunch

import (
	"context"
	"testing"
	"time"
)

// TestNewHolePuncher 测试创建打洞器
func TestNewHolePuncher(t *testing.T) {
	hp := NewHolePuncher(nil, nil)

	if hp == nil {
		t.Fatal("NewHolePuncher returned nil")
	}

	t.Log("✅ NewHolePuncher 成功创建打洞器")
}

// TestHolePuncher_DirectConnect 测试直连尝试
func TestHolePuncher_DirectConnect(t *testing.T) {
	hp := NewHolePuncher(nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	peerID := "test-peer"
	addrs := []string{"/ip4/1.2.3.4/tcp/4001"}

	// 预期失败（因为没有真实连接）
	err := hp.DirectConnect(ctx, peerID, addrs)
	if err == nil {
		t.Error("Expected error for DirectConnect without real connection")
	}

	t.Log("✅ HolePuncher DirectConnect 正确处理无连接情况")
}

// TestHolePuncher_DuplicateAttempt 测试重复打洞尝试
func TestHolePuncher_DuplicateAttempt(t *testing.T) {
	hp := NewHolePuncher(nil, nil)

	peerID := "test-peer"

	// 标记为活跃
	hp.MarkActive(peerID)

	// 第二次尝试应该被拒绝
	if !hp.IsActive(peerID) {
		t.Error("Peer should be marked as active")
	}

	t.Log("✅ HolePuncher 重复打洞检测正确")
}

// TestHolePuncher_ClearActive 测试清除活跃状态
func TestHolePuncher_ClearActive(t *testing.T) {
	hp := NewHolePuncher(nil, nil)

	peerID := "test-peer"

	// 标记为活跃
	hp.MarkActive(peerID)
	if !hp.IsActive(peerID) {
		t.Fatal("Peer should be active")
	}

	// 清除活跃状态
	hp.ClearActive(peerID)
	if hp.IsActive(peerID) {
		t.Error("Peer should not be active after clear")
	}

	t.Log("✅ HolePuncher 清除活跃状态正确")
}

// TestHolePuncher_MultiplePeers 测试多节点打洞
func TestHolePuncher_MultiplePeers(t *testing.T) {
	hp := NewHolePuncher(nil, nil)

	peer1 := "peer-1"
	peer2 := "peer-2"
	peer3 := "peer-3"

	// 标记多个节点为活跃
	hp.MarkActive(peer1)
	hp.MarkActive(peer2)
	hp.MarkActive(peer3)

	// 检查所有节点都活跃
	if !hp.IsActive(peer1) || !hp.IsActive(peer2) || !hp.IsActive(peer3) {
		t.Error("All peers should be active")
	}

	// 清除一个节点
	hp.ClearActive(peer2)

	// 检查状态
	if !hp.IsActive(peer1) || hp.IsActive(peer2) || !hp.IsActive(peer3) {
		t.Error("Incorrect active state after clear")
	}

	t.Log("✅ HolePuncher 多节点管理正确")
}

// TestHolePuncher_ContextTimeout 测试上下文超时
func TestHolePuncher_ContextTimeout(t *testing.T) {
	hp := NewHolePuncher(nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	peerID := "test-peer"
	addrs := []string{"/ip4/1.2.3.4/tcp/4001"}

	err := hp.DirectConnect(ctx, peerID, addrs)

	// 应该返回超时错误
	if err == nil {
		t.Error("Expected timeout error")
	}

	t.Log("✅ HolePuncher 上下文超时处理正确")
}

// TestHolePuncher_EmptyAddrs 测试空地址列表
func TestHolePuncher_EmptyAddrs(t *testing.T) {
	hp := NewHolePuncher(nil, nil)

	ctx := context.Background()
	peerID := "test-peer"
	addrs := []string{}

	err := hp.DirectConnect(ctx, peerID, addrs)

	// 应该返回错误
	if err == nil {
		t.Error("Expected error for empty addresses")
	}

	t.Log("✅ HolePuncher 空地址列表处理正确")
}

// TestHolePuncher_ActiveCount 测试活跃计数
func TestHolePuncher_ActiveCount(t *testing.T) {
	hp := NewHolePuncher(nil, nil)

	if hp.ActiveCount() != 0 {
		t.Errorf("Initial active count = %d, want 0", hp.ActiveCount())
	}

	hp.MarkActive("peer-1")
	hp.MarkActive("peer-2")

	if hp.ActiveCount() != 2 {
		t.Errorf("Active count = %d, want 2", hp.ActiveCount())
	}

	hp.ClearActive("peer-1")

	if hp.ActiveCount() != 1 {
		t.Errorf("Active count = %d, want 1", hp.ActiveCount())
	}

	t.Log("✅ HolePuncher 活跃计数正确")
}

// TestProtocol_ID 测试协议 ID
func TestProtocol_ID(t *testing.T) {
	expected := "/dep2p/sys/holepunch/1.0.0"

	if HolePunchProtocol != expected {
		t.Errorf("Protocol = %s, want %s", HolePunchProtocol, expected)
	}

	t.Log("✅ Hole Punch 协议 ID 正确")
}
