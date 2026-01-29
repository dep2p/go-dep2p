package multiaddr

import (
	"testing"
)

// TestNewMultiaddr 测试从字符串创建多地址
func TestNewMultiaddr(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"IPv4 + TCP", "/ip4/127.0.0.1/tcp/4001", false},
		{"IPv6 + TCP", "/ip6/::1/tcp/4001", false},
		{"IPv4 + UDP + QUIC", "/ip4/192.168.1.1/udp/4001/quic-v1", false},
		{"Complex with P2P", "/ip4/1.2.3.4/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", false},
		{"Empty", "", true},
		{"No leading slash", "ip4/127.0.0.1", true},
		{"Unknown protocol", "/unknown/value", true},
		{"Incomplete", "/ip4", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMultiaddr(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMultiaddr() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNewMultiaddrBytes 测试从字节创建多地址
func TestNewMultiaddrBytes(t *testing.T) {
	tests := []struct {
		name    string
		prepare func() []byte
		wantErr bool
	}{
		{
			"Valid bytes",
			func() []byte {
				// /ip4/127.0.0.1/tcp/4001 的二进制表示
				return []byte{0x04, 127, 0, 0, 1, 0x06, 0x0f, 0xa1}
			},
			false,
		},
		{
			"Empty bytes",
			func() []byte { return []byte{} },
			true,
		},
		{
			"Invalid protocol code",
			func() []byte { return []byte{0xff, 0xff, 0xff} },
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMultiaddrBytes(tt.prepare())
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMultiaddrBytes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestMultiaddr_String 测试字符串表示
func TestMultiaddr_String(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"IPv4 + TCP", "/ip4/127.0.0.1/tcp/4001"},
		{"IPv6 + TCP", "/ip6/::1/tcp/4001"},
		{"IPv4 + UDP + QUIC", "/ip4/192.168.1.1/udp/4001/quic-v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := NewMultiaddr(tt.addr)
			if err != nil {
				t.Fatalf("NewMultiaddr() error = %v", err)
			}
			if got := ma.String(); got != tt.addr {
				t.Errorf("String() = %v, want %v", got, tt.addr)
			}
		})
	}
}

// TestMultiaddr_Equal 测试地址相等性
func TestMultiaddr_Equal(t *testing.T) {
	ma1, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ma2, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ma3, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4002")

	if !ma1.Equal(ma2) {
		t.Error("Equal multiaddrs should be equal")
	}

	if ma1.Equal(ma3) {
		t.Error("Different multiaddrs should not be equal")
	}

	if ma1.Equal(nil) {
		t.Error("Multiaddr should not equal nil")
	}
}

// TestMultiaddr_Protocols 测试协议提取
func TestMultiaddr_Protocols(t *testing.T) {
	tests := []struct {
		name      string
		addr      string
		wantCodes []int
		wantNames []string
	}{
		{
			"IPv4 + TCP",
			"/ip4/127.0.0.1/tcp/4001",
			[]int{P_IP4, P_TCP},
			[]string{"ip4", "tcp"},
		},
		{
			"IPv6 + UDP + QUIC",
			"/ip6/::1/udp/4001/quic-v1",
			[]int{P_IP6, P_UDP, P_QUIC_V1},
			[]string{"ip6", "udp", "quic-v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ma, err := NewMultiaddr(tt.addr)
			if err != nil {
				t.Fatalf("NewMultiaddr() error = %v", err)
			}

			protos := ma.Protocols()
			if len(protos) != len(tt.wantCodes) {
				t.Errorf("Protocols() count = %d, want %d", len(protos), len(tt.wantCodes))
				return
			}

			for i, proto := range protos {
				if proto.Code != tt.wantCodes[i] {
					t.Errorf("Protocol[%d].Code = %d, want %d", i, proto.Code, tt.wantCodes[i])
				}
				if proto.Name != tt.wantNames[i] {
					t.Errorf("Protocol[%d].Name = %s, want %s", i, proto.Name, tt.wantNames[i])
				}
			}
		})
	}
}

// TestMultiaddr_Encapsulate 测试封装
func TestMultiaddr_Encapsulate(t *testing.T) {
	ma1, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	ma2, _ := NewMultiaddr("/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

	result := ma1.Encapsulate(ma2)
	expected := "/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"

	if result.String() != expected {
		t.Errorf("Encapsulate() = %v, want %v", result.String(), expected)
	}
}

// TestMultiaddr_Decapsulate 测试解封装
func TestMultiaddr_Decapsulate(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	toRemove, _ := NewMultiaddr("/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

	result := ma.Decapsulate(toRemove)
	expected := "/ip4/127.0.0.1/tcp/4001"

	if result.String() != expected {
		t.Errorf("Decapsulate() = %v, want %v", result.String(), expected)
	}
}

// TestMultiaddr_ValueForProtocol 测试协议值获取
func TestMultiaddr_ValueForProtocol(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 测试 IP4
	val, err := ma.ValueForProtocol(P_IP4)
	if err != nil {
		t.Errorf("ValueForProtocol(P_IP4) error = %v", err)
	}
	if val != "127.0.0.1" {
		t.Errorf("ValueForProtocol(P_IP4) = %v, want 127.0.0.1", val)
	}

	// 测试 TCP
	val, err = ma.ValueForProtocol(P_TCP)
	if err != nil {
		t.Errorf("ValueForProtocol(P_TCP) error = %v", err)
	}
	if val != "4001" {
		t.Errorf("ValueForProtocol(P_TCP) = %v, want 4001", val)
	}

	// 测试不存在的协议
	_, err = ma.ValueForProtocol(P_UDP)
	if err == nil {
		t.Error("ValueForProtocol() should return error for non-existent protocol")
	}
}

// BenchmarkNewMultiaddr 基准测试地址解析
func BenchmarkNewMultiaddr(b *testing.B) {
	addr := "/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewMultiaddr(addr)
	}
}

// BenchmarkMultiaddr_String 基准测试字符串转换
func BenchmarkMultiaddr_String(b *testing.B) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ma.String()
	}
}

// BenchmarkMultiaddr_Bytes 基准测试字节转换
func BenchmarkMultiaddr_Bytes(b *testing.B) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ma.Bytes()
	}
}

