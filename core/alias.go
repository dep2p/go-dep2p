// Package core 通过类型别名提供对基础的、核心的 go-dep2p 原语的便捷访问。
package core

import (
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"

	"github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// Multiaddr 是 github.com/dep2p/multiformats/multiaddr 中 Multiaddr 类型的别名。
//
// 更多信息请参考该类型的文档。
type Multiaddr = multiaddr.Multiaddr

// PeerID 是 peer.ID 的别名。
//
// 更多信息请参考该类型的文档。
type PeerID = peer.ID

// ProtocolID 是 protocol.ID 的别名。
//
// 更多信息请参考该类型的文档。
type ProtocolID = protocol.ID

// PeerAddrInfo 是 peer.AddrInfo 的别名。
//
// 更多信息请参考该类型的文档。
type PeerAddrInfo = peer.AddrInfo

// Host 是 host.Host 的别名。
//
// 更多信息请参考该类型的文档。
type Host = host.Host

// Network 是 network.Network 的别名。
//
// 更多信息请参考该类型的文档。
type Network = network.Network

// Conn 是 network.Conn 的别名。
//
// 更多信息请参考该类型的文档。
type Conn = network.Conn

// Stream 是 network.Stream 的别名。
//
// 更多信息请参考该类型的文档。
type Stream = network.Stream
