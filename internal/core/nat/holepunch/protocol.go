package holepunch

// HolePunchMsg 打洞协议消息
type HolePunchMsg struct {
	// Type 消息类型
	Type MsgType

	// ObservedAddrs 观察到的地址
	ObservedAddrs []string

	// Nonce 随机数（用于同步）
	Nonce []byte
}

// MsgType 消息类型
type MsgType int

const (
	// MsgTypeConnect 连接请求
	MsgTypeConnect MsgType = iota
	// MsgTypeSync 同步消息
	MsgTypeSync
)

func (t MsgType) String() string {
	switch t {
	case MsgTypeConnect:
		return "CONNECT"
	case MsgTypeSync:
		return "SYNC"
	default:
		return "UNKNOWN"
	}
}
