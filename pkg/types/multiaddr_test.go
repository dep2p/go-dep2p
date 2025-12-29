package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMultiaddr(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// 有效的 multiaddr
		{"ipv4 udp quic", "/ip4/1.2.3.4/udp/4001/quic-v1", false},
		{"ipv4 tcp", "/ip4/1.2.3.4/tcp/4001", false},
		{"ipv6 udp quic", "/ip6/::1/udp/4001/quic-v1", false},
		{"dns4", "/dns4/example.com/udp/4001/quic-v1", false},
		{"with peer id", "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", false},
		{"relay simple", "/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N/p2p-circuit/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx6N", false},
		{"relay full", "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N/p2p-circuit/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx6N", false},

		// 无效格式
		{"empty", "", true},
		{"host:port format", "1.2.3.4:4001", true},
		{"no leading slash", "ip4/1.2.3.4/udp/4001", true},
		{"unknown protocol", "/unknown/1.2.3.4/udp/4001", true},
		{"too short", "/ip4", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := ParseMultiaddr(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.input, ma.String())
			}
		})
	}
}

func TestMustParseMultiaddr(t *testing.T) {
	// 有效输入不 panic
	assert.NotPanics(t, func() {
		ma := MustParseMultiaddr("/ip4/1.2.3.4/udp/4001/quic-v1")
		assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", ma.String())
	})

	// 无效输入 panic
	assert.Panics(t, func() {
		MustParseMultiaddr("invalid")
	})
}

func TestFromHostPort(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		port      int
		transport string
		want      string
		wantErr   bool
	}{
		{"ipv4 quic", "1.2.3.4", 4001, "udp/quic-v1", "/ip4/1.2.3.4/udp/4001/quic-v1", false},
		{"ipv6 quic", "::1", 4001, "udp/quic-v1", "/ip6/::1/udp/4001/quic-v1", false},
		{"dns4 quic", "example.com", 4001, "udp/quic-v1", "/dns4/example.com/udp/4001/quic-v1", false},
		{"ipv4 tcp", "1.2.3.4", 4001, "tcp", "/ip4/1.2.3.4/tcp/4001", false},

		// 错误情况
		{"empty host", "", 4001, "udp/quic-v1", "", true},
		{"invalid port 0", "1.2.3.4", 0, "udp/quic-v1", "", true},
		{"invalid port negative", "1.2.3.4", -1, "udp/quic-v1", "", true},
		{"empty transport", "1.2.3.4", 4001, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := FromHostPort(tt.host, tt.port, tt.transport)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, ma.String())
			}
		})
	}
}

func TestMultiaddr_IP(t *testing.T) {
	tests := []struct {
		name     string
		multiaddr string
		wantIP   string
	}{
		{"ipv4", "/ip4/1.2.3.4/udp/4001/quic-v1", "1.2.3.4"},
		{"ipv6", "/ip6/::1/udp/4001/quic-v1", "::1"},
		{"ipv6 full", "/ip6/2001:db8::1/udp/4001/quic-v1", "2001:db8::1"},
		{"dns4 no ip", "/dns4/example.com/udp/4001/quic-v1", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := Multiaddr(tt.multiaddr)
			ip := ma.IP()
			if tt.wantIP == "" {
				assert.Nil(t, ip)
			} else {
				require.NotNil(t, ip)
				assert.Equal(t, tt.wantIP, ip.String())
			}
		})
	}
}

func TestMultiaddr_Port(t *testing.T) {
	tests := []struct {
		name     string
		multiaddr string
		wantPort int
	}{
		{"udp port", "/ip4/1.2.3.4/udp/4001/quic-v1", 4001},
		{"tcp port", "/ip4/1.2.3.4/tcp/8080", 8080},
		{"no port", "/ip4/1.2.3.4", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := Multiaddr(tt.multiaddr)
			assert.Equal(t, tt.wantPort, ma.Port())
		})
	}
}

