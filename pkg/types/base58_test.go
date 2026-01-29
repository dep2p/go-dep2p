package types

import (
	"bytes"
	"testing"
)

func TestBase58Encode(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: "",
		},
		{
			name:     "single zero byte",
			input:    []byte{0},
			expected: "1",
		},
		{
			name:     "two zero bytes",
			input:    []byte{0, 0},
			expected: "11",
		},
		{
			name:     "simple bytes",
			input:    []byte{1, 2, 3},
			expected: "Ldp",
		},
		{
			name:     "leading zeros with data",
			input:    []byte{0, 0, 1, 2, 3},
			expected: "11Ldp",
		},
		{
			name:     "hello world",
			input:    []byte("Hello World"),
			expected: "JxF12TrwUP45BMd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Base58Encode(tt.input)
			if result != tt.expected {
				t.Errorf("Base58Encode(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBase58Decode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
		wantErr  bool
	}{
		{
			name:     "empty input",
			input:    "",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "single 1",
			input:    "1",
			expected: []byte{0},
			wantErr:  false,
		},
		{
			name:     "two 1s",
			input:    "11",
			expected: []byte{0, 0},
			wantErr:  false,
		},
		{
			name:     "simple decode",
			input:    "Ldp",
			expected: []byte{1, 2, 3},
			wantErr:  false,
		},
		{
			name:     "leading zeros with data",
			input:    "11Ldp",
			expected: []byte{0, 0, 1, 2, 3},
			wantErr:  false,
		},
		{
			name:     "hello world",
			input:    "JxF12TrwUP45BMd",
			expected: []byte("Hello World"),
			wantErr:  false,
		},
		{
			name:     "invalid character 0",
			input:    "0abc",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid character O",
			input:    "abcO",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid character I",
			input:    "abcI",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid character l",
			input:    "abcl",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Base58Decode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Base58Decode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("Base58Decode(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBase58RoundTrip(t *testing.T) {
	tests := [][]byte{
		{},
		{0},
		{0, 0, 0},
		{1, 2, 3, 4, 5},
		{0, 0, 1, 2, 3, 4, 5},
		[]byte("Hello, World!"),
		{255, 255, 255, 255},
		make([]byte, 32), // 32 zeros
	}

	// Test with random 32-byte data
	randomData := make([]byte, 32)
	for i := range randomData {
		randomData[i] = byte(i * 7)
	}
	tests = append(tests, randomData)

	for i, original := range tests {
		encoded := Base58Encode(original)
		decoded, err := Base58Decode(encoded)
		if err != nil {
			t.Errorf("Test %d: Base58Decode failed: %v", i, err)
			continue
		}
		if !bytes.Equal(decoded, original) {
			t.Errorf("Test %d: round trip failed\n  original: %v\n  encoded:  %q\n  decoded:  %v",
				i, original, encoded, decoded)
		}
	}
}

func BenchmarkBase58Encode(b *testing.B) {
	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Base58Encode(data)
	}
}

func BenchmarkBase58Decode(b *testing.B) {
	data := make([]byte, 32)
	for i := range data {
		data[i] = byte(i)
	}
	encoded := Base58Encode(data)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Base58Decode(encoded)
	}
}
