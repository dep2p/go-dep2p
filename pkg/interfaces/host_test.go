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

// MockHost 模拟 Host 接口实现
type MockHost struct {
	id      string
	addrs   []string
	closed  bool
	streams map[string]interfaces.Stream
}

func NewMockHost(id string) *MockHost {
	return &MockHost{
		id:      id,
		addrs:   []string{"/ip4/127.0.0.1/tcp/4001"},
		streams: make(map[string]interfaces.Stream),
	}
}

func (m *MockHost) ID() string {
	return m.id
}

func (m *MockHost) Addrs() []string {
	return m.addrs
}

func (m *MockHost) Listen(addrs ...string) error {
	return nil
}

func (m *MockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
	return nil
}

func (m *MockHost) SetStreamHandler(protocolID string, handler interfaces.StreamHandler) {
	// Mock implementation
}

func (m *MockHost) RemoveStreamHandler(protocolID string) {
	// Mock implementation
}

func (m *MockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (interfaces.Stream, error) {
	return NewMockStream(), nil
}

func (m *MockHost) Peerstore() interfaces.Peerstore {
	return nil
}

func (m *MockHost) EventBus() interfaces.EventBus {
	return nil
}

func (m *MockHost) Close() error {
	m.closed = true
	return nil
}

func (m *MockHost) AdvertisedAddrs() []string {
	return m.Addrs()
}

func (m *MockHost) ShareableAddrs() []string {
	return nil
}

func (m *MockHost) HolePunchAddrs() []string {
	return nil
}

func (m *MockHost) SetReachabilityCoordinator(coordinator interfaces.ReachabilityCoordinator) {
	// no-op for mock
}

func (m *MockHost) Network() interfaces.Swarm {
	return nil
}

func (m *MockHost) HandleInboundStream(stream interfaces.Stream) {
	// Mock implementation: no-op
}

// MockStream 模拟 Stream 接口实现
type MockStream struct {
	data     []byte
	closed   bool
	protocol string
}

func NewMockStream() *MockStream {
	return &MockStream{
		data:     make([]byte, 0),
		protocol: "/test/1.0.0",
	}
}

func (m *MockStream) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (m *MockStream) Write(p []byte) (n int, err error) {
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *MockStream) Close() error {
	m.closed = true
	return nil
}

func (m *MockStream) Reset() error {
	m.closed = true
	return nil
}

func (m *MockStream) Protocol() string {
	return m.protocol
}

func (m *MockStream) SetProtocol(protocol string) {
	m.protocol = protocol
}

func (m *MockStream) Conn() interfaces.Connection {
	return nil
}

func (m *MockStream) IsClosed() bool {
	return m.closed
}

func (m *MockStream) CloseWrite() error {
	return nil
}

func (m *MockStream) CloseRead() error {
	return nil
}

func (m *MockStream) SetDeadline(t time.Time) error {
	return nil
}

func (m *MockStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *MockStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *MockStream) Stat() types.StreamStat {
	return types.StreamStat{
		Direction:    types.DirUnknown,
		Opened:       time.Now(),
		Protocol:     types.ProtocolID(m.protocol),
		BytesRead:    0,
		BytesWritten: 0,
	}
}

func (m *MockStream) State() types.StreamState {
	if m.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// ============================================================================
// 接口契约测试
// ============================================================================

// TestHostInterface 验证 Host 接口存在
func TestHostInterface(t *testing.T) {
	var _ interfaces.Host = (*MockHost)(nil)
}

// TestHost_ID 测试 ID 方法
func TestHost_ID(t *testing.T) {
	host := NewMockHost("test-host-id")

	if host.ID() != "test-host-id" {
		t.Errorf("ID() = %v, want test-host-id", host.ID())
	}
}

// TestHost_Addrs 测试 Addrs 方法
func TestHost_Addrs(t *testing.T) {
	host := NewMockHost("test-host")

	addrs := host.Addrs()
	if len(addrs) == 0 {
		t.Error("Addrs() returned empty list")
	}
}

// TestHost_Close 测试 Close 方法
func TestHost_Close(t *testing.T) {
	host := NewMockHost("test-host")

	err := host.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	if !host.closed {
		t.Error("Close() did not set closed flag")
	}
}

// TestStreamInterface 验证 Stream 接口存在
func TestStreamInterface(t *testing.T) {
	var _ interfaces.Stream = (*MockStream)(nil)
}

// TestStream_Write 测试 Write 方法
func TestStream_Write(t *testing.T) {
	stream := NewMockStream()

	data := []byte("hello")
	n, err := stream.Write(data)
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}

	if n != len(data) {
		t.Errorf("Write() = %d, want %d", n, len(data))
	}
}

// TestStream_Close 测试 Close 方法
func TestStream_Close(t *testing.T) {
	stream := NewMockStream()

	err := stream.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	if !stream.closed {
		t.Error("Close() did not set closed flag")
	}
}
