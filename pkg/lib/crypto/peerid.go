package crypto

import (
	"crypto/sha256"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              PeerID 派生
// ============================================================================

// PeerIDFromPublicKey 从公钥派生 PeerID
//
// 派生算法：Base58(SHA256(序列化公钥))
//
// 这与 pkg/types.PeerIDFromPublicKey 兼容，但接受 crypto.PublicKey 接口。
func PeerIDFromPublicKey(pub PublicKey) (types.PeerID, error) {
	if pub == nil {
		return types.EmptyPeerID, ErrNilPublicKey
	}

	// 序列化公钥
	data, err := MarshalPublicKey(pub)
	if err != nil {
		return types.EmptyPeerID, err
	}

	// SHA256 哈希
	hash := sha256.Sum256(data)

	// Base58 编码
	encoded := types.Base58Encode(hash[:])

	return types.PeerID(encoded), nil
}

// IDFromPublicKey 是 PeerIDFromPublicKey 的别名
func IDFromPublicKey(pub PublicKey) (types.PeerID, error) {
	return PeerIDFromPublicKey(pub)
}

// PeerIDFromPrivateKey 从私钥派生 PeerID
//
// 通过获取私钥对应的公钥，然后派生 PeerID。
func PeerIDFromPrivateKey(priv PrivateKey) (types.PeerID, error) {
	if priv == nil {
		return types.EmptyPeerID, ErrNilPrivateKey
	}

	return PeerIDFromPublicKey(priv.GetPublic())
}

// IDFromPrivateKey 是 PeerIDFromPrivateKey 的别名
func IDFromPrivateKey(priv PrivateKey) (types.PeerID, error) {
	return PeerIDFromPrivateKey(priv)
}

// ============================================================================
//                              公钥到 ID 的快速哈希
// ============================================================================

// PublicKeyHash 返回公钥的 SHA256 哈希（32 字节）
//
// 用于 DHT 路由等需要原始哈希的场景。
func PublicKeyHash(pub PublicKey) ([32]byte, error) {
	if pub == nil {
		return [32]byte{}, ErrNilPublicKey
	}

	data, err := MarshalPublicKey(pub)
	if err != nil {
		return [32]byte{}, err
	}

	return sha256.Sum256(data), nil
}

// ============================================================================
//                              密钥到 PeerID 验证
// ============================================================================

// VerifyPeerID 验证公钥是否对应给定的 PeerID
func VerifyPeerID(pub PublicKey, id types.PeerID) (bool, error) {
	derivedID, err := PeerIDFromPublicKey(pub)
	if err != nil {
		return false, err
	}
	return derivedID == id, nil
}
