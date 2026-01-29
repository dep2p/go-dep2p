package interfaces_test

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// Mock 实现
// ============================================================================

// MockDiscovery 模拟 Discovery 接口实现
type MockDiscovery struct {
	peers []types.PeerInfo
}

func NewMockDiscovery() *MockDiscovery {
	return &MockDiscovery{
		peers: make([]types.PeerInfo, 0),
	}
}

func (m *MockDiscovery) FindPeers(ctx context.Context, ns string, opts ...interfaces.DiscoveryOption) (<-chan types.PeerInfo, error) {
	ch := make(chan types.PeerInfo, len(m.peers))
	for _, peer := range m.peers {
		ch <- peer
	}
	close(ch)
	return ch, nil
}

func (m *MockDiscovery) Advertise(ctx context.Context, ns string, opts ...interfaces.DiscoveryOption) (time.Duration, error) {
	return time.Hour, nil
}

func (m *MockDiscovery) Start(ctx context.Context) error {
	return nil
}

func (m *MockDiscovery) Stop(ctx context.Context) error {
	return nil
}

// ============================================================================
// 接口契约测试
// ============================================================================

// TestDiscoveryInterface 验证 Discovery 接口存在
func TestDiscoveryInterface(t *testing.T) {
	var _ interfaces.Discovery = (*MockDiscovery)(nil)
}

// TestDiscovery_FindPeers 测试 FindPeers 方法
func TestDiscovery_FindPeers(t *testing.T) {
	disco := NewMockDiscovery()

	peers, err := disco.FindPeers(context.Background(), "test-ns")
	if err != nil {
		t.Errorf("FindPeers() failed: %v", err)
	}

	if peers == nil {
		t.Error("FindPeers() returned nil")
	}
}

// TestDiscovery_Advertise 测试 Advertise 方法
func TestDiscovery_Advertise(t *testing.T) {
	disco := NewMockDiscovery()

	ttl, err := disco.Advertise(context.Background(), "test-ns")
	if err != nil {
		t.Errorf("Advertise() failed: %v", err)
	}

	if ttl == 0 {
		t.Error("Advertise() returned zero TTL")
	}
}
