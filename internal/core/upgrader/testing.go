// Package upgrader 实现连接升级器
package upgrader

import (
	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// testIdentity 创建测试用身份
func testIdentity() (*identity.Identity, error) {
	return identity.Generate()
}

// testPeerID 创建测试用 PeerID
func testPeerID(id *identity.Identity) types.PeerID {
	return types.PeerID(id.PeerID())
}