func TestMultiaddr_HostPort(t *testing.T) {
	tests := []struct {
		name     string
		multiaddr string
		want     string
	}{
		{"ipv4", "/ip4/1.2.3.4/udp/4001/quic-v1", "1.2.3.4:4001"},
		{"ipv6", "/ip6/::1/udp/4001/quic-v1", "[::1]:4001"},
		{"dns4 empty", "/dns4/example.com/udp/4001/quic-v1", ""},
		{"relay empty", "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := Multiaddr(tt.multiaddr)
			assert.Equal(t, tt.want, ma.HostPort())
		})
	}
}

func TestMultiaddr_Transport(t *testing.T) {
	tests := []struct {
		name     string
		multiaddr string
		want     string
	}{
		{"quic-v1", "/ip4/1.2.3.4/udp/4001/quic-v1", "quic-v1"},
		{"tcp", "/ip4/1.2.3.4/tcp/4001", "tcp"},
		{"udp", "/ip4/1.2.3.4/udp/4001", "udp"},
		{"relay", "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest", "p2p-circuit"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := Multiaddr(tt.multiaddr)
			assert.Equal(t, tt.want, ma.Transport())
		})
	}
}

func TestMultiaddr_IsRelay(t *testing.T) {
	tests := []struct {
		name     string
		multiaddr string
		want     bool
	}{
		{"relay full", "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest", true},
		{"relay simple", "/p2p/QmRelay/p2p-circuit/p2p/QmDest", true},
		{"not relay", "/ip4/1.2.3.4/udp/4001/quic-v1", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := Multiaddr(tt.multiaddr)
			assert.Equal(t, tt.want, ma.IsRelay())
		})
	}
}

func TestMultiaddr_IsPublicPrivateLoopback(t *testing.T) {
	tests := []struct {
		name       string
		multiaddr   string
		isPublic   bool
		isPrivate  bool
		isLoopback bool
	}{
		{"public", "/ip4/8.8.8.8/udp/4001/quic-v1", true, false, false},
		{"private 192", "/ip4/192.168.1.1/udp/4001/quic-v1", false, true, false},
		{"private 10", "/ip4/10.0.0.1/udp/4001/quic-v1", false, true, false},
		{"loopback", "/ip4/127.0.0.1/udp/4001/quic-v1", false, false, true},
		{"loopback ipv6", "/ip6/::1/udp/4001/quic-v1", false, false, true},
		{"dns4 no ip", "/dns4/example.com/udp/4001/quic-v1", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := Multiaddr(tt.multiaddr)
			assert.Equal(t, tt.isPublic, ma.IsPublic(), "IsPublic")
			assert.Equal(t, tt.isPrivate, ma.IsPrivate(), "IsPrivate")
			assert.Equal(t, tt.isLoopback, ma.IsLoopback(), "IsLoopback")
		})
	}
}

func TestMultiaddr_WithPeerID(t *testing.T) {
	// 创建有效的 NodeID 用于测试（使用固定字节序列）
	var nodeID NodeID
	for i := 0; i < 32; i++ {
		nodeID[i] = byte(i + 1)
	}
	nodeIDStr := nodeID.String()

	tests := []struct {
		name      string
		multiaddr string
		want      string
	}{
		{
			"add peer id",
			"/ip4/1.2.3.4/udp/4001/quic-v1",
			"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + nodeIDStr,
		},
		{
			"replace peer id",
			"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/OtherID",
			"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + nodeIDStr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := Multiaddr(tt.multiaddr)
			result := ma.WithPeerID(nodeID)
			assert.Equal(t, tt.want, result.String())
		})
	}
}

func TestMultiaddr_WithoutPeerID(t *testing.T) {
	tests := []struct {
		name     string
		multiaddr string
		want     string
	}{
		{
			"remove peer id",
			"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmNode",
			"/ip4/1.2.3.4/udp/4001/quic-v1",
		},
		{
			"no peer id",
			"/ip4/1.2.3.4/udp/4001/quic-v1",
			"/ip4/1.2.3.4/udp/4001/quic-v1",
		},
		{
			"relay keep relay id",
			"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest",
			"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma := Multiaddr(tt.multiaddr)
			result := ma.WithoutPeerID()
			assert.Equal(t, tt.want, result.String())
		})
	}
}

