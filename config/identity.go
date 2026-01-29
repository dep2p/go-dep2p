package config

import (
	"errors"
)

// IdentityConfig 身份配置
//
// 管理节点的身份标识和密钥：
//   - 密钥类型（Ed25519/RSA/ECDSA/Secp256k1）
//   - 密钥存储路径
//   - 密钥生成参数
type IdentityConfig struct {
	// KeyType 密钥类型
	// 可选值: "Ed25519", "RSA", "ECDSA", "Secp256k1"
	// 推荐使用 Ed25519（默认）
	KeyType string `json:"key_type"`

	// KeyFile 密钥文件路径
	// 如果为空，将在内存中生成临时密钥
	// 生产环境建议持久化存储
	KeyFile string `json:"key_file"`

	// RSABits RSA 密钥位数
	// 仅当 KeyType="RSA" 时有效
	// 推荐 2048 或 4096
	RSABits int `json:"rsa_bits,omitempty"`

	// AutoGenerate 当密钥文件不存在时是否自动生成
	AutoGenerate bool `json:"auto_generate"`
}

// DefaultIdentityConfig 返回默认身份配置
func DefaultIdentityConfig() IdentityConfig {
	return IdentityConfig{
		KeyType:      "Ed25519",  // 默认使用 Ed25519：安全性高、性能好、密钥短
		KeyFile:      "",         // 默认空：内存中生成临时密钥，生产环境应设置持久化路径
		RSABits:      2048,       // RSA 密钥位数：2048 位（仅当 KeyType="RSA" 时使用）
		AutoGenerate: true,       // 默认启用：当 KeyFile 不存在时自动生成新密钥
	}
}

// Validate 验证身份配置
func (c IdentityConfig) Validate() error {
	// 验证密钥类型
	switch c.KeyType {
	case "Ed25519", "RSA", "ECDSA", "Secp256k1":
		// 有效类型
	default:
		return errors.New("invalid key type: must be Ed25519, RSA, ECDSA, or Secp256k1")
	}

	// 验证 RSA 位数
	if c.KeyType == "RSA" {
		if c.RSABits < 2048 {
			return errors.New("RSA key bits must be at least 2048")
		}
		if c.RSABits > 8192 {
			return errors.New("RSA key bits must not exceed 8192")
		}
	}

	return nil
}

// WithKeyType 设置密钥类型
func (c IdentityConfig) WithKeyType(keyType string) IdentityConfig {
	c.KeyType = keyType
	return c
}

// WithKeyFile 设置密钥文件路径
func (c IdentityConfig) WithKeyFile(path string) IdentityConfig {
	c.KeyFile = path
	return c
}

// WithRSABits 设置 RSA 密钥位数
func (c IdentityConfig) WithRSABits(bits int) IdentityConfig {
	c.RSABits = bits
	return c
}

// WithAutoGenerate 设置是否自动生成密钥
func (c IdentityConfig) WithAutoGenerate(auto bool) IdentityConfig {
	c.AutoGenerate = auto
	return c
}
