package identity

import (
	"crypto/sha256"

	cryptoif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// NodeIDFromPublicKey 从公钥派生 NodeID
//
// 使用 SHA256(PublicKeyBytes) 作为 NodeID。
// 这确保了 NodeID 与公钥之间的唯一对应关系。
func NodeIDFromPublicKey(pubKey cryptoif.PublicKey) types.NodeID {
	hash := sha256.Sum256(pubKey.Bytes())
	return types.NodeID(hash)
}

