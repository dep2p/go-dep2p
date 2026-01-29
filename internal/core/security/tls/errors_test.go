package tls

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestErrors_Types 测试错误类型
func TestErrors_Types(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrNoCertificate", ErrNoCertificate, "no certificate"},
		{"ErrPeerIDMismatch", ErrPeerIDMismatch, "peer ID mismatch"},
		{"ErrNoPublicKeyExtension", ErrNoPublicKeyExtension, "no public key extension"},
		{"ErrInvalidPublicKey", ErrInvalidPublicKey, "invalid public key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.err)
			assert.Contains(t, tt.err.Error(), tt.msg)
		})
	}
}

// TestErrors_Wrapping 测试错误包装
func TestErrors_Wrapping(t *testing.T) {
	baseErr := ErrPeerIDMismatch
	wrappedErr := errors.Join(baseErr, errors.New("additional context"))

	assert.True(t, errors.Is(wrappedErr, baseErr))
}

// TestErrors_Uniqueness 测试错误唯一性
func TestErrors_Uniqueness(t *testing.T) {
	allErrors := []error{
		ErrNoCertificate,
		ErrPeerIDMismatch,
		ErrNoPublicKeyExtension,
		ErrInvalidPublicKey,
	}

	// 确保所有错误都不同
	for i, err1 := range allErrors {
		for j, err2 := range allErrors {
			if i != j {
				assert.NotEqual(t, err1, err2, "errors should be unique")
			}
		}
	}
}
