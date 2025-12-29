// Package netaddr 定义网络地址的基础接口
//
// 本包提供无依赖的 Address 接口，可被 endpoint、relay、transport 等包引用，
// 避免循环依赖问题。
package netaddr

// Address 网络地址接口
//
// Address 接口定义了网络地址的基本操作。
// 不同类型的地址（IP、DNS、中继等）都实现此接口。
//
// 本接口位于依赖层次的底层，不依赖 endpoint/relay 等上层接口包。
type Address interface {
	// Network 返回网络类型
	//
	// 常见返回值: "ip4", "ip6", "dns", "relay", "p2p-circuit"
	Network() string

	// String 返回地址字符串表示
	//
	// 格式示例:
	//   - "/ip4/192.168.1.1/tcp/8000"
	//   - "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmNodeID"
	//   - "/ip4/.../p2p/relay/p2p-circuit/p2p/target"
	String() string

	// Bytes 返回地址的字节表示
	Bytes() []byte

	// Equal 比较两个地址是否相等
	Equal(other Address) bool

	// IsPublic 是否是公网地址
	IsPublic() bool

	// IsPrivate 是否是私网地址
	IsPrivate() bool

	// IsLoopback 是否是回环地址
	IsLoopback() bool

	// Multiaddr 返回标准 multiaddr 格式的地址字符串
	//
	// 格式示例:
	//   - "/ip4/192.168.1.1/udp/8000/quic-v1"
	//   - "/ip6/::1/udp/4001/quic-v1"
	//   - "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/NodeID/p2p-circuit/p2p/TargetID"
	//
	// 注意: 此方法返回的地址必须以 / 开头，符合 multiaddr 标准格式。
	// 这与 String() 方法不同，String() 可能返回 host:port 格式用于日志和调试。
	Multiaddr() string
}

