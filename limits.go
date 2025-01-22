package dep2p

import (
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/p2p/host/autonat"
	rcmgr "github.com/dep2p/go-dep2p/p2p/host/resource-manager"
	circuit "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/proto"
	relayv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/relay"
	"github.com/dep2p/go-dep2p/p2p/protocol/holepunch"
	"github.com/dep2p/go-dep2p/p2p/protocol/identify"
	"github.com/dep2p/go-dep2p/p2p/protocol/ping"
)

// SetDefaultServiceLimits 为内置的 dep2p 服务设置默认限制
// 参数：
//   - config: *rcmgr.ScalingLimitConfig 资源限制配置对象
func SetDefaultServiceLimits(config *rcmgr.ScalingLimitConfig) {
	// 为 identify 服务设置基础限制和增长限制
	// 服务级别限制:
	// - 入站流数量: 64
	// - 出站流数量: 64
	// - 总流数: 128
	// - 内存使用: 4MB
	// 增长限制与基础限制相同
	config.AddServiceLimit(
		identify.ServiceName,
		rcmgr.BaseLimit{StreamsInbound: 64, StreamsOutbound: 64, Streams: 128, Memory: 4 << 20},
		rcmgr.BaseLimitIncrease{StreamsInbound: 64, StreamsOutbound: 64, Streams: 128, Memory: 4 << 20},
	)

	// 为 identify 服务的每个对等节点设置限制
	// - 入站流数量: 16
	// - 出站流数量: 16
	// - 总流数: 32
	// - 内存使用: 1MB
	// 无增长限制
	config.AddServicePeerLimit(
		identify.ServiceName,
		rcmgr.BaseLimit{StreamsInbound: 16, StreamsOutbound: 16, Streams: 32, Memory: 1 << 20},
		rcmgr.BaseLimitIncrease{},
	)

	// 为 identify 协议(包括 identify 和 identify push)设置限制
	for _, id := range [...]protocol.ID{identify.ID, identify.IDPush} {
		// 协议级别限制:
		// - 入站流数量: 64
		// - 出站流数量: 64
		// - 总流数: 128
		// - 内存使用: 4MB
		// 增长限制与基础限制相同
		config.AddProtocolLimit(
			id,
			rcmgr.BaseLimit{StreamsInbound: 64, StreamsOutbound: 64, Streams: 128, Memory: 4 << 20},
			rcmgr.BaseLimitIncrease{StreamsInbound: 64, StreamsOutbound: 64, Streams: 128, Memory: 4 << 20},
		)

		// 为协议的每个对等节点设置限制
		// - 入站流数量: 16
		// - 出站流数量: 16
		// - 总流数: 32
		// - 内存使用: 32 * (256MB + 16KB)
		// 无增长限制
		config.AddProtocolPeerLimit(
			id,
			rcmgr.BaseLimit{StreamsInbound: 16, StreamsOutbound: 16, Streams: 32, Memory: 32 * (256<<20 + 16<<10)},
			rcmgr.BaseLimitIncrease{},
		)
	}

	// 为 ping 服务和协议设置限制
	// 服务和协议级别限制:
	// - 入站流数量: 64
	// - 出站流数量: 64
	// - 总流数: 64
	// - 内存使用: 4MB
	// 增长限制与基础限制相同
	addServiceAndProtocolLimit(config,
		ping.ServiceName, ping.ID,
		rcmgr.BaseLimit{StreamsInbound: 64, StreamsOutbound: 64, Streams: 64, Memory: 4 << 20},
		rcmgr.BaseLimitIncrease{StreamsInbound: 64, StreamsOutbound: 64, Streams: 64, Memory: 4 << 20},
	)

	// 为 ping 服务和协议的每个对等节点设置限制
	// - 入站流数量: 2
	// - 出站流数量: 3
	// - 总流数: 4
	// - 内存使用: 32 * (256MB + 16KB)
	// 无增长限制
	addServicePeerAndProtocolPeerLimit(
		config,
		ping.ServiceName, ping.ID,
		rcmgr.BaseLimit{StreamsInbound: 2, StreamsOutbound: 3, Streams: 4, Memory: 32 * (256<<20 + 16<<10)},
		rcmgr.BaseLimitIncrease{},
	)

	// 为 autonat 服务和协议设置限制
	// 服务和协议级别限制:
	// - 入站流数量: 64
	// - 出站流数量: 64
	// - 总流数: 64
	// - 内存使用: 4MB
	// 增长限制:
	// - 入站流数量: +4
	// - 出站流数量: +4
	// - 总流数: +4
	// - 内存使用: +2MB
	addServiceAndProtocolLimit(config,
		autonat.ServiceName, autonat.AutoNATProto,
		rcmgr.BaseLimit{StreamsInbound: 64, StreamsOutbound: 64, Streams: 64, Memory: 4 << 20},
		rcmgr.BaseLimitIncrease{StreamsInbound: 4, StreamsOutbound: 4, Streams: 4, Memory: 2 << 20},
	)

	// 为 autonat 服务和协议的每个对等节点设置限制
	// - 入站流数量: 2
	// - 出站流数量: 2
	// - 总流数: 2
	// - 内存使用: 1MB
	// 无增长限制
	addServicePeerAndProtocolPeerLimit(
		config,
		autonat.ServiceName, autonat.AutoNATProto,
		rcmgr.BaseLimit{StreamsInbound: 2, StreamsOutbound: 2, Streams: 2, Memory: 1 << 20},
		rcmgr.BaseLimitIncrease{},
	)

	// 为 holepunch 服务和协议设置限制
	// 服务和协议级别限制:
	// - 入站流数量: 32
	// - 出站流数量: 32
	// - 总流数: 64
	// - 内存使用: 4MB
	// 增长限制:
	// - 入站流数量: +8
	// - 出站流数量: +8
	// - 总流数: +16
	// - 内存使用: +4MB
	addServiceAndProtocolLimit(config,
		holepunch.ServiceName, holepunch.Protocol,
		rcmgr.BaseLimit{StreamsInbound: 32, StreamsOutbound: 32, Streams: 64, Memory: 4 << 20},
		rcmgr.BaseLimitIncrease{StreamsInbound: 8, StreamsOutbound: 8, Streams: 16, Memory: 4 << 20},
	)

	// 为 holepunch 服务和协议的每个对等节点设置限制
	// - 入站流数量: 2
	// - 出站流数量: 2
	// - 总流数: 2
	// - 内存使用: 1MB
	// 无增长限制
	addServicePeerAndProtocolPeerLimit(config,
		holepunch.ServiceName, holepunch.Protocol,
		rcmgr.BaseLimit{StreamsInbound: 2, StreamsOutbound: 2, Streams: 2, Memory: 1 << 20},
		rcmgr.BaseLimitIncrease{},
	)

	// 为 relay/v2 服务设置限制
	// 服务级别限制:
	// - 入站流数量: 256
	// - 出站流数量: 256
	// - 总流数: 256
	// - 内存使用: 16MB
	// 增长限制与基础限制相同
	config.AddServiceLimit(
		relayv2.ServiceName,
		rcmgr.BaseLimit{StreamsInbound: 256, StreamsOutbound: 256, Streams: 256, Memory: 16 << 20},
		rcmgr.BaseLimitIncrease{StreamsInbound: 256, StreamsOutbound: 256, Streams: 256, Memory: 16 << 20},
	)

	// 为 relay/v2 服务的每个对等节点设置限制
	// - 入站流数量: 64
	// - 出站流数量: 64
	// - 总流数: 64
	// - 内存使用: 1MB
	// 无增长限制
	config.AddServicePeerLimit(
		relayv2.ServiceName,
		rcmgr.BaseLimit{StreamsInbound: 64, StreamsOutbound: 64, Streams: 64, Memory: 1 << 20},
		rcmgr.BaseLimitIncrease{},
	)

	// 为 circuit 协议(包括 hop 和 stop)设置限制
	for _, proto := range [...]protocol.ID{circuit.ProtoIDv2Hop, circuit.ProtoIDv2Stop} {
		// 协议级别限制:
		// - 入站流数量: 640
		// - 出站流数量: 640
		// - 总流数: 640
		// - 内存使用: 16MB
		// 增长限制与基础限制相同
		config.AddProtocolLimit(
			proto,
			rcmgr.BaseLimit{StreamsInbound: 640, StreamsOutbound: 640, Streams: 640, Memory: 16 << 20},
			rcmgr.BaseLimitIncrease{StreamsInbound: 640, StreamsOutbound: 640, Streams: 640, Memory: 16 << 20},
		)

		// 为协议的每个对等节点设置限制
		// - 入站流数量: 128
		// - 出站流数量: 128
		// - 总流数: 128
		// - 内存使用: 32MB
		// 无增长限制
		config.AddProtocolPeerLimit(
			proto,
			rcmgr.BaseLimit{StreamsInbound: 128, StreamsOutbound: 128, Streams: 128, Memory: 32 << 20},
			rcmgr.BaseLimitIncrease{},
		)
	}
}

