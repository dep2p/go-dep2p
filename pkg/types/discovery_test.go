package types

import (
	"testing"
	"time"
)

func TestPeerInfo_String(t *testing.T) {
	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	pi := PeerInfo{
		ID:    PeerID("12D3KooWTest"),
		Addrs: []Multiaddr{addr},
	}
	s := pi.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestPeerInfo_HasAddrs(t *testing.T) {
	pi := PeerInfo{ID: PeerID("test")}
	if pi.HasAddrs() {
		t.Error("HasAddrs() = true for empty addrs")
	}

	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	pi.Addrs = []Multiaddr{addr}
	if !pi.HasAddrs() {
		t.Error("HasAddrs() = false for non-empty addrs")
	}
}

func TestPeerInfo_IsExpired(t *testing.T) {
	pi := PeerInfo{
		ID:           PeerID("test"),
		DiscoveredAt: time.Now().Add(-2 * time.Hour),
	}

	if !pi.IsExpired(time.Hour) {
		t.Error("IsExpired(1h) = false for 2h old")
	}

	pi.DiscoveredAt = time.Now()
	if pi.IsExpired(time.Hour) {
		t.Error("IsExpired(1h) = true for fresh")
	}
}

func TestPeerInfo_AddrsToStrings(t *testing.T) {
	addr1, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := NewMultiaddr("/ip6/::1/tcp/4001")
	
	pi := PeerInfo{
		ID: PeerID("test"),
		Addrs: []Multiaddr{
			addr1,
			addr2,
		},
	}

	strs := pi.AddrsToStrings()
	if len(strs) != 2 {
		t.Errorf("AddrsToStrings() len = %d, want 2", len(strs))
	}
	if strs[0] != "/ip4/127.0.0.1/tcp/4001" {
		t.Errorf("AddrsToStrings()[0] = %q", strs[0])
	}
}

func TestNewPeerInfo(t *testing.T) {
	id := PeerID("12D3KooWTest")
	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addrs := []Multiaddr{addr}

	pi := NewPeerInfo(id, addrs)

	if pi.ID != id {
		t.Errorf("NewPeerInfo().ID = %q, want %q", pi.ID, id)
	}
	if len(pi.Addrs) != 1 {
		t.Errorf("NewPeerInfo().Addrs len = %d", len(pi.Addrs))
	}
	if pi.DiscoveredAt.IsZero() {
		t.Error("NewPeerInfo().DiscoveredAt is zero")
	}
}

func TestNewPeerInfoWithSource(t *testing.T) {
	id := PeerID("12D3KooWTest")
	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addrs := []Multiaddr{addr}

	pi := NewPeerInfoWithSource(id, addrs, SourceDHT)

	if pi.Source != SourceDHT {
		t.Errorf("Source = %q, want %q", pi.Source, SourceDHT)
	}
}

func TestNewPeerInfoFromStrings(t *testing.T) {
	id := PeerID("12D3KooWTest")
	addrStrs := []string{
		"/ip4/127.0.0.1/tcp/4001",
		"/ip6/::1/tcp/4001",
		"invalid", // should be ignored
	}

	pi := NewPeerInfoFromStrings(id, addrStrs)

	if pi.ID != id {
		t.Errorf("ID = %q", pi.ID)
	}
	// Invalid addr should be filtered out
	if len(pi.Addrs) != 2 {
		t.Errorf("Addrs len = %d, want 2", len(pi.Addrs))
	}
}

func TestAddrInfo_String(t *testing.T) {
	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ai := AddrInfo{
		ID:    PeerID("12D3KooWTest"),
		Addrs: []Multiaddr{addr},
	}
	s := ai.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestAddrInfo_HasAddrs(t *testing.T) {
	ai := AddrInfo{ID: PeerID("test")}
	if ai.HasAddrs() {
		t.Error("HasAddrs() = true for empty")
	}

	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ai.Addrs = []Multiaddr{addr}
	if !ai.HasAddrs() {
		t.Error("HasAddrs() = false for non-empty")
	}
}

func TestAddrInfo_ToPeerInfo(t *testing.T) {
	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ai := AddrInfo{
		ID:    PeerID("12D3KooWTest"),
		Addrs: []Multiaddr{addr},
	}

	pi := ai.ToPeerInfo()

	if pi.ID != ai.ID {
		t.Errorf("ToPeerInfo().ID = %q", pi.ID)
	}
	if len(pi.Addrs) != len(ai.Addrs) {
		t.Errorf("ToPeerInfo().Addrs len = %d", len(pi.Addrs))
	}
	if pi.DiscoveredAt.IsZero() {
		t.Error("ToPeerInfo().DiscoveredAt is zero")
	}
}

func TestNewAddrInfo(t *testing.T) {
	id := PeerID("12D3KooWTest")
	addr, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addrs := []Multiaddr{addr}

	ai := NewAddrInfo(id, addrs)

	if ai.ID != id {
		t.Errorf("NewAddrInfo().ID = %q", ai.ID)
	}
	if len(ai.Addrs) != 1 {
		t.Errorf("NewAddrInfo().Addrs len = %d", len(ai.Addrs))
	}
}

func TestAddrInfoFromString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWTest", false},
		{"invalid format", "/ip4/127.0.0.1/tcp/4001", true}, // no p2p
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AddrInfoFromString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddrInfoFromString(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestAddrInfoFromP2pAddr(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWTest")

	ai, err := AddrInfoFromP2pAddr(ma)
	if err != nil {
		t.Fatalf("AddrInfoFromP2pAddr() error = %v", err)
	}

	if ai.ID != "12D3KooWTest" {
		t.Errorf("ID = %q", ai.ID)
	}
	if len(ai.Addrs) != 1 {
		t.Errorf("Addrs len = %d", len(ai.Addrs))
	}
	if ai.Addrs[0].String() != "/ip4/127.0.0.1/tcp/4001" {
		t.Errorf("Addrs[0] = %q", ai.Addrs[0])
	}

	// Empty multiaddr
	_, err = AddrInfoFromP2pAddr(nil)
	if err == nil {
		t.Error("AddrInfoFromP2pAddr(empty) should return error")
	}
}

func TestAddrInfoToP2pAddrs(t *testing.T) {
	addr1, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := NewMultiaddr("/ip6/::1/tcp/4001")
	
	ai := &AddrInfo{
		ID: PeerID("12D3KooWTest"),
		Addrs: []Multiaddr{
			addr1,
			addr2,
		},
	}

	addrs, err := AddrInfoToP2pAddrs(ai)
	if err != nil {
		t.Fatalf("AddrInfoToP2pAddrs() error = %v", err)
	}

	if len(addrs) != 2 {
		t.Errorf("len = %d, want 2", len(addrs))
	}

	// Check format
	for _, addr := range addrs {
		if !HasProtocol(addr, ProtocolP2P) {
			t.Errorf("addr %q should have p2p", addr)
		}
	}

	// Empty addrs
	ai2 := &AddrInfo{ID: PeerID("12D3KooWTest")}
	addrs2, _ := AddrInfoToP2pAddrs(ai2)
	if len(addrs2) != 1 {
		t.Errorf("empty addrs should return just p2p, got %d", len(addrs2))
	}
}

func TestPeerInfoSlice(t *testing.T) {
	slice := PeerInfoSlice{
		{ID: PeerID("c")},
		{ID: PeerID("a")},
		{ID: PeerID("b")},
	}

	if slice.Len() != 3 {
		t.Errorf("Len() = %d", slice.Len())
	}

	if !slice.Less(1, 0) { // "a" < "c"
		t.Error("Less() failed")
	}

	slice.Swap(0, 1)
	if slice[0].ID != "a" {
		t.Error("Swap() failed")
	}
}

func TestAddrInfoSlice(t *testing.T) {
	slice := AddrInfoSlice{
		{ID: PeerID("c")},
		{ID: PeerID("a")},
		{ID: PeerID("b")},
	}

	if slice.Len() != 3 {
		t.Errorf("Len() = %d", slice.Len())
	}

	if !slice.Less(1, 0) {
		t.Error("Less() failed")
	}

	slice.Swap(0, 1)
	if slice[0].ID != "a" {
		t.Error("Swap() failed")
	}
}

func TestExtractPeerIDs(t *testing.T) {
	infos := []PeerInfo{
		{ID: PeerID("a")},
		{ID: PeerID("b")},
		{ID: PeerID("c")},
	}

	ids := ExtractPeerIDs(infos)
	if len(ids) != 3 {
		t.Errorf("len = %d", len(ids))
	}
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Error("IDs not extracted correctly")
	}
}

func TestExtractAddrInfoIDs(t *testing.T) {
	infos := []AddrInfo{
		{ID: PeerID("a")},
		{ID: PeerID("b")},
		{ID: PeerID("c")},
	}

	ids := ExtractAddrInfoIDs(infos)
	if len(ids) != 3 {
		t.Errorf("len = %d", len(ids))
	}
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Error("IDs not extracted correctly")
	}
}
