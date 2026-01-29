package natpmp

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
//                     NATPMPError 测试
// ============================================================================

func TestNATPMPError_Error(t *testing.T) {
	// 无 Cause
	err := &NATPMPError{Message: "test error"}
	assert.Equal(t, "natpmp: test error", err.Error())

	// 有 Cause
	cause := errors.New("underlying error")
	err = &NATPMPError{Message: "test error", Cause: cause}
	assert.Equal(t, "natpmp: test error: underlying error", err.Error())
}

func TestNATPMPError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &NATPMPError{Message: "test error", Cause: cause}

	assert.Equal(t, cause, err.Unwrap())
	assert.True(t, errors.Is(err, cause))
}

func TestNATPMPError_Unwrap_Nil(t *testing.T) {
	err := &NATPMPError{Message: "test error"}
	assert.Nil(t, err.Unwrap())
}

// ============================================================================
//                     MappingError 测试
// ============================================================================

func TestMappingError_Error(t *testing.T) {
	cause := errors.New("connection refused")
	err := &MappingError{
		Protocol: "UDP",
		Port:     4001,
		Cause:    cause,
	}

	expected := "natpmp: mapping UDP port 4001 failed: connection refused"
	assert.Equal(t, expected, err.Error())
}

func TestMappingError_Unwrap(t *testing.T) {
	cause := errors.New("connection refused")
	err := &MappingError{
		Protocol: "TCP",
		Port:     4001,
		Cause:    cause,
	}

	assert.Equal(t, cause, err.Unwrap())
	assert.True(t, errors.Is(err, cause))
}

// ============================================================================
//                     SetReachabilityCoordinator 测试
// ============================================================================

func TestNATPMPMapper_SetReachabilityCoordinator(t *testing.T) {
	mapper := &NATPMPMapper{
		gateway:  net.ParseIP("192.168.1.1"),
		mappings: make(map[int]*Mapping),
	}

	// 初始为 nil
	assert.Nil(t, mapper.coordinator)

	// 设置 coordinator
	mockCoord := &mockCoordinator{}
	mapper.SetReachabilityCoordinator(mockCoord)

	assert.Equal(t, mockCoord, mapper.coordinator)
}

// ============================================================================
//                     Start/Stop 测试
// ============================================================================

func TestNATPMPMapper_StartStop(t *testing.T) {
	mapper := &NATPMPMapper{
		gateway:  net.ParseIP("192.168.1.1"),
		mappings: make(map[int]*Mapping),
	}

	// 启动
	mapper.Start(context.Background())
	assert.NotNil(t, mapper.ctx)
	assert.NotNil(t, mapper.cancel)

	// 停止
	mapper.Stop()
	// 验证 context 被取消
	select {
	case <-mapper.ctx.Done():
		// 成功
	default:
		t.Error("Context should be cancelled after Stop")
	}
}

func TestNATPMPMapper_Stop_NilCancel(t *testing.T) {
	mapper := &NATPMPMapper{
		gateway:  net.ParseIP("192.168.1.1"),
		mappings: make(map[int]*Mapping),
	}

	// Stop 在未 Start 时不应 panic
	mapper.Stop()
}

// ============================================================================
//                     renewLoop 测试
// ============================================================================

func TestNATPMPMapper_RenewLoop_Cancel(t *testing.T) {
	mapper := &NATPMPMapper{
		gateway:  net.ParseIP("192.168.1.1"),
		mappings: make(map[int]*Mapping),
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		mapper.renewLoop(ctx)
		close(done)
	}()

	// 立即取消
	cancel()

	// 验证循环退出
	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Error("renewLoop did not exit after context cancel")
	}
}

// ============================================================================
//                     reportMappedAddressToCoordinator 测试
// ============================================================================

func TestNATPMPMapper_ReportMappedAddress_NoCoordinator(t *testing.T) {
	mapper := &NATPMPMapper{
		gateway:  net.ParseIP("192.168.1.1"),
		mappings: make(map[int]*Mapping),
	}

	// 没有 coordinator 时不应 panic
	mapper.reportMappedAddressToCoordinator("UDP", 4001)
}

// ============================================================================
//                     ErrNoGateway 测试
// ============================================================================

func TestErrNoGateway(t *testing.T) {
	assert.NotNil(t, ErrNoGateway)
	assert.Equal(t, "natpmp: no gateway found", ErrNoGateway.Error())
}

// ============================================================================
//                     Mock 实现
// ============================================================================

// mockCoordinator 是 ReachabilityCoordinator 的 mock
type mockCoordinator struct {
	lastAddr     string
	lastSource   string
	lastPriority int
}

func (m *mockCoordinator) AdvertisedAddrs() []string                 { return nil }
func (m *mockCoordinator) VerifiedDirectAddresses() []string         { return nil }
func (m *mockCoordinator) CandidateDirectAddresses() []string        { return nil }
func (m *mockCoordinator) RelayAddresses() []string                  { return nil }
func (m *mockCoordinator) BootstrapCandidates(nodeID string) []pkgif.BootstrapCandidate {
	return nil
}
func (m *mockCoordinator) SetOnAddressChanged(callback func([]string)) {}
func (m *mockCoordinator) OnDirectAddressCandidate(addr string, source string, priority pkgif.AddressPriority) {
	m.lastAddr = addr
	m.lastSource = source
	m.lastPriority = int(priority)
}
func (m *mockCoordinator) UpdateDirectCandidates(source string, candidates []pkgif.CandidateUpdate) {
}
func (m *mockCoordinator) OnDirectAddressVerified(addr string, source string, priority pkgif.AddressPriority) {
}
func (m *mockCoordinator) OnDirectAddressExpired(addr string)  {}
func (m *mockCoordinator) OnRelayReserved(addrs []string)      {}
func (m *mockCoordinator) OnInboundWitness(dialedAddr string, remotePeerID string, remoteIP string) {
}
func (m *mockCoordinator) HasRelayAddress() bool               { return false }
func (m *mockCoordinator) HasVerifiedDirectAddress() bool      { return false }
func (m *mockCoordinator) Start(ctx context.Context) error     { return nil }
func (m *mockCoordinator) Stop() error                         { return nil }
