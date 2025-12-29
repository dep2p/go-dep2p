// Package noise 提供基于 Noise Protocol 的安全传输实现
package noise

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Identity Binding Protocol
// ============================================================================
//
// Noise 握手消息 payload 中携带 identity 证明，实现 Noise 静态密钥与
// dep2p identity 的强绑定。
//
// payload 格式:
//   - magic:    4 bytes ("D2P1")
//   - version:  1 byte (当前为 1)
//   - keyType:  1 byte (对应 types.KeyType)
//   - pubLen:   2 bytes (big-endian, 公钥长度)
//   - pubKey:   pubLen bytes (identity 公钥)
//   - sigLen:   2 bytes (big-endian, 签名长度)
//   - signature: sigLen bytes (对绑定消息的签名)
//
// 签名内容: SHA256("dep2p/noise-binding/v1" || noiseStaticPubKey)

const (
	// IdentityBindingMagic 协议魔数
	IdentityBindingMagic = "D2P1"

	// IdentityBindingVersion 协议版本
	IdentityBindingVersion = 1

	// IdentityBindingPrefix 签名前缀
	IdentityBindingPrefix = "dep2p/noise-binding/v1"

	// MaxPublicKeySize 最大公钥大小
	MaxPublicKeySize = 1024

	// MaxSignatureSize 最大签名大小
	MaxSignatureSize = 1024
)

// 错误定义
var (
	// ErrInvalidBindingMagic 无效的魔数
	ErrInvalidBindingMagic = errors.New("invalid identity binding magic")

	// ErrUnsupportedBindingVersion 不支持的版本
	ErrUnsupportedBindingVersion = errors.New("unsupported identity binding version")

	// ErrInvalidBindingPayload 无效的绑定 payload
	ErrInvalidBindingPayload = errors.New("invalid identity binding payload")

	// ErrBindingSignatureInvalid 签名验证失败
	ErrBindingSignatureInvalid = errors.New("identity binding signature invalid")

	// ErrPublicKeyTooLarge 公钥过大
	ErrPublicKeyTooLarge = errors.New("public key too large")

	// ErrSignatureTooLarge 签名过大
	ErrSignatureTooLarge = errors.New("signature too large")
)

// IdentityBinding identity 绑定信息
type IdentityBinding struct {
	// Identity 公钥
	PublicKey identityif.PublicKey

	// 派生的 NodeID
	NodeID types.NodeID

	// KeyType 密钥类型
	KeyType types.KeyType
}

// CreateBindingMessage 创建绑定消息（用于签名）
func CreateBindingMessage(noiseStaticPubKey []byte) []byte {
	// SHA256("dep2p/noise-binding/v1" || noiseStaticPubKey)
	h := sha256.New()
	h.Write([]byte(IdentityBindingPrefix))
	h.Write(noiseStaticPubKey)
	return h.Sum(nil)
}

// EncodeIdentityBindingPayload 编码 identity 绑定 payload
func EncodeIdentityBindingPayload(ident identityif.Identity, noiseStaticPubKey []byte) ([]byte, error) {
	if ident == nil {
		return nil, fmt.Errorf("identity 不能为空")
	}

	pubKey := ident.PublicKey()
	pubKeyBytes := pubKey.Bytes()
	keyType := pubKey.Type()

	if len(pubKeyBytes) > MaxPublicKeySize {
		return nil, ErrPublicKeyTooLarge
	}

	// 创建绑定消息并签名
	bindingMsg := CreateBindingMessage(noiseStaticPubKey)
	signature, err := ident.Sign(bindingMsg)
	if err != nil {
		return nil, fmt.Errorf("签名失败: %w", err)
	}

	if len(signature) > MaxSignatureSize {
		return nil, ErrSignatureTooLarge
	}

	// 编码 payload
	// magic(4) + version(1) + keyType(1) + pubLen(2) + pubKey + sigLen(2) + sig
	payloadLen := 4 + 1 + 1 + 2 + len(pubKeyBytes) + 2 + len(signature)
	buf := bytes.NewBuffer(make([]byte, 0, payloadLen))

	// magic
	buf.WriteString(IdentityBindingMagic)

	// version
	buf.WriteByte(IdentityBindingVersion)

	// keyType
	buf.WriteByte(byte(keyType))

	// pubLen + pubKey
	_ = binary.Write(buf, binary.BigEndian, uint16(len(pubKeyBytes))) //nolint:gosec // G115: 公钥长度由密码算法限制
	buf.Write(pubKeyBytes)

	// sigLen + signature
	_ = binary.Write(buf, binary.BigEndian, uint16(len(signature))) //nolint:gosec // G115: 签名长度由密码算法限制
	buf.Write(signature)

	return buf.Bytes(), nil
}

