package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

// ============================================================================
//                              密钥文件格式
// ============================================================================

// 密钥文件格式：
//
//   ┌────────────────────────────────────────────────────────────┐
//   │                    密钥文件                                 │
//   ├────────────────────────────────────────────────────────────┤
//   │  Magic:     "DEP2P-KEY"  (9 bytes)                         │
//   │  Version:   uint8                                           │
//   │  Type:      uint8 (KeyType)                                │
//   │  Encrypted: uint8 (0=否, 1=是)                              │
//   │  Data:      密钥数据或加密数据                               │
//   └────────────────────────────────────────────────────────────┘
//
//   加密数据格式：
//   ┌────────────────────────────────────────────────────────────┐
//   │  Salt:       16 bytes                                       │
//   │  Nonce:      12 bytes                                       │
//   │  Ciphertext: 变长（AES-GCM 加密）                           │
//   └────────────────────────────────────────────────────────────┘

const (
	keyFileMagic   = "DEP2P-KEY"
	keyFileVersion = 1

	// 加密参数
	saltSize  = 16
	nonceSize = 12

	// Argon2 参数
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32
)

// ============================================================================
//                              Keystore 接口
// ============================================================================

// Keystore 密钥存储接口
type Keystore interface {
	// Has 检查是否存在指定 ID 的密钥
	Has(id string) (bool, error)

	// Put 存储密钥
	Put(id string, key PrivateKey) error

	// Get 获取密钥
	Get(id string) (PrivateKey, error)

	// Delete 删除密钥
	Delete(id string) error

	// List 列出所有密钥 ID
	List() ([]string, error)
}

// ============================================================================
//                              文件系统密钥存储
// ============================================================================

// FSKeystore 基于文件系统的密钥存储
type FSKeystore struct {
	dir      string
	password []byte // 可选：用于加密存储
}

// NewFSKeystore 创建文件系统密钥存储
//
// 参数：
//   - dir: 存储目录
//   - password: 加密密码（为空则不加密）
func NewFSKeystore(dir string, password []byte) (*FSKeystore, error) {
	// 创建目录
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	return &FSKeystore{
		dir:      dir,
		password: password,
	}, nil
}

// Has 检查是否存在指定 ID 的密钥
func (ks *FSKeystore) Has(id string) (bool, error) {
	path := ks.keyPath(id)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// Put 存储密钥
func (ks *FSKeystore) Put(id string, key PrivateKey) error {
	// 检查是否已存在
	exists, err := ks.Has(id)
	if err != nil {
		return err
	}
	if exists {
		return ErrKeyExists
	}

	// 序列化密钥
	data, err := ks.encodeKey(key)
	if err != nil {
		return err
	}

	// 写入文件
	path := ks.keyPath(id)
	return os.WriteFile(path, data, 0600)
}

// Get 获取密钥
func (ks *FSKeystore) Get(id string) (PrivateKey, error) {
	path := ks.keyPath(id)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, ErrKeyNotFound
	}
	if err != nil {
		return nil, err
	}

	return ks.decodeKey(data)
}

// Delete 删除密钥
func (ks *FSKeystore) Delete(id string) error {
	path := ks.keyPath(id)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return ErrKeyNotFound
	}
	return err
}

// List 列出所有密钥 ID
func (ks *FSKeystore) List() ([]string, error) {
	entries, err := os.ReadDir(ks.dir)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".key" {
			id := entry.Name()[:len(entry.Name())-4] // 移除 .key 后缀
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// keyPath 返回密钥文件路径
func (ks *FSKeystore) keyPath(id string) string {
	return filepath.Join(ks.dir, id+".key")
}

// encodeKey 编码密钥（可选加密）
func (ks *FSKeystore) encodeKey(key PrivateKey) ([]byte, error) {
	// 获取原始密钥数据
	raw, err := key.Raw()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	// 写入魔数
	buf.WriteString(keyFileMagic)

	// 写入版本
	buf.WriteByte(keyFileVersion)

	// 写入密钥类型
	buf.WriteByte(byte(key.Type()))

	if len(ks.password) > 0 {
		// 加密存储
		buf.WriteByte(1) // encrypted = true

		encrypted, err := encryptData(raw, ks.password)
		if err != nil {
			return nil, err
		}
		buf.Write(encrypted)
	} else {
		// 明文存储
		buf.WriteByte(0) // encrypted = false
		buf.Write(raw)
	}

	return buf.Bytes(), nil
}

// decodeKey 解码密钥
func (ks *FSKeystore) decodeKey(data []byte) (PrivateKey, error) {
	if len(data) < len(keyFileMagic)+3 {
		return nil, ErrInvalidKeyFile
	}

	// 验证魔数
	if string(data[:len(keyFileMagic)]) != keyFileMagic {
		return nil, ErrInvalidKeyFile
	}

	offset := len(keyFileMagic)

	// 读取版本
	version := data[offset]
	if version != keyFileVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidKeyFile, version)
	}
	offset++

	// 读取密钥类型
	keyType := KeyType(data[offset])
	offset++

	// 读取加密标志
	encrypted := data[offset] == 1
	offset++

	// 获取密钥数据
	keyData := data[offset:]

	if encrypted {
		if len(ks.password) == 0 {
			return nil, ErrInvalidPassword
		}
		var err error
		keyData, err = decryptData(keyData, ks.password)
		if err != nil {
			return nil, err
		}
	}

	return UnmarshalPrivateKey(keyType, keyData)
}

