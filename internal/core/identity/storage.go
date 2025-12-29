package identity

import (
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	cryptoif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// PEM 类型常量
const (
	pemTypeEd25519Private = "ED25519 PRIVATE KEY"
	pemTypeEd25519Public  = "ED25519 PUBLIC KEY"
	pemTypeECDSAPrivate   = "EC PRIVATE KEY"
	pemTypeECDSAPublic    = "EC PUBLIC KEY"
)

// 错误定义
var (
	// ErrInvalidPEM 无效的 PEM 数据
	ErrInvalidPEM = errors.New("invalid PEM data")
	// ErrUnsupportedKeyType 不支持的密钥类型
	ErrUnsupportedKeyType = errors.New("unsupported key type")
	// ErrKeyNotFound 密钥未找到
	ErrKeyNotFound = errors.New("key not found")
)

// ============================================================================
//                              私钥持久化
// ============================================================================

// SavePrivateKeyPEM 保存私钥到 PEM 文件
//
// 使用原子写操作（临时文件 + rename）防止部分写入导致的文件损坏。
// 文件权限设置为 0600，仅所有者可读写。
func SavePrivateKeyPEM(key cryptoif.PrivateKey, path string) error {
	var pemType string
	switch key.Type() {
	case types.KeyTypeEd25519:
		pemType = pemTypeEd25519Private
	case types.KeyTypeECDSAP256, types.KeyTypeECDSAP384:
		pemType = pemTypeECDSAPrivate
	default:
		return ErrUnsupportedKeyType
	}

	block := &pem.Block{
		Type:  pemType,
		Bytes: key.Bytes(),
	}

	data := pem.EncodeToMemory(block)
	return atomicWriteFile(path, data, 0600)
}

// LoadPrivateKeyPEM 从 PEM 文件加载私钥
func LoadPrivateKeyPEM(path string) (cryptoif.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrInvalidPEM
	}

	return parsePrivateKeyPEM(block)
}

// parsePrivateKeyPEM 解析 PEM 块为私钥
func parsePrivateKeyPEM(block *pem.Block) (cryptoif.PrivateKey, error) {
	switch block.Type {
	case pemTypeEd25519Private:
		if len(block.Bytes) != ed25519.PrivateKeySize {
			return nil, ErrInvalidKeySize
		}
		return NewEd25519PrivateKey(block.Bytes)
	case pemTypeECDSAPrivate, "PRIVATE KEY":
		// 尝试解析 ECDSA 私钥
		return ECDSAPrivateKeyFromPEM(pem.EncodeToMemory(block))
	default:
		return nil, ErrUnsupportedKeyType
	}
}

// ============================================================================
//                              公钥持久化
// ============================================================================

// SavePublicKeyPEM 保存公钥到 PEM 文件
//
// 使用原子写操作防止部分写入导致的文件损坏。
func SavePublicKeyPEM(key cryptoif.PublicKey, path string) error {
	var pemType string
	var keyBytes []byte

	switch key.Type() {
	case types.KeyTypeEd25519:
		pemType = pemTypeEd25519Public
		keyBytes = key.Bytes()
	case types.KeyTypeECDSAP256, types.KeyTypeECDSAP384:
		// ECDSA 公钥使用 X.509 PKIX 格式
		ecdsaKey, ok := key.(*ECDSAPublicKey)
		if !ok {
			return ErrUnsupportedKeyType
		}
		pemData, err := ECDSAPublicKeyToPEM(ecdsaKey)
		if err != nil {
			return err
		}
		return atomicWriteFile(path, pemData, 0600)
	default:
		return ErrUnsupportedKeyType
	}

	block := &pem.Block{
		Type:  pemType,
		Bytes: keyBytes,
	}

	data := pem.EncodeToMemory(block)
	return atomicWriteFile(path, data, 0600) // 仅所有者读写
}

// LoadPublicKeyPEM 从 PEM 文件加载公钥
func LoadPublicKeyPEM(path string) (cryptoif.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrInvalidPEM
	}

	return parsePublicKeyPEM(block)
}

// parsePublicKeyPEM 解析 PEM 块为公钥
func parsePublicKeyPEM(block *pem.Block) (cryptoif.PublicKey, error) {
	switch block.Type {
	case pemTypeEd25519Public:
		if len(block.Bytes) != ed25519.PublicKeySize {
			return nil, ErrInvalidKeySize
		}
		return NewEd25519PublicKey(block.Bytes)
	case pemTypeECDSAPublic, "PUBLIC KEY":
		// 尝试解析 ECDSA 公钥
		return ECDSAPublicKeyFromPEM(pem.EncodeToMemory(block))
	default:
		return nil, ErrUnsupportedKeyType
	}
}

// ============================================================================
//                              从字节创建密钥
// ============================================================================

// PrivateKeyFromBytes 从字节创建私钥
func PrivateKeyFromBytes(keyBytes []byte, keyType types.KeyType) (cryptoif.PrivateKey, error) {
	switch keyType {
	case types.KeyTypeEd25519:
		return NewEd25519PrivateKey(keyBytes)
	case types.KeyTypeECDSAP256, types.KeyTypeECDSAP384:
		// ECDSA 私钥使用 PKCS8 格式
		block := &pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: keyBytes,
		}
		return ECDSAPrivateKeyFromPEM(pem.EncodeToMemory(block))
	default:
		return nil, ErrUnsupportedKeyType
	}
}

// PublicKeyFromBytes 从字节创建公钥
func PublicKeyFromBytes(keyBytes []byte, keyType types.KeyType) (cryptoif.PublicKey, error) {
	switch keyType {
	case types.KeyTypeEd25519:
		return NewEd25519PublicKey(keyBytes)
	case types.KeyTypeECDSAP256:
		return NewECDSAPublicKeyFromBytes(keyBytes, nil) // 自动检测 P-256
	case types.KeyTypeECDSAP384:
		return NewECDSAPublicKeyFromBytes(keyBytes, nil) // 自动检测 P-384
	default:
		return nil, ErrUnsupportedKeyType
	}
}

// ============================================================================
//                              原子写操作
// ============================================================================

// atomicWriteFile 原子写文件
//
// 使用临时文件 + rename 策略，防止部分写入导致的文件损坏。
// 流程：
//  1. 写入临时文件（同目录下，前缀 .tmp-）
//  2. 同步到磁盘
//  3. 原子 rename 到目标路径
//
// 如果任何步骤失败，目标文件保持不变。
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	// 在同目录创建临时文件
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()

	// 确保失败时清理临时文件
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath) // 清理时忽略错误
		}
	}()

	// 写入数据
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	// 同步到磁盘
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("同步临时文件失败: %w", err)
	}

	// 设置权限
	if err := tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("设置文件权限失败: %w", err)
	}

	// 关闭文件
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	// 原子 rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("原子 rename 失败: %w", err)
	}

	success = true
	return nil
}

