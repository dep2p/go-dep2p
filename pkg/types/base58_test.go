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
			name:     "empty",
			input:    []byte{},
			expected: "",
		},
		{
			name:     "single zero",
			input:    []byte{0},
			expected: "1",
		},
		{
			name:     "multiple zeros",
			input:    []byte{0, 0, 0},
			expected: "111",
		},
		{
			name:     "hello",
			input:    []byte("Hello"),
			expected: "9Ajdvzr",
		},
		{
			name:     "32 bytes",
			input:    make([]byte, 32),
			expected: "11111111111111111111111111111111", // 32 leading zeros
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
			name:     "empty",
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
			name:     "multiple 1s",
			input:    "111",
			expected: []byte{0, 0, 0},
			wantErr:  false,
		},
		{
			name:     "hello",
			input:    "9Ajdvzr",
			expected: []byte("Hello"),
			wantErr:  false,
		},
		{
			name:     "invalid char O",
			input:    "O",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid char 0",
			input:    "0",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid char I",
			input:    "I",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid char l",
			input:    "l",
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
	testCases := [][]byte{
		{},
		{0},
		{0, 0, 0, 0},
		{1, 2, 3, 4, 5},
		make([]byte, 32), // 32 zeros
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
	}

	// Add some random-like data
	for i := 0; i < 32; i++ {
		testCases = append(testCases, func() []byte {
			b := make([]byte, 32)
			for j := 0; j < 32; j++ {
				b[j] = byte((i*17 + j*31) % 256)
			}
			return b
		}())
	}

	for i, tc := range testCases {
		encoded := Base58Encode(tc)
		decoded, err := Base58Decode(encoded)
		if err != nil {
			t.Errorf("case %d: Base58Decode(Base58Encode(%v)) error: %v", i, tc, err)
			continue
		}

		// Handle nil vs empty slice
		if len(tc) == 0 && len(decoded) == 0 {
			continue
		}

		if !bytes.Equal(decoded, tc) {
			t.Errorf("case %d: Base58Decode(Base58Encode(%v)) = %v, want original", i, tc, decoded)
		}
	}
}

