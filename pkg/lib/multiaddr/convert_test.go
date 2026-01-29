package multiaddr

import (
	"net"
	"testing"
)

// TestMultiaddr_ToTCPAddr 测试转换为 TCP 地址
func TestMultiaddr_ToTCPAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantIP   string
		wantPort int
		wantErr  bool
	}{
		{
			"IPv4 + TCP",
			"/ip4/127.0.0.1/tcp/4001",
			"127.0.0.1",
			4001,
			false,
		},
		{
			"IPv6 + TCP",
			"/ip6/::1/tcp/8080",
			"::1",
			8080,
			false,
		},
		{
			"No TCP",
			"/ip4/127.0.0.1",
			"",
			0,
			true,
		},
		{
			"UDP instead of TCP",
			"/ip4/127.0.0.1/udp/4001",
			"",
			0,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := NewMultiaddr(tt.addr)
			if err != nil {
				t.Fatalf("NewMultiaddr() error = %v", err)
			}

			tcpAddr, err := ma.ToTCPAddr()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToTCPAddr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tcpAddr.IP.String() != tt.wantIP {
					t.Errorf("ToTCPAddr() IP = %v, want %v", tcpAddr.IP.String(), tt.wantIP)
				}
				if tcpAddr.Port != tt.wantPort {
					t.Errorf("ToTCPAddr() Port = %v, want %v", tcpAddr.Port, tt.wantPort)
				}
			}
		})
	}
}

// TestMultiaddr_ToUDPAddr 测试转换为 UDP 地址
func TestMultiaddr_ToUDPAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantIP   string
		wantPort int
		wantErr  bool
	}{
		{
			"IPv4 + UDP",
			"/ip4/192.168.1.1/udp/5000",
			"192.168.1.1",
			5000,
			false,
		},
		{
			"IPv6 + UDP",
			"/ip6/fe80::1/udp/9000",
			"fe80::1",
			9000,
			false,
		},
		{
			"No UDP",
			"/ip4/192.168.1.1",
			"",
			0,
			true,
		},
		{
			"TCP instead of UDP",
			"/ip4/192.168.1.1/tcp/5000",
			"",
			0,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := NewMultiaddr(tt.addr)
			if err != nil {
				t.Fatalf("NewMultiaddr() error = %v", err)
			}

			udpAddr, err := ma.ToUDPAddr()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToUDPAddr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if udpAddr.IP.String() != tt.wantIP {
					t.Errorf("ToUDPAddr() IP = %v, want %v", udpAddr.IP.String(), tt.wantIP)
				}
				if udpAddr.Port != tt.wantPort {
					t.Errorf("ToUDPAddr() Port = %v, want %v", udpAddr.Port, tt.wantPort)
				}
			}
		})
	}
}

// TestFromTCPAddr 测试从 TCP 地址创建多地址
func TestFromTCPAddr(t *testing.T) {
	tests := []struct {
		name     string
		tcpAddr  *net.TCPAddr
		wantAddr string
	}{
		{
			"IPv4",
			&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4001},
			"/ip4/127.0.0.1/tcp/4001",
		},
		{
			"IPv6",
			&net.TCPAddr{IP: net.ParseIP("::1"), Port: 8080},
			"/ip6/::1/tcp/8080",
		},
		{
			"IPv4-mapped IPv6",
			&net.TCPAddr{IP: net.ParseIP("::ffff:192.168.1.1"), Port: 9000},
			"/ip4/192.168.1.1/tcp/9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := FromTCPAddr(tt.tcpAddr)
			if err != nil {
				t.Fatalf("FromTCPAddr() error = %v", err)
			}

			if ma.String() != tt.wantAddr {
				t.Errorf("FromTCPAddr() = %v, want %v", ma.String(), tt.wantAddr)
			}
		})
	}
}

// TestFromUDPAddr 测试从 UDP 地址创建多地址
func TestFromUDPAddr(t *testing.T) {
	tests := []struct {
		name     string
		udpAddr  *net.UDPAddr
		wantAddr string
	}{
		{
			"IPv4",
			&net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5000},
			"/ip4/192.168.1.1/udp/5000",
		},
		{
			"IPv6",
			&net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: 6000},
			"/ip6/fe80::1/udp/6000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := FromUDPAddr(tt.udpAddr)
			if err != nil {
				t.Fatalf("FromUDPAddr() error = %v", err)
			}

			if ma.String() != tt.wantAddr {
				t.Errorf("FromUDPAddr() = %v, want %v", ma.String(), tt.wantAddr)
			}
		})
	}
}

// TestFromNetAddr 测试从 net.Addr 创建多地址
func TestFromNetAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     net.Addr
		wantAddr string
		wantErr  bool
	}{
		{
			"TCP IPv4",
			&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4001},
			"/ip4/127.0.0.1/tcp/4001",
			false,
		},
		{
			"UDP IPv4",
			&net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5000},
			"/ip4/192.168.1.1/udp/5000",
			false,
		},
		{
			"TCP IPv6",
			&net.TCPAddr{IP: net.ParseIP("::1"), Port: 8080},
			"/ip6/::1/tcp/8080",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := FromNetAddr(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromNetAddr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && ma.String() != tt.wantAddr {
				t.Errorf("FromNetAddr() = %v, want %v", ma.String(), tt.wantAddr)
			}
		})
	}
}

// TestRoundTripNetAddr 测试 net.Addr 往返转换
func TestRoundTripNetAddr(t *testing.T) {
	t.Run("TCP", func(t *testing.T) {
		original := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4001}
		ma, _ := FromTCPAddr(original)
		result, _ := ma.ToTCPAddr()

		if !original.IP.Equal(result.IP) || original.Port != result.Port {
			t.Errorf("TCP round trip failed: got %v, want %v", result, original)
		}
	})

	t.Run("UDP", func(t *testing.T) {
		original := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5000}
		ma, _ := FromUDPAddr(original)
		result, _ := ma.ToUDPAddr()

		if !original.IP.Equal(result.IP) || original.Port != result.Port {
			t.Errorf("UDP round trip failed: got %v, want %v", result, original)
		}
	})
}

// BenchmarkFromTCPAddr 基准测试 TCP 转多地址
func BenchmarkFromTCPAddr(b *testing.B) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4001}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = FromTCPAddr(addr)
	}
}

// BenchmarkToTCPAddr 基准测试多地址转 TCP
func BenchmarkToTCPAddr(b *testing.B) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ma.ToTCPAddr()
	}
}

// BenchmarkFromUDPAddr 基准测试 UDP 转多地址
func BenchmarkFromUDPAddr(b *testing.B) {
	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5000}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = FromUDPAddr(addr)
	}
}

// BenchmarkToUDPAddr 基准测试多地址转 UDP
func BenchmarkToUDPAddr(b *testing.B) {
	ma, _ := NewMultiaddr("/ip4/192.168.1.1/udp/5000")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ma.ToUDPAddr()
	}
}
