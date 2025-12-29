package dep2p

import (
	"testing"

	"github.com/dep2p/go-dep2p/internal/util/addrutil"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// TestParseNodeID_Base58 测试 Base58 格式的 NodeID 解析
func TestParseNodeID_Base58(t *testing.T) {
	// 创建一个已知的 NodeID
	var original types.NodeID
	for i := 0; i < 32; i++ {
		original[i] = byte(i + 1)
	}

	// 获取 Base58 表示
	base58Str := original.String()
	t.Logf("Base58 NodeID: %s", base58Str)

	// 解析回来
	parsed, err := types.ParseNodeID(base58Str)
	if err != nil {
		t.Fatalf("ParseNodeID(%q) error: %v", base58Str, err)
	}

	if !parsed.Equal(original) {
		t.Errorf("ParseNodeID round-trip failed")
	}
}

// TestParseFullAddr 测试完整地址解析
func TestParseFullAddr(t *testing.T) {
	// 创建测试 NodeID
	var peerID types.NodeID
	for i := 0; i < 32; i++ {
		peerID[i] = byte(i + 1)
	}

	tests := []struct {
		name         string
		fullAddr     string
		wantDialAddr string
		wantErr      bool
	}{
		{
			name:         "valid ip4 udp quic",
			fullAddr:     "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + peerID.String(),
			wantDialAddr: "/ip4/1.2.3.4/udp/4001/quic-v1",
			wantErr:      false,
		},
		{
			name:         "valid dns4",
			fullAddr:     "/dns4/bootstrap.dep2p.io/udp/4001/quic-v1/p2p/" + peerID.String(),
			wantDialAddr: "/dns4/bootstrap.dep2p.io/udp/4001/quic-v1",
			wantErr:      false,
		},
		{
			name:     "missing p2p",
			fullAddr: "/ip4/1.2.3.4/udp/4001/quic-v1",
			wantErr:  true,
		},
		{
			name:     "empty",
			fullAddr: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPeerID, gotDialAddr, err := addrutil.ParseFullAddr(tt.fullAddr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFullAddr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if !gotPeerID.Equal(peerID) {
					t.Errorf("ParseFullAddr() peerID mismatch")
				}
				if gotDialAddr != tt.wantDialAddr {
					t.Errorf("ParseFullAddr() dialAddr = %q, want %q", gotDialAddr, tt.wantDialAddr)
				}
			}
		})
	}
}

// TestBuildFullAddr 测试完整地址构建
func TestBuildFullAddr(t *testing.T) {
	var peerID types.NodeID
	for i := 0; i < 32; i++ {
		peerID[i] = byte(i + 1)
	}

	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	fullAddr, err := addrutil.BuildFullAddr(addr, peerID)
	if err != nil {
		t.Fatalf("BuildFullAddr() error: %v", err)
	}

	// 验证结果可以被解析回来
	gotPeerID, gotDialAddr, err := addrutil.ParseFullAddr(fullAddr)
	if err != nil {
		t.Fatalf("ParseFullAddr() error: %v", err)
	}

	if !gotPeerID.Equal(peerID) {
		t.Errorf("Round-trip peerID mismatch")
	}

	if gotDialAddr != addr {
		t.Errorf("Round-trip addr = %q, want %q", gotDialAddr, addr)
	}
}

// TestWithBootstrapPeers_GenesisNode 测试创世节点（无 bootstrap）配置
func TestWithBootstrapPeers_GenesisNode(t *testing.T) {
	opts := newOptions()

	// 显式调用无参数表示创世节点
	opt := WithBootstrapPeers()
	if err := opt(opts); err != nil {
		t.Fatalf("WithBootstrapPeers() error: %v", err)
	}

	// 验证 bootstrapPeersSet 标志被设置
	if !opts.discovery.bootstrapPeersSet {
		t.Error("bootstrapPeersSet should be true after WithBootstrapPeers()")
	}

	// 验证 peers 为空（创世节点无 bootstrap）
	if len(opts.discovery.bootstrapPeers) != 0 {
		t.Errorf("bootstrapPeers should be empty, got %v", opts.discovery.bootstrapPeers)
	}
}

// createTestNodeID 创建确定性的测试 NodeID
func createTestNodeID(seed byte) types.NodeID {
	var id types.NodeID
	for i := 0; i < 32; i++ {
		id[i] = byte((int(seed)*17 + i*31) % 256)
	}
	return id
}

// TestWithBootstrapPeers_CustomPeers 测试自定义 bootstrap 节点
func TestWithBootstrapPeers_CustomPeers(t *testing.T) {
	opts := newOptions()

	// 使用有效的 NodeID（dep2p 使用 32 字节 NodeID，Base58 编码）
	nodeID1 := createTestNodeID(1).String()
	nodeID2 := createTestNodeID(2).String()
	peers := []string{
		"/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + nodeID1,
		"/ip4/5.6.7.8/udp/4001/quic-v1/p2p/" + nodeID2,
	}

	opt := WithBootstrapPeers(peers...)
	if err := opt(opts); err != nil {
		t.Fatalf("WithBootstrapPeers() error: %v", err)
	}

	if !opts.discovery.bootstrapPeersSet {
		t.Error("bootstrapPeersSet should be true")
	}

	if len(opts.discovery.bootstrapPeers) != 2 {
		t.Errorf("bootstrapPeers count = %d, want 2", len(opts.discovery.bootstrapPeers))
	}
}

// TestPreset_BootstrapPeers 测试 Preset 的 bootstrap 配置
func TestPreset_BootstrapPeers(t *testing.T) {
	// Desktop/Mobile/Server 应该有默认 bootstrap
	presets := []*Preset{PresetDesktop, PresetMobile, PresetServer}
	for _, p := range presets {
		// 当前 DefaultBootstrapPeers 为空（待部署），所以 BootstrapPeers 也为空
		// 但 Preset 结构应该支持 BootstrapPeers 字段
		t.Logf("Preset %s: BootstrapPeers=%v", p.Name, p.BootstrapPeers)
	}

	// Minimal/Test 应该没有 bootstrap
	if len(PresetMinimal.BootstrapPeers) != 0 {
		t.Errorf("PresetMinimal should have no bootstrap peers")
	}
	if len(PresetTest.BootstrapPeers) != 0 {
		t.Errorf("PresetTest should have no bootstrap peers")
	}
}
