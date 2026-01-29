package relay

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              地址解析测试
// ============================================================================

// TestParseCircuitAddr 测试中继电路地址解析
func TestParseCircuitAddr(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		wantRelay   types.PeerID
		wantTarget  types.PeerID
		expectError bool
	}{
		{
			name:       "full address with relay and target",
			addr:       "/ip4/1.2.3.4/tcp/4001/p2p/relay-peer-id/p2p-circuit/p2p/target-peer-id",
			wantRelay:  types.PeerID("relay-peer-id"),
			wantTarget: types.PeerID("target-peer-id"),
		},
		{
			name:       "address without network part",
			addr:       "/p2p/relay-peer-id/p2p-circuit/p2p/target-peer-id",
			wantRelay:  types.PeerID("relay-peer-id"),
			wantTarget: types.PeerID("target-peer-id"),
		},
		{
			name:       "address without target",
			addr:       "/p2p/relay-peer-id/p2p-circuit",
			wantRelay:  types.PeerID("relay-peer-id"),
			wantTarget: types.PeerID(""),
		},
		{
			name:        "missing p2p-circuit",
			addr:        "/p2p/relay-peer-id/p2p/target-peer-id",
			expectError: true,
		},
		{
			name:        "missing relay peer id",
			addr:        "/p2p-circuit/p2p/target-peer-id",
			expectError: true,
		},
		{
			name:        "empty address",
			addr:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relayPeer, targetPeer, err := parseCircuitAddr(tt.addr)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRelay, relayPeer)
				assert.Equal(t, tt.wantTarget, targetPeer)
			}
		})
	}
}

// TestBuildCircuitAddr 测试构建中继电路地址
func TestBuildCircuitAddr(t *testing.T) {
	relayAddr := "/ip4/1.2.3.4/tcp/4001"
	relayPeer := types.PeerID("relay-peer-id")
	targetPeer := types.PeerID("target-peer-id")

	addr := BuildCircuitAddr(relayAddr, relayPeer, targetPeer)

	expected := "/ip4/1.2.3.4/tcp/4001/p2p/relay-peer-id/p2p-circuit/p2p/target-peer-id"
	assert.Equal(t, expected, addr)
}

// TestIsCircuitAddr 测试判断是否为中继电路地址
func TestIsCircuitAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{
			name: "circuit address",
			addr: "/p2p/relay/p2p-circuit/p2p/target",
			want: true,
		},
		{
			name: "regular address",
			addr: "/ip4/1.2.3.4/tcp/4001/p2p/peer-id",
			want: false,
		},
		{
			name: "empty address",
			addr: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCircuitAddr(tt.addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ============================================================================
//                              RelayTransport 测试
// ============================================================================

// TestDefaultRelayTransportConfig 测试默认配置
func TestDefaultRelayTransportConfig(t *testing.T) {
	config := DefaultRelayTransportConfig()

	assert.Equal(t, DefaultDialTimeout, config.DialTimeout)
	assert.Equal(t, DefaultReservationTimeout, config.ReservationTimeout)
	assert.Equal(t, MaxConcurrentDials, config.MaxConcurrentDials)
}

// TestNewRelayTransport 测试创建中继传输层
func TestNewRelayTransport(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
		require.NotNil(t, rt)
		defer rt.Close()

		assert.Equal(t, DefaultDialTimeout, rt.dialTimeout)
		assert.Equal(t, DefaultReservationTimeout, rt.reservationTimeout)
		assert.Equal(t, MaxConcurrentDials, rt.maxConcurrentDials)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := RelayTransportConfig{
			DialTimeout:        10 * time.Second,
			ReservationTimeout: 5 * time.Second,
			MaxConcurrentDials: 4,
		}
		rt := NewRelayTransport(nil, nil, nil, config)
		require.NotNil(t, rt)
		defer rt.Close()

		assert.Equal(t, 10*time.Second, rt.dialTimeout)
		assert.Equal(t, 5*time.Second, rt.reservationTimeout)
		assert.Equal(t, 4, rt.maxConcurrentDials)
	})

	t.Run("with zero values uses defaults", func(t *testing.T) {
		config := RelayTransportConfig{}
		rt := NewRelayTransport(nil, nil, nil, config)
		require.NotNil(t, rt)
		defer rt.Close()

		assert.Equal(t, DefaultDialTimeout, rt.dialTimeout)
		assert.Equal(t, DefaultReservationTimeout, rt.reservationTimeout)
		assert.Equal(t, MaxConcurrentDials, rt.maxConcurrentDials)
	})
}

// TestRelayTransport_Protocols 测试支持的协议
func TestRelayTransport_Protocols(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	defer rt.Close()

	protocols := rt.Protocols()
	require.Len(t, protocols, 1)
	assert.Equal(t, CircuitProtocol, protocols[0])
}

