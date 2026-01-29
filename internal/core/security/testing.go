// Package security 实现安全传输
package security

import (
	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// testIdentity 创建测试用身份
func testIdentity() (*identity.Identity, error) {
	return identity.Generate()
}

// testPeerID 创建测试用的 PeerID
func testPeerID() types.PeerID {
	id, _ := identity.Generate()
	return types.PeerID(id.PeerID())
}
