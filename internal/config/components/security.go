package components

import (
	"github.com/dep2p/go-dep2p/internal/config"
)

// SecurityOptions 安全选项
type SecurityOptions struct {
	// Protocol 安全协议
	// 可选: tls (默认), noise
	Protocol string

	// TLS TLS 配置
	TLS TLSOptions
}

// TLSOptions TLS 选项
type TLSOptions struct {
	// MinVersion 最小 TLS 版本
	MinVersion string

	// CipherSuites 加密套件
	CipherSuites []string
}

// NewSecurityOptions 从配置创建安全选项
func NewSecurityOptions(cfg *config.SecurityConfig) *SecurityOptions {
	return &SecurityOptions{
		Protocol: cfg.Protocol,
		TLS: TLSOptions{
			MinVersion:   cfg.TLS.MinVersion,
			CipherSuites: cfg.TLS.CipherSuites,
		},
	}
}

// DefaultSecurityOptions 默认安全选项
func DefaultSecurityOptions() *SecurityOptions {
	return &SecurityOptions{
		Protocol: "tls",
		TLS: TLSOptions{
			MinVersion:   "1.3",
			CipherSuites: nil, // 使用默认
		},
	}
}

