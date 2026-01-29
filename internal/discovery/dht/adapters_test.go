// Package dht 提供分布式哈希表实现
//
// 本文件包含适配器的单元测试
package dht

import (
	"context"
	"testing"
	"time"

	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mock 实现
// ============================================================================

// mockAddressBook 模拟地址簿实现
type mockAddressBook struct {
	entries  map[string]realmif.MemberEntry
	onlineID types.NodeID
}

func newMockAddressBook() *mockAddressBook {
	return &mockAddressBook{
		entries: make(map[string]realmif.MemberEntry),
	}
}

func (m *mockAddressBook) Register(ctx context.Context, entry realmif.MemberEntry) error {
	m.entries[entry.NodeID.String()] = entry
	return nil
}

func (m *mockAddressBook) Query(ctx context.Context, nodeID types.NodeID) (realmif.MemberEntry, error) {
	if entry, ok := m.entries[nodeID.String()]; ok {
		return entry, nil
	}
	return realmif.MemberEntry{}, nil
}

func (m *mockAddressBook) Update(ctx context.Context, entry realmif.MemberEntry) error {
	if _, ok := m.entries[entry.NodeID.String()]; !ok {
		return nil // 模拟不存在
	}
	m.entries[entry.NodeID.String()] = entry
	return nil
}

func (m *mockAddressBook) Remove(ctx context.Context, nodeID types.NodeID) error {
	delete(m.entries, nodeID.String())
	return nil
}

func (m *mockAddressBook) Members(ctx context.Context) ([]realmif.MemberEntry, error) {
	var result []realmif.MemberEntry
	for _, entry := range m.entries {
		result = append(result, entry)
	}
	return result, nil
}

func (m *mockAddressBook) OnlineMembers(ctx context.Context) ([]realmif.MemberEntry, error) {
	var result []realmif.MemberEntry
	for _, entry := range m.entries {
		if entry.Online {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (m *mockAddressBook) SetOnline(ctx context.Context, nodeID types.NodeID, online bool) error {
	if entry, ok := m.entries[nodeID.String()]; ok {
		entry.Online = online
		m.entries[nodeID.String()] = entry
	}
	return nil
}

func (m *mockAddressBook) Close() error {
	return nil
}

// mockCoordinator 模拟可达性协调器实现
type mockCoordinator struct {
	verifiedAddrs []string
	relayAddrs    []string
	hasVerified   bool
	hasRelay      bool
}

func newMockCoordinator() *mockCoordinator {
	return &mockCoordinator{}
}

func (m *mockCoordinator) VerifiedDirectAddresses() []string {
	return m.verifiedAddrs
}

func (m *mockCoordinator) RelayAddresses() []string {
	return m.relayAddrs
}

func (m *mockCoordinator) AdvertisedAddrs() []string {
	result := make([]string, 0, len(m.verifiedAddrs)+len(m.relayAddrs))
	result = append(result, m.verifiedAddrs...)
	result = append(result, m.relayAddrs...)
	return result
}

func (m *mockCoordinator) HasVerifiedDirectAddress() bool {
	return m.hasVerified
}

func (m *mockCoordinator) HasRelayAddress() bool {
	return m.hasRelay
}

// ============================================================================
//                              RelayAddressBookAdapter 测试
// ============================================================================

func TestRelayAddressBookAdapter_GetPeerAddresses(t *testing.T) {
	t.Run("nil address book returns empty", func(t *testing.T) {
		adapter := NewRelayAddressBookAdapter(nil)
		direct, relay, found := adapter.GetPeerAddresses(context.Background(), "")
		if found {
			t.Error("expected found=false for nil address book")
		}
		if len(direct) != 0 || len(relay) != 0 {
			t.Error("expected empty address lists")
		}
	})

	t.Run("empty address book returns not found", func(t *testing.T) {
		mockAB := newMockAddressBook()
		adapter := NewRelayAddressBookAdapter(mockAB)

		nodeID := types.NodeID("") // 空 NodeID
		direct, relay, found := adapter.GetPeerAddresses(context.Background(), nodeID)
		if found {
			t.Error("expected found=false for empty address book")
		}
		if len(direct) != 0 || len(relay) != 0 {
			t.Error("expected empty address lists")
		}
	})

	t.Run("returns addresses correctly", func(t *testing.T) {
		mockAB := newMockAddressBook()
		adapter := NewRelayAddressBookAdapter(mockAB)

		// 创建测试地址
		directAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
		relayAddr, _ := types.NewMultiaddr("/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW/p2p-circuit/p2p/other")

		nodeID := types.NodeID("test-node-id-1234567")

		entry := realmif.MemberEntry{
			NodeID:      nodeID,
			DirectAddrs: []types.Multiaddr{directAddr, relayAddr},
			Online:      true,
			LastSeen:    time.Now(),
		}

		// 注册条目
		mockAB.Register(context.Background(), entry)

		// 查询地址
		direct, relay, found := adapter.GetPeerAddresses(context.Background(), nodeID)
		if !found {
			t.Error("expected found=true")
		}
		if len(direct) != 1 {
			t.Errorf("expected 1 direct address, got %d", len(direct))
		}
		if len(relay) != 1 {
			t.Errorf("expected 1 relay address, got %d", len(relay))
		}
	})
}

func TestRelayAddressBookAdapter_InvalidateCache(t *testing.T) {
	t.Run("sets node offline", func(t *testing.T) {
		mockAB := newMockAddressBook()
		adapter := NewRelayAddressBookAdapter(mockAB)

		nodeID := types.NodeID("test-node-id-1234567")

		entry := realmif.MemberEntry{
			NodeID: nodeID,
			Online: true,
		}

		mockAB.Register(context.Background(), entry)

		// 使缓存失效
		err := adapter.InvalidateCache(context.Background(), nodeID)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// 验证节点已离线
		updated, _ := mockAB.Query(context.Background(), nodeID)
		if updated.Online {
			t.Error("expected node to be offline after cache invalidation")
		}
	})
}

// ============================================================================
//                              CoordinatorReachabilityAdapter 测试
// ============================================================================

func TestCoordinatorReachabilityAdapter_CheckReachability(t *testing.T) {
	t.Run("nil coordinator returns unknown", func(t *testing.T) {
		adapter := NewCoordinatorReachabilityAdapter(nil)
		reachability, natType := adapter.CheckReachability(context.Background())
		if reachability != types.ReachabilityUnknown {
			t.Errorf("expected ReachabilityUnknown, got %v", reachability)
		}
		if natType != types.NATTypeUnknown {
			t.Errorf("expected NATTypeUnknown, got %v", natType)
		}
	})

	t.Run("returns public when verified direct addresses exist", func(t *testing.T) {
		mockCoord := newMockCoordinator()
		mockCoord.hasVerified = true
		mockCoord.verifiedAddrs = []string{"/ip4/1.2.3.4/tcp/4001"}

		adapter := NewCoordinatorReachabilityAdapter(mockCoord)
		reachability, natType := adapter.CheckReachability(context.Background())

		if reachability != types.ReachabilityPublic {
			t.Errorf("expected ReachabilityPublic, got %v", reachability)
		}
		if natType != types.NATTypeNone {
			t.Errorf("expected NATTypeNone, got %v", natType)
		}
	})

	t.Run("returns private when only relay addresses exist", func(t *testing.T) {
		mockCoord := newMockCoordinator()
		mockCoord.hasVerified = false
		mockCoord.hasRelay = true
		mockCoord.relayAddrs = []string{"/ip4/1.2.3.4/tcp/4001/p2p/relay/p2p-circuit"}

		adapter := NewCoordinatorReachabilityAdapter(mockCoord)
		reachability, natType := adapter.CheckReachability(context.Background())

		if reachability != types.ReachabilityPrivate {
			t.Errorf("expected ReachabilityPrivate, got %v", reachability)
		}
		if natType != types.NATTypeSymmetric {
			t.Errorf("expected NATTypeSymmetric, got %v", natType)
		}
	})
}

func TestCoordinatorReachabilityAdapter_VerifyAddresses(t *testing.T) {
	t.Run("nil coordinator returns all unverified", func(t *testing.T) {
		adapter := NewCoordinatorReachabilityAdapter(nil)
		addrs := []string{"/ip4/1.2.3.4/tcp/4001", "/ip4/5.6.7.8/tcp/4001"}

		verified, unverified := adapter.VerifyAddresses(context.Background(), addrs)

		if len(verified) != 0 {
			t.Errorf("expected 0 verified addresses, got %d", len(verified))
		}
		if len(unverified) != 2 {
			t.Errorf("expected 2 unverified addresses, got %d", len(unverified))
		}
	})

	t.Run("classifies addresses correctly", func(t *testing.T) {
		mockCoord := newMockCoordinator()
		mockCoord.verifiedAddrs = []string{"/ip4/1.2.3.4/tcp/4001"}
		mockCoord.relayAddrs = []string{"/ip4/relay/tcp/4001/p2p-circuit"}

		adapter := NewCoordinatorReachabilityAdapter(mockCoord)
		addrs := []string{
			"/ip4/1.2.3.4/tcp/4001", // 已验证
			"/ip4/5.6.7.8/tcp/4001", // 未验证
		}

		verified, unverified := adapter.VerifyAddresses(context.Background(), addrs)

		if len(verified) != 1 {
			t.Errorf("expected 1 verified address, got %d", len(verified))
		}
		if len(unverified) != 1 {
			t.Errorf("expected 1 unverified address, got %d", len(unverified))
		}
	})

	t.Run("relay addresses are always verified", func(t *testing.T) {
		mockCoord := newMockCoordinator()
		adapter := NewCoordinatorReachabilityAdapter(mockCoord)

		addrs := []string{"/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW/p2p-circuit/p2p/other"}

		verified, unverified := adapter.VerifyAddresses(context.Background(), addrs)

		if len(verified) != 1 {
			t.Errorf("expected 1 verified address (relay), got %d", len(verified))
		}
		if len(unverified) != 0 {
			t.Errorf("expected 0 unverified addresses, got %d", len(unverified))
		}
	})
}

func TestCoordinatorReachabilityAdapter_GetVerifiedDirectAddresses(t *testing.T) {
	t.Run("returns verified addresses from coordinator", func(t *testing.T) {
		mockCoord := newMockCoordinator()
		mockCoord.verifiedAddrs = []string{"/ip4/1.2.3.4/tcp/4001", "/ip4/5.6.7.8/tcp/4001"}

		adapter := NewCoordinatorReachabilityAdapter(mockCoord)
		addrs := adapter.GetVerifiedDirectAddresses(context.Background())

		if len(addrs) != 2 {
			t.Errorf("expected 2 addresses, got %d", len(addrs))
		}
	})

	t.Run("nil coordinator returns nil", func(t *testing.T) {
		adapter := NewCoordinatorReachabilityAdapter(nil)
		addrs := adapter.GetVerifiedDirectAddresses(context.Background())

		if addrs != nil {
			t.Errorf("expected nil, got %v", addrs)
		}
	})
}

func TestCoordinatorReachabilityAdapter_IsDirectlyReachable(t *testing.T) {
	t.Run("returns true when has verified addresses", func(t *testing.T) {
		mockCoord := newMockCoordinator()
		mockCoord.hasVerified = true

		adapter := NewCoordinatorReachabilityAdapter(mockCoord)
		if !adapter.IsDirectlyReachable(context.Background()) {
			t.Error("expected IsDirectlyReachable=true")
		}
	})

	t.Run("returns false when no verified addresses", func(t *testing.T) {
		mockCoord := newMockCoordinator()
		mockCoord.hasVerified = false

		adapter := NewCoordinatorReachabilityAdapter(mockCoord)
		if adapter.IsDirectlyReachable(context.Background()) {
			t.Error("expected IsDirectlyReachable=false")
		}
	})

	t.Run("nil coordinator returns false", func(t *testing.T) {
		adapter := NewCoordinatorReachabilityAdapter(nil)
		if adapter.IsDirectlyReachable(context.Background()) {
			t.Error("expected IsDirectlyReachable=false for nil coordinator")
		}
	})
}

// ============================================================================
//                              工厂函数测试
// ============================================================================

func TestCreateAddressBookAdapter(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := CreateAddressBookAdapter(nil)
		if result != nil {
			t.Error("expected nil result")
		}
	})

	t.Run("valid address book returns adapter", func(t *testing.T) {
		mockAB := newMockAddressBook()
		result := CreateAddressBookAdapter(mockAB)
		if result == nil {
			t.Error("expected non-nil adapter")
		}
	})

	t.Run("invalid type returns nil", func(t *testing.T) {
		result := CreateAddressBookAdapter("invalid")
		if result != nil {
			t.Error("expected nil for invalid type")
		}
	})
}

func TestCreateReachabilityAdapter(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := CreateReachabilityAdapter(nil)
		if result != nil {
			t.Error("expected nil result")
		}
	})

	t.Run("valid coordinator returns adapter", func(t *testing.T) {
		mockCoord := newMockCoordinator()
		result := CreateReachabilityAdapter(mockCoord)
		if result == nil {
			t.Error("expected non-nil adapter")
		}
	})

	t.Run("invalid type returns nil", func(t *testing.T) {
		result := CreateReachabilityAdapter("invalid")
		if result != nil {
			t.Error("expected nil for invalid type")
		}
	})
}

// ============================================================================
//                              接口验证测试
// ============================================================================

func TestInterfaceImplementation(t *testing.T) {
	t.Run("RelayAddressBookAdapter implements AddressBookProvider", func(t *testing.T) {
		var _ AddressBookProvider = (*RelayAddressBookAdapter)(nil)
	})

	t.Run("CoordinatorReachabilityAdapter implements ReachabilityChecker", func(t *testing.T) {
		var _ ReachabilityChecker = (*CoordinatorReachabilityAdapter)(nil)
	})
}
