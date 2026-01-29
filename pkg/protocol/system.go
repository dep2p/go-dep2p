package protocol

// ============================================================================
//                           系统协议 ID
// ============================================================================
//
// 系统协议是基础设施协议，无需 Realm 成员资格，任何节点可用。
// 格式: /dep2p/sys/<protocol>/<version>

// 身份识别协议
const (
	// Identify 身份识别协议
	// 用于节点间交换身份信息
	Identify ID = "/dep2p/sys/identify/1.0.0"

	// IdentifyPush 身份推送协议
	// 用于主动推送身份变更
	IdentifyPush ID = "/dep2p/sys/identify/push/1.0.0"
)

// 存活检测协议
const (
	// Ping Ping 协议
	// 用于检测节点连通性和测量延迟
	Ping ID = "/dep2p/sys/ping/1.0.0"

	// Heartbeat 心跳协议
	// 用于维持连接活跃状态
	Heartbeat ID = "/dep2p/sys/heartbeat/1.0.0"
)

// NAT 穿透协议
const (
	// AutoNAT AutoNAT 协议
	// 用于检测 NAT 类型和可达性
	AutoNAT ID = "/dep2p/sys/autonat/1.0.0"

	// HolePunch 打洞协议
	// 用于 NAT 穿透打洞
	HolePunch ID = "/dep2p/sys/holepunch/1.0.0"
)

// 发现协议
const (
	// DHT 分布式哈希表协议
	// 基于 Kademlia 算法的节点发现
	DHT ID = "/dep2p/sys/dht/1.0.0"

	// Rendezvous 会合点协议
	// 基于命名空间的节点发现
	Rendezvous ID = "/dep2p/sys/rendezvous/1.0.0"
)

// 可达性验证协议
const (
	// Reachability 可达性验证协议
	// 用于 dial-back 验证
	Reachability ID = "/dep2p/sys/reachability/1.0.0"

	// ReachabilityWitness 入站见证协议
	// 用于验证入站连接能力
	ReachabilityWitness ID = "/dep2p/sys/reachability/witness/1.0.0"

	// AddrMgmt 地址管理协议
	// 用于地址通告和管理
	AddrMgmt ID = "/dep2p/sys/addr-mgmt/1.0.0"

	// AddrExchange 地址交换协议
	// 用于连接升级时的地址交换
	AddrExchange ID = "/dep2p/sys/addr-exchange/1.0.0"
)

// 消息投递协议
const (
	// DeliveryAck 消息投递确认协议
	// 用于确认消息已送达
	DeliveryAck ID = "/dep2p/sys/delivery/ack/1.0.0"
)

// Gateway 协议
const (
	// GatewayRelay Gateway 中继协议
	// 用于 Gateway 节点间的中继
	GatewayRelay ID = "/dep2p/sys/gateway/relay/1.0.0"
)

// SystemProtocols 返回所有系统协议列表（不含 Relay）
func SystemProtocols() []ID {
	return []ID{
		// 身份识别
		Identify,
		IdentifyPush,
		// 存活检测
		Ping,
		Heartbeat,
		// NAT 穿透
		AutoNAT,
		HolePunch,
		// 发现
		DHT,
		Rendezvous,
		// 可达性
		Reachability,
		ReachabilityWitness,
		AddrMgmt,
		AddrExchange,
		// 消息投递
		DeliveryAck,
		// Gateway
		GatewayRelay,
	}
}

// AllSystemProtocols 返回所有系统协议列表（含 Relay）
func AllSystemProtocols() []ID {
	sys := SystemProtocols()
	relay := RelayProtocols()
	return append(sys, relay...)
}
