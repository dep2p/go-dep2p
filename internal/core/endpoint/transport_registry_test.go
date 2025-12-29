package endpoint

import (
	"context"
	"testing"

	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// ============================================================================
//                              Mock Transport
// ============================================================================

type mockTransport struct {
	protocols []string
	proxy     bool
	canDial   func(addr endpointif.Address) bool
	closed    bool
}

func newMockTransport(protocols []string, proxy bool) *mockTransport {
	return &mockTransport{
		protocols: protocols,
		proxy:     proxy,
		canDial: func(addr endpointif.Address) bool {
			return true
		},
	}
}

func (m *mockTransport) Dial(ctx context.Context, addr endpointif.Address) (transportif.Conn, error) {
	return nil, nil
}

func (m *mockTransport) DialWithOptions(ctx context.Context, addr endpointif.Address, opts transportif.DialOptions) (transportif.Conn, error) {
	return nil, nil
}

func (m *mockTransport) Listen(addr endpointif.Address) (transportif.Listener, error) {
	return nil, nil
}

func (m *mockTransport) ListenWithOptions(addr endpointif.Address, opts transportif.ListenOptions) (transportif.Listener, error) {
	return nil, nil
}

func (m *mockTransport) Protocols() []string {
	return m.protocols
}

func (m *mockTransport) CanDial(addr endpointif.Address) bool {
	if m.canDial != nil {
		return m.canDial(addr)
	}
	return true
}

func (m *mockTransport) Proxy() bool {
	return m.proxy
}

func (m *mockTransport) Close() error {
	m.closed = true
	return nil
}

// Mock Address
type mockAddress struct {
	addr string
}

func (a *mockAddress) String() string { return a.addr }
func (a *mockAddress) Network() string {
	if len(a.addr) > 4 && a.addr[:4] == "/ip4" {
		return "ip4"
	}
	return "unknown"
}
func (a *mockAddress) Bytes() []byte { return []byte(a.addr) }
func (a *mockAddress) Equal(other endpointif.Address) bool {
	return a.addr == other.String()
}
func (a *mockAddress) IsPublic() bool   { return true }
func (a *mockAddress) IsPrivate() bool  { return false }
func (a *mockAddress) IsLoopback() bool { return false }
func (a *mockAddress) Multiaddr() string {
	// 如果已经是 multiaddr 格式，直接返回
	if len(a.addr) > 0 && a.addr[0] == '/' {
		return a.addr
	}
	// 否则简单包装
	return "/ip4/" + a.addr
}

// ============================================================================
//                              TransportRegistry Tests
// ============================================================================

func TestTransportRegistry_AddTransport(t *testing.T) {
	r := NewTransportRegistry()

	quic := newMockTransport([]string{"quic-v1", "quic"}, false)
	err := r.AddTransport(quic)
	if err != nil {
		t.Fatalf("AddTransport(quic) error: %v", err)
	}

	// 验证协议已注册
	protocols := r.Protocols()
	if len(protocols) != 2 {
		t.Errorf("Protocols() = %v, want 2 protocols", protocols)
	}

	// 添加同协议应失败
	quic2 := newMockTransport([]string{"quic-v1"}, false)
	err = r.AddTransport(quic2)
	if err != transportif.ErrTransportExists {
		t.Errorf("AddTransport duplicate: got %v, want ErrTransportExists", err)
	}
}

func TestTransportRegistry_RemoveTransport(t *testing.T) {
	r := NewTransportRegistry()

	quic := newMockTransport([]string{"quic-v1", "quic"}, false)
	_ = r.AddTransport(quic)

	// 移除
	err := r.RemoveTransport("quic-v1")
	if err != nil {
		t.Fatalf("RemoveTransport error: %v", err)
	}

	// 验证所有协议都被移除
	if len(r.Protocols()) != 0 {
		t.Errorf("Protocols after remove: %v, want empty", r.Protocols())
	}

	// 移除不存在的协议
	err = r.RemoveTransport("nonexistent")
	if err != transportif.ErrTransportNotFound {
		t.Errorf("RemoveTransport nonexistent: got %v, want ErrTransportNotFound", err)
	}
}

func TestTransportRegistry_TransportForDialing_Direct(t *testing.T) {
	r := NewTransportRegistry()

	quic := newMockTransport([]string{"quic-v1"}, false)
	quic.canDial = func(addr endpointif.Address) bool {
		return addr.String() == "/ip4/1.2.3.4/udp/4001/quic-v1"
	}
	_ = r.AddTransport(quic)

	tcp := newMockTransport([]string{"tcp"}, false)
	tcp.canDial = func(addr endpointif.Address) bool {
		return addr.String() == "/ip4/1.2.3.4/tcp/4001"
	}
	_ = r.AddTransport(tcp)

	tests := []struct {
		name     string
		addr     string
		wantNil  bool
		wantProto string
	}{
		{
			name:      "QUIC address",
			addr:      "/ip4/1.2.3.4/udp/4001/quic-v1",
			wantNil:   false,
			wantProto: "quic-v1",
		},
		{
			name:      "TCP address",
			addr:      "/ip4/1.2.3.4/tcp/4001",
			wantNil:   false,
			wantProto: "tcp",
		},
		{
			name:    "Unknown address",
			addr:    "/dns4/example.com/ws",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := &mockAddress{addr: tt.addr}
			transport := r.TransportForDialing(addr)

			if tt.wantNil {
				if transport != nil {
					t.Errorf("TransportForDialing(%q) = %v, want nil", tt.addr, transport)
				}
			} else {
				if transport == nil {
					t.Fatalf("TransportForDialing(%q) = nil, want transport", tt.addr)
				}
				protos := transport.Protocols()
				found := false
				for _, p := range protos {
					if p == tt.wantProto {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("TransportForDialing(%q) protocols = %v, want %q", tt.addr, protos, tt.wantProto)
				}
			}
		})
	}
}

func TestTransportRegistry_TransportForDialing_Relay(t *testing.T) {
	r := NewTransportRegistry()

	relay := newMockTransport([]string{"p2p-circuit"}, true)
	_ = r.AddTransport(relay)

	quic := newMockTransport([]string{"quic-v1"}, false)
	_ = r.AddTransport(quic)

	tests := []struct {
		name      string
		addr      string
		wantProxy bool
	}{
		{
			name:      "Relay address",
			addr:      "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest",
			wantProxy: true,
		},
		{
			name:      "Relay address without dest",
			addr:      "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit",
			wantProxy: true,
		},
		{
			name:      "Direct QUIC address",
			addr:      "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmNode",
			wantProxy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := &mockAddress{addr: tt.addr}
			transport := r.TransportForDialing(addr)

			if transport == nil {
				t.Fatalf("TransportForDialing(%q) = nil", tt.addr)
			}

			if transport.Proxy() != tt.wantProxy {
				t.Errorf("TransportForDialing(%q).Proxy() = %v, want %v",
					tt.addr, transport.Proxy(), tt.wantProxy)
			}
		})
	}
}

