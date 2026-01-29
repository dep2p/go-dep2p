package messaging

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

func TestBuildProtocolID(t *testing.T) {
	realmID := "realm-123"
	protocol := "myprotocol"

	protocolID := buildProtocolID(realmID, protocol)

	expected := "/dep2p/app/realm-123/myprotocol/1.0.0"
	assert.Equal(t, interfaces.ProtocolID(expected), protocolID)
}

func TestValidateProtocol(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		expectErr bool
	}{
		{
			name:      "valid protocol",
			protocol:  "myprotocol",
			expectErr: false,
		},
		{
			name:      "valid protocol with underscore",
			protocol:  "my_protocol",
			expectErr: false,
		},
		{
			name:      "valid protocol with dash",
			protocol:  "my-protocol",
			expectErr: false,
		},
		{
			name:      "empty protocol",
			protocol:  "",
			expectErr: true,
		},
		{
			name:      "protocol with space",
			protocol:  "my protocol",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProtocol(tt.protocol)
			if tt.expectErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidProtocol)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuildProtocolID_Format(t *testing.T) {
	tests := []struct {
		name     string
		realmID  string
		protocol string
		expected string
	}{
		{
			name:     "standard",
			realmID:  "realm-1",
			protocol: "chat",
			expected: "/dep2p/app/realm-1/chat/1.0.0",
		},
		{
			name:     "with underscore",
			realmID:  "realm_2",
			protocol: "file_transfer",
			expected: "/dep2p/app/realm_2/file_transfer/1.0.0",
		},
		{
			name:     "with dash",
			realmID:  "realm-test",
			protocol: "my-protocol",
			expected: "/dep2p/app/realm-test/my-protocol/1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProtocolID(tt.realmID, tt.protocol)
			assert.Equal(t, interfaces.ProtocolID(tt.expected), result)
		})
	}
}
