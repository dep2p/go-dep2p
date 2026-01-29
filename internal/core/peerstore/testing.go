// Package peerstore 实现节点信息存储
package peerstore

import (
	"fmt"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// testPeerID 创建测试用的 PeerID
func testPeerID(s string) types.PeerID {
	return types.PeerID(s)
}

// testMultiaddr 创建测试用的 Multiaddr
func testMultiaddr(s string) types.Multiaddr {
	addr, _ := types.NewMultiaddr(s)
	return addr
}

// testProtocolID 创建测试用的 ProtocolID
func testProtocolID(s string) types.ProtocolID {
	return types.ProtocolID(s)
}

// testPubKey 创建测试用的 PublicKey
func testPubKey(s string) pkgif.PublicKey {
	// 简化实现：返回一个假的公钥
	return &testPublicKey{data: []byte(s)}
}

// testPublicKey 测试用的公钥实现
type testPublicKey struct {
	data []byte
}

func (k *testPublicKey) Raw() ([]byte, error) {
	return k.data, nil
}

func (k *testPublicKey) Type() pkgif.KeyType {
	return pkgif.KeyTypeEd25519
}

func (k *testPublicKey) Equals(other pkgif.PublicKey) bool {
	if other == nil {
		return false
	}
	otherData, err := other.Raw()
	if err != nil {
		return false
	}
	return string(k.data) == string(otherData)
}

func (k *testPublicKey) Verify(_ []byte, _ []byte) (bool, error) {
	return true, nil
}

func (k *testPublicKey) String() string {
	return fmt.Sprintf("TestPubKey(%s)", string(k.data))
}