// DecodeAndVerifyIdentityBindingPayload 解码并验证 identity 绑定 payload
//
// keyFactory 用于从字节还原公钥，通过 Fx 依赖注入获取。
func DecodeAndVerifyIdentityBindingPayload(payload []byte, noiseStaticPubKey []byte, keyFactory identityif.KeyFactory) (*IdentityBinding, error) {
	if len(payload) < 10 { // 最小长度: magic(4) + version(1) + keyType(1) + pubLen(2) + sigLen(2)
		return nil, ErrInvalidBindingPayload
	}

	r := bytes.NewReader(payload)

	// 读取 magic
	magic := make([]byte, 4)
	if _, err := r.Read(magic); err != nil {
		return nil, fmt.Errorf("读取 magic 失败: %w", err)
	}
	if string(magic) != IdentityBindingMagic {
		return nil, ErrInvalidBindingMagic
	}

	// 读取 version
	version, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("读取 version 失败: %w", err)
	}
	if version != IdentityBindingVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedBindingVersion, version)
	}

	// 读取 keyType
	keyTypeByte, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("读取 keyType 失败: %w", err)
	}
	keyType := types.KeyType(keyTypeByte)

	// 读取 pubLen
	var pubLen uint16
	if err := binary.Read(r, binary.BigEndian, &pubLen); err != nil {
		return nil, fmt.Errorf("读取 pubLen 失败: %w", err)
	}
	if pubLen > MaxPublicKeySize {
		return nil, ErrPublicKeyTooLarge
	}

	// 读取 pubKey
	pubKeyBytes := make([]byte, pubLen)
	if _, err := r.Read(pubKeyBytes); err != nil {
		return nil, fmt.Errorf("读取 pubKey 失败: %w", err)
	}

	// 读取 sigLen
	var sigLen uint16
	if err := binary.Read(r, binary.BigEndian, &sigLen); err != nil {
		return nil, fmt.Errorf("读取 sigLen 失败: %w", err)
	}
	if sigLen > MaxSignatureSize {
		return nil, ErrSignatureTooLarge
	}

	// 读取 signature
	signature := make([]byte, sigLen)
	if _, err := r.Read(signature); err != nil {
		return nil, fmt.Errorf("读取 signature 失败: %w", err)
	}

	// 从字节还原公钥（使用通过 Fx 注入的 KeyFactory）
	pubKey, err := keyFactory.PublicKeyFromBytes(pubKeyBytes, keyType)
	if err != nil {
		return nil, fmt.Errorf("还原公钥失败: %w", err)
	}

	// 验证签名
	bindingMsg := CreateBindingMessage(noiseStaticPubKey)
	valid, err := pubKey.Verify(bindingMsg, signature)
	if err != nil {
		return nil, fmt.Errorf("签名验证出错: %w", err)
	}
	if !valid {
		return nil, ErrBindingSignatureInvalid
	}

	// 派生 NodeID（使用接口层的公共函数）
	nodeID := identityif.NodeIDFromPublicKey(pubKey)

	return &IdentityBinding{
		PublicKey: pubKey,
		NodeID:    nodeID,
		KeyType:   keyType,
	}, nil
}

