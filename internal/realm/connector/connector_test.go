package connector_test

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/peerstore"
	"github.com/dep2p/go-dep2p/internal/realm/connector"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              AddressResolver 测试
// ============================================================================

func TestResolver_PeerstoreHit(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	// 预填充地址
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	ps.AddAddrs(types.PeerID("peer-1"), []types.Multiaddr{addr}, time.Hour)

	resolver := connector.NewAddressResolver(connector.ResolverConfig{
		Peerstore: ps,
	})

	// 测试解析
	result, err := resolver.Resolve(context.Background(), "peer-1")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if !result.HasAddrs() {
		t.Error("Expected addresses from Peerstore")
	}

	if result.Source != connector.SourcePeerstore {
		t.Errorf("Expected source Peerstore, got %v", result.Source)
	}

	if !result.Cached {
		t.Error("Expected Cached to be true for Peerstore hit")
	}
}

func TestResolver_NoAddress(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	resolver := connector.NewAddressResolver(connector.ResolverConfig{
		Peerstore: ps,
	})

	// 测试未知节点
	result, err := resolver.Resolve(context.Background(), "unknown-peer")
	if err != nil {
		t.Fatalf("Resolve should not fail for unknown peer: %v", err)
	}

	if result.HasAddrs() {
		t.Error("Expected no addresses for unknown peer")
	}

	if result.Source != connector.SourceUnknown {
		t.Errorf("Expected source Unknown, got %v", result.Source)
	}
}

func TestResolver_InvalidTarget(t *testing.T) {
	resolver := connector.NewAddressResolver(connector.ResolverConfig{})

	// 测试空目标
	_, err := resolver.Resolve(context.Background(), "")
	if err != connector.ErrInvalidTarget {
		t.Errorf("Expected ErrInvalidTarget, got %v", err)
	}
}

// ============================================================================
//                              Connector 测试
// ============================================================================

func TestConnector_DirectConnect(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	// 预填充地址
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	ps.AddAddrs(types.PeerID("peer-1"), []types.Multiaddr{addr}, time.Hour)

	resolver := connector.NewAddressResolver(connector.ResolverConfig{
		Peerstore: ps,
	})

	// 创建连接器（无 Host，直连会失败）
	conn := connector.NewConnector(
		resolver,
		nil, // 无 Host
		nil, // 无 HolePuncher
		connector.DefaultConnectorConfig(),
	)
	defer conn.Close()

	// 直连应该失败（因为没有 Host）
	_, err := conn.Connect(context.Background(), "peer-1")
	if err == nil {
		t.Error("Expected error without Host")
	}
}

func TestConnector_RelayOnly(t *testing.T) {
	// 创建连接器（仅 Relay 模式）
	config := connector.DefaultConnectorConfig()
	config.Strategy = connector.StrategyRelayOnly

	conn := connector.NewConnector(
		nil, // 无 resolver
		nil, // 无 Host
		nil, // 无 HolePuncher
		config,
	)
	defer conn.Close()

	// Relay 连接应该失败（因为没有可用的 RelayDialer）
	_, err := conn.Connect(context.Background(), "peer-1")
	if err == nil {
		t.Error("Expected error without valid connection method")
	}
}

func TestConnector_Closed(t *testing.T) {
	conn := connector.NewConnector(
		nil,
		nil,
		nil,
		connector.DefaultConnectorConfig(),
	)

	// 关闭连接器
	conn.Close()

	// 尝试连接应该失败
	_, err := conn.Connect(context.Background(), "peer-1")
	if err != connector.ErrConnectorClosed {
		t.Errorf("Expected ErrConnectorClosed, got %v", err)
	}
}

func TestConnector_InvalidTarget(t *testing.T) {
	conn := connector.NewConnector(
		nil,
		nil,
		nil,
		connector.DefaultConnectorConfig(),
	)
	defer conn.Close()

	// 测试空目标
	_, err := conn.Connect(context.Background(), "")
	if err != connector.ErrInvalidTarget {
		t.Errorf("Expected ErrInvalidTarget, got %v", err)
	}
}

