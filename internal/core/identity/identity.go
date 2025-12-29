package identity

import (
	cryptoif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Identity 实现
// ============================================================================

// identity Identity 接口的实现
type identity struct {
	privateKey cryptoif.PrivateKey
	publicKey  cryptoif.PublicKey
	nodeID     types.NodeID
}

// 确保实现接口
var _ identityif.Identity = (*identity)(nil)

// NewIdentity 从私钥创建身份
func NewIdentity(priv cryptoif.PrivateKey) *identity {
	pub := priv.PublicKey()
	return &identity{
		privateKey: priv,
		publicKey:  pub,
		nodeID:     NodeIDFromPublicKey(pub),
	}
}

// NewIdentityFromKeyPair 从密钥对创建身份
func NewIdentityFromKeyPair(priv cryptoif.PrivateKey, pub cryptoif.PublicKey) *identity {
	return &identity{
		privateKey: priv,
		publicKey:  pub,
		nodeID:     NodeIDFromPublicKey(pub),
	}
}

// ID 返回节点 ID
func (i *identity) ID() types.NodeID {
	return i.nodeID
}

// PublicKey 返回公钥
func (i *identity) PublicKey() cryptoif.PublicKey {
	return i.publicKey
}

// PrivateKey 返回私钥
func (i *identity) PrivateKey() cryptoif.PrivateKey {
	return i.privateKey
}

// Sign 签名数据
func (i *identity) Sign(data []byte) ([]byte, error) {
	return i.privateKey.Sign(data)
}

// Verify 验证签名
func (i *identity) Verify(data, signature []byte, pubKey cryptoif.PublicKey) (bool, error) {
	return pubKey.Verify(data, signature)
}

// KeyType 返回密钥类型
func (i *identity) KeyType() types.KeyType {
	return i.privateKey.Type()
}

