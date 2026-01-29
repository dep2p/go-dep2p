// Package identity 实现身份管理
package identity

import (
	"fmt"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/identity")

// ============================================================================
// Identity 实现
// ============================================================================

// Identity 身份实现
//
// Identity 封装了节点的密码学身份，包括密钥对和派生的 PeerID。
type Identity struct {
	peerID  string
	privKey pkgif.PrivateKey
	pubKey  pkgif.PublicKey
}

// 确保实现接口
var _ pkgif.Identity = (*Identity)(nil)

// ============================================================================
// 构造函数
// ============================================================================

// New 从私钥创建身份
//
// 该函数：
//  1. 从私钥派生公钥
//  2. 从公钥派生 PeerID
//  3. 创建 Identity 实例
//
// 参数：
//   - privKey: 私钥
//
// 返回：
//   - *Identity: 身份实例
//   - error: 创建失败时的错误
func New(privKey pkgif.PrivateKey) (*Identity, error) {
	if privKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}

	// 从私钥获取公钥
	pubKey := privKey.PublicKey()
	if pubKey == nil {
		return nil, fmt.Errorf("failed to derive public key")
	}

	// 从公钥派生 PeerID
	peerID, err := PeerIDFromPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive peer id: %w", err)
	}

	return &Identity{
		peerID:  peerID,
		privKey: privKey,
		pubKey:  pubKey,
	}, nil
}

// FromKeyPair 从密钥对创建身份
func FromKeyPair(privKey pkgif.PrivateKey, pubKey pkgif.PublicKey) (*Identity, error) {
	if privKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}

	if pubKey == nil {
		return nil, fmt.Errorf("public key is nil")
	}

	// 验证密钥对匹配
	derivedPub := privKey.PublicKey()
	if !derivedPub.Equals(pubKey) {
		return nil, fmt.Errorf("key pair mismatch")
	}

	// 从公钥派生 PeerID
	peerID, err := PeerIDFromPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive peer id: %w", err)
	}

	return &Identity{
		peerID:  peerID,
		privKey: privKey,
		pubKey:  pubKey,
	}, nil
}

// Generate 生成新的身份（Ed25519）
func Generate() (*Identity, error) {
	logger.Debug("生成新身份")
	priv, pub, err := GenerateEd25519Key()
	if err != nil {
		logger.Error("生成密钥对失败", "error", err)
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	identity, err := FromKeyPair(priv, pub)
	if err == nil {
		logger.Debug("身份生成成功", "peerID", identity.PeerID()[:8])
	}
	return identity, err
}

// ============================================================================
// Identity 接口实现
// ============================================================================

// PeerID 返回节点 ID
func (i *Identity) PeerID() string {
	return i.peerID
}

// PublicKey 返回公钥
func (i *Identity) PublicKey() pkgif.PublicKey {
	return i.pubKey
}

// PrivateKey 返回私钥
func (i *Identity) PrivateKey() pkgif.PrivateKey {
	return i.privKey
}

// Sign 签名数据
func (i *Identity) Sign(data []byte) ([]byte, error) {
	return Sign(i.privKey, data)
}

// Verify 验证签名
//
// 使用本身份的公钥验证签名。
func (i *Identity) Verify(data, sig []byte) (bool, error) {
	return Verify(i.pubKey, data, sig)
}

// ============================================================================
// 辅助方法
// ============================================================================

// String 返回身份的字符串表示（PeerID）
func (i *Identity) String() string {
	return i.peerID
}

// KeyType 返回密钥类型
func (i *Identity) KeyType() pkgif.KeyType {
	return i.pubKey.Type()
}