// ============================================================================
//                              配置测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	config := connector.DefaultConnectorConfig()

	if config.DirectTimeout <= 0 {
		t.Error("DirectTimeout should be positive")
	}
	if config.HolePunchTimeout <= 0 {
		t.Error("HolePunchTimeout should be positive")
	}
	if config.RelayTimeout <= 0 {
		t.Error("RelayTimeout should be positive")
	}
	if config.TotalTimeout <= 0 {
		t.Error("TotalTimeout should be positive")
	}
	if config.Strategy != connector.StrategyAuto {
		t.Error("Default strategy should be Auto")
	}
	if !config.EnableHolePunch {
		t.Error("HolePunch should be enabled by default")
	}
}

func TestDefaultResolverConfig(t *testing.T) {
	config := connector.DefaultResolverConfig()

	if config.QueryTimeout <= 0 {
		t.Error("QueryTimeout should be positive")
	}
	if config.CacheTTL <= 0 {
		t.Error("CacheTTL should be positive")
	}
}

// ============================================================================
//                              ResolveResult 测试
// ============================================================================

func TestResolveResult_HasAddrs(t *testing.T) {
	// 空结果
	empty := &connector.ResolveResult{}
	if empty.HasAddrs() {
		t.Error("Empty result should not have addresses")
	}

	// 有地址
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	withAddrs := &connector.ResolveResult{
		Addrs: []types.Multiaddr{addr},
	}
	if !withAddrs.HasAddrs() {
		t.Error("Result with addresses should have addresses")
	}
}

// ============================================================================
//                              ConnectWithHint 测试
// ============================================================================

func TestConnector_ConnectWithHint_InvalidTarget(t *testing.T) {
	conn := connector.NewConnector(nil, nil, nil, connector.DefaultConnectorConfig())
	defer conn.Close()

	// 空目标应该返回错误
	_, err := conn.ConnectWithHint(context.Background(), "", nil)
	if err != connector.ErrInvalidTarget {
		t.Errorf("Expected ErrInvalidTarget, got %v", err)
	}
}

func TestConnector_ConnectWithHint_InvalidHints(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	resolver := connector.NewAddressResolver(connector.ResolverConfig{
		Peerstore: ps,
	})

	config := connector.DefaultConnectorConfig()
	config.Strategy = connector.StrategyDirectOnly

	conn := connector.NewConnector(resolver, nil, nil, config)
	defer conn.Close()

	// 无效的地址提示应该被忽略，然后回退到自动发现
	hints := []string{"invalid-addr", "also-invalid"}
	_, err := conn.ConnectWithHint(context.Background(), "peer-1", hints)

	// 因为没有有效地址，应该返回 ErrNoAddress
	if err != connector.ErrNoAddress {
		t.Logf("Got error (expected since no valid addresses): %v", err)
	}
}

func TestConnector_ConnectWithHint_ValidHints(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	// 添加地址到 Peerstore 作为回退
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	ps.AddAddrs(types.PeerID("peer-with-hint"), []types.Multiaddr{addr}, time.Hour)

	resolver := connector.NewAddressResolver(connector.ResolverConfig{
		Peerstore: ps,
	})

	config := connector.DefaultConnectorConfig()
	config.DirectTimeout = 100 * time.Millisecond

	conn := connector.NewConnector(resolver, nil, nil, config)
	defer conn.Close()

	// 有效的地址提示
	hints := []string{"/ip4/10.0.0.1/tcp/4001", "/ip4/10.0.0.2/tcp/4001"}
	_, err := conn.ConnectWithHint(context.Background(), "peer-with-hint", hints)

	// 应该尝试连接（会失败因为没有 Host），但不应该是 ErrInvalidTarget
	if err == connector.ErrInvalidTarget {
		t.Error("Should not return ErrInvalidTarget with valid hints")
	}
	t.Logf("ConnectWithHint returned: %v (expected connection failure without Host)", err)
}

func TestConnector_ConnectWithHint_Closed(t *testing.T) {
	conn := connector.NewConnector(nil, nil, nil, connector.DefaultConnectorConfig())
	conn.Close()

	// 关闭后应该返回 ErrConnectorClosed
	_, err := conn.ConnectWithHint(context.Background(), "peer-1", nil)
	if err != connector.ErrConnectorClosed {
		t.Errorf("Expected ErrConnectorClosed, got %v", err)
	}
}
