package auth

import (
	"context"
	"fmt"
	"time"
)

// ============================================================================
//                              认证配置
// ============================================================================

// AuthConfig 认证配置
type AuthConfig struct {
	// PSK 预共享密钥（用于 PSK 模式）
	PSK []byte

	// PeerID 本地节点 ID
	PeerID string

	// CertPath 证书路径（用于 Cert 模式）
	CertPath string

	// KeyPath 私钥路径（用于 Cert 模式）
	KeyPath string

	// CustomValidator 自定义验证器（用于 Custom 模式）
	CustomValidator func(ctx context.Context, peerID string, proof []byte) (bool, error)

	// Timeout 认证超时时间
	Timeout time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// ReplayWindow 重放攻击防护时间窗口
	ReplayWindow time.Duration

	// NonceSize nonce 大小（字节）
	NonceSize int
}

// DefaultAuthConfig 返回默认配置
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		ReplayWindow: 5 * time.Minute,
		NonceSize:    32,
	}
}

// Validate 验证配置
func (c *AuthConfig) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("%w: Timeout must be positive", ErrInvalidConfig)
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("%w: MaxRetries must be non-negative", ErrInvalidConfig)
	}

	if c.ReplayWindow <= 0 {
		return fmt.Errorf("%w: ReplayWindow must be positive", ErrInvalidConfig)
	}

	if c.NonceSize <= 0 || c.NonceSize > 256 {
		return fmt.Errorf("%w: NonceSize must be between 1 and 256", ErrInvalidConfig)
	}

	if c.PeerID == "" {
		return fmt.Errorf("%w: PeerID cannot be empty", ErrInvalidConfig)
	}

	// PSK 模式验证
	if len(c.PSK) > 0 {
		if len(c.PSK) < 16 {
			return fmt.Errorf("%w: PSK must be at least 16 bytes", ErrInvalidPSK)
		}
		if len(c.PSK) > 256 {
			return fmt.Errorf("%w: PSK must not exceed 256 bytes", ErrInvalidPSK)
		}
	}

	return nil
}

// Clone 克隆配置
func (c *AuthConfig) Clone() *AuthConfig {
	config := &AuthConfig{
		PeerID:       c.PeerID,
		CertPath:     c.CertPath,
		KeyPath:      c.KeyPath,
		Timeout:      c.Timeout,
		MaxRetries:   c.MaxRetries,
		ReplayWindow: c.ReplayWindow,
		NonceSize:    c.NonceSize,
	}

	// 复制 PSK
	if len(c.PSK) > 0 {
		config.PSK = make([]byte, len(c.PSK))
		copy(config.PSK, c.PSK)
	}

	return config
}
