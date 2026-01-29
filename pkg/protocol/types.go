package protocol

import (
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ID 是协议标识符的类型别名
// 使用 types.ProtocolID 确保全局类型一致性
type ID = types.ProtocolID

// 协议前缀常量
const (
	// PrefixSys 系统协议前缀
	PrefixSys = "/dep2p/sys"

	// PrefixRelay Relay 协议前缀（系统协议特例）
	PrefixRelay = "/dep2p/relay"

	// PrefixRealm Realm 协议前缀
	PrefixRealm = "/dep2p/realm"

	// PrefixApp App 协议前缀
	PrefixApp = "/dep2p/app"
)
