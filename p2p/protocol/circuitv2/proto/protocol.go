package proto

// 中继协议的版本和类型常量
const (
	// ProtoIDv2Hop 是中继协议 v2 版本的 hop 协议标识符
	// 用于在中继节点之间建立连接
	ProtoIDv2Hop = "/dep2p/circuit/relay/0.2.0/hop"

	// ProtoIDv2Stop 是中继协议 v2 版本的 stop 协议标识符
	// 用于终止中继连接
	ProtoIDv2Stop = "/dep2p/circuit/relay/0.2.0/stop"
)
