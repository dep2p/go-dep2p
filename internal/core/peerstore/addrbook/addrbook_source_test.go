package addrbook_test

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/peerstore/addrbook"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              AddressSource 测试
// ============================================================================

func TestAddressSource_String(t *testing.T) {
	tests := []struct {
		source   addrbook.AddressSource
		expected string
	}{
		{addrbook.SourceUnknown, "unknown"},
		{addrbook.SourceManual, "manual"},
		{addrbook.SourcePeerstore, "peerstore"},
		{addrbook.SourceMemberList, "memberlist"},
		{addrbook.SourceRelay, "relay"},
		{addrbook.SourceDHT, "dht"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.source.String(); got != tt.expected {
				t.Errorf("AddressSource.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// ============================================================================
//                              AddrBook 来源支持测试
// ============================================================================

func TestAddrBook_AddAddrsWithSource(t *testing.T) {
	ab := addrbook.New()
	defer ab.Close()

	peerID := types.PeerID("peer-1")
	addr1, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/192.168.1.1/udp/4001/quic")

	// 添加带来源的地址
	addrs := []addrbook.AddressWithSource{
		{Addr: addr1, Source: addrbook.SourceMemberList, TTL: 30 * time.Minute},
		{Addr: addr2, Source: addrbook.SourceRelay, TTL: 15 * time.Minute},
	}
	ab.AddAddrsWithSource(peerID, addrs)

	// 验证地址数量
	result := ab.Addrs(peerID)
	if len(result) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(result))
	}
}

func TestAddrBook_AddrsWithSource(t *testing.T) {
	ab := addrbook.New()
	defer ab.Close()

	peerID := types.PeerID("peer-1")
	addr1, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/192.168.1.1/udp/4001/quic")

	// 添加带来源的地址
	addrs := []addrbook.AddressWithSource{
		{Addr: addr1, Source: addrbook.SourceMemberList, TTL: 30 * time.Minute},
		{Addr: addr2, Source: addrbook.SourceRelay, TTL: 15 * time.Minute},
	}
	ab.AddAddrsWithSource(peerID, addrs)

	// 获取带来源的地址
	result := ab.AddrsWithSource(peerID)
	if len(result) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(result))
	}

	// 验证来源标记
	sourceMap := make(map[string]addrbook.AddressSource)
	for _, aws := range result {
		sourceMap[aws.Addr.String()] = aws.Source
	}

	if sourceMap[addr1.String()] != addrbook.SourceMemberList {
		t.Errorf("Expected source MemberList for addr1, got %v", sourceMap[addr1.String()])
	}
	if sourceMap[addr2.String()] != addrbook.SourceRelay {
		t.Errorf("Expected source Relay for addr2, got %v", sourceMap[addr2.String()])
	}
}

func TestAddrBook_GetAddrSource(t *testing.T) {
	ab := addrbook.New()
	defer ab.Close()

	peerID := types.PeerID("peer-1")
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")

	// 添加带来源的地址
	addrs := []addrbook.AddressWithSource{
		{Addr: addr, Source: addrbook.SourceDHT, TTL: 10 * time.Minute},
	}
	ab.AddAddrsWithSource(peerID, addrs)

	// 获取地址来源
	source := ab.GetAddrSource(peerID, addr)
	if source != addrbook.SourceDHT {
		t.Errorf("Expected source DHT, got %v", source)
	}

	// 不存在的地址返回 Unknown
	nonExistAddr, _ := types.NewMultiaddr("/ip4/10.0.0.1/tcp/4001")
	source = ab.GetAddrSource(peerID, nonExistAddr)
	if source != addrbook.SourceUnknown {
		t.Errorf("Expected source Unknown for non-exist addr, got %v", source)
	}

	// 不存在的 peer 返回 Unknown
	source = ab.GetAddrSource(types.PeerID("non-exist"), addr)
	if source != addrbook.SourceUnknown {
		t.Errorf("Expected source Unknown for non-exist peer, got %v", source)
	}
}

func TestAddrBook_SetAddrsWithSource(t *testing.T) {
	ab := addrbook.New()
	defer ab.Close()

	peerID := types.PeerID("peer-1")
	addr1, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/192.168.1.2/tcp/4001")
	addr3, _ := types.NewMultiaddr("/ip4/192.168.1.3/tcp/4001")

	// 先添加一些地址
	ab.AddAddrsWithSource(peerID, []addrbook.AddressWithSource{
		{Addr: addr1, Source: addrbook.SourceMemberList, TTL: 30 * time.Minute},
		{Addr: addr2, Source: addrbook.SourceRelay, TTL: 15 * time.Minute},
	})

	// 使用 SetAddrsWithSource 覆盖
	ab.SetAddrsWithSource(peerID, []addrbook.AddressWithSource{
		{Addr: addr3, Source: addrbook.SourceManual, TTL: 24 * time.Hour},
	})

	// 验证只有新地址
	result := ab.Addrs(peerID)
	if len(result) != 1 {
		t.Errorf("Expected 1 address after set, got %d", len(result))
	}

	// 验证来源
	source := ab.GetAddrSource(peerID, addr3)
	if source != addrbook.SourceManual {
		t.Errorf("Expected source Manual, got %v", source)
	}
}

func TestAddrBook_AddAddrs_DefaultSource(t *testing.T) {
	ab := addrbook.New()
	defer ab.Close()

	peerID := types.PeerID("peer-1")
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")

	// 使用标准方法添加（不带来源）
	ab.AddAddrs(peerID, []types.Multiaddr{addr}, time.Hour)

	// 来源应该是 Unknown
	source := ab.GetAddrSource(peerID, addr)
	if source != addrbook.SourceUnknown {
		t.Errorf("Expected source Unknown for standard AddAddrs, got %v", source)
	}
}

func TestAddrBook_UpdateSourceOnExisting(t *testing.T) {
	ab := addrbook.New()
	defer ab.Close()

	peerID := types.PeerID("peer-1")
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")

	// 先添加来源为 MemberList
	ab.AddAddrsWithSource(peerID, []addrbook.AddressWithSource{
		{Addr: addr, Source: addrbook.SourceMemberList, TTL: 30 * time.Minute},
	})

	// 再次添加同一地址，来源为 Relay
	ab.AddAddrsWithSource(peerID, []addrbook.AddressWithSource{
		{Addr: addr, Source: addrbook.SourceRelay, TTL: 15 * time.Minute},
	})

	// 来源应该更新为 Relay
	source := ab.GetAddrSource(peerID, addr)
	if source != addrbook.SourceRelay {
		t.Errorf("Expected source Relay after update, got %v", source)
	}
}
