package addrutil

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// createTestNodeID creates a deterministic NodeID for testing
func createTestNodeID(seed byte) types.NodeID {
	var id types.NodeID
	for i := 0; i < 32; i++ {
		id[i] = byte((int(seed)*17 + i*31) % 256)
	}
	return id
}

func TestParseFullAddr_Valid(t *testing.T) {
	peerID := createTestNodeID(1)
	peerIDStr := peerID.String()

	tests := []struct {
		name         string
		fullAddr     string
		wantDialAddr string
	}{
		{
			name:         "ip4 udp quic",
			fullAddr:     "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + peerIDStr,
			wantDialAddr: "/ip4/1.2.3.4/udp/4001/quic-v1",
		},
		{
			name:         "ip6 udp quic",
			fullAddr:     "/ip6/::1/udp/4001/quic-v1/p2p/" + peerIDStr,
			wantDialAddr: "/ip6/::1/udp/4001/quic-v1",
		},
		{
			name:         "dns4",
			fullAddr:     "/dns4/bootstrap.dep2p.io/udp/4001/quic-v1/p2p/" + peerIDStr,
			wantDialAddr: "/dns4/bootstrap.dep2p.io/udp/4001/quic-v1",
		},
		{
			name:         "ip4 tcp",
			fullAddr:     "/ip4/192.168.1.1/tcp/8000/p2p/" + peerIDStr,
			wantDialAddr: "/ip4/192.168.1.1/tcp/8000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPeerID, gotDialAddr, err := ParseFullAddr(tt.fullAddr)
			if err != nil {
				t.Fatalf("ParseFullAddr(%q) error: %v", tt.fullAddr, err)
			}

			if !gotPeerID.Equal(peerID) {
				t.Errorf("ParseFullAddr() peerID = %v, want %v", gotPeerID, peerID)
			}

			if gotDialAddr != tt.wantDialAddr {
				t.Errorf("ParseFullAddr() dialAddr = %q, want %q", gotDialAddr, tt.wantDialAddr)
			}
		})
	}
}

func TestParseFullAddr_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		fullAddr string
		wantErr  error
	}{
		{
			name:     "empty",
			fullAddr: "",
			wantErr:  ErrEmptyAddress,
		},
		{
			name:     "no p2p",
			fullAddr: "/ip4/1.2.3.4/udp/4001/quic-v1",
			wantErr:  ErrMissingPeerID,
		},
		{
			name:     "p2p not at end",
			fullAddr: "/ip4/1.2.3.4/p2p/abc/udp/4001",
			wantErr:  ErrPeerIDNotAtEnd,
		},
		{
			name:     "invalid peer ID",
			fullAddr: "/ip4/1.2.3.4/udp/4001/p2p/invalid",
			wantErr:  ErrInvalidPeerID,
		},
		{
			name:     "only p2p",
			fullAddr: "/p2p/abc",
			wantErr:  ErrInvalidPeerID, // invalid NodeID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseFullAddr(tt.fullAddr)
			if err == nil {
				t.Errorf("ParseFullAddr(%q) expected error, got nil", tt.fullAddr)
			}
		})
	}
}

func TestBuildFullAddr(t *testing.T) {
	peerID := createTestNodeID(2)

	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{
			name:    "valid addr",
			addr:    "/ip4/1.2.3.4/udp/4001/quic-v1",
			wantErr: false,
		},
		{
			name:    "empty addr",
			addr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildFullAddr(tt.addr, peerID)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildFullAddr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the result can be parsed back
				gotID, gotAddr, err := ParseFullAddr(result)
				if err != nil {
					t.Fatalf("ParseFullAddr(BuildFullAddr()) error: %v", err)
				}

				if !gotID.Equal(peerID) {
					t.Errorf("Round trip peerID mismatch")
				}

				if gotAddr != tt.addr {
					t.Errorf("Round trip addr = %q, want %q", gotAddr, tt.addr)
				}
			}
		})
	}
}

func TestBuildFullAddr_AlreadyHasPeerID(t *testing.T) {
	peerID := createTestNodeID(3)
	addr := "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + peerID.String()

	// Same peerID should succeed
	result, err := BuildFullAddr(addr, peerID)
	if err != nil {
		t.Fatalf("BuildFullAddr() with same peerID error: %v", err)
	}
	if result != addr {
		t.Errorf("BuildFullAddr() = %q, want %q", result, addr)
	}

	// Different peerID should fail
	otherID := createTestNodeID(4)
	_, err = BuildFullAddr(addr, otherID)
	if err == nil {
		t.Error("BuildFullAddr() with different peerID should fail")
	}
}

