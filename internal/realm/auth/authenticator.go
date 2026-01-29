package auth

import (
	"fmt"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              认证器工厂
// ============================================================================

// AuthenticatorFactory 认证器工厂
type AuthenticatorFactory struct{}

// NewAuthenticatorFactory 创建认证器工厂
func NewAuthenticatorFactory() *AuthenticatorFactory {
	return &AuthenticatorFactory{}
}

// CreateAuthenticator 创建认证器
func (f *AuthenticatorFactory) CreateAuthenticator(
	mode interfaces.AuthMode,
	config AuthConfig,
) (interfaces.Authenticator, error) {
	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, err
	}

	switch mode {
	case interfaces.AuthModePSK:
		return f.createPSKAuthenticator(config)

	case interfaces.AuthModeCert:
		return f.createCertAuthenticator(config)

	case interfaces.AuthModeCustom:
		return f.createCustomAuthenticator(config)

	default:
		return nil, fmt.Errorf("%w: unknown auth mode: %v", ErrInvalidConfig, mode)
	}
}

// createPSKAuthenticator 创建 PSK 认证器
func (f *AuthenticatorFactory) createPSKAuthenticator(config AuthConfig) (interfaces.Authenticator, error) {
	if len(config.PSK) == 0 {
		return nil, fmt.Errorf("%w: PSK is required for PSK mode", ErrInvalidPSK)
	}

	return NewPSKAuthenticator(config.PSK, config.PeerID)
}

// createCertAuthenticator 创建证书认证器
func (f *AuthenticatorFactory) createCertAuthenticator(config AuthConfig) (interfaces.Authenticator, error) {
	if config.CertPath == "" || config.KeyPath == "" {
		return nil, fmt.Errorf("%w: cert and key paths are required for Cert mode", ErrInvalidCert)
	}

	return NewCertAuthenticator(config.CertPath, config.KeyPath, config.PeerID)
}

// createCustomAuthenticator 创建自定义认证器
func (f *AuthenticatorFactory) createCustomAuthenticator(config AuthConfig) (interfaces.Authenticator, error) {
	if config.CustomValidator == nil {
		return nil, fmt.Errorf("%w: custom validator is required for Custom mode", ErrInvalidConfig)
	}

	// 使用配置的 RealmID（如果有），否则生成默认
	realmID := "custom-realm"

	return NewCustomAuthenticator(realmID, config.PeerID, config.CustomValidator), nil
}