// addServiceAndProtocolLimit 为服务和协议添加资源限制
// 参数：
//   - config: *rcmgr.ScalingLimitConfig 资源限制配置对象
//   - service: string 服务名称
//   - proto: protocol.ID 协议ID
//   - limit: rcmgr.BaseLimit 基础限制
//   - increase: rcmgr.BaseLimitIncrease 限制增长配置
func addServiceAndProtocolLimit(config *rcmgr.ScalingLimitConfig, service string, proto protocol.ID, limit rcmgr.BaseLimit, increase rcmgr.BaseLimitIncrease) {
	config.AddServiceLimit(service, limit, increase)
	config.AddProtocolLimit(proto, limit, increase)
}

// addServicePeerAndProtocolPeerLimit 为服务和协议的对等节点添加资源限制
// 参数：
//   - config: *rcmgr.ScalingLimitConfig 资源限制配置对象
//   - service: string 服务名称
//   - proto: protocol.ID 协议ID
//   - limit: rcmgr.BaseLimit 基础限制
//   - increase: rcmgr.BaseLimitIncrease 限制增长配置
func addServicePeerAndProtocolPeerLimit(config *rcmgr.ScalingLimitConfig, service string, proto protocol.ID, limit rcmgr.BaseLimit, increase rcmgr.BaseLimitIncrease) {
	config.AddServicePeerLimit(service, limit, increase)
	config.AddProtocolPeerLimit(proto, limit, increase)
}