// TestMultiaddr_MarshalJSON 测试 JSON 序列化
func TestMultiaddr_MarshalJSON(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 类型断言以访问 MarshalJSON 方法
	impl := ma.(*multiaddr)
	data, err := impl.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	expected := `"/ip4/127.0.0.1/tcp/4001"`
	if string(data) != expected {
		t.Errorf("MarshalJSON() = %s, want %s", string(data), expected)
	}
}

// TestMultiaddr_UnmarshalJSON 测试 JSON 反序列化
func TestMultiaddr_UnmarshalJSON(t *testing.T) {
	data := []byte(`"/ip4/127.0.0.1/tcp/4001"`)

	var ma multiaddr
	err := ma.UnmarshalJSON(data)
	if err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if ma.String() != "/ip4/127.0.0.1/tcp/4001" {
		t.Errorf("UnmarshalJSON() result = %s, want /ip4/127.0.0.1/tcp/4001", ma.String())
	}
}

// TestMultiaddr_MarshalBinary 测试二进制序列化
func TestMultiaddr_MarshalBinary(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	impl := ma.(*multiaddr)
	data, err := impl.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("MarshalBinary() returned empty bytes")
	}
}

// TestMultiaddr_UnmarshalBinary 测试二进制反序列化
func TestMultiaddr_UnmarshalBinary(t *testing.T) {
	original, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	data := original.Bytes()

	var ma multiaddr
	err := ma.UnmarshalBinary(data)
	if err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}

	if !ma.Equal(original) {
		t.Error("UnmarshalBinary() result not equal to original")
	}
}

// TestMultiaddr_MarshalText 测试文本序列化
func TestMultiaddr_MarshalText(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	impl := ma.(*multiaddr)
	data, err := impl.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}

	if string(data) != "/ip4/127.0.0.1/tcp/4001" {
		t.Errorf("MarshalText() = %s, want /ip4/127.0.0.1/tcp/4001", string(data))
	}
}

// TestMultiaddr_UnmarshalText 测试文本反序列化
func TestMultiaddr_UnmarshalText(t *testing.T) {
	data := []byte("/ip4/127.0.0.1/tcp/4001")

	var ma multiaddr
	err := ma.UnmarshalText(data)
	if err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}

	if ma.String() != "/ip4/127.0.0.1/tcp/4001" {
		t.Errorf("UnmarshalText() result = %s, want /ip4/127.0.0.1/tcp/4001", ma.String())
	}
}

// TestCast 测试强制转换
func TestCast(t *testing.T) {
	// 从已知有效的字节创建
	b, _ := stringToBytes("/ip4/127.0.0.1/tcp/4001")
	ma := Cast(b)

	if ma.String() != "/ip4/127.0.0.1/tcp/4001" {
		t.Errorf("Cast() result = %s, want /ip4/127.0.0.1/tcp/4001", ma.String())
	}
}

// TestMultiaddr_Protocols_Complex 测试复杂地址的协议提取
func TestMultiaddr_Protocols_Complex(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/1.2.3.4/tcp/4001/p2p/QmYyQ/p2p-circuit")

	protos := ma.Protocols()
	expectedCodes := []int{P_IP4, P_TCP, P_P2P, P_P2P_CIRCUIT}

	if len(protos) != len(expectedCodes) {
		t.Errorf("Protocols() count = %d, want %d", len(protos), len(expectedCodes))
		return
	}

	for i, proto := range protos {
		if proto.Code != expectedCodes[i] {
			t.Errorf("Protocol[%d].Code = %d, want %d", i, proto.Code, expectedCodes[i])
		}
	}
}

// TestMultiaddr_ValueForProtocol_NotFound 测试获取不存在的协议值
func TestMultiaddr_ValueForProtocol_NotFound(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	_, err := ma.ValueForProtocol(P_UDP)
	if err == nil {
		t.Error("ValueForProtocol() should return error for non-existent protocol")
	}
}

// TestMultiaddr_Encapsulate_Nil 测试封装 nil
func TestMultiaddr_Encapsulate_Nil(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	result := ma.Encapsulate(nil)

	if !result.Equal(ma) {
		t.Error("Encapsulate(nil) should return self")
	}
}

// TestMultiaddr_Decapsulate_Nil 测试解封装 nil
func TestMultiaddr_Decapsulate_Nil(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	result := ma.Decapsulate(nil)

	if !result.Equal(ma) {
		t.Error("Decapsulate(nil) should return self")
	}
}

// TestMultiaddr_Decapsulate_NotMatching 测试解封装不匹配的后缀
func TestMultiaddr_Decapsulate_NotMatching(t *testing.T) {
	ma, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	other, _ := NewMultiaddr("/udp/5000")

	result := ma.Decapsulate(other)

	if !result.Equal(ma) {
		t.Error("Decapsulate(non-matching) should return self")
	}
}

// TestMultiaddr_Decapsulate_TooLong 测试解封装比自己长的地址
func TestMultiaddr_Decapsulate_TooLong(t *testing.T) {
	ma, _ := NewMultiaddr("/tcp/4001")
	other, _ := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	result := ma.Decapsulate(other)

	if !result.Equal(ma) {
		t.Error("Decapsulate(longer) should return self")
	}
}
