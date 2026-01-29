package relay

import (
	"context"
	"testing"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// TestProvideAutoRelay_StaticRelays 验证静态中继配置可被注入
func TestProvideAutoRelay_StaticRelays(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Relay.StaticRelays = []string{"relay-a", "relay-b"}

	input := AutoRelayInput{
		Config:    cfg,
		Swarm:     mocks.NewMockSwarm("local-peer"),
		Host:      mocks.NewMockHost("local-peer"),
		Peerstore: mocks.NewMockPeerstore(),
	}

	autoRelay := ProvideAutoRelay(input)
	if autoRelay == nil {
		t.Fatal("ProvideAutoRelay returned nil")
	}

	ctx := context.Background()
	if err := autoRelay.Start(ctx); err != nil {
		t.Fatalf("AutoRelay Start failed: %v", err)
	}
	defer autoRelay.Stop()

	status := autoRelay.Status()
	if status.NumCandidates < len(cfg.Relay.StaticRelays) {
		t.Errorf("NumCandidates = %d, want >= %d", status.NumCandidates, len(cfg.Relay.StaticRelays))
	}
}

// TestProvideAutoRelay_RelayAddr 验证 RelayAddr 配置会被转换为 StaticRelays
// 修复 
// 修复 5: 地址需要同时写入 Peerstore
func TestProvideAutoRelay_RelayAddr(t *testing.T) {
	cfg := config.NewConfig()
	// 使用有效的 multiaddr 格式，包含 /p2p/ 组件
	cfg.Relay.RelayAddr = "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWTest"

	peerstore := mocks.NewMockPeerstore()
	input := AutoRelayInput{
		Config:    cfg,
		Swarm:     mocks.NewMockSwarm("local-peer"),
		Host:      mocks.NewMockHost("local-peer"),
		Peerstore: peerstore,
	}

	autoRelay := ProvideAutoRelay(input)
	if autoRelay == nil {
		t.Fatal("ProvideAutoRelay returned nil")
	}

	ctx := context.Background()
	if err := autoRelay.Start(ctx); err != nil {
		t.Fatalf("AutoRelay Start failed: %v", err)
	}
	defer autoRelay.Stop()

	status := autoRelay.Status()
	// RelayAddr 应该被转换为 1 个 StaticRelay
	if status.NumCandidates < 1 {
		t.Errorf("NumCandidates = %d, want >= 1 (RelayAddr should be converted to StaticRelay)", status.NumCandidates)
	}

	// 验证地址已写入 Peerstore (5 修复)
	addrs := peerstore.Addrs("12D3KooWTest")
	if len(addrs) == 0 {
		t.Errorf("Peerstore should contain addresses for relay peer 12D3KooWTest, got 0 addresses")
	}
}

// TestProvideAutoRelay_RelayAddrAndStaticRelays 验证 RelayAddr 和 StaticRelays 可以同时使用
func TestProvideAutoRelay_RelayAddrAndStaticRelays(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Relay.StaticRelays = []string{"relay-a", "relay-b"}
	cfg.Relay.RelayAddr = "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWTest"

	input := AutoRelayInput{
		Config:    cfg,
		Swarm:     mocks.NewMockSwarm("local-peer"),
		Host:      mocks.NewMockHost("local-peer"),
		Peerstore: mocks.NewMockPeerstore(),
	}

	autoRelay := ProvideAutoRelay(input)
	if autoRelay == nil {
		t.Fatal("ProvideAutoRelay returned nil")
	}

	ctx := context.Background()
	if err := autoRelay.Start(ctx); err != nil {
		t.Fatalf("AutoRelay Start failed: %v", err)
	}
	defer autoRelay.Stop()

	status := autoRelay.Status()
	// 应该有 3 个候选中继：2 个来自 StaticRelays + 1 个来自 RelayAddr
	expectedMin := len(cfg.Relay.StaticRelays) + 1
	if status.NumCandidates < expectedMin {
		t.Errorf("NumCandidates = %d, want >= %d (StaticRelays + RelayAddr)", status.NumCandidates, expectedMin)
	}
}

// TestProvideAutoRelay_InvalidRelayAddr 验证无效的 RelayAddr 不会导致崩溃
func TestProvideAutoRelay_InvalidRelayAddr(t *testing.T) {
	cfg := config.NewConfig()
	// 无效的地址格式（缺少 /p2p/ 组件）
	cfg.Relay.RelayAddr = "/ip4/127.0.0.1/tcp/4001"

	input := AutoRelayInput{
		Config:    cfg,
		Swarm:     mocks.NewMockSwarm("local-peer"),
		Host:      mocks.NewMockHost("local-peer"),
		Peerstore: mocks.NewMockPeerstore(),
	}

	// 不应该崩溃
	autoRelay := ProvideAutoRelay(input)
	if autoRelay == nil {
		t.Fatal("ProvideAutoRelay returned nil")
	}

	ctx := context.Background()
	if err := autoRelay.Start(ctx); err != nil {
		t.Fatalf("AutoRelay Start failed: %v", err)
	}
	defer autoRelay.Stop()

	// 无效地址应该被忽略，NumCandidates 应该为 0
	status := autoRelay.Status()
	if status.NumCandidates != 0 {
		t.Errorf("NumCandidates = %d, want 0 (invalid RelayAddr should be ignored)", status.NumCandidates)
	}
}
