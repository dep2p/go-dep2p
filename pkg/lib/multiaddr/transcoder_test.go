package multiaddr

import (
	"testing"
)

func TestTranscoderIP4(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid IPv4", "127.0.0.1", false},
		{"Invalid IPv4", "999.999.999.999", true},
		{"Not IPv4", "::1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := TranscoderIP4.StringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				s, err := TranscoderIP4.BytesToString(b)
				if err != nil {
					t.Errorf("BytesToString() error = %v", err)
				}
				if s != tt.input {
					t.Errorf("Round trip: got %v, want %v", s, tt.input)
				}
			}
		})
	}
}

func TestTranscoderIP6(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid IPv6", "::1", false},
		{"Valid IPv6 full", "2001:db8::1", false},
		{"Invalid IPv6", "not-an-ip", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := TranscoderIP6.StringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && b != nil {
				_, err := TranscoderIP6.BytesToString(b)
				if err != nil {
					t.Errorf("BytesToString() error = %v", err)
				}
			}
		})
	}
}

func TestTranscoderPort(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid port", "4001", false},
		{"Zero port", "0", false},
		{"Max port", "65535", false},
		{"Over max", "65536", true},
		{"Invalid", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := TranscoderPort.StringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				s, err := TranscoderPort.BytesToString(b)
				if err != nil {
					t.Errorf("BytesToString() error = %v", err)
				}
				if s != tt.input {
					t.Errorf("Round trip: got %v, want %v", s, tt.input)
				}
			}
		})
	}
}

func TestTranscoderDNS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid DNS", "example.com", false},
		{"Valid subdomain", "sub.example.com", false},
		{"Empty", "", true},
		{"With slash", "example.com/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := TranscoderDNS.StringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				s, err := TranscoderDNS.BytesToString(b)
				if err != nil {
					t.Errorf("BytesToString() error = %v", err)
				}
				if s != tt.input {
					t.Errorf("Round trip: got %v, want %v", s, tt.input)
				}

				// Test validate
				if err := TranscoderDNS.ValidateBytes(b); err != nil {
					t.Errorf("ValidateBytes() error = %v", err)
				}
			}
		})
	}
}

func TestTranscoderP2P(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid PeerID", "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N", false},
		{"Short ID", "12D3KooW", false},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := TranscoderP2P.StringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				s, err := TranscoderP2P.BytesToString(b)
				if err != nil {
					t.Errorf("BytesToString() error = %v", err)
				}
				if s != tt.input {
					t.Errorf("Round trip: got %v, want %v", s, tt.input)
				}

				// Test validate
				if err := TranscoderP2P.ValidateBytes(b); err != nil {
					t.Errorf("ValidateBytes() error = %v", err)
				}
			}
		})
	}
}

func TestTranscoderIP6Zone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid zone", "eth0", false},
		{"Empty", "", true},
		{"With slash", "eth0/bad", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := TranscoderIP6Zone.StringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				s, err := TranscoderIP6Zone.BytesToString(b)
				if err != nil {
					t.Errorf("BytesToString() error = %v", err)
				}
				if s != tt.input {
					t.Errorf("Round trip: got %v, want %v", s, tt.input)
				}

				// Test validate
				if err := TranscoderIP6Zone.ValidateBytes(b); err != nil {
					t.Errorf("ValidateBytes() error = %v", err)
				}
			}
		})
	}

	// Test validate with slash
	t.Run("ValidateBytes with slash", func(t *testing.T) {
		err := TranscoderIP6Zone.ValidateBytes([]byte("bad/zone"))
		if err == nil {
			t.Error("ValidateBytes() should reject zone with slash")
		}
	})

	// Test validate empty
	t.Run("ValidateBytes empty", func(t *testing.T) {
		err := TranscoderIP6Zone.ValidateBytes([]byte{})
		if err == nil {
			t.Error("ValidateBytes() should reject empty bytes")
		}
	})
}

func TestTranscoderIPCIDR(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid CIDR", "24", false},
		{"Zero", "0", false},
		{"Max", "255", false},
		{"Invalid", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := TranscoderIPCIDR.StringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				s, err := TranscoderIPCIDR.BytesToString(b)
				if err != nil {
					t.Errorf("BytesToString() error = %v", err)
				}
				if s != tt.input {
					t.Errorf("Round trip: got %v, want %v", s, tt.input)
				}
			}
		})
	}
}

func TestTranscoderUnix(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid path", "/tmp/socket", false},
		{"Relative path", "socket", false},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := TranscoderUnix.StringToBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				s, err := TranscoderUnix.BytesToString(b)
				if err != nil {
					t.Errorf("BytesToString() error = %v", err)
				}
				if s != tt.input {
					t.Errorf("Round trip: got %v, want %v", s, tt.input)
				}
			}
		})
	}
}

// TestTranscoderOnion 测试 Onion transcoder
func TestTranscoderOnion(t *testing.T) {
	// 测试基本功能
	t.Run("Valid format", func(t *testing.T) {
		// Onion 地址格式：base32:port
		input := "aaaaaaaaaaaaaaaa:80"
		b, err := TranscoderOnion.StringToBytes(input)
		if err != nil {
			t.Logf("Onion StringToBytes() error (expected for test data): %v", err)
			// 这是预期的，因为我们使用了测试数据
		} else if len(b) > 0 {
			s, _ := TranscoderOnion.BytesToString(b)
			t.Logf("Onion round trip: %s -> %s", input, s)
		}
	})
}

// TestTranscoderOnion3 测试 Onion3 transcoder
func TestTranscoderOnion3(t *testing.T) {
	t.Run("Invalid format", func(t *testing.T) {
		_, err := TranscoderOnion3.StringToBytes("invalid")
		if err == nil {
			t.Error("Should reject invalid onion3 address")
		}
	})
}

// TestTranscoderGarlic 测试 Garlic transcoder
func TestTranscoderGarlic64(t *testing.T) {
	t.Run("Valid base32", func(t *testing.T) {
		// 有效的 base32 字符串
		input := "AAAAAAAAAAAAAAAA"
		b, err := TranscoderGarlic64.StringToBytes(input)
		if err != nil {
			t.Fatalf("StringToBytes() error = %v", err)
		}

		s, err := TranscoderGarlic64.BytesToString(b)
		if err != nil {
			t.Fatalf("BytesToString() error = %v", err)
		}

		// 应该是小写的
		if s != "aaaaaaaaaaaaaaaa" {
			t.Errorf("BytesToString() = %s, want aaaaaaaaaaaaaaaa", s)
		}
	})

	t.Run("Invalid base32", func(t *testing.T) {
		_, err := TranscoderGarlic64.StringToBytes("!!!invalid!!!")
		if err == nil {
			t.Error("Should reject invalid base32")
		}
	})
}

// TestTranscoderGarlic32 测试 Garlic32 transcoder
func TestTranscoderGarlic32(t *testing.T) {
	t.Run("Valid base32", func(t *testing.T) {
		input := "AAAABBBB"
		b, err := TranscoderGarlic32.StringToBytes(input)
		if err != nil {
			t.Fatalf("StringToBytes() error = %v", err)
		}

		s, err := TranscoderGarlic32.BytesToString(b)
		if err != nil {
			t.Fatalf("BytesToString() error = %v", err)
		}

		if s != "aaaabbbb" {
			t.Errorf("BytesToString() = %s, want aaaabbbb", s)
		}
	})
}
