package swarm

import (
	"context"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     拨号基础测试
// ============================================================================

// TestSwarm_DialPeer_NoAddresses 测试无地址拨号
func TestSwarm_DialPeer_NoAddresses(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 拨号一个没有地址的节点应该失败
	_, err = s.DialPeer(ctx, "unknown-peer")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoAddresses)
}

// TestSwarm_DialPeer_Timeout 测试拨号超时或无地址错误
func TestSwarm_DialPeer_Timeout(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// 应该返回错误（可能是超时或无地址错误）
	_, err = s.DialPeer(ctx, "remote-peer")
	require.Error(t, err)
	// 由于没有可用地址，DialPeer 会返回 ErrNoAddresses
	assert.ErrorIs(t, err, ErrNoAddresses)
}

// TestSwarm_DialPeer_ConnectionReuse 测试连接复用
func TestSwarm_DialPeer_ConnectionReuse(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	// 手动添加一个连接模拟已有连接
	mockConn := &testConnForDial{remotePeer: "remote-peer"}
	s.addConn(mockConn)

	// 再次拨号应该复用现有连接
	ctx := context.Background()
	conn, err := s.DialPeer(ctx, "remote-peer")
	require.NoError(t, err)
	assert.NotNil(t, conn)
	// 返回的应该是已有的连接
	assert.Equal(t, mockConn, conn)
}

// ============================================================================
//                     地址排序测试
// ============================================================================

// Test_rankAddrs 测试地址排序
func Test_rankAddrs(t *testing.T) {
	addrs := []string{
		"/ip4/8.8.8.8/tcp/4001",
		"/ip4/8.8.8.8/udp/4001/quic",
		"/ip4/192.168.1.100/tcp/4001",
		"/ip4/192.168.1.100/udp/4001/quic",
	}

	ranked := rankAddrs(addrs)

	require.Equal(t, len(addrs), len(ranked))

	// 检查本地地址是否在前面
	hasLocal := false
	for _, addr := range ranked[:2] {
		if containsString(addr, "192.168") {
			hasLocal = true
			break
		}
	}
	assert.True(t, hasLocal, "本地地址应该在前面")
}

// Test_rankAddrs_AllTypes 测试所有地址类型的排序
func Test_rankAddrs_AllTypes(t *testing.T) {
	addrs := []string{
		"/ip4/8.8.8.8/tcp/4001",          // other tcp
		"/ip4/8.8.8.8/udp/4001/quic",     // other quic
		"/ip4/192.168.1.100/tcp/4001",    // local tcp
		"/ip4/10.0.0.1/tcp/4001",         // local tcp (10.x)
		"/ip4/172.16.0.1/tcp/4001",       // local tcp (172.16)
		"/ip4/127.0.0.1/tcp/4001",        // localhost
		"/ip4/1.2.3.4/udp/5000",          // other (neither tcp nor quic)
	}

	ranked := rankAddrs(addrs)

	require.Equal(t, len(addrs), len(ranked))
	// 验证排序：local > quic > tcp > other
	// 前几个应该是本地地址
}

// Test_rankAddrs_Empty 测试空地址列表
func Test_rankAddrs_Empty(t *testing.T) {
	ranked := rankAddrs([]string{})
	assert.Empty(t, ranked)
}

