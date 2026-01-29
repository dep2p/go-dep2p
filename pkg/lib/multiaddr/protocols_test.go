package multiaddr

import (
	"testing"
)

// TestProtocolWithName 测试根据名称获取协议
func TestProtocolWithName(t *testing.T) {
	tests := []struct {
		name      string
		protoName string
		wantCode  int
		wantFound bool
	}{
		{"IP4", "ip4", P_IP4, true},
		{"IP6", "ip6", P_IP6, true},
		{"TCP", "tcp", P_TCP, true},
		{"UDP", "udp", P_UDP, true},
		{"QUIC", "quic", P_QUIC, true},
		{"QUIC-V1", "quic-v1", P_QUIC_V1, true},
		{"P2P", "p2p", P_P2P, true},
		{"WS", "ws", P_WS, true},
		{"WSS", "wss", P_WSS, true},
		{"DNS", "dns", P_DNS, true},
		{"DNS4", "dns4", P_DNS4, true},
		{"DNS6", "dns6", P_DNS6, true},
		{"DNSADDR", "dnsaddr", P_DNSADDR, true},
		{"Unknown", "unknown", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto := ProtocolWithName(tt.protoName)
			if tt.wantFound {
				if proto.Code != tt.wantCode {
					t.Errorf("ProtocolWithName(%s).Code = %d, want %d", tt.protoName, proto.Code, tt.wantCode)
				}
				if proto.Name != tt.protoName {
					t.Errorf("ProtocolWithName(%s).Name = %s, want %s", tt.protoName, proto.Name, tt.protoName)
				}
			} else {
				if proto.Code != 0 {
					t.Errorf("ProtocolWithName(%s) should return zero protocol", tt.protoName)
				}
			}
		})
	}
}

// TestProtocolWithCode 测试根据代码获取协议
func TestProtocolWithCode(t *testing.T) {
	tests := []struct {
		name      string
		code      int
		wantName  string
		wantFound bool
	}{
		{"IP4", P_IP4, "ip4", true},
		{"IP6", P_IP6, "ip6", true},
		{"TCP", P_TCP, "tcp", true},
		{"UDP", P_UDP, "udp", true},
		{"QUIC", P_QUIC, "quic", true},
		{"QUIC-V1", P_QUIC_V1, "quic-v1", true},
		{"P2P", P_P2P, "p2p", true},
		{"WS", P_WS, "ws", true},
		{"WSS", P_WSS, "wss", true},
		{"Unknown", 99999, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto := ProtocolWithCode(tt.code)
			if tt.wantFound {
				if proto.Name != tt.wantName {
					t.Errorf("ProtocolWithCode(%d).Name = %s, want %s", tt.code, proto.Name, tt.wantName)
				}
				if proto.Code != tt.code {
					t.Errorf("ProtocolWithCode(%d).Code = %d, want %d", tt.code, proto.Code, tt.code)
				}
			} else {
				if proto.Code != 0 {
					t.Errorf("ProtocolWithCode(%d) should return zero protocol", tt.code)
				}
			}
		})
	}
}

// TestProtocolSizes 测试协议数据大小
func TestProtocolSizes(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		wantSize int
	}{
		{"IP4", P_IP4, 32},
		{"IP6", P_IP6, 128},
		{"TCP", P_TCP, 16},
		{"UDP", P_UDP, 16},
		{"QUIC", P_QUIC, 0},
		{"QUIC-V1", P_QUIC_V1, 0},
		{"WS", P_WS, 0},
		{"WSS", P_WSS, 0},
		{"P2P", P_P2P, LengthPrefixedVarSize},
		{"DNS", P_DNS, LengthPrefixedVarSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto := ProtocolWithCode(tt.code)
			if proto.Size != tt.wantSize {
				t.Errorf("Protocol %s size = %d, want %d", tt.name, proto.Size, tt.wantSize)
			}
		})
	}
}

