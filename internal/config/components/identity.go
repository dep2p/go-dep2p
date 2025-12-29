package components

import (
	"github.com/dep2p/go-dep2p/internal/config"
)

// IdentityOptions 身份选项
type IdentityOptions struct {
	// KeyFile 密钥文件路径
	// 如果为空，将自动生成新密钥
	KeyFile string

	// KeyType 密钥类型
	// 可选: ed25519 (默认)
	KeyType string
}

// NewIdentityOptions 从配置创建身份选项
func NewIdentityOptions(cfg *config.IdentityConfig) *IdentityOptions {
	return &IdentityOptions{
		KeyFile: cfg.KeyFile,
		KeyType: cfg.KeyType,
	}
}

// DefaultIdentityOptions 默认身份选项
func DefaultIdentityOptions() *IdentityOptions {
	return &IdentityOptions{
		KeyFile: "",
		KeyType: "ed25519",
	}
}

// NeedsGenerate 是否需要生成新密钥
func (o *IdentityOptions) NeedsGenerate() bool {
	return o.KeyFile == ""
}