func TestMultiaddr_RelayOperations(t *testing.T) {
	// 创建有效的测试用 NodeID
	var relayID NodeID
	for i := 0; i < 32; i++ {
		relayID[i] = byte(i + 1)
	}
	relayIDStr := relayID.String()

	var destID NodeID
	for i := 0; i < 32; i++ {
		destID[i] = byte(i + 33)
	}
	destIDStr := destID.String()

	t.Run("BuildRelayAddr", func(t *testing.T) {
		relayBase := Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayIDStr)
		result, err := relayBase.BuildRelayAddr(destID)
		require.NoError(t, err)
		assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/"+relayIDStr+"/p2p-circuit/p2p/"+destIDStr, result.String())
	})

	t.Run("BuildRelayAddr no dest", func(t *testing.T) {
		relayBase := Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayIDStr)
		result, err := relayBase.BuildRelayAddr(NodeID{})
		require.NoError(t, err)
		assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/"+relayIDStr+"/p2p-circuit", result.String())
	})

	t.Run("RelayID and DestID", func(t *testing.T) {
		ma := Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayIDStr + "/p2p-circuit/p2p/" + destIDStr)
		assert.Equal(t, relayID, ma.RelayID())
		assert.Equal(t, destID, ma.DestID())
	})

	t.Run("RelayBaseAddr", func(t *testing.T) {
		ma := Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayIDStr + "/p2p-circuit/p2p/" + destIDStr)
		base := ma.RelayBaseAddr()
		assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/"+relayIDStr, base.String())
	})

	t.Run("IsDialableRelayAddr", func(t *testing.T) {
		// 可拨号的 relay 地址（有底层地址）
		dialable := Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayIDStr + "/p2p-circuit/p2p/" + destIDStr)
		assert.True(t, dialable.IsDialableRelayAddr())

		// 不可拨号的 relay 地址（简写格式）
		notDialable := Multiaddr("/p2p/" + relayIDStr + "/p2p-circuit/p2p/" + destIDStr)
		assert.False(t, notDialable.IsDialableRelayAddr())

		// 非 relay 地址
		notRelay := Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1")
		assert.False(t, notRelay.IsDialableRelayAddr())
	})
}

func TestMultiaddr_ParseRelayAddrInfo(t *testing.T) {
	// 创建有效的测试用 NodeID
	var relayID NodeID
	for i := 0; i < 32; i++ {
		relayID[i] = byte(i + 1)
	}
	relayIDStr := relayID.String()

	var destID NodeID
	for i := 0; i < 32; i++ {
		destID[i] = byte(i + 33)
	}
	destIDStr := destID.String()

	t.Run("full relay addr", func(t *testing.T) {
		ma := Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayIDStr + "/p2p-circuit/p2p/" + destIDStr)
		base, rID, dID, err := ma.ParseRelayAddrInfo()
		require.NoError(t, err)
		assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/"+relayIDStr, base.String())
		assert.Equal(t, relayID, rID)
		assert.Equal(t, destID, dID)
	})

	t.Run("not relay addr", func(t *testing.T) {
		ma := Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1")
		_, _, _, err := ma.ParseRelayAddrInfo()
		assert.ErrorIs(t, err, ErrNotRelayAddr)
	})
}

func TestMultiaddr_Idempotent(t *testing.T) {
	// 创建有效的测试用 NodeID
	var nodeID1 NodeID
	for i := 0; i < 32; i++ {
		nodeID1[i] = byte(i + 1)
	}
	nodeID1Str := nodeID1.String()

	var nodeID2 NodeID
	for i := 0; i < 32; i++ {
		nodeID2[i] = byte(i + 33)
	}
	nodeID2Str := nodeID2.String()

	// 测试幂等性：解析后再转字符串再解析，结果应相同
	inputs := []string{
		"/ip4/1.2.3.4/udp/4001/quic-v1",
		"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + nodeID1Str,
		"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + nodeID1Str + "/p2p-circuit/p2p/" + nodeID2Str,
	}

	for _, input := range inputs {
		name := input
		if len(name) > 30 {
			name = name[:30] + "..."
		}
		t.Run(name, func(t *testing.T) {
			ma1, err := ParseMultiaddr(input)
			require.NoError(t, err)

			ma2, err := ParseMultiaddr(ma1.String())
			require.NoError(t, err)

			assert.Equal(t, ma1, ma2)
		})
	}
}