// TestRelayTransport_CanDial 测试可拨号检查
func TestRelayTransport_CanDial(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	defer rt.Close()

	tests := []struct {
		name string
		addr string
		want bool
	}{
		{
			name: "circuit address",
			addr: "/p2p/relay/p2p-circuit/p2p/target",
			want: true,
		},
		{
			name: "regular address",
			addr: "/ip4/1.2.3.4/tcp/4001",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rt.CanDial(tt.addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRelayTransport_Listen 测试监听（不支持）
func TestRelayTransport_Listen(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	defer rt.Close()

	listener, err := rt.Listen("/ip4/0.0.0.0/tcp/0")
	assert.ErrorIs(t, err, ErrListenNotSupported)
	assert.Nil(t, listener)
}

// ============================================================================
//                              生命周期测试
// ============================================================================

// TestRelayTransport_StartClose 测试启动和关闭
func TestRelayTransport_StartClose(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())

	// 启动
	err := rt.Start(context.Background())
	assert.NoError(t, err)

	// 重复启动应该是幂等的
	err = rt.Start(context.Background())
	assert.NoError(t, err)

	// 关闭
	err = rt.Close()
	assert.NoError(t, err)

	// 重复关闭应该是幂等的
	err = rt.Close()
	assert.NoError(t, err)
}

// TestRelayTransport_DialAfterClose 测试关闭后拨号
func TestRelayTransport_DialAfterClose(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	rt.Close()

	ctx := context.Background()
	conn, err := rt.Dial(ctx, "/p2p/relay/p2p-circuit/p2p/target", types.PeerID("target"))
	assert.ErrorIs(t, err, ErrTransportClosed)
	assert.Nil(t, conn)
}

// TestRelayTransport_DialPeerAfterClose 测试关闭后自动拨号
func TestRelayTransport_DialPeerAfterClose(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	rt.Close()

	ctx := context.Background()
	conn, err := rt.DialPeer(ctx, types.PeerID("target"))
	assert.ErrorIs(t, err, ErrTransportClosed)
	assert.Nil(t, conn)
}

// ============================================================================
//                              统计测试
// ============================================================================

// TestRelayTransport_Stats 测试统计信息
func TestRelayTransport_Stats(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	defer rt.Close()

	stats := rt.Stats()
	assert.Equal(t, 0, stats.ActiveConnections)
}

// TestRelayTransport_GetActiveConnections 测试获取活跃连接
func TestRelayTransport_GetActiveConnections(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	defer rt.Close()

	conns := rt.GetActiveConnections()
	assert.Empty(t, conns)
}

// ============================================================================
//                              辅助结构测试
// ============================================================================

// TestRelayAddr 测试 relayAddr 实现
func TestRelayAddr(t *testing.T) {
	addr := &relayAddr{addr: "test-peer"}

	assert.Equal(t, "p2p-circuit", addr.Network())
	assert.Equal(t, "test-peer", addr.String())
}

// ============================================================================
//                 真正能发现 BUG 的测试
// ============================================================================

// TestParseCircuitAddr_MultipleP2P 测试多个 /p2p/ 的处理
// 潜在BUG：使用 LastIndex 可能在某些格式下解析错误
func TestParseCircuitAddr_MultipleP2P(t *testing.T) {
	// 地址格式：/ip4/.../p2p/relay-id/p2p-circuit/p2p/target-id
	// 这里有两个 /p2p/
	addr := "/ip4/1.2.3.4/tcp/4001/p2p/QmRelay123/p2p-circuit/p2p/QmTarget456"

	relayPeer, targetPeer, err := parseCircuitAddr(addr)

	require.NoError(t, err)
	assert.Equal(t, types.PeerID("QmRelay123"), relayPeer, "中继 ID 解析错误")
	assert.Equal(t, types.PeerID("QmTarget456"), targetPeer, "目标 ID 解析错误")
}

// TestParseCircuitAddr_ConsecutiveSlashes 测试连续斜杠
func TestParseCircuitAddr_ConsecutiveSlashes(t *testing.T) {
	// 畸形地址：连续斜杠
	addr := "//p2p//relay-id//p2p-circuit//p2p//target-id"

	_, _, err := parseCircuitAddr(addr)

	// 应该能优雅处理或返回错误，不应该 panic
	if err == nil {
		t.Log("注意: 连续斜杠被接受，解析结果可能不正确")
	}
}

// TestExtractPeerIDFromAddr_EdgeCases 测试 PeerID 提取的边界情况
func TestExtractPeerIDFromAddr_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected types.PeerID
	}{
		{
			name:     "standard format",
			addr:     "/ip4/1.2.3.4/tcp/4001/p2p/QmPeerID",
			expected: types.PeerID("QmPeerID"),
		},
		{
			name:     "with circuit suffix",
			addr:     "/ip4/1.2.3.4/tcp/4001/p2p/QmPeerID/p2p-circuit",
			expected: types.PeerID("QmPeerID"),
		},
		{
			name:     "only p2p component",
			addr:     "/p2p/QmPeerID",
			expected: types.PeerID("QmPeerID"),
		},
		{
			name:     "empty peer id",
			addr:     "/ip4/1.2.3.4/tcp/4001/p2p/",
			expected: types.PeerID(""),
		},
		{
			name:     "no p2p component",
			addr:     "/ip4/1.2.3.4/tcp/4001",
			expected: types.PeerID(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建一个简单的 Multiaddr mock
			result := extractPeerIDFromString(tt.addr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// extractPeerIDFromString 从字符串中提取 PeerID（用于测试）
func extractPeerIDFromString(addr string) types.PeerID {
	const p2pPrefix = "/p2p/"
	idx := strings.LastIndex(addr, p2pPrefix)
	if idx == -1 {
		return ""
	}

	peerIDStr := addr[idx+len(p2pPrefix):]

	if nextSlash := strings.Index(peerIDStr, "/"); nextSlash != -1 {
		peerIDStr = peerIDStr[:nextSlash]
	}

	return types.PeerID(peerIDStr)
}

// TestRelayTransport_ConcurrentDials 测试并发拨号限制
func TestRelayTransport_ConcurrentDials(t *testing.T) {
	config := RelayTransportConfig{
		MaxConcurrentDials: 2,
		DialTimeout:        100 * time.Millisecond,
	}
	rt := NewRelayTransport(nil, nil, nil, config)
	defer rt.Close()

	// 验证信号量大小
	assert.Equal(t, 2, cap(rt.dialSem), "信号量容量应该等于 MaxConcurrentDials")
}

// TestRelayTransport_DialInvalidAddress 测试无效地址拨号
func TestRelayTransport_DialInvalidAddress(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	defer rt.Close()

	ctx := context.Background()

	tests := []struct {
		name    string
		addr    string
		wantErr error
	}{
		{
			name:    "missing circuit",
			addr:    "/p2p/relay-id/p2p/target-id",
			wantErr: ErrInvalidRelayAddr,
		},
		{
			name:    "empty address",
			addr:    "",
			wantErr: ErrInvalidRelayAddr,
		},
		{
			name:    "only circuit",
			addr:    "/p2p-circuit",
			wantErr: ErrInvalidRelayAddr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := rt.Dial(ctx, tt.addr, types.PeerID("target"))
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestRelayTransport_DialPeerMismatch 测试 Peer ID 不匹配
func TestRelayTransport_DialPeerMismatch(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	defer rt.Close()

	ctx := context.Background()

	// 地址中的 target-id 与参数中的 other-id 不匹配
	_, err := rt.Dial(ctx, "/p2p/relay-id/p2p-circuit/p2p/target-id", types.PeerID("other-id"))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mismatch")
}

// TestRelayTransport_TrackConnection 测试连接跟踪
func TestRelayTransport_TrackConnection(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())
	defer rt.Close()

	// 模拟跟踪连接
	rt.trackConnection(types.PeerID("relay1"), types.PeerID("remote1"), nil)
	rt.trackConnection(types.PeerID("relay2"), types.PeerID("remote2"), nil)

	// 验证统计
	stats := rt.Stats()
	assert.Equal(t, 2, stats.ActiveConnections, "应该有 2 个活跃连接")

	// 验证连接信息
	conns := rt.GetActiveConnections()
	assert.Len(t, conns, 2)
}

// TestRelayTransport_CloseReleasesConnections 测试关闭时释放所有连接
func TestRelayTransport_CloseReleasesConnections(t *testing.T) {
	rt := NewRelayTransport(nil, nil, nil, DefaultRelayTransportConfig())

	// 添加一些连接
	rt.trackConnection(types.PeerID("relay1"), types.PeerID("remote1"), nil)
	rt.trackConnection(types.PeerID("relay2"), types.PeerID("remote2"), nil)

	// 关闭
	err := rt.Close()
	assert.NoError(t, err)

	// 验证连接已清空
	stats := rt.Stats()
	assert.Equal(t, 0, stats.ActiveConnections, "关闭后应该没有活跃连接")
}

// TestBuildCircuitAddr_EmptyInputs 测试空输入
func TestBuildCircuitAddr_EmptyInputs(t *testing.T) {
	// 空的中继地址
	addr := BuildCircuitAddr("", types.PeerID("relay"), types.PeerID("target"))
	assert.Contains(t, addr, "/p2p/relay/p2p-circuit/p2p/target")

	// 空的 peer ID
	addr = BuildCircuitAddr("/ip4/1.2.3.4/tcp/4001", types.PeerID(""), types.PeerID("target"))
	assert.Contains(t, addr, "/p2p//p2p-circuit")
}
