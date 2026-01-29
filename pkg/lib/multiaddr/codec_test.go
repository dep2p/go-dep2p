package multiaddr

import (
	"bytes"
	"testing"
)

// TestStringToBytes 测试字符串到字节的编码
func TestStringToBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"IPv4 + TCP", "/ip4/127.0.0.1/tcp/4001", false},
		{"IPv6 + TCP", "/ip6/::1/tcp/4001", false},
		{"DNS + TCP", "/dns/example.com/tcp/80", false},
		{"Complex", "/ip4/1.2.3.4/tcp/4001/p2p/QmcEPrat8ShnCph8WjkREzt5CPXF2RwhYxYBALDcLC1iV6", false},
		{"Empty", "", true},
		{"No leading slash", "ip4/127.0.0.1", true},
		{"Unknown protocol", "/unknown/value", true},
		{"Trailing slashes", "/ip4/127.0.0.1//", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("stringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) == 0 {
				t.Error("stringToBytes() returned empty bytes")
			}
		})
	}
}

// TestBytesToString 测试字节到字符串的解码
func TestBytesToString(t *testing.T) {
	tests := []struct {
		name    string
		input   func() []byte
		want    string
		wantErr bool
	}{
		{
			"IPv4 + TCP",
			func() []byte {
				// /ip4/127.0.0.1/tcp/4001
				return []byte{0x04, 127, 0, 0, 1, 0x06, 0x0f, 0xa1}
			},
			"/ip4/127.0.0.1/tcp/4001",
			false,
		},
		{
			"Empty",
			func() []byte { return []byte{} },
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bytesToString(tt.input())
			if (err != nil) != tt.wantErr {
				t.Errorf("bytesToString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("bytesToString() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRoundTrip 测试编解码往返
func TestRoundTrip(t *testing.T) {
	tests := []string{
		"/ip4/127.0.0.1/tcp/4001",
		"/ip6/::1/tcp/4001",
		"/ip4/192.168.1.1/udp/4001/quic-v1",
		"/ip4/1.2.3.4/tcp/4001/p2p/QmcEPrat8ShnCph8WjkREzt5CPXF2RwhYxYBALDcLC1iV6",
		"/dns/example.com/tcp/443/wss",
		"/dns4/test.local/tcp/8080",
		"/dns6/ipv6.local/tcp/9090",
	}

	for _, addr := range tests {
		t.Run(addr, func(t *testing.T) {
			// String -> Bytes
			b, err := stringToBytes(addr)
			if err != nil {
				t.Fatalf("stringToBytes() error = %v", err)
			}

			// Bytes -> String
			s, err := bytesToString(b)
			if err != nil {
				t.Fatalf("bytesToString() error = %v", err)
			}

			if s != addr {
				t.Errorf("RoundTrip: got %v, want %v", s, addr)
			}
		})
	}
}

// TestValidateBytes 测试字节验证
func TestValidateBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   func() []byte
		wantErr bool
	}{
		{
			"Valid IPv4 + TCP",
			func() []byte {
				b, _ := stringToBytes("/ip4/127.0.0.1/tcp/4001")
				return b
			},
			false,
		},
		{
			"Empty",
			func() []byte { return []byte{} },
			true,
		},
		{
			"Invalid protocol code",
			func() []byte { return []byte{0xff, 0xff, 0xff} },
			true,
		},
		{
			"Truncated",
			func() []byte {
				b, _ := stringToBytes("/ip4/127.0.0.1/tcp/4001")
				return b[:3] // 截断
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBytes(tt.input())
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBytes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBinarySizeForAddr 测试地址大小计算
func TestBinarySizeForAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantSize int
	}{
		{
			"IPv4 only",
			"/ip4/127.0.0.1",
			1 + 4, // varint(code) + 4 bytes
		},
		{
			"IPv4 + TCP",
			"/ip4/127.0.0.1/tcp/4001",
			1 + 4 + 1 + 2, // ip4 + tcp
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := stringToBytes(tt.addr)
			if err != nil {
				t.Fatalf("stringToBytes() error = %v", err)
			}
			if len(b) != tt.wantSize {
				t.Errorf("Binary size = %d, want %d", len(b), tt.wantSize)
			}
		})
	}
}

// TestCodecEdgeCases 测试边界情况
func TestCodecEdgeCases(t *testing.T) {
	t.Run("Multiple slashes", func(t *testing.T) {
		// 多个斜杠应该被正确处理
		b1, _ := stringToBytes("/ip4/127.0.0.1/")
		b2, _ := stringToBytes("/ip4/127.0.0.1")
		if !bytes.Equal(b1, b2) {
			t.Error("Trailing slashes should be ignored")
		}
	})

	t.Run("Zero port", func(t *testing.T) {
		_, err := stringToBytes("/ip4/127.0.0.1/tcp/0")
		if err != nil {
			t.Errorf("Zero port should be valid: %v", err)
		}
	})

	t.Run("Max port", func(t *testing.T) {
		_, err := stringToBytes("/ip4/127.0.0.1/tcp/65535")
		if err != nil {
			t.Errorf("Max port should be valid: %v", err)
		}
	})

	t.Run("Over max port", func(t *testing.T) {
		_, err := stringToBytes("/ip4/127.0.0.1/tcp/65536")
		if err == nil {
			t.Error("Over max port should be invalid")
		}
	})
}

// BenchmarkStringToBytes 基准测试编码
func BenchmarkStringToBytes(b *testing.B) {
	addr := "/ip4/127.0.0.1/tcp/4001/p2p/QmcEPrat8ShnCph8WjkREzt5CPXF2RwhYxYBALDcLC1iV6"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stringToBytes(addr)
	}
}

// BenchmarkBytesToString 基准测试解码
func BenchmarkBytesToString(b *testing.B) {
	bytes, _ := stringToBytes("/ip4/127.0.0.1/tcp/4001/p2p/QmcEPrat8ShnCph8WjkREzt5CPXF2RwhYxYBALDcLC1iV6")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bytesToString(bytes)
	}
}

// BenchmarkRoundTrip 基准测试往返
func BenchmarkRoundTrip(b *testing.B) {
	addr := "/ip4/127.0.0.1/tcp/4001"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bytes, _ := stringToBytes(addr)
		_, _ = bytesToString(bytes)
	}
}