// TestProtocolsHaveTranscoders 测试协议是否有 transcoder
func TestProtocolsHaveTranscoders(t *testing.T) {
	protosNeedingTranscoders := []int{
		P_IP4, P_IP6, P_TCP, P_UDP,
		P_DNS, P_DNS4, P_DNS6, P_DNSADDR,
		P_P2P,
	}

	for _, code := range protosNeedingTranscoders {
		proto := ProtocolWithCode(code)
		if proto.Transcoder == nil {
			t.Errorf("Protocol %s (code %d) should have a transcoder", proto.Name, code)
		}
	}
}

// TestProtocolsWithoutTranscoders 测试无需 transcoder 的协议
func TestProtocolsWithoutTranscoders(t *testing.T) {
	protosWithoutTranscoders := []int{
		P_QUIC, P_QUIC_V1, P_WS, P_WSS,
	}

	for _, code := range protosWithoutTranscoders {
		proto := ProtocolWithCode(code)
		// 这些协议可能有也可能没有 transcoder（无数据）
		// 主要测试它们不会导致错误
		if proto.Code == 0 {
			t.Errorf("Protocol with code %d not found", code)
		}
	}
}

// TestAllProtocols 测试所有协议是否正确注册
func TestAllProtocols(t *testing.T) {
	// 测试所有定义的协议常量
	codes := []int{
		P_IP4, P_TCP, P_DNS, P_DNS4, P_DNS6, P_DNSADDR,
		P_UDP, P_IP6, P_QUIC, P_QUIC_V1, P_P2P,
		P_WS, P_WSS,
	}

	for _, code := range codes {
		proto := ProtocolWithCode(code)
		if proto.Code == 0 {
			t.Errorf("Protocol with code %d not registered", code)
			continue
		}

		// 验证名称查找
		proto2 := ProtocolWithName(proto.Name)
		if proto2.Code != code {
			t.Errorf("Name lookup mismatch: %s -> %d, want %d", proto.Name, proto2.Code, code)
		}
	}
}

// TestProtocol_String 测试协议字符串表示
func TestProtocol_String(t *testing.T) {
	proto := ProtocolWithCode(P_IP4)
	if proto.String() != "ip4" {
		t.Errorf("Protocol.String() = %s, want ip4", proto.String())
	}

	proto = ProtocolWithCode(P_TCP)
	if proto.String() != "tcp" {
		t.Errorf("Protocol.String() = %s, want tcp", proto.String())
	}
}

// TestProtocolCodeToVarint 测试协议代码到 varint 的转换
func TestProtocolCodeToVarint(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"IP4", P_IP4},
		{"TCP", P_TCP},
		{"UDP", P_UDP},
		{"P2P", P_P2P},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto := ProtocolWithCode(tt.code)
			if len(proto.VCode) == 0 {
				t.Error("VCode should not be empty")
			}

			// 验证 VCode 的第一个字节
			if proto.VCode[0] == 0 && tt.code != 0 {
				t.Error("VCode should not start with 0 for non-zero code")
			}
		})
	}
}

// BenchmarkProtocolWithName 基准测试协议名称查找
func BenchmarkProtocolWithName(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ProtocolWithName("ip4")
	}
}

// BenchmarkProtocolWithCode 基准测试协议代码查找
func BenchmarkProtocolWithCode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ProtocolWithCode(P_IP4)
	}
}

// TestProtocolsWithString 测试从字符串提取协议
func TestProtocolsWithString(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantProtos []string
		wantErr    bool
	}{
		{
			"Simple",
			"/ip4/127.0.0.1/tcp/4001",
			[]string{"ip4", "tcp"},
			false,
		},
		{
			"Complex",
			"/ip4/1.2.3.4/tcp/4001/p2p/QmYyQ",
			[]string{"ip4", "tcp", "p2p"},
			false,
		},
		{
			"Empty",
			"",
			nil,
			false,
		},
		{
			"Unknown protocol",
			"/unknown/value",
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protos, err := ProtocolsWithString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProtocolsWithString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(protos) > 0 {
				if len(protos) != len(tt.wantProtos) {
					t.Errorf("ProtocolsWithString() count = %d, want %d", len(protos), len(tt.wantProtos))
				}
			}
		})
	}
}
