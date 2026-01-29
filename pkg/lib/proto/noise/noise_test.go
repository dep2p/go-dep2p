package noise

import (
	"bytes"
	"testing"
)

func TestNoiseHandshakePayload_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		payload *NoiseHandshakePayload
	}{
		{
			name: "完整 payload",
			payload: &NoiseHandshakePayload{
				IdentityKey: []byte("test-identity-key-32-bytes-long!"),
				IdentitySig: []byte("test-signature-64-bytes-long-for-ed25519-signature-format!!!!!!!!"),
			},
		},
		{
			name: "只有 identity_key",
			payload: &NoiseHandshakePayload{
				IdentityKey: []byte("only-key"),
			},
		},
		{
			name: "只有 identity_sig",
			payload: &NoiseHandshakePayload{
				IdentitySig: []byte("only-sig"),
			},
		},
		{
			name:    "空 payload",
			payload: &NoiseHandshakePayload{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := tt.payload.Marshal()
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal
			got := &NoiseHandshakePayload{}
			if err := got.Unmarshal(data); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			// 比较
			if !bytes.Equal(got.IdentityKey, tt.payload.IdentityKey) {
				t.Errorf("IdentityKey mismatch: got %v, want %v", got.IdentityKey, tt.payload.IdentityKey)
			}
			if !bytes.Equal(got.IdentitySig, tt.payload.IdentitySig) {
				t.Errorf("IdentitySig mismatch: got %v, want %v", got.IdentitySig, tt.payload.IdentitySig)
			}
		})
	}
}

func TestNoiseHandshakePayload_Unmarshal_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "无效 wire type",
			data: []byte{0x08, 0x01}, // field 1, wire type 0 (varint)
		},
		{
			name: "长度超出数据",
			data: []byte{0x0a, 0xff, 0x01}, // field 1, length 255 (超出)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &NoiseHandshakePayload{}
			if err := p.Unmarshal(tt.data); err == nil {
				t.Error("Unmarshal() expected error, got nil")
			}
		})
	}
}

func TestVarint(t *testing.T) {
	tests := []struct {
		value uint64
	}{
		{0},
		{1},
		{127},
		{128},
		{16383},
		{16384},
		{1000000},
	}

	for _, tt := range tests {
		buf := appendVarint(nil, tt.value)
		got, n := consumeVarint(buf)
		if n < 0 {
			t.Errorf("consumeVarint(%d) failed", tt.value)
			continue
		}
		if got != tt.value {
			t.Errorf("varint roundtrip: got %d, want %d", got, tt.value)
		}
	}
}
