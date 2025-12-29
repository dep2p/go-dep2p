package tcp

import (
	"testing"
)

func TestParseAddress(t *testing.T) {
	tests := []struct {
		input   string
		network string
		host    string
		port    int
		wantErr bool
	}{
		{"/ip4/127.0.0.1/tcp/4001", "tcp4", "127.0.0.1", 4001, false},
		{"/ip4/0.0.0.0/tcp/0", "tcp4", "0.0.0.0", 0, false},
		{"/ip6/::1/tcp/4001", "tcp6", "::1", 4001, false},
		{"/ip6/::/tcp/8080", "tcp6", "::", 8080, false},
		{"/dns4/example.com/tcp/443", "tcp4", "example.com", 443, false},
		{"/dns6/example.com/tcp/443", "tcp6", "example.com", 443, false},
		{"/ip4/127.0.0.1/udp/4001", "", "", 0, true},
		{"/quic-v1/127.0.0.1/4001", "", "", 0, true},
		{"invalid", "", "", 0, true},
	}

	for _, tt := range tests {
		addr, err := ParseAddress(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseAddress(%q) should fail", tt.input)
			}
			continue
		}

		if err != nil {
			t.Errorf("ParseAddress(%q) failed: %v", tt.input, err)
			continue
		}

		if addr.network != tt.network {
			t.Errorf("ParseAddress(%q).network = %q, want %q", tt.input, addr.network, tt.network)
		}
		if addr.host != tt.host {
			t.Errorf("ParseAddress(%q).host = %q, want %q", tt.input, addr.host, tt.host)
		}
		if addr.port != tt.port {
			t.Errorf("ParseAddress(%q).port = %d, want %d", tt.input, addr.port, tt.port)
		}
	}
}

func TestMustParseAddress(t *testing.T) {
	// 有效地址
	addr := MustParseAddress("/ip4/127.0.0.1/tcp/4001")
	if addr == nil {
		t.Error("MustParseAddress returned nil")
	}

	// 无效地址应该 panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseAddress should panic on invalid address")
		}
	}()
	MustParseAddress("invalid")
}

func TestAddress_String(t *testing.T) {
	tests := []struct {
		network  string
		host     string
		port     int
		expected string
	}{
		{"tcp4", "127.0.0.1", 4001, "/ip4/127.0.0.1/tcp/4001"},
		{"tcp6", "::1", 4001, "/ip6/::1/tcp/4001"},
		{"tcp", "192.168.1.1", 8080, "/ip4/192.168.1.1/tcp/8080"},
	}

	for _, tt := range tests {
		addr := NewAddress(tt.network, tt.host, tt.port)
		result := addr.String()
		if result != tt.expected {
			t.Errorf("Address.String() = %q, want %q", result, tt.expected)
		}
	}
}

func TestAddress_IsPublic(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"127.0.0.1", false},       // loopback
		{"192.168.1.1", false},     // private
		{"10.0.0.1", false},        // private
		{"172.16.0.1", false},      // private
		{"8.8.8.8", true},          // public
		{"1.1.1.1", true},          // public
		{"example.com", true},      // domain
		{"::1", false},             // IPv6 loopback
		{"fe80::1", false},         // link-local
	}

	for _, tt := range tests {
		addr := NewAddress("tcp4", tt.host, 4001)
		result := addr.IsPublic()
		if result != tt.expected {
			t.Errorf("Address(%s).IsPublic() = %v, want %v", tt.host, result, tt.expected)
		}
	}
}

func TestAddress_IsPrivate(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"127.0.0.1", false},       // loopback is not private
		{"192.168.1.1", true},      // private
		{"10.0.0.1", true},         // private
		{"172.16.0.1", true},       // private
		{"8.8.8.8", false},         // public
		{"example.com", false},     // domain
	}

	for _, tt := range tests {
		addr := NewAddress("tcp4", tt.host, 4001)
		result := addr.IsPrivate()
		if result != tt.expected {
			t.Errorf("Address(%s).IsPrivate() = %v, want %v", tt.host, result, tt.expected)
		}
	}
}

func TestAddress_IsLoopback(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"192.168.1.1", false},
		{"::1", true},
		{"example.com", false},
	}

	for _, tt := range tests {
		addr := NewAddress("tcp", tt.host, 4001)
		result := addr.IsLoopback()
		if result != tt.expected {
			t.Errorf("Address(%s).IsLoopback() = %v, want %v", tt.host, result, tt.expected)
		}
	}
}

func TestAddress_Equal(t *testing.T) {
	addr1 := NewAddress("tcp4", "127.0.0.1", 4001)
	addr2 := NewAddress("tcp4", "127.0.0.1", 4001)
	addr3 := NewAddress("tcp4", "127.0.0.1", 4002)
	addr4 := NewAddress("tcp4", "127.0.0.2", 4001)

	if !addr1.Equal(addr2) {
		t.Error("addr1 should equal addr2")
	}
	if addr1.Equal(addr3) {
		t.Error("addr1 should not equal addr3 (different port)")
	}
	if addr1.Equal(addr4) {
		t.Error("addr1 should not equal addr4 (different host)")
	}
	if addr1.Equal(nil) {
		t.Error("addr1 should not equal nil")
	}
}

func TestAddress_NetDialString(t *testing.T) {
	tests := []struct {
		host     string
		port     int
		expected string
	}{
		{"127.0.0.1", 4001, "127.0.0.1:4001"},
		{"::1", 4001, "[::1]:4001"},
		{"example.com", 443, "example.com:443"},
	}

	for _, tt := range tests {
		addr := NewAddress("tcp", tt.host, tt.port)
		result := addr.NetDialString()
		if result != tt.expected {
			t.Errorf("NetDialString() = %q, want %q", result, tt.expected)
		}
	}
}

func TestAddress_Bytes(t *testing.T) {
	addr := NewAddress("tcp4", "127.0.0.1", 4001)
	bytes := addr.Bytes()
	if string(bytes) != addr.String() {
		t.Errorf("Bytes() = %q, want %q", string(bytes), addr.String())
	}
}

func TestAddress_Network(t *testing.T) {
	addr := NewAddress("tcp4", "127.0.0.1", 4001)
	if addr.Network() != "tcp4" {
		t.Errorf("Network() = %q, want %q", addr.Network(), "tcp4")
	}
}

func TestAddress_HostAndPort(t *testing.T) {
	addr := NewAddress("tcp4", "127.0.0.1", 4001)
	if addr.Host() != "127.0.0.1" {
		t.Errorf("Host() = %q, want %q", addr.Host(), "127.0.0.1")
	}
	if addr.Port() != 4001 {
		t.Errorf("Port() = %d, want %d", addr.Port(), 4001)
	}
}

