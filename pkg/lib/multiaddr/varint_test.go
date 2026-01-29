package multiaddr

import (
	"testing"
)

func TestCodeToVarint(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"IP4", P_IP4},
		{"TCP", P_TCP},
		{"UDP", P_UDP},
		{"P2P", P_P2P},
		{"Zero", 0},
		{"Large", 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := codeToVarint(tt.code)
			if len(b) == 0 {
				t.Error("codeToVarint returned empty bytes")
			}

			// Verify we can decode it back
			code, n, err := readVarintCode(b)
			if err != nil {
				t.Errorf("readVarintCode() error = %v", err)
			}
			if code != tt.code {
				t.Errorf("Round trip: got %d, want %d", code, tt.code)
			}
			if n != len(b) {
				t.Errorf("Bytes read mismatch: got %d, want %d", n, len(b))
			}
		})
	}
}

func TestReadVarintCode(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    int
		wantN   int
		wantErr bool
	}{
		{"Valid small", []byte{0x04}, 4, 1, false},
		{"Valid large", []byte{0x90, 0x01}, 144, 2, false},
		{"Empty", []byte{}, 0, 0, true},
		{"Truncated", []byte{0x80}, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, n, err := readVarintCode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("readVarintCode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if code != tt.want {
					t.Errorf("readVarintCode() code = %d, want %d", code, tt.want)
				}
				if n != tt.wantN {
					t.Errorf("readVarintCode() n = %d, want %d", n, tt.wantN)
				}
			}
		})
	}
}

func TestUvarintEncode(t *testing.T) {
	tests := []struct {
		name  string
		input uint64
	}{
		{"Zero", 0},
		{"Small", 127},
		{"Medium", 300},
		{"Large", 100000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := uvarintEncode(tt.input)
			if len(b) == 0 {
				t.Error("uvarintEncode returned empty bytes")
			}

			// Verify we can decode it back
			val, n, err := uvarintDecode(b)
			if err != nil {
				t.Errorf("uvarintDecode() error = %v", err)
			}
			if val != tt.input {
				t.Errorf("Round trip: got %d, want %d", val, tt.input)
			}
			if n != len(b) {
				t.Errorf("Bytes read mismatch: got %d, want %d", n, len(b))
			}
		})
	}
}

func TestUvarintEncode_Decode_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input uint64
	}{
		{"Zero", 0},
		{"7-bit max", 127},
		{"8-bit", 128},
		{"14-bit max", 16383},
		{"Large", 1000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded := uvarintEncode(tt.input)
			if len(encoded) == 0 {
				t.Error("uvarintEncode returned empty bytes")
			}

			// Decode
			decoded, n, err := uvarintDecode(encoded)
			if err != nil {
				t.Errorf("uvarintDecode() error = %v", err)
			}
			if decoded != tt.input {
				t.Errorf("Round trip: got %d, want %d", decoded, tt.input)
			}
			if n != len(encoded) {
				t.Errorf("Bytes read mismatch: got %d, want %d", n, len(encoded))
			}
		})
	}
}

func BenchmarkCodeToVarint(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = codeToVarint(P_IP4)
	}
}

func BenchmarkReadVarintCode(b *testing.B) {
	data := codeToVarint(P_IP4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = readVarintCode(data)
	}
}
