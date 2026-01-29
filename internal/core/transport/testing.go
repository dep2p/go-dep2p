// Package transport 测试辅助函数
package transport

import (
	"fmt"

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

// testQUICAddr 创建测试用的 QUIC 地址
func testQUICAddr(port int) types.Multiaddr {
	return testMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/udp/%d/quic-v1", port))
}

// testTCPAddr 创建测试用的 TCP 地址
func testTCPAddr(port int) types.Multiaddr {
	return testMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
}
