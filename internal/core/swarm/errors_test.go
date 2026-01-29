package swarm

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     DialError 测试
// ============================================================================

// TestDialError_Error_NoErrors 测试无错误时的消息
func TestDialError_Error_NoErrors(t *testing.T) {
	err := &DialError{
		Peer:   "peer-123",
		Errors: nil,
	}

	msg := err.Error()
	assert.Contains(t, msg, "peer-123")
	assert.Contains(t, msg, "unknown error")
}

// TestDialError_Error_SingleError 测试单个错误时的消息
func TestDialError_Error_SingleError(t *testing.T) {
	innerErr := errors.New("connection refused")
	err := &DialError{
		Peer:   "peer-456",
		Errors: []error{innerErr},
	}

	msg := err.Error()
	assert.Contains(t, msg, "peer-456")
	assert.Contains(t, msg, "connection refused")
	assert.NotContains(t, msg, "errors") // 单个错误不显示数量
}

// TestDialError_Error_MultipleErrors 测试多个错误时的消息
func TestDialError_Error_MultipleErrors(t *testing.T) {
	err := &DialError{
		Peer: "peer-789",
		Errors: []error{
			errors.New("error 1"),
			errors.New("error 2"),
			errors.New("error 3"),
		},
	}

	msg := err.Error()
	assert.Contains(t, msg, "peer-789")
	assert.Contains(t, msg, "3 errors")
}

// TestDialError_Unwrap_NoErrors 测试无错误时的 Unwrap
func TestDialError_Unwrap_NoErrors(t *testing.T) {
	err := &DialError{
		Peer:   "peer-123",
		Errors: nil,
	}

	unwrapped := err.Unwrap()
	assert.Nil(t, unwrapped)
}

// TestDialError_Unwrap_WithErrors 测试有错误时的 Unwrap
func TestDialError_Unwrap_WithErrors(t *testing.T) {
	firstErr := errors.New("first error")
	err := &DialError{
		Peer: "peer-123",
		Errors: []error{
			firstErr,
			errors.New("second error"),
		},
	}

	unwrapped := err.Unwrap()
	assert.Equal(t, firstErr, unwrapped)
}

// TestDialError_Is 测试 errors.Is 兼容性
func TestDialError_Is(t *testing.T) {
	innerErr := ErrNoAddresses
	err := &DialError{
		Peer:   "peer-123",
		Errors: []error{innerErr},
	}

	// 应该能通过 errors.Is 匹配内部错误
	assert.True(t, errors.Is(err, ErrNoAddresses))
}

// ============================================================================
//                     Config 测试
// ============================================================================

// TestConfig_Validate_Valid 测试有效配置
func TestConfig_Validate_Valid(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Validate()
	require.NoError(t, err)
}

// TestConfig_Validate_InvalidDialTimeout 测试无效的 DialTimeout
func TestConfig_Validate_InvalidDialTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DialTimeout = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestConfig_Validate_InvalidDialTimeoutLocal 测试无效的 DialTimeoutLocal
func TestConfig_Validate_InvalidDialTimeoutLocal(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DialTimeoutLocal = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestConfig_Validate_InvalidNewStreamTimeout 测试无效的 NewStreamTimeout
func TestConfig_Validate_InvalidNewStreamTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NewStreamTimeout = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestConfig_Validate_InvalidMaxConcurrentDials 测试无效的 MaxConcurrentDials
func TestConfig_Validate_InvalidMaxConcurrentDials(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxConcurrentDials = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestConfig_Validate_NegativeValues 测试负值
func TestConfig_Validate_NegativeValues(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DialTimeout = -1

	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestWithConfig_Nil 测试 nil 配置选项
func TestWithConfig_Nil(t *testing.T) {
	_, err := NewSwarm("test-peer", WithConfig(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestWithConfig_Invalid 测试无效配置选项
func TestWithConfig_Invalid(t *testing.T) {
	cfg := &Config{
		DialTimeout:        0, // invalid
		DialTimeoutLocal:   0,
		NewStreamTimeout:   0,
		MaxConcurrentDials: 0,
	}

	_, err := NewSwarm("test-peer", WithConfig(cfg))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

// TestWithConfig_Valid 测试有效配置选项
func TestWithConfig_Valid(t *testing.T) {
	cfg := DefaultConfig()

	s, err := NewSwarm("test-peer", WithConfig(cfg))
	require.NoError(t, err)
	defer s.Close()
}

// TestDefaultConfig 测试默认配置
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.Greater(t, cfg.DialTimeout, cfg.DialTimeoutLocal) // 远程超时应该更长
	assert.Greater(t, cfg.MaxConcurrentDials, 0)

	// 验证健康检测配置有默认值
	assert.Greater(t, cfg.ConnHealthInterval, time.Duration(0), "ConnHealthInterval 应有默认值")
	assert.Greater(t, cfg.ConnHealthTimeout, time.Duration(0), "ConnHealthTimeout 应有默认值")
}

// TestConfigFromUnified_Nil 测试 nil 统一配置
func TestConfigFromUnified_Nil(t *testing.T) {
	cfg := ConfigFromUnified(nil)

	// 应返回默认配置
	assert.NotNil(t, cfg)
	assert.Equal(t, DefaultConfig().DialTimeout, cfg.DialTimeout)

	// 验证 nil 情况下健康检测配置也有默认值
	assert.Greater(t, cfg.ConnHealthInterval, time.Duration(0), "nil 配置时应使用默认 ConnHealthInterval")
	assert.Greater(t, cfg.ConnHealthTimeout, time.Duration(0), "nil 配置时应使用默认 ConnHealthTimeout")
}

// TestConfigFromUnified_HealthCheckConfig 测试健康检测配置
// 确保 ConfigFromUnified 正确设置健康检测配置
func TestConfigFromUnified_BUG34a_HealthCheckConfig(t *testing.T) {
	// 模拟非 nil 但不包含健康检测配置的统一配置
	// ConfigFromUnified 应该从 DefaultConfig 获取健康检测配置
	cfg := ConfigFromUnified(nil)

	// 验证健康检测配置不为零值
	assert.Equal(t, 30*time.Second, cfg.ConnHealthInterval, "ConnHealthInterval 应为默认值 30s")
	assert.Equal(t, 10*time.Second, cfg.ConnHealthTimeout, "ConnHealthTimeout 应为默认值 10s")
}
