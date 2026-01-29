package protocol

// ============================================================================
//                           Relay 协议 ID
// ============================================================================
//
// Relay 是系统协议的特例，使用 /dep2p/relay/ 前缀而非 /dep2p/sys/relay/
// 这是为了与 Circuit v2 规范保持一致
// 格式: /dep2p/relay/<version>/{hop,stop}

const (
	// RelayHop HOP 协议
	// 用于建立中继连接（发起端）
	RelayHop ID = "/dep2p/relay/1.0.0/hop"

	// RelayStop STOP 协议
	// 用于接收中继连接（接收端）
	RelayStop ID = "/dep2p/relay/1.0.0/stop"
)

// RelayNamespace 用于 DHT 发现的命名空间
const RelayNamespace = "relay/1.0.0"

// RelayProtocols 返回所有 Relay 协议
func RelayProtocols() []ID {
	return []ID{RelayHop, RelayStop}
}
