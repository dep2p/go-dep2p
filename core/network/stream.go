package network

import (
	"github.com/dep2p/go-dep2p/core/protocol"
)

// Stream 表示 dep2p 网络中两个代理之间的双向通道。
// "代理"可以根据需要进行细分,可以是"请求->响应"对,也可以是完整的协议。
//
// Stream 在底层由多路复用器支持。
type Stream interface {
	MuxedStream

	// ID 返回一个标识符,在本次运行期间唯一标识此主机中的此 Stream。
	// Stream ID 可能在重启后重复。
	ID() string

	// Protocol 返回流的协议 ID
	Protocol() protocol.ID
	// SetProtocol 设置流的协议 ID
	SetProtocol(id protocol.ID) error

	// Stat 返回与此流相关的元数据。
	Stat() Stats

	// Conn 返回此流所属的连接。
	Conn() Conn

	// Scope 返回此流的资源范围的用户视图
	Scope() StreamScope
}