// ============================================================================
//                              加密辅助函数
// ============================================================================

// encryptData 使用 AES-GCM 加密数据
func encryptData(plaintext, password []byte) ([]byte, error) {
	// 生成随机盐
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	// 派生密钥
	key := argon2.IDKey(password, salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// 创建 AES-GCM
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 生成随机 nonce
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// 加密
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// 组装结果：salt || nonce || ciphertext
	result := make([]byte, saltSize+nonceSize+len(ciphertext))
	copy(result[:saltSize], salt)
	copy(result[saltSize:saltSize+nonceSize], nonce)
	copy(result[saltSize+nonceSize:], ciphertext)

	return result, nil
}

// decryptData 使用 AES-GCM 解密数据
func decryptData(data, password []byte) ([]byte, error) {
	if len(data) < saltSize+nonceSize {
		return nil, ErrDecryptionFailed
	}

	// 解析数据
	salt := data[:saltSize]
	nonce := data[saltSize : saltSize+nonceSize]
	ciphertext := data[saltSize+nonceSize:]

	// 派生密钥
	key := argon2.IDKey(password, salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// 创建 AES-GCM
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 解密
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// ============================================================================
//                              内存密钥存储
// ============================================================================

// MemKeystore 内存密钥存储（用于测试）
type MemKeystore struct {
	keys map[string]PrivateKey
}

// NewMemKeystore 创建内存密钥存储
func NewMemKeystore() *MemKeystore {
	return &MemKeystore{
		keys: make(map[string]PrivateKey),
	}
}

// Has 检查是否存在指定 ID 的密钥
func (ks *MemKeystore) Has(id string) (bool, error) {
	_, ok := ks.keys[id]
	return ok, nil
}

// Put 存储密钥
func (ks *MemKeystore) Put(id string, key PrivateKey) error {
	if _, ok := ks.keys[id]; ok {
		return ErrKeyExists
	}
	ks.keys[id] = key
	return nil
}

// Get 获取密钥
func (ks *MemKeystore) Get(id string) (PrivateKey, error) {
	key, ok := ks.keys[id]
	if !ok {
		return nil, ErrKeyNotFound
	}
	return key, nil
}

// Delete 删除密钥
func (ks *MemKeystore) Delete(id string) error {
	if _, ok := ks.keys[id]; !ok {
		return ErrKeyNotFound
	}
	delete(ks.keys, id)
	return nil
}

// List 列出所有密钥 ID
func (ks *MemKeystore) List() ([]string, error) {
	ids := make([]string, 0, len(ks.keys))
	for id := range ks.keys {
		ids = append(ids, id)
	}
	return ids, nil
}

// ============================================================================
//                              密码哈希
// ============================================================================

// HashPassword 使用 Argon2id 哈希密码
//
// 用于验证密码，而非直接存储。
func HashPassword(password []byte) []byte {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		// crypto/rand 读取失败是严重错误，应该 panic
		panic("crypto/rand read failed: " + err.Error())
	}
	return argon2.IDKey(password, salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
}

// DeriveKey 从密码派生加密密钥
//
// 参数：
//   - password: 用户密码
//   - salt: 随机盐
//
// 返回 32 字节的加密密钥。
func DeriveKey(password, salt []byte) []byte {
	return argon2.IDKey(password, salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
}

// SecureZero 安全清零字节切片
//
// 用于清除内存中的敏感数据。
func SecureZero(b []byte) {
	for i := range b {
		b[i] = 0
	}
	// 使用 SHA256 哈希防止编译器优化
	_ = sha256.Sum256(b)
}
