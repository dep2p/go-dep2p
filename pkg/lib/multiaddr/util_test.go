package multiaddr

import (
	"testing"
)

// TestSplit 测试分离传输地址和 P2P 组件
func TestSplit(t *testing.T) {
	tests := []struct {
		name          string
		addr          string
		wantTransport string
		wantPeerID    string
	}{
		{
			"With P2P",
			"/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
			"/ip4/127.0.0.1/tcp/4001",
			"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
		},
		{
			"Without P2P",
			"/ip4/127.0.0.1/tcp/4001",
			"/ip4/127.0.0.1/tcp/4001",
			"",
		},
		{
			"Only P2P",
			"/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
			"",
			"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := NewMultiaddr(tt.addr)
			if err != nil {
				t.Fatalf("NewMultiaddr() error = %v", err)
			}

			transport, peerID := Split(ma)

			var transportStr string
			if transport != nil {
				transportStr = transport.String()
			}

			if transportStr != tt.wantTransport {
				t.Errorf("Split() transport = %v, want %v", transportStr, tt.wantTransport)
			}
			if peerID != tt.wantPeerID {
				t.Errorf("Split() peerID = %v, want %v", peerID, tt.wantPeerID)
			}
		})
	}
}

// TestJoin 测试合并传输地址和 P2P 组件
func TestJoin(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		peerID    string
		wantAddr  string
	}{
		{
			"Full address",
			"/ip4/127.0.0.1/tcp/4001",
			"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
			"/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
		},
		{
			"Empty transport",
			"",
			"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
			"/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var transport Multiaddr
			var err error
			if tt.transport != "" {
				transport, err = NewMultiaddr(tt.transport)
				if err != nil {
					t.Fatalf("NewMultiaddr() error = %v", err)
				}
			}

			result := Join(transport, tt.peerID)
			if result.String() != tt.wantAddr {
				t.Errorf("Join() = %v, want %v", result.String(), tt.wantAddr)
			}
		})
	}
}

// TestSplitJoinRoundTrip 测试 Split 和 Join 的往返
func TestSplitJoinRoundTrip(t *testing.T) {
	original := "/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"
	ma, _ := NewMultiaddr(original)

	transport, peerID := Split(ma)
	result := Join(transport, peerID)

	if result.String() != original {
		t.Errorf("Split/Join round trip: got %v, want %v", result.String(), original)
	}
}

// TestFilterAddrs 测试地址过滤
func TestFilterAddrs(t *testing.T) {
	addrs := []Multiaddr{}
	for _, s := range []string{
		"/ip4/127.0.0.1/tcp/4001",
		"/ip4/192.168.1.1/tcp/4001",
		"/ip6/::1/tcp/4001",
		"/ip4/10.0.0.1/udp/5000",
	} {
		ma, _ := NewMultiaddr(s)
		addrs = append(addrs, ma)
	}

	t.Run("Filter TCP only", func(t *testing.T) {
		filtered := FilterAddrs(addrs, func(ma Multiaddr) bool {
			protos := ma.Protocols()
			for _, p := range protos {
				if p.Code == P_TCP {
					return true
				}
			}
			return false
		})

		if len(filtered) != 3 {
			t.Errorf("FilterAddrs() count = %d, want 3", len(filtered))
		}
	})

	t.Run("Filter IPv4 only", func(t *testing.T) {
		filtered := FilterAddrs(addrs, func(ma Multiaddr) bool {
			protos := ma.Protocols()
			return len(protos) > 0 && protos[0].Code == P_IP4
		})

		if len(filtered) != 3 {
			t.Errorf("FilterAddrs() count = %d, want 3", len(filtered))
		}
	})

	t.Run("Filter none", func(t *testing.T) {
		filtered := FilterAddrs(addrs, func(ma Multiaddr) bool {
			return false
		})

		if len(filtered) != 0 {
			t.Errorf("FilterAddrs() count = %d, want 0", len(filtered))
		}
	})
}

// TestUniqueAddrs 测试地址去重
func TestUniqueAddrs(t *testing.T) {
	addrs := []Multiaddr{}
	for _, s := range []string{
		"/ip4/127.0.0.1/tcp/4001",
		"/ip4/127.0.0.1/tcp/4001", // 重复
		"/ip4/192.168.1.1/tcp/4001",
		"/ip4/127.0.0.1/tcp/4001", // 重复
	} {
		ma, _ := NewMultiaddr(s)
		addrs = append(addrs, ma)
	}

	unique := UniqueAddrs(addrs)

	if len(unique) != 2 {
		t.Errorf("UniqueAddrs() count = %d, want 2", len(unique))
	}

	// 验证顺序保持
	if unique[0].String() != "/ip4/127.0.0.1/tcp/4001" {
		t.Error("UniqueAddrs() should preserve order")
	}
}

// TestHasProtocol 测试协议检查
func TestHasProtocol(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

	if !HasProtocol(ma, P_IP4) {
		t.Error("HasProtocol(P_IP4) should be true")
	}

	if !HasProtocol(ma, P_TCP) {
		t.Error("HasProtocol(P_TCP) should be true")
	}

	if !HasProtocol(ma, P_P2P) {
		t.Error("HasProtocol(P_P2P) should be true")
	}

	if HasProtocol(ma, P_UDP) {
		t.Error("HasProtocol(P_UDP) should be false")
	}
}

