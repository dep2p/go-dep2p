package config

import (
	"errors"
	"time"
)

// SecurityConfig 安全传输配置
//
// 配置节点支持的安全传输协议：
//   - TLS 1.3: 标准 TLS 协议
//   - Noise: libp2p Noise 协议
type SecurityConfig struct {
	// EnableTLS 是否启用 TLS 1.3
	EnableTLS bool `json:"enable_tls"`

	// EnableNoise 是否启用 Noise
	EnableNoise bool `json:"enable_noise"`

	// PreferredProtocol 首选协议
	// 可选值: "tls", "noise"
	PreferredProtocol string `json:"preferred_protocol"`

	// NegotiateTimeout 协议协商超时
	NegotiateTimeout Duration `json:"negotiate_timeout"`

	// TLS TLS 配置
	TLS TLSConfig `json:"tls,omitempty"`

	// Noise Noise 配置
	Noise NoiseConfig `json:"noise,omitempty"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	// MinVersion 最小 TLS 版本
	// 0x0304 = TLS 1.3（推荐）
	MinVersion uint16 `json:"min_version,omitempty"`

	// MaxVersion 最大 TLS 版本
	// 0 表示使用最新版本
	MaxVersion uint16 `json:"max_version,omitempty"`

	// CertValidityPeriod 证书有效期
	CertValidityPeriod Duration `json:"cert_validity_period,omitempty"`

	// ClientAuth 是否要求客户端认证
	ClientAuth bool `json:"client_auth,omitempty"`
}

// NoiseConfig Noise 配置
type NoiseConfig struct {
	// HandshakeTimeout 握手超时
	HandshakeTimeout Duration `json:"handshake_timeout,omitempty"`

	// Pattern Noise 握手模式
	// 默认使用 "XX" 模式
	Pattern string `json:"pattern,omitempty"`
}

// DefaultSecurityConfig 返回默认安全配置
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		// ════════════════════════════════════════════════════════════════════
		// 安全协议启用配置
		// ════════════════════════════════════════════════════════════════════
		EnableTLS:   true,  // 启用 TLS 1.3：标准化程度高，广泛支持
		EnableNoise: true,  // 启用 Noise：libp2p 原生协议，更简洁

		// ════════════════════════════════════════════════════════════════════
		// 协议选择
		// ════════════════════════════════════════════════════════════════════
		PreferredProtocol: "tls", // 首选 TLS：更广泛的生态支持

		// ════════════════════════════════════════════════════════════════════
		// 协商配置
		// ════════════════════════════════════════════════════════════════════
		NegotiateTimeout: Duration(60 * time.Second), // 协商超时：60 秒，包括多轮协议协商

		// ════════════════════════════════════════════════════════════════════
		// TLS 配置
		// ════════════════════════════════════════════════════════════════════
		TLS: TLSConfig{
			MinVersion:         0x0304,                        // 最小版本：TLS 1.3（0x0304），拒绝旧版本
			MaxVersion:         0,                             // 最大版本：0 表示使用最新可用版本
			CertValidityPeriod: Duration(365 * 24 * time.Hour), // 证书有效期：1 年
			ClientAuth:         false,                         // 客户端认证：默认不要求（P2P 使用 PeerID）
		},

		// ════════════════════════════════════════════════════════════════════
		// Noise 配置
		// ════════════════════════════════════════════════════════════════════
		Noise: NoiseConfig{
			HandshakeTimeout: Duration(30 * time.Second), // 握手超时：30 秒
			Pattern:          "XX",                       // Noise 模式：XX（双向匿名认证）
		},
	}
}

// Validate 验证安全配置
func (c SecurityConfig) Validate() error {
	// 至少启用一种安全协议
	if !c.EnableTLS && !c.EnableNoise {
		return errors.New("at least one security protocol must be enabled")
	}

	// 验证首选协议
	switch c.PreferredProtocol {
	case "tls":
		if !c.EnableTLS {
			return errors.New("preferred protocol is tls but TLS is disabled")
		}
	case "noise":
		if !c.EnableNoise {
			return errors.New("preferred protocol is noise but Noise is disabled")
		}
	default:
		return errors.New("preferred protocol must be 'tls' or 'noise'")
	}

	// 验证协商超时
	if c.NegotiateTimeout <= 0 {
		return errors.New("negotiate timeout must be positive")
	}

	// 验证 TLS 配置
	if c.EnableTLS {
		if c.TLS.MinVersion < 0x0303 { // TLS 1.2
			return errors.New("TLS min version must be at least TLS 1.2 (0x0303)")
		}
		if c.TLS.MaxVersion != 0 && c.TLS.MaxVersion < c.TLS.MinVersion {
			return errors.New("TLS max version must be greater than or equal to min version")
		}
		if c.TLS.CertValidityPeriod <= 0 {
			return errors.New("TLS cert validity period must be positive")
		}
	}

	// 验证 Noise 配置
	if c.EnableNoise {
		if c.Noise.HandshakeTimeout <= 0 {
			return errors.New("noise handshake timeout must be positive")
		}
		if c.Noise.Pattern != "XX" && c.Noise.Pattern != "IK" {
			return errors.New("noise pattern must be 'XX' or 'IK'")
		}
	}

	return nil
}

// WithTLS 设置是否启用 TLS
func (c SecurityConfig) WithTLS(enabled bool) SecurityConfig {
	c.EnableTLS = enabled
	return c
}

// WithNoise 设置是否启用 Noise
func (c SecurityConfig) WithNoise(enabled bool) SecurityConfig {
	c.EnableNoise = enabled
	return c
}

// WithPreferredProtocol 设置首选协议
func (c SecurityConfig) WithPreferredProtocol(protocol string) SecurityConfig {
	c.PreferredProtocol = protocol
	return c
}

// WithNegotiateTimeout 设置协商超时
func (c SecurityConfig) WithNegotiateTimeout(timeout time.Duration) SecurityConfig {
	c.NegotiateTimeout = Duration(timeout)
	return c
}
