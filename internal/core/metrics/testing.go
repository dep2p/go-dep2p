package metrics

import (
	"github.com/dep2p/go-dep2p/pkg/types"
)

// testPeerID 创建测试用的 PeerID
func testPeerID(s string) types.PeerID {
	return types.PeerID(s)
}

// testProtocolID 创建测试用的 ProtocolID
func testProtocolID(s string) types.ProtocolID {
	return types.ProtocolID(s)
}
