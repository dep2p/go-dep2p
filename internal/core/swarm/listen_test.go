package swarm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     Mock 类型
// ============================================================================

// mockListener 模拟监听器（线程安全）
type mockListener struct {
	addr      types.Multiaddr
	acceptErr error
	closed    int32 // 使用 atomic
	acceptCh  chan pkgif.Connection
	closeCh   chan struct{} // 用于通知关闭
}

func newMockListener(addr types.Multiaddr) *mockListener {
	return &mockListener{
		addr:    addr,
		closeCh: make(chan struct{}),
	}
}

func (m *mockListener) Accept() (pkgif.Connection, error) {
	if m.acceptErr != nil {
		return nil, m.acceptErr
	}
	// 检查是否已关闭
	select {
	case <-m.closeCh:
		return nil, errors.New("listener closed")
	default:
	}
	if m.acceptCh != nil {
		select {
		case conn := <-m.acceptCh:
			if conn == nil {
				return nil, errors.New("listener closed")
			}
			return conn, nil
		case <-m.closeCh:
			return nil, errors.New("listener closed")
		}
	}
	// 默认等待关闭
	<-m.closeCh
	return nil, errors.New("listener closed")
}

func (m *mockListener) Addr() types.Multiaddr {
	return m.addr
}

func (m *mockListener) Multiaddr() types.Multiaddr {
	return m.addr
}

func (m *mockListener) Close() error {
	if atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		close(m.closeCh)
	}
	return nil
}

// mockTransport 模拟传输层
type mockTransport struct {
	listenErr error
	dialErr   error
	listener  pkgif.Listener
}

func (m *mockTransport) Dial(ctx context.Context, raddr types.Multiaddr, peer types.PeerID) (pkgif.Connection, error) {
	if m.dialErr != nil {
		return nil, m.dialErr
	}
	return nil, nil
}

func (m *mockTransport) Listen(laddr types.Multiaddr) (pkgif.Listener, error) {
	if m.listenErr != nil {
		return nil, m.listenErr
	}
	if m.listener != nil {
		return m.listener, nil
	}
	return newMockListener(laddr), nil
}

func (m *mockTransport) Protocols() []int {
	return []int{6} // TCP protocol number
}

func (m *mockTransport) CanDial(addr types.Multiaddr) bool {
	return true
}

func (m *mockTransport) Close() error {
	return nil
}

// ============================================================================
//                     监听测试
// ============================================================================

// TestSwarm_Listen_NoAddresses 测试空地址监听
func TestSwarm_Listen_NoAddresses(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	err = s.Listen()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no addresses")
}

// TestSwarm_Listen_WithTransport 测试有传输层时的监听
func TestSwarm_Listen_WithTransport(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	// 添加 mock 传输层
	transport := &mockTransport{}
	err = s.AddTransport("tcp", transport)
	require.NoError(t, err)

	// 监听应该成功
	err = s.Listen("/ip4/127.0.0.1/tcp/0")
	require.NoError(t, err)

	// 验证监听地址
	addrs := s.ListenAddrs()
	assert.NotEmpty(t, addrs)
}

// TestSwarm_Listen_MultipleAddrs_PartialSuccess 测试部分监听成功
func TestSwarm_Listen_MultipleAddrs_PartialSuccess(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	// 添加 TCP 传输层
	transport := &mockTransport{}
	err = s.AddTransport("tcp", transport)
	require.NoError(t, err)

	// 同时监听 TCP 和 QUIC，但只有 TCP 有传输层
	// 至少一个成功就应该返回 nil
	err = s.Listen("/ip4/127.0.0.1/tcp/0", "/ip4/127.0.0.1/udp/0/quic")
	require.NoError(t, err) // 至少 TCP 成功
}

// TestSwarm_Listen_AllFail 测试所有监听失败
func TestSwarm_Listen_AllFail(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	// 添加会失败的传输层
	transport := &mockTransport{listenErr: errors.New("listen failed")}
	err = s.AddTransport("tcp", transport)
	require.NoError(t, err)

	// 所有监听都失败
	err = s.Listen("/ip4/127.0.0.1/tcp/0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to listen")
}

// TestSwarm_Listen_NoTransport 测试无传输层监听
func TestSwarm_Listen_NoTransport(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()
	
	err = s.Listen("/ip4/127.0.0.1/tcp/0")
	if err == nil {
		t.Error("Listen() should fail without transport")
	}
	
	t.Log("✅ 无传输层监听正确失败")
}

// TestSwarm_Listen_Closed 测试关闭后监听
func TestSwarm_Listen_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()
	
	err = s.Listen("/ip4/127.0.0.1/tcp/0")
	if err == nil {
		t.Error("Listen() should fail after close")
	}
	
	t.Log("✅ 关闭后监听正确失败")
}

// TestSwarm_ListenAddrs_Empty 测试空监听地址
func TestSwarm_ListenAddrs_Empty(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()
	
	addrs := s.ListenAddrs()
	if len(addrs) != 0 {
		t.Errorf("ListenAddrs() = %d, want 0", len(addrs))
	}
	
	t.Log("✅ 空监听地址正确返回")
}

// TestSwarm_ListenAddrs_Closed 测试关闭后获取监听地址
func TestSwarm_ListenAddrs_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()
	
	addrs := s.ListenAddrs()
	if addrs != nil {
		t.Errorf("ListenAddrs() should return nil after close")
	}
	
	t.Log("✅ 关闭后获取监听地址返回 nil")
}

// TestSwarm_selectTransportForListen_QUIC 测试 QUIC 传输层选择
func TestSwarm_selectTransportForListen_QUIC(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()
	
	// 没有传输层时返回 nil
	transport := s.selectTransportForListen("/ip4/127.0.0.1/udp/0/quic")
	if transport != nil {
		t.Error("selectTransportForListen should return nil without quic transport")
	}
	
	t.Log("✅ QUIC 传输层选择正确")
}

// TestSwarm_selectTransportForListen_TCP 测试 TCP 传输层选择
func TestSwarm_selectTransportForListen_TCP(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()
	
	// 没有传输层时返回 nil
	transport := s.selectTransportForListen("/ip4/127.0.0.1/tcp/0")
	if transport != nil {
		t.Error("selectTransportForListen should return nil without tcp transport")
	}
	
	t.Log("✅ TCP 传输层选择正确")
}

// TestSwarm_selectTransportForListen_Default 测试默认传输层选择
func TestSwarm_selectTransportForListen_Default(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()
	
	// 没有传输层时返回 nil
	transport := s.selectTransportForListen("/ip4/127.0.0.1/unknown/0")
	if transport != nil {
		t.Error("selectTransportForListen should return nil without transport")
	}
	
	t.Log("✅ 默认传输层选择正确")
}

// TestSwarm_Listen_InvalidAddr 测试无效地址监听
func TestSwarm_Listen_InvalidAddr(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()
	
	// 无效地址应该失败
	err = s.Listen("invalid-address")
	if err == nil {
		t.Error("Listen() should fail for invalid address")
	}
	
	t.Log("✅ 无效地址监听正确失败")
}