func TestTransportRegistry_Transports_Dedup(t *testing.T) {
	r := NewTransportRegistry()

	// 一个传输注册多个协议
	quic := newMockTransport([]string{"quic-v1", "quic"}, false)
	_ = r.AddTransport(quic)

	transports := r.Transports()
	if len(transports) != 1 {
		t.Errorf("Transports() = %d transports, want 1 (deduped)", len(transports))
	}
}

func TestTransportRegistry_Close(t *testing.T) {
	r := NewTransportRegistry()

	quic := newMockTransport([]string{"quic-v1"}, false)
	tcp := newMockTransport([]string{"tcp"}, false)
	_ = r.AddTransport(quic)
	_ = r.AddTransport(tcp)

	err := r.Close()
	if err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	if !quic.closed {
		t.Error("QUIC transport not closed")
	}
	if !tcp.closed {
		t.Error("TCP transport not closed")
	}

	if len(r.Protocols()) != 0 {
		t.Errorf("Protocols after Close: %v, want empty", r.Protocols())
	}
}

// ============================================================================
//                              AddressRanker Tests
// ============================================================================

func TestDefaultAddressRanker_RankAddresses(t *testing.T) {
	ranker := &DefaultAddressRanker{}

	addrs := []endpointif.Address{
		&mockAddress{addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest"},
		&mockAddress{addr: "/ip4/5.6.7.8/udp/4001/quic-v1"},
		&mockAddress{addr: "/ip4/9.10.11.12/tcp/4001"},
		&mockAddress{addr: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay2/p2p-circuit"},
	}

	ranked := ranker.RankAddresses(addrs)

	if len(ranked) != 4 {
		t.Fatalf("RankAddresses returned %d addresses, want 4", len(ranked))
	}

	// 前两个应该是直连地址
	for i := 0; i < 2; i++ {
		addr := ranked[i].String()
		if addr != "/ip4/5.6.7.8/udp/4001/quic-v1" && addr != "/ip4/9.10.11.12/tcp/4001" {
			t.Errorf("ranked[%d] = %q, want direct address", i, addr)
		}
	}

	// 后两个应该是中继地址
	for i := 2; i < 4; i++ {
		addr := ranked[i].String()
		if addr == "/ip4/5.6.7.8/udp/4001/quic-v1" || addr == "/ip4/9.10.11.12/tcp/4001" {
			t.Errorf("ranked[%d] = %q, want relay address", i, addr)
		}
	}
}

func TestDefaultAddressRanker_EmptyAndSingle(t *testing.T) {
	ranker := &DefaultAddressRanker{}

	// 空数组
	empty := ranker.RankAddresses(nil)
	if len(empty) != 0 {
		t.Errorf("RankAddresses(nil) = %v, want empty", empty)
	}

	// 单个地址
	single := []endpointif.Address{&mockAddress{addr: "/ip4/1.2.3.4/tcp/4001"}}
	ranked := ranker.RankAddresses(single)
	if len(ranked) != 1 || ranked[0].String() != "/ip4/1.2.3.4/tcp/4001" {
		t.Errorf("RankAddresses(single) = %v, want original", ranked)
	}
}

