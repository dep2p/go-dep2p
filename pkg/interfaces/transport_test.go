package interfaces_test

import (
	"context"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// Mock 实现
// ============================================================================

// MockTransport 模拟 Transport 接口实现
type MockTransport struct {
	protocol string
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		protocol: "/quic-v1",
	}
}

func (m *MockTransport) Dial(ctx context.Context, raddr types.Multiaddr, peerID types.PeerID) (interfaces.Connection, error) {
	return nil, nil
}

func (m *MockTransport) CanDial(addr types.Multiaddr) bool {
	return true
}

func (m *MockTransport) Listen(laddr types.Multiaddr) (interfaces.Listener, error) {
	return nil, nil
}

func (m *MockTransport) Protocols() []int {
	return []int{273} // QUIC protocol number
}

func (m *MockTransport) Close() error {
	return nil
}

// ============================================================================
// 接口契约测试
// ============================================================================

// TestTransportInterface 验证 Transport 接口存在
func TestTransportInterface(t *testing.T) {
	var _ interfaces.Transport = (*MockTransport)(nil)
}

// TestTransport_CanDial 测试 CanDial 方法
func TestTransport_CanDial(t *testing.T) {
	transport := NewMockTransport()

	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	if !transport.CanDial(addr) {
		t.Error("CanDial() should return true for valid address")
	}
}

// TestTransport_Protocols 测试 Protocols 方法
func TestTransport_Protocols(t *testing.T) {
	transport := NewMockTransport()

	protocols := transport.Protocols()
	if len(protocols) == 0 {
		t.Error("Protocols() returned empty list")
	}
}
