package event

import "github.com/dep2p/go-dep2p/core/network"

// EvtNATDeviceTypeChanged 是一个事件结构体,当传输协议的 NAT 设备类型发生变化时会发出此事件。
//
// 注意: 此事件仅在 AutoNAT 可达性为 Private 时才有意义。
// 此事件的消费者还应该同时消费 `EvtLocalReachabilityChanged` 事件,并且仅在 `EvtLocalReachabilityChanged` 的可达性为 Private 时才解释此事件。
type EvtNATDeviceTypeChanged struct {
	// TransportProtocol 是已确定 NAT 设备类型的传输协议。
	TransportProtocol network.NATTransportProtocol
	// NatDeviceType 表示该传输协议的 NAT 设备类型。
	// 目前可以是 `Cone NAT` 或 `Symmetric NAT`。
	// 请参阅 `network.NATDeviceType` 枚举的详细文档,以更好地理解这些类型的含义以及它们如何影响连接性和打洞。
	NatDeviceType network.NATDeviceType
}
