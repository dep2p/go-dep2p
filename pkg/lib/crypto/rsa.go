package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"io"
)

// RSA 密钥常量
const (
	// RSAMinKeySize RSA 最小密钥大小（位）
	RSAMinKeySize = 2048
	// RSADefaultKeySize RSA 默认密钥大小（位）
	RSADefaultKeySize = 2048
	// RSAMaxKeySize RSA 最大密钥大小（位）
	RSAMaxKeySize = 8192
)

// ============================================================================
//                              RSAPublicKey
// ============================================================================

// RSAPublicKey RSA 公钥实现
type RSAPublicKey struct {
	k *rsa.PublicKey
}

// Raw 返回 PKIX 格式的公钥字节
func (k *RSAPublicKey) Raw() ([]byte, error) {
	return x509.MarshalPKIXPublicKey(k.k)
}

// Type 返回密钥类型
func (k *RSAPublicKey) Type() KeyType {
	return KeyTypeRSA
}

// Equals 比较两个公钥是否相等
func (k *RSAPublicKey) Equals(other Key) bool {
	rk, ok := other.(*RSAPublicKey)
	if !ok {
		return KeyEqual(k, other)
	}
	return k.k.N.Cmp(rk.k.N) == 0 && k.k.E == rk.k.E
}

// Verify 使用此公钥验证签名（PKCS#1 v1.5 + SHA-256）
func (k *RSAPublicKey) Verify(data, sig []byte) (bool, error) {
	hash := sha256.Sum256(data)
	err := rsa.VerifyPKCS1v15(k.k, crypto.SHA256, hash[:], sig)
	return err == nil, nil
}

// ============================================================================
//                              RSAPrivateKey
// ============================================================================

// RSAPrivateKey RSA 私钥实现
type RSAPrivateKey struct {
	k *rsa.PrivateKey
}

// Raw 返回 PKCS#1 格式的私钥字节
func (k *RSAPrivateKey) Raw() ([]byte, error) {
	return x509.MarshalPKCS1PrivateKey(k.k), nil
}

// Type 返回密钥类型
func (k *RSAPrivateKey) Type() KeyType {
	return KeyTypeRSA
}

// Equals 比较两个私钥是否相等
func (k *RSAPrivateKey) Equals(other Key) bool {
	rk, ok := other.(*RSAPrivateKey)
	if !ok {
		return KeyEqual(k, other)
	}
	return k.k.D.Cmp(rk.k.D) == 0 && k.k.N.Cmp(rk.k.N) == 0
}

// GetPublic 返回对应的公钥
func (k *RSAPrivateKey) GetPublic() PublicKey {
	return &RSAPublicKey{k: &k.k.PublicKey}
}

// PublicKey 返回对应的公钥（兼容 pkg/interfaces.PrivateKey 接口）
func (k *RSAPrivateKey) PublicKey() PublicKey {
	return k.GetPublic()
}

// Sign 使用此私钥签名数据（PKCS#1 v1.5 + SHA-256）
func (k *RSAPrivateKey) Sign(data []byte) ([]byte, error) {
	hash := sha256.Sum256(data)
	return rsa.SignPKCS1v15(rand.Reader, k.k, crypto.SHA256, hash[:])
}

// ============================================================================
//                              工厂函数
// ============================================================================

// GenerateRSAKey 生成新的 RSA 密钥对
//
// 参数：
//   - bits: 密钥大小（位），推荐 2048 或 4096
//   - src: 随机源
func GenerateRSAKey(bits int, src io.Reader) (PrivateKey, PublicKey, error) {
	if bits < RSAMinKeySize {
		return nil, nil, fmt.Errorf("RSA key size must be at least %d bits", RSAMinKeySize)
	}
	if bits > RSAMaxKeySize {
		return nil, nil, fmt.Errorf("RSA key size must be at most %d bits", RSAMaxKeySize)
	}

	priv, err := rsa.GenerateKey(src, bits)
	if err != nil {
		return nil, nil, err
	}
	return &RSAPrivateKey{k: priv}, &RSAPublicKey{k: &priv.PublicKey}, nil
}

// UnmarshalRSAPublicKey 从字节反序列化 RSA 公钥
//
// 支持 PKIX/X.509 格式
func UnmarshalRSAPublicKey(data []byte) (PublicKey, error) {
	pub, err := x509.ParsePKIXPublicKey(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPublicKey, err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, ErrInvalidPublicKey
	}

	if rsaPub.N.BitLen() < RSAMinKeySize {
		return nil, fmt.Errorf("%w: RSA key too small", ErrInvalidPublicKey)
	}

	return &RSAPublicKey{k: rsaPub}, nil
}

// UnmarshalRSAPrivateKey 从字节反序列化 RSA 私钥
//
// 支持 PKCS#1 和 PKCS#8 格式
func UnmarshalRSAPrivateKey(data []byte) (PrivateKey, error) {
	// 尝试 PKCS#1 格式
	if priv, err := x509.ParsePKCS1PrivateKey(data); err == nil {
		if priv.N.BitLen() < RSAMinKeySize {
			return nil, fmt.Errorf("%w: RSA key too small", ErrInvalidPrivateKey)
		}
		return &RSAPrivateKey{k: priv}, nil
	}

	// 尝试 PKCS#8 格式
	if key, err := x509.ParsePKCS8PrivateKey(data); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			if rsaKey.N.BitLen() < RSAMinKeySize {
				return nil, fmt.Errorf("%w: RSA key too small", ErrInvalidPrivateKey)
			}
			return &RSAPrivateKey{k: rsaKey}, nil
		}
	}

	return nil, ErrInvalidPrivateKey
}
