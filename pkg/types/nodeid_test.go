package types

import (
	"testing"
)

func TestNodeIDString(t *testing.T) {
	// Create a known NodeID
	var id NodeID
	for i := 0; i < 32; i++ {
		id[i] = byte(i)
	}

	// String should return Base58
	s := id.String()
	if s == "" {
		t.Error("NodeID.String() returned empty string")
	}
}

func TestNodeIDShortString(t *testing.T) {
	var id NodeID
	for i := 0; i < 32; i++ {
		id[i] = byte(i)
	}

	short := id.ShortString()
	if len(short) > 8 {
		t.Errorf("NodeID.ShortString() = %q, expected at most 8 chars", short)
	}
}

func TestEmptyNodeIDString(t *testing.T) {
	var id NodeID // zero value
	s := id.String()
	if s != "" {
		t.Errorf("EmptyNodeID.String() = %q, want empty string", s)
	}
}

func TestParseNodeID_Base58(t *testing.T) {
	// Create a known NodeID and get its Base58 representation
	var original NodeID
	for i := 0; i < 32; i++ {
		original[i] = byte(i + 1) // non-zero to avoid leading zeros
	}

	base58Str := original.String()

	// Parse it back
	parsed, err := ParseNodeID(base58Str)
	if err != nil {
		t.Fatalf("ParseNodeID(%q) error: %v", base58Str, err)
	}

	if !parsed.Equal(original) {
		t.Errorf("ParseNodeID(%q) = %v, want %v", base58Str, parsed, original)
	}
}

func TestParseNodeID_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"too short hex", "0102030405"},
		{"invalid base58 char O", "O123456789"},
		{"invalid base58 char 0", "0123456789"},
		{"invalid base58 char I", "I123456789"},
		{"invalid base58 char l", "l123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseNodeID(tt.input)
			if err == nil {
				t.Errorf("ParseNodeID(%q) expected error, got nil", tt.input)
			}
		})
	}
}

func TestNodeIDRoundTrip(t *testing.T) {
	// Test multiple NodeIDs
	for i := 0; i < 10; i++ {
		var original NodeID
		for j := 0; j < 32; j++ {
			original[j] = byte((i*17 + j*31) % 256)
		}

		// Round trip through Base58
		str := original.String()
		parsed, err := ParseNodeID(str)
		if err != nil {
			t.Errorf("case %d: ParseNodeID error: %v", i, err)
			continue
		}

		if !parsed.Equal(original) {
			t.Errorf("case %d: round trip failed", i)
		}
	}
}

func TestNodeIDFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{"valid 32 bytes", make([]byte, 32), false},
		{"too short", make([]byte, 16), true},
		{"too long", make([]byte, 64), true},
		{"nil", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NodeIDFromBytes(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeIDFromBytes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeIDEqual(t *testing.T) {
	var id1, id2, id3 NodeID
	for i := 0; i < 32; i++ {
		id1[i] = byte(i)
		id2[i] = byte(i)
		id3[i] = byte(i + 1)
	}

	if !id1.Equal(id2) {
		t.Error("Equal NodeIDs should be equal")
	}

	if id1.Equal(id3) {
		t.Error("Different NodeIDs should not be equal")
	}
}

func TestNodeIDIsEmpty(t *testing.T) {
	var empty NodeID
	if !empty.IsEmpty() {
		t.Error("Zero NodeID should be empty")
	}

	var nonEmpty NodeID
	nonEmpty[0] = 1
	if nonEmpty.IsEmpty() {
		t.Error("Non-zero NodeID should not be empty")
	}
}

