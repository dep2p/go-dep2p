// Package identity 实现身份管理
package identity

import (
	"errors"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
// 错误定义
// ============================================================================

var (
	// ErrInvalidPeerID 无效的 PeerID
	ErrInvalidPeerID = errors.New("invalid peer id")
	// ErrEmptyPublicKey 空公钥
	ErrEmptyPublicKey = errors.New("empty public key")
)

// ============================================================================
// PeerID 派生（使用 pkg/crypto）
// ============================================================================

// PeerIDFromPublicKey 从公钥派生 PeerID
//
// 此函数是 pkg/crypto.PeerIDFromPublicKey 的包装，返回 string 类型。
//
// 派生算法：Base58(SHA256(序列化公钥))
//
// 返回：Base58 编码的字符串
func PeerIDFromPublicKey(pub pkgif.PublicKey) (string, error) {
	if pub == nil {
		return "", ErrEmptyPublicKey
	}

	// 获取底层的 crypto.PublicKey
	var cryptoPub crypto.PublicKey

	// 如果是 publicKeyAdapter，直接提取
	if adapter, ok := pub.(*publicKeyAdapter); ok {
		cryptoPub = adapter.PublicKey
	} else {
		// 否则通过 Raw() 和重新反序列化
		raw, err := pub.Raw()
		if err != nil {
			return "", err
		}
		cryptoPub, err = crypto.UnmarshalPublicKey(crypto.KeyType(pub.Type()), raw)
		if err != nil {
			return "", err
		}
	}

	// 使用 pkg/crypto 派生 PeerID
	peerID, err := crypto.PeerIDFromPublicKey(cryptoPub)
	if err != nil {
		return "", err
	}

	return string(peerID), nil
}

// ValidatePeerID 验证 PeerID 格式是否有效
func ValidatePeerID(peerID string) error {
	if peerID == "" {
		return ErrInvalidPeerID
	}

	// 简单验证：Base58 解码
	_, err := types.Base58Decode(peerID)
	if err != nil {
		return ErrInvalidPeerID
	}

	return nil
}

// ParsePeerID 解析 PeerID 字符串
func ParsePeerID(s string) (string, error) {
	if err := ValidatePeerID(s); err != nil {
		return "", err
	}
	return s, nil
}
