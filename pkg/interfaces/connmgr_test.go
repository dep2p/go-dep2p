package interfaces_test

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// Mock 实现
// ============================================================================

// MockConnMgr 模拟 ConnManager 接口实现
type MockConnMgr struct {
	conns      map[string]bool
	tags       map[string]map[string]int
	protected  map[string]map[string]bool
}

func NewMockConnMgr() *MockConnMgr {
	return &MockConnMgr{
		conns:     make(map[string]bool),
		tags:      make(map[string]map[string]int),
		protected: make(map[string]map[string]bool),
	}
}

func (m *MockConnMgr) TagPeer(peerID string, tag string, weight int) {
	if m.tags[peerID] == nil {
		m.tags[peerID] = make(map[string]int)
	}
	m.tags[peerID][tag] = weight
}

func (m *MockConnMgr) UntagPeer(peerID string, tag string) {
	if m.tags[peerID] != nil {
		delete(m.tags[peerID], tag)
	}
}

func (m *MockConnMgr) UpsertTag(peerID string, tag string, upsert func(int) int) {
	// Mock implementation
}

func (m *MockConnMgr) GetTagInfo(peerID string) *interfaces.TagInfo {
	return nil
}

func (m *MockConnMgr) TrimOpenConns(ctx context.Context) {
	// Mock implementation
}

func (m *MockConnMgr) Notifee() interfaces.SwarmNotifier {
	return nil
}

func (m *MockConnMgr) Protect(peerID string, tag string) {
	if m.protected[peerID] == nil {
		m.protected[peerID] = make(map[string]bool)
	}
	m.protected[peerID][tag] = true
}

func (m *MockConnMgr) Unprotect(peerID string, tag string) bool {
	if m.protected[peerID] != nil {
		delete(m.protected[peerID], tag)
		return true
	}
	return false
}

func (m *MockConnMgr) IsProtected(peerID string, tag string) bool {
	if m.protected[peerID] != nil {
		return m.protected[peerID][tag]
	}
	return false
}

func (m *MockConnMgr) ConnCount() int {
	return len(m.conns)
}

func (m *MockConnMgr) DialedConnCount() int {
	return 0
}

func (m *MockConnMgr) InboundConnCount() int {
	return 0
}

func (m *MockConnMgr) SetLimits(low, high int) {
	// Mock implementation
}

func (m *MockConnMgr) GetLimits() (low, high int) {
	return 0, 0
}

func (m *MockConnMgr) TriggerTrim() {
	// Mock implementation
}

func (m *MockConnMgr) Close() error {
	return nil
}

// msgrate 集成方法
func (m *MockConnMgr) UpdatePeerRate(peerID string, kind uint64, elapsed time.Duration, items int) {
	// Mock implementation
}

func (m *MockConnMgr) GetPeerCapacity(peerID string, kind uint64, targetRTT time.Duration) int {
	return 100 // Mock: 返回默认容量
}

func (m *MockConnMgr) GetTargetRTT() time.Duration {
	return 500 * time.Millisecond // Mock: 返回默认 RTT
}

// ============================================================================
// 接口契约测试
// ============================================================================

// TestConnMgrInterface 验证 ConnManager 接口存在
func TestConnMgrInterface(t *testing.T) {
	var _ interfaces.ConnManager = (*MockConnMgr)(nil)
}

// TestConnMgr_Protect 测试 Protect 方法
func TestConnMgr_Protect(t *testing.T) {
	cm := NewMockConnMgr()
	peer := "test-peer"

	cm.Protect(peer, "test-tag")

	if !cm.IsProtected(peer, "test-tag") {
		t.Error("Protect() did not protect connection")
	}
}

// TestConnMgr_Unprotect 测试 Unprotect 方法
func TestConnMgr_Unprotect(t *testing.T) {
	cm := NewMockConnMgr()
	peer := "test-peer"

	cm.Protect(peer, "test-tag")
	result := cm.Unprotect(peer, "test-tag")

	if !result {
		t.Error("Unprotect() should return true")
	}

	if cm.IsProtected(peer, "test-tag") {
		t.Error("Unprotect() did not unprotect connection")
	}
}

// TestConnMgr_TagPeer 测试 TagPeer 方法
func TestConnMgr_TagPeer(t *testing.T) {
	cm := NewMockConnMgr()
	peer := "test-peer"

	cm.TagPeer(peer, "important", 100)

	// 验证标签已设置（通过 tags map）
	if cm.tags[peer] == nil {
		t.Error("TagPeer() did not create tags map")
	}

	if cm.tags[peer]["important"] != 100 {
		t.Errorf("TagPeer() weight = %d, want 100", cm.tags[peer]["important"])
	}
}
