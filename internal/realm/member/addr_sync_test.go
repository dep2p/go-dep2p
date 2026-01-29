package member_test

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/peerstore"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/internal/realm/member"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              AddrSyncer 测试
// ============================================================================

func TestAddrSyncer_OnMemberJoined(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	syncer := member.NewAddrSyncer(member.AddrSyncerConfig{
		RealmID:   "test-realm",
		Peerstore: ps,
	})

	// 创建成员
	m := &member.Member{
		PeerID:  "peer-1",
		RealmID: "test-realm",
		Addrs:   []string{"/ip4/192.168.1.1/tcp/4001", "/ip4/192.168.1.1/udp/4001/quic"},
		Online:  true,
	}

	// 同步地址
	syncer.OnMemberJoined(m)

	// 验证地址已同步到 Peerstore
	addrs := ps.Addrs(types.PeerID("peer-1"))
	if len(addrs) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(addrs))
	}
}

func TestAddrSyncer_OnMemberUpdated(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	syncer := member.NewAddrSyncer(member.AddrSyncerConfig{
		RealmID:   "test-realm",
		Peerstore: ps,
	})

	// 初始成员
	m := &member.Member{
		PeerID:  "peer-1",
		RealmID: "test-realm",
		Addrs:   []string{"/ip4/192.168.1.1/tcp/4001"},
		Online:  true,
	}
	syncer.OnMemberJoined(m)

	// 更新成员地址
	m.Addrs = []string{"/ip4/10.0.0.1/tcp/4001", "/ip4/10.0.0.1/udp/4001/quic"}
	syncer.OnMemberUpdated(m)

	// 验证地址已更新
	addrs := ps.Addrs(types.PeerID("peer-1"))
	if len(addrs) < 2 {
		t.Errorf("Expected at least 2 addresses after update, got %d", len(addrs))
	}
}

func TestAddrSyncer_OnMemberInfoUpdated(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	syncer := member.NewAddrSyncer(member.AddrSyncerConfig{
		RealmID:   "test-realm",
		Peerstore: ps,
	})

	// 使用 MemberInfo 更新
	info := &interfaces.MemberInfo{
		PeerID:   "peer-2",
		RealmID:  "test-realm",
		Addrs:    []string{"/ip4/192.168.1.2/tcp/4001"},
		Online:   true,
		LastSeen: time.Now(),
	}
	syncer.OnMemberInfoUpdated(info)

	// 验证地址已同步
	addrs := ps.Addrs(types.PeerID("peer-2"))
	if len(addrs) != 1 {
		t.Errorf("Expected 1 address, got %d", len(addrs))
	}
}

func TestAddrSyncer_SyncFromRelay(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	syncer := member.NewAddrSyncer(member.AddrSyncerConfig{
		RealmID:   "test-realm",
		Peerstore: ps,
	})

	// 从 Relay 同步地址
	addr1, _ := types.NewMultiaddr("/ip4/192.168.1.3/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/192.168.1.3/udp/4001/quic")
	syncer.SyncFromRelay(types.PeerID("peer-3"), []types.Multiaddr{addr1, addr2})

	// 验证地址已同步
	addrs := ps.Addrs(types.PeerID("peer-3"))
	if len(addrs) != 2 {
		t.Errorf("Expected 2 addresses from Relay, got %d", len(addrs))
	}

	// 验证来源标记
	addrsWithSource := ps.AddrsWithSource(types.PeerID("peer-3"))
	for _, aws := range addrsWithSource {
		if aws.Source != peerstore.SourceRelay {
			t.Errorf("Expected source Relay, got %v", aws.Source)
		}
	}
}

func TestAddrSyncer_SyncFromDHT(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	syncer := member.NewAddrSyncer(member.AddrSyncerConfig{
		RealmID:   "test-realm",
		Peerstore: ps,
	})

	// 从 DHT 同步地址
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.4/tcp/4001")
	syncer.SyncFromDHT(types.PeerID("peer-4"), []types.Multiaddr{addr})

	// 验证地址已同步
	addrs := ps.Addrs(types.PeerID("peer-4"))
	if len(addrs) != 1 {
		t.Errorf("Expected 1 address from DHT, got %d", len(addrs))
	}

	// 验证来源标记
	addrsWithSource := ps.AddrsWithSource(types.PeerID("peer-4"))
	for _, aws := range addrsWithSource {
		if aws.Source != peerstore.SourceDHT {
			t.Errorf("Expected source DHT, got %v", aws.Source)
		}
	}
}

func TestAddrSyncer_NilPeerstore(t *testing.T) {
	// 测试 Peerstore 为 nil 时不 panic
	syncer := member.NewAddrSyncer(member.AddrSyncerConfig{
		RealmID:   "test-realm",
		Peerstore: nil,
	})

	m := &member.Member{
		PeerID:  "peer-1",
		RealmID: "test-realm",
		Addrs:   []string{"/ip4/192.168.1.1/tcp/4001"},
	}

	// 应该不 panic
	syncer.OnMemberJoined(m)
	syncer.OnMemberUpdated(m)
	syncer.OnMemberLeft("peer-1")
}

func TestAddrSyncer_EmptyAddrs(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	syncer := member.NewAddrSyncer(member.AddrSyncerConfig{
		RealmID:   "test-realm",
		Peerstore: ps,
	})

	// 成员没有地址
	m := &member.Member{
		PeerID:  "peer-1",
		RealmID: "test-realm",
		Addrs:   []string{},
	}

	// 应该不 panic，也不添加任何地址
	syncer.OnMemberJoined(m)

	addrs := ps.Addrs(types.PeerID("peer-1"))
	if len(addrs) != 0 {
		t.Errorf("Expected 0 addresses, got %d", len(addrs))
	}
}

func TestAddrSyncer_InvalidAddrs(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	syncer := member.NewAddrSyncer(member.AddrSyncerConfig{
		RealmID:   "test-realm",
		Peerstore: ps,
	})

	// 包含无效地址
	m := &member.Member{
		PeerID:  "peer-1",
		RealmID: "test-realm",
		Addrs:   []string{"invalid-addr", "/ip4/192.168.1.1/tcp/4001"},
	}

	// 应该跳过无效地址，只同步有效的
	syncer.OnMemberJoined(m)

	addrs := ps.Addrs(types.PeerID("peer-1"))
	if len(addrs) != 1 {
		t.Errorf("Expected 1 valid address, got %d", len(addrs))
	}
}