// BenchmarkSplit 基准测试 Split
func BenchmarkSplit(b *testing.B) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Split(ma)
	}
}

// BenchmarkJoin 基准测试 Join
func BenchmarkJoin(b *testing.B) {
	transport, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	peerID := "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Join(transport, peerID)
	}
}

// BenchmarkFilterAddrs 基准测试 FilterAddrs
func BenchmarkFilterAddrs(b *testing.B) {
	addrs := []Multiaddr{}
	for i := 0; i < 100; i++ {
		ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
		addrs = append(addrs, ma)
	}

	filter := func(ma Multiaddr) bool {
		return HasProtocol(ma, P_TCP)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FilterAddrs(addrs, filter)
	}
}

// TestIsTCPMultiaddr 测试 TCP 检查函数
func TestIsTCPMultiaddr(t *testing.T) {
	tcp, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	udp, _ := NewMultiaddr("/ip4/127.0.0.1/udp/5000")

	if !IsTCPMultiaddr(tcp) {
		t.Error("IsTCPMultiaddr() should return true for TCP address")
	}

	if IsTCPMultiaddr(udp) {
		t.Error("IsTCPMultiaddr() should return false for UDP address")
	}
}

// TestIsUDPMultiaddr 测试 UDP 检查函数
func TestIsUDPMultiaddr(t *testing.T) {
	tcp, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	udp, _ := NewMultiaddr("/ip4/127.0.0.1/udp/5000")

	if !IsUDPMultiaddr(udp) {
		t.Error("IsUDPMultiaddr() should return true for UDP address")
	}

	if IsUDPMultiaddr(tcp) {
		t.Error("IsUDPMultiaddr() should return false for TCP address")
	}
}

// TestIsIP4Multiaddr 测试 IPv4 检查函数
func TestIsIP4Multiaddr(t *testing.T) {
	ip4, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ip6, _ := NewMultiaddr("/ip6/::1/tcp/4001")

	if !IsIP4Multiaddr(ip4) {
		t.Error("IsIP4Multiaddr() should return true for IPv4 address")
	}

	if IsIP4Multiaddr(ip6) {
		t.Error("IsIP4Multiaddr() should return false for IPv6 address")
	}
}

// TestIsIP6Multiaddr 测试 IPv6 检查函数
func TestIsIP6Multiaddr(t *testing.T) {
	ip4, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ip6, _ := NewMultiaddr("/ip6/::1/tcp/4001")

	if !IsIP6Multiaddr(ip6) {
		t.Error("IsIP6Multiaddr() should return true for IPv6 address")
	}

	if IsIP6Multiaddr(ip4) {
		t.Error("IsIP6Multiaddr() should return false for IPv4 address")
	}
}

// TestIsIPMultiaddr 测试 IP 检查函数
func TestIsIPMultiaddr(t *testing.T) {
	ip4, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ip6, _ := NewMultiaddr("/ip6/::1/tcp/4001")
	p2p, _ := NewMultiaddr("/p2p/QmYyQ")

	if !IsIPMultiaddr(ip4) {
		t.Error("IsIPMultiaddr() should return true for IPv4 address")
	}

	if !IsIPMultiaddr(ip6) {
		t.Error("IsIPMultiaddr() should return true for IPv6 address")
	}

	if IsIPMultiaddr(p2p) {
		t.Error("IsIPMultiaddr() should return false for non-IP address")
	}
}

// TestGetPeerID 测试 PeerID 提取
func TestGetPeerID(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{
			"With PeerID",
			"/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
			"QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
			false,
		},
		{
			"Without PeerID",
			"/ip4/127.0.0.1/tcp/4001",
			"",
			true,
		},
		{
			"Nil addr",
			"",
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ma Multiaddr
			var err error
			if tt.addr != "" {
				ma, err = NewMultiaddr(tt.addr)
				if err != nil {
					t.Fatalf("NewMultiaddr() error = %v", err)
				}
			}

			peerID, err := GetPeerID(ma)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPeerID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && peerID != tt.want {
				t.Errorf("GetPeerID() = %v, want %v", peerID, tt.want)
			}
		})
	}
}

// TestWithPeerID 测试添加/替换 PeerID
func TestWithPeerID(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	result, err := WithPeerID(ma, "QmNewPeerID")
	if err != nil {
		t.Fatalf("WithPeerID() error = %v", err)
	}

	peerID, _ := GetPeerID(result)
	if peerID != "QmNewPeerID" {
		t.Errorf("WithPeerID() result PeerID = %v, want QmNewPeerID", peerID)
	}

	// Test with nil
	_, err = WithPeerID(nil, "QmTest")
	if err == nil {
		t.Error("WithPeerID(nil) should return error")
	}
}

// TestWithoutPeerID 测试移除 PeerID
func TestWithoutPeerID(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmYyQ")

	result := WithoutPeerID(ma)

	if result.String() != "/ip4/127.0.0.1/tcp/4001" {
		t.Errorf("WithoutPeerID() = %v, want /ip4/127.0.0.1/tcp/4001", result.String())
	}

	// Test with nil
	result = WithoutPeerID(nil)
	if result != nil {
		t.Error("WithoutPeerID(nil) should return nil")
	}
}

// TestHasProtocol_Nil 测试 HasProtocol 处理 nil
func TestHasProtocol_Nil(t *testing.T) {
	if HasProtocol(nil, P_TCP) {
		t.Error("HasProtocol(nil) should return false")
	}
}