// TestBuildFullAddr_RelayDialAddr tests BuildFullAddr with Relay dial addresses
// (addresses ending with /p2p-circuit, needing target /p2p/<peerID> appended)
func TestBuildFullAddr_RelayDialAddr(t *testing.T) {
	relayID := createTestNodeID(10)
	selfID := createTestNodeID(11)

	// Relay dial address (ends with /p2p-circuit)
	relayDialAddr := "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayID.String() + "/p2p-circuit"

	// Should successfully append /p2p/<selfID>
	result, err := BuildFullAddr(relayDialAddr, selfID)
	if err != nil {
		t.Fatalf("BuildFullAddr(relayDialAddr, selfID) error: %v", err)
	}

	expected := relayDialAddr + "/p2p/" + selfID.String()
	if result != expected {
		t.Errorf("BuildFullAddr() = %q, want %q", result, expected)
	}

	// Verify the result is a valid relay full address
	gotRelayID, gotTargetID, _, err := ParseRelayAddr(result)
	if err != nil {
		t.Fatalf("ParseRelayAddr(result) error: %v", err)
	}
	if !gotRelayID.Equal(relayID) {
		t.Errorf("ParseRelayAddr() relayID mismatch")
	}
	if !gotTargetID.Equal(selfID) {
		t.Errorf("ParseRelayAddr() targetID mismatch")
	}
}

// TestBuildFullAddr_RelayFullAddr tests BuildFullAddr with complete Relay addresses
// (addresses already containing /p2p-circuit/p2p/<target>)
func TestBuildFullAddr_RelayFullAddr(t *testing.T) {
	relayID := createTestNodeID(12)
	targetID := createTestNodeID(13)
	otherID := createTestNodeID(14)

	// Complete relay full address
	relayFullAddr := "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayID.String() +
		"/p2p-circuit/p2p/" + targetID.String()

	// Same targetID should succeed and return original address
	result, err := BuildFullAddr(relayFullAddr, targetID)
	if err != nil {
		t.Fatalf("BuildFullAddr(relayFullAddr, targetID) error: %v", err)
	}
	if result != relayFullAddr {
		t.Errorf("BuildFullAddr() = %q, want %q", result, relayFullAddr)
	}

	// Different targetID should fail
	_, err = BuildFullAddr(relayFullAddr, otherID)
	if err == nil {
		t.Error("BuildFullAddr() with different targetID should fail")
	}
}

func TestExtractPeerID(t *testing.T) {
	peerID := createTestNodeID(5)

	tests := []struct {
		name    string
		addr    string
		wantID  types.NodeID
		wantErr bool
	}{
		{
			name:    "with p2p",
			addr:    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + peerID.String(),
			wantID:  peerID,
			wantErr: false,
		},
		{
			name:    "without p2p",
			addr:    "/ip4/1.2.3.4/udp/4001/quic-v1",
			wantID:  types.EmptyNodeID,
			wantErr: false,
		},
		{
			name:    "invalid p2p",
			addr:    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/invalid",
			wantID:  types.EmptyNodeID,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := ExtractPeerID(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractPeerID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !gotID.Equal(tt.wantID) {
				t.Errorf("ExtractPeerID() = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestStripPeerID(t *testing.T) {
	peerID := createTestNodeID(6)

	tests := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "with p2p at end",
			addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + peerID.String(),
			want: "/ip4/1.2.3.4/udp/4001/quic-v1",
		},
		{
			name: "without p2p",
			addr: "/ip4/1.2.3.4/udp/4001/quic-v1",
			want: "/ip4/1.2.3.4/udp/4001/quic-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripPeerID(tt.addr)
			if got != tt.want {
				t.Errorf("StripPeerID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasPeerID(t *testing.T) {
	peerID := createTestNodeID(7)

	tests := []struct {
		name string
		addr string
		want bool
	}{
		{
			name: "with p2p",
			addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + peerID.String(),
			want: true,
		},
		{
			name: "without p2p",
			addr: "/ip4/1.2.3.4/udp/4001/quic-v1",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasPeerID(tt.addr); got != tt.want {
				t.Errorf("HasPeerID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRelayAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{
			name: "relay address",
			addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/RelayID/p2p-circuit/p2p/TargetID",
			want: true,
		},
		{
			name: "direct address",
			addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/NodeID",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRelayAddr(tt.addr); got != tt.want {
				t.Errorf("IsRelayAddr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRelayAddr(t *testing.T) {
	relayID := createTestNodeID(8)
	targetID := createTestNodeID(9)

	relayAddr := "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayID.String() + "/p2p-circuit/p2p/" + targetID.String()

	gotRelayID, gotTargetID, gotBaseAddr, err := ParseRelayAddr(relayAddr)
	if err != nil {
		t.Fatalf("ParseRelayAddr() error: %v", err)
	}

	if !gotRelayID.Equal(relayID) {
		t.Errorf("ParseRelayAddr() relayID = %v, want %v", gotRelayID, relayID)
	}

	if !gotTargetID.Equal(targetID) {
		t.Errorf("ParseRelayAddr() targetID = %v, want %v", gotTargetID, targetID)
	}

	if gotBaseAddr != "/ip4/1.2.3.4/udp/4001/quic-v1" {
		t.Errorf("ParseRelayAddr() baseAddr = %q, unexpected", gotBaseAddr)
	}
}

func TestParseRelayAddr_NotRelay(t *testing.T) {
	addr := "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/NodeID"
	_, _, _, err := ParseRelayAddr(addr)
	if err == nil {
		t.Error("ParseRelayAddr() with non-relay address should fail")
	}
}

