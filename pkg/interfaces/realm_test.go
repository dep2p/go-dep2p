package interfaces_test

import (
	"context"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
// Mock 实现
// ============================================================================

// MockRealm 模拟 Realm 接口实现
type MockRealm struct {
	id      string
	name    string
	members []string
}

func NewMockRealm(id string, name string) *MockRealm {
	return &MockRealm{
		id:      id,
		name:    name,
		members: make([]string, 0),
	}
}

func (m *MockRealm) ID() string {
	return m.id
}

func (m *MockRealm) Name() string {
	return m.name
}

func (m *MockRealm) Join(ctx context.Context) error {
	return nil
}

func (m *MockRealm) Leave(ctx context.Context) error {
	return nil
}

func (m *MockRealm) Members() []string {
	return m.members
}

func (m *MockRealm) IsMember(peerID string) bool {
	for _, member := range m.members {
		if member == peerID {
			return true
		}
	}
	return false
}

func (m *MockRealm) Messaging() interfaces.Messaging {
	return nil
}

func (m *MockRealm) PubSub() interfaces.PubSub {
	return nil
}

func (m *MockRealm) Streams() interfaces.Streams {
	return nil
}

func (m *MockRealm) Liveness() interfaces.Liveness {
	return nil
}

func (m *MockRealm) PSK() []byte {
	return nil
}

func (m *MockRealm) Authenticate(ctx context.Context, peerID string, proof []byte) (bool, error) {
	return true, nil
}

func (m *MockRealm) GenerateProof(ctx context.Context) ([]byte, error) {
	return []byte("mock-proof"), nil
}

func (m *MockRealm) Close() error {
	return nil
}

func (m *MockRealm) EventBus() interfaces.EventBus {
	return nil
}

func (m *MockRealm) Connect(ctx context.Context, target string) (interfaces.Connection, error) {
	return nil, nil
}

func (m *MockRealm) ConnectWithHint(ctx context.Context, target string, hints []string) (interfaces.Connection, error) {
	return nil, nil
}

// addMember 辅助方法（测试用）
func (m *MockRealm) addMember(peerID string) {
	m.members = append(m.members, peerID)
}

// removeMember 辅助方法（测试用）
func (m *MockRealm) removeMember(peerID string) {
	for i, member := range m.members {
		if member == peerID {
			m.members = append(m.members[:i], m.members[i+1:]...)
			return
		}
	}
}

// ============================================================================
// 接口契约测试
// ============================================================================

// TestRealmInterface 验证 Realm 接口存在
func TestRealmInterface(t *testing.T) {
	var _ interfaces.Realm = (*MockRealm)(nil)
}

// TestRealm_ID 测试 ID 方法
func TestRealm_ID(t *testing.T) {
	realm := NewMockRealm("test-realm-id", "Test Realm")

	if realm.ID() != "test-realm-id" {
		t.Errorf("ID() = %v, want test-realm-id", realm.ID())
	}
}

// TestRealm_Name 测试 Name 方法
func TestRealm_Name(t *testing.T) {
	realm := NewMockRealm("id", "Test Realm")

	if realm.Name() != "Test Realm" {
		t.Errorf("Name() = %v, want Test Realm", realm.Name())
	}
}

// TestRealm_Members 测试 Members 方法
func TestRealm_Members(t *testing.T) {
	realm := NewMockRealm("test-realm", "Test")

	members := realm.Members()
	if members == nil {
		t.Error("Members() returned nil")
	}
}

// TestRealm_IsMember 测试 IsMember 方法
func TestRealm_IsMember(t *testing.T) {
	realm := NewMockRealm("test-realm", "Test")
	peer := "test-peer"

	// 初始不是成员
	if realm.IsMember(peer) {
		t.Error("IsMember() should return false initially")
	}

	// 加入后是成员
	realm.addMember(peer)
	if !realm.IsMember(peer) {
		t.Error("IsMember() should return true after addMember")
	}
}

// TestRealm_Join 测试 Join 方法
func TestRealm_Join(t *testing.T) {
	realm := NewMockRealm("test-realm", "Test")

	err := realm.Join(context.Background())
	if err != nil {
		t.Errorf("Join() failed: %v", err)
	}
}

// TestRealm_Leave 测试 Leave 方法
func TestRealm_Leave(t *testing.T) {
	realm := NewMockRealm("test-realm", "Test")

	err := realm.Leave(context.Background())
	if err != nil {
		t.Errorf("Leave() failed: %v", err)
	}
}

// TestRealm_Close 测试 Close 方法
func TestRealm_Close(t *testing.T) {
	realm := NewMockRealm("test-realm", "Test")

	err := realm.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}