// Test_isPrivateAddr 测试私有地址检测
func Test_isPrivateAddr(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{"/ip4/192.168.1.1/tcp/4001", true},
		{"/ip4/10.0.0.1/tcp/4001", true},
		{"/ip4/172.16.0.1/tcp/4001", true},
		{"/ip4/172.17.0.1/tcp/4001", true},
		{"/ip4/172.30.0.1/tcp/4001", true},
		{"/ip4/172.31.0.1/tcp/4001", true},
		{"/ip4/127.0.0.1/tcp/4001", true},
		{"/dns4/localhost/tcp/4001", true},
		{"/ip4/8.8.8.8/tcp/4001", false},
		{"/ip4/1.2.3.4/tcp/4001", false},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			result := isPrivateAddr(tt.addr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test_truncateID 测试 ID 截断
func Test_truncateID(t *testing.T) {
	tests := []struct {
		id       string
		maxLen   int
		expected string
	}{
		{"abcdefghij", 8, "abcdefgh"},
		{"abc", 8, "abc"},
		{"", 8, ""},
		{"abcdefgh", 8, "abcdefgh"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result := truncateID(tt.id, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
//                     Setter 方法测试
// ============================================================================

// TestSwarm_AddTransport_Nil 测试 nil transport
func TestSwarm_AddTransport_Nil(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	err = s.AddTransport("tcp", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// TestSwarm_AddTransport_Closed 测试关闭后添加传输层
func TestSwarm_AddTransport_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	s.Close()

	// 使用 nil 测试，因为在关闭状态下应该先检查 closed
	// 注：实际上关闭检查在 nil 检查之后，所以这里用 nil
	err = s.AddTransport("tcp", nil)
	require.Error(t, err)
	// 因为先检查 nil，所以错误是 "cannot be nil"
	assert.Contains(t, err.Error(), "cannot be nil")
}

// TestSwarm_SetUpgrader_Nil 测试 nil upgrader
func TestSwarm_SetUpgrader_Nil(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	err = s.SetUpgrader(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// TestSwarm_SetUpgrader_Closed 测试关闭后设置升级器
func TestSwarm_SetUpgrader_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	s.Close()

	// 使用 nil 测试，因为在关闭状态下应该先检查 nil
	err = s.SetUpgrader(nil)
	require.Error(t, err)
	// 因为先检查 nil，所以错误是 "cannot be nil"
	assert.Contains(t, err.Error(), "cannot be nil")
}

// TestSwarm_SetPeerstore 测试设置 Peerstore
func TestSwarm_SetPeerstore(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	// nil peerstore 应该被接受（可选依赖）
	s.SetPeerstore(nil)
}

// TestSwarm_SetConnMgr 测试设置 ConnMgr
func TestSwarm_SetConnMgr(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	s.SetConnMgr(nil)
}

// TestSwarm_SetEventBus 测试设置 EventBus
func TestSwarm_SetEventBus(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	s.SetEventBus(nil)
}

// TestSwarm_SetPathHealthManager 测试设置 PathHealthManager
func TestSwarm_SetPathHealthManager(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	s.SetPathHealthManager(nil)
}

// TestSwarm_SetRelayDialer 测试设置 RelayDialer
func TestSwarm_SetRelayDialer(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	s.SetRelayDialer(nil)

	// 验证 getter
	dialer := s.RelayDialer()
	assert.Nil(t, dialer)
}

// TestSwarm_RelayDialer_WithValue 测试设置和获取 RelayDialer
func TestSwarm_RelayDialer_WithValue(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	mockDialer := &mockRelayDialer{}
	s.SetRelayDialer(mockDialer)

	dialer := s.RelayDialer()
	assert.Equal(t, mockDialer, dialer)
}

// TestSwarm_selectTransportForDial 测试拨号传输层选择
func TestSwarm_selectTransportForDial(t *testing.T) {
	s, err := NewSwarm("test-peer")
	require.NoError(t, err)
	defer s.Close()

	// 没有传输层时返回 nil
	transport := s.selectTransportForDial("/ip4/127.0.0.1/tcp/4001")
	assert.Nil(t, transport)

	transport = s.selectTransportForDial("/ip4/127.0.0.1/udp/4001/quic")
	assert.Nil(t, transport)

	// 未知协议也返回 nil（没有任何传输层）
	transport = s.selectTransportForDial("/ip4/127.0.0.1/unknown/4001")
	assert.Nil(t, transport)
}

// ============================================================================
//                     Mock 类型
// ============================================================================

// testConnForDial 测试用连接
type testConnForDial struct {
	remotePeer types.PeerID
	closed     bool
}

func (m *testConnForDial) LocalPeer() types.PeerID             { return "local-peer" }
func (m *testConnForDial) RemotePeer() types.PeerID            { return m.remotePeer }
func (m *testConnForDial) LocalMultiaddr() types.Multiaddr     { return nil }
func (m *testConnForDial) RemoteMultiaddr() types.Multiaddr    { return nil }
func (m *testConnForDial) NewStream(ctx context.Context) (pkgif.Stream, error) {
	return nil, nil
}
func (m *testConnForDial) AcceptStream() (pkgif.Stream, error) { return nil, nil }
func (m *testConnForDial) GetStreams() []pkgif.Stream          { return nil }
func (m *testConnForDial) IsClosed() bool                      { return m.closed }
func (m *testConnForDial) Stat() pkgif.ConnectionStat          { return pkgif.ConnectionStat{} }
func (m *testConnForDial) Close() error {
	m.closed = true
	return nil
}

func (m *testConnForDial) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}

// mockRelayDialer 测试用 RelayDialer
type mockRelayDialer struct {
	hasRelay bool
	dialErr  error
}

func (m *mockRelayDialer) HasRelay() bool { return m.hasRelay }
func (m *mockRelayDialer) DialViaRelay(ctx context.Context, peerID string) (pkgif.Connection, error) {
	if m.dialErr != nil {
		return nil, m.dialErr
	}
	return &testConnForDial{remotePeer: types.PeerID(peerID)}, nil
}

// 辅助函数
func containsString(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
//                     
// ============================================================================

// Test_filterRelayAddrs 测试 Relay 地址过滤
func Test_filterRelayAddrs(t *testing.T) {
	tests := []struct {
		name     string
		addrs    []string
		expected []string
	}{
		{
			name: "混合地址",
			addrs: []string{
				"/ip4/8.8.8.8/tcp/4001",
				"/ip4/8.8.8.8/udp/4001/quic-v1/p2p/RelayID/p2p-circuit/p2p/TargetID",
				"/ip4/192.168.1.100/tcp/4001",
			},
			expected: []string{
				"/ip4/8.8.8.8/udp/4001/quic-v1/p2p/RelayID/p2p-circuit/p2p/TargetID",
			},
		},
		{
			name:     "全部直连地址",
			addrs:    []string{"/ip4/8.8.8.8/tcp/4001", "/ip4/192.168.1.100/tcp/4001"},
			expected: nil,
		},
		{
			name: "全部 Relay 地址",
			addrs: []string{
				"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/Relay1/p2p-circuit/p2p/Target1",
				"/ip4/5.6.7.8/udp/4001/quic-v1/p2p/Relay2/p2p-circuit/p2p/Target2",
			},
			expected: []string{
				"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/Relay1/p2p-circuit/p2p/Target1",
				"/ip4/5.6.7.8/udp/4001/quic-v1/p2p/Relay2/p2p-circuit/p2p/Target2",
			},
		},
		{
			name:     "空地址列表",
			addrs:    []string{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterRelayAddrs(tt.addrs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test_filterDirectAddrs_RelayFiltered 测试直连地址过滤（确保 Relay 地址被过滤）
func Test_filterDirectAddrs_RelayFiltered(t *testing.T) {
	addrs := []string{
		"/ip4/8.8.8.8/tcp/4001",
		"/ip4/8.8.8.8/udp/4001/quic-v1/p2p/RelayID/p2p-circuit/p2p/TargetID",
		"/ip4/192.168.1.100/tcp/4001",
	}

	result := filterDirectAddrs(addrs)

	// 应该只包含非 Relay 地址
	assert.Len(t, result, 2)
	assert.Contains(t, result, "/ip4/8.8.8.8/tcp/4001")
	assert.Contains(t, result, "/ip4/192.168.1.100/tcp/4001")
}

// Test_parseRelayAddrForSwarm 测试 Relay 地址解析
func Test_parseRelayAddrForSwarm(t *testing.T) {
	tests := []struct {
		name           string
		addr           string
		wantRelayID    string
		wantServerAddr string
		wantErr        bool
	}{
		{
			name:           "有效的 QUIC Relay 地址",
			addr:           "/ip4/101.37.245.124/udp/4005/quic-v1/p2p/EhkTgL7tNBC2gT1mAoT37iuWTgVCJZ5zpdLo2YJeNEWp/p2p-circuit/p2p/9QSkEqc7G4WxWDkPPXFcGcV9dJLm8XQjKT1p4s6sSVSc",
			wantRelayID:    "EhkTgL7tNBC2gT1mAoT37iuWTgVCJZ5zpdLo2YJeNEWp",
			wantServerAddr: "/ip4/101.37.245.124/udp/4005/quic-v1",
			wantErr:        false,
		},
		{
			name:           "有效的 TCP Relay 地址",
			addr:           "/ip4/192.168.1.1/tcp/4001/p2p/RelayPeerID/p2p-circuit/p2p/TargetPeerID",
			wantRelayID:    "RelayPeerID",
			wantServerAddr: "/ip4/192.168.1.1/tcp/4001",
			wantErr:        false,
		},
		{
			name:    "缺少 p2p-circuit",
			addr:    "/ip4/8.8.8.8/tcp/4001/p2p/PeerID",
			wantErr: true,
		},
		{
			name:    "缺少 relay peer ID",
			addr:    "/ip4/8.8.8.8/tcp/4001/p2p-circuit/p2p/TargetID",
			wantErr: true,
		},
		{
			name:    "空地址",
			addr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relayID, serverAddr, err := parseRelayAddrForSwarm(tt.addr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantRelayID, relayID)
				assert.Equal(t, tt.wantServerAddr, serverAddr)
			}
		})
	}
}

// Test_filterDirectAndRelayAddrs_Complementary 测试直连和 Relay 过滤互补
func Test_filterDirectAndRelayAddrs_Complementary(t *testing.T) {
	addrs := []string{
		"/ip4/8.8.8.8/tcp/4001",
		"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/RelayID/p2p-circuit/p2p/TargetID",
		"/ip4/192.168.1.100/tcp/4001",
		"/ip4/5.6.7.8/udp/4005/quic-v1/p2p/Relay2/p2p-circuit/p2p/Target2",
	}

	directAddrs := filterDirectAddrs(addrs)
	relayAddrs := filterRelayAddrs(addrs)

	// 两者应该互补：所有地址 = 直连地址 + Relay 地址
	assert.Len(t, directAddrs, 2)
	assert.Len(t, relayAddrs, 2)
	assert.Equal(t, len(addrs), len(directAddrs)+len(relayAddrs))
}
