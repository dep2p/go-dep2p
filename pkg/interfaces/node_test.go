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

// MockNode 模拟 Node 接口实现
type MockNode struct {
	id        string
	host      interfaces.Host
	discovery interfaces.Discovery
	started   bool
}

func NewMockNode(id string) *MockNode {
	return &MockNode{
		id:   id,
		host: NewMockHost(id),
	}
}

func (m *MockNode) ID() string {
	return m.id
}

func (m *MockNode) Host() interfaces.Host {
	return m.host
}

func (m *MockNode) Realm(id string) (interfaces.Realm, error) {
	return nil, nil
}

func (m *MockNode) Discovery() interfaces.Discovery {
	return m.discovery
}

func (m *MockNode) Metrics() interfaces.Metrics {
	return nil
}

func (m *MockNode) Start(ctx context.Context) error {
	m.started = true
	return nil
}

func (m *MockNode) Stop(ctx context.Context) error {
	m.started = false
	return nil
}

func (m *MockNode) Close() error {
	return nil
}

func (m *MockNode) NetworkChange() {
	// Mock 实现：不做任何操作
}

func (m *MockNode) OnNetworkChange(callback func(event interfaces.NetworkChangeEvent)) {
	// Mock 实现：不做任何操作
}

// Bootstrap 能力开关
func (m *MockNode) EnableBootstrap(ctx context.Context) error {
	return nil
}

func (m *MockNode) DisableBootstrap(ctx context.Context) error {
	return nil
}

func (m *MockNode) IsBootstrapEnabled() bool {
	return false
}

func (m *MockNode) BootstrapStats() interfaces.BootstrapStats {
	return interfaces.BootstrapStats{}
}

// Relay 能力开关（v2.0 统一接口）
func (m *MockNode) EnableRelay(ctx context.Context) error {
	return nil
}

func (m *MockNode) DisableRelay(ctx context.Context) error {
	return nil
}

func (m *MockNode) IsRelayEnabled() bool {
	return false
}

func (m *MockNode) SetRelayAddr(addr types.Multiaddr) error {
	return nil
}

func (m *MockNode) RemoveRelayAddr() error {
	return nil
}

func (m *MockNode) RelayAddr() (types.Multiaddr, bool) {
	return nil, false
}

func (m *MockNode) RelayStats() interfaces.RelayStats {
	return interfaces.RelayStats{}
}

// ReadyLevel API（Phase D 对齐）
func (m *MockNode) ReadyLevel() interfaces.ReadyLevel {
	return interfaces.ReadyLevelCreated
}

func (m *MockNode) WaitReady(ctx context.Context, level interfaces.ReadyLevel) error {
	return nil
}

func (m *MockNode) OnReadyLevelChange(callback func(level interfaces.ReadyLevel)) {
	// Mock 实现：不做任何操作
}

// ============================================================================
// 接口契约测试
// ============================================================================

// TestNodeInterface 验证 Node 接口存在
func TestNodeInterface(t *testing.T) {
	var _ interfaces.Node = (*MockNode)(nil)
}

// TestNode_ID 测试 ID 方法
func TestNode_ID(t *testing.T) {
	node := NewMockNode("test-node-id")

	if node.ID() != "test-node-id" {
		t.Errorf("ID() = %v, want test-node-id", node.ID())
	}
}

// TestNode_Host 测试 Host 方法
func TestNode_Host(t *testing.T) {
	node := NewMockNode("test-node")

	host := node.Host()
	if host == nil {
		t.Error("Host() returned nil")
	}
}

// TestNode_Start 测试 Start 方法
func TestNode_Start(t *testing.T) {
	node := NewMockNode("test-node")

	err := node.Start(context.Background())
	if err != nil {
		t.Errorf("Start() failed: %v", err)
	}

	if !node.started {
		t.Error("Start() did not set started flag")
	}
}

// TestNode_Stop 测试 Stop 方法
func TestNode_Stop(t *testing.T) {
	node := NewMockNode("test-node")
	node.Start(context.Background())

	err := node.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}

	if node.started {
		t.Error("Stop() did not clear started flag")
	}
}
