package relay

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

type transportifConn = transportif.Conn

// ============================================================================
//                              Mock 实现
// ============================================================================

type mockRelayAddress struct {
	addr string
}

func (a *mockRelayAddress) Network() string   { return "relay" }
func (a *mockRelayAddress) String() string    { return a.addr }
func (a *mockRelayAddress) Bytes() []byte     { return []byte(a.addr) }
func (a *mockRelayAddress) IsPublic() bool    { return true }
func (a *mockRelayAddress) IsPrivate() bool   { return false }
func (a *mockRelayAddress) IsLoopback() bool  { return false }
func (a *mockRelayAddress) Multiaddr() string { return a.addr } // relay 地址通常已是 multiaddr 格式
func (a *mockRelayAddress) Equal(other endpoint.Address) bool {
	return a.addr == other.String()
}

// ============================================================================
//                              RelayTransport Tests
// ============================================================================

func TestRelayTransport_Protocols(t *testing.T) {
	rt := &RelayTransport{
		incoming: make(chan transportifConn, 32),
	}

	protocols := rt.Protocols()
	if len(protocols) != 1 {
		t.Fatalf("Protocols() = %v, want 1 protocol", protocols)
	}
	if protocols[0] != types.RelayAddrProtocol {
		t.Errorf("Protocols()[0] = %q, want %q", protocols[0], types.RelayAddrProtocol)
	}
}

func TestRelayTransport_Proxy(t *testing.T) {
	rt := &RelayTransport{
		incoming: make(chan transportifConn, 32),
	}

	if !rt.Proxy() {
		t.Error("Proxy() = false, want true (relay is a proxy transport)")
	}
}

func TestRelayTransport_CanDial(t *testing.T) {
	rt := &RelayTransport{
		incoming: make(chan transportifConn, 32),
	}

	tests := []struct {
		name    string
		addr    string
		canDial bool
	}{
		{
			name:    "Relay address with dest",
			addr:    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest",
			canDial: true,
		},
		{
			name:    "Relay address without dest",
			addr:    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit",
			canDial: true,
		},
		{
			name:    "Direct QUIC address",
			addr:    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmNode",
			canDial: false,
		},
		{
			name:    "Direct TCP address",
			addr:    "/ip4/1.2.3.4/tcp/4001",
			canDial: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := &mockRelayAddress{addr: tt.addr}
			if rt.CanDial(addr) != tt.canDial {
				t.Errorf("CanDial(%q) = %v, want %v", tt.addr, !tt.canDial, tt.canDial)
			}
		})
	}
}

func TestRelayTransport_Dial_InvalidAddress(t *testing.T) {
	rt := &RelayTransport{
		incoming: make(chan transportifConn, 32),
	}

	// Test dial with non-relay address
	addr := &mockRelayAddress{addr: "/ip4/1.2.3.4/udp/4001/quic-v1"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := rt.Dial(ctx, addr)
	if err == nil {
		t.Error("Dial() with non-relay address should fail")
	}
}

func TestRelayTransport_Dial_MissingDest(t *testing.T) {
	rt := &RelayTransport{
		incoming: make(chan transportifConn, 32),
	}

	// Test dial with relay address but no destination
	addr := &mockRelayAddress{addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := rt.Dial(ctx, addr)
	if err == nil {
		t.Error("Dial() with relay address missing dest should fail")
	}
}

func TestRelayTransport_Dial_Closed(t *testing.T) {
	rt := &RelayTransport{
		incoming: make(chan transportifConn, 32),
	}

	// Close the transport
	rt.Close()

	addr := &mockRelayAddress{addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := rt.Dial(ctx, addr)
	if err == nil {
		t.Error("Dial() on closed transport should fail")
	}
}

func TestRelayTransport_Close(t *testing.T) {
	rt := &RelayTransport{
		incoming: make(chan transportifConn, 32),
	}

	// First close should succeed
	err := rt.Close()
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// Second close should be idempotent
	err = rt.Close()
	if err != nil {
		t.Fatalf("Second Close() error: %v", err)
	}
}

// ============================================================================
//                              relayListener Tests
// ============================================================================

func TestRelayListener_Accept_Closed(t *testing.T) {
	listener := &relayListener{
		incoming: make(chan transportifConn, 32),
		closeCh:  make(chan struct{}),
	}

	// Close the listener
	listener.Close()

	_, err := listener.Accept()
	if err == nil {
		t.Error("Accept() on closed listener should fail")
	}
}

func TestRelayListener_Close_Idempotent(t *testing.T) {
	listener := &relayListener{
		incoming: make(chan transportifConn, 32),
		closeCh:  make(chan struct{}),
	}

	// First close
	err := listener.Close()
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// Second close should be idempotent
	err = listener.Close()
	if err != nil {
		t.Fatalf("Second Close() error: %v", err)
	}
}

func TestRelayListener_Addr(t *testing.T) {
	addr := &mockRelayAddress{addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit"}
	listener := &relayListener{
		addr:     addr,
		incoming: make(chan transportifConn, 32),
		closeCh:  make(chan struct{}),
	}

	if listener.Addr().String() != addr.String() {
		t.Errorf("Addr() = %q, want %q", listener.Addr().String(), addr.String())
	}
}

func TestRelayListener_Multiaddr(t *testing.T) {
	addr := &mockRelayAddress{addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit"}
	listener := &relayListener{
		addr:     addr,
		incoming: make(chan transportifConn, 32),
		closeCh:  make(chan struct{}),
	}

	if listener.Multiaddr() != addr.String() {
		t.Errorf("Multiaddr() = %q, want %q", listener.Multiaddr(), addr.String())
	}
}


