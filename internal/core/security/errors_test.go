package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	// 验证错误定义
	assert.NotNil(t, ErrNoCertificate)
	assert.NotNil(t, ErrPeerIDMismatch)
	assert.NotNil(t, ErrNoPublicKeyExtension)
	assert.NotNil(t, ErrInvalidPublicKey)
}
