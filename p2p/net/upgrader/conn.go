package upgrader

import (
	"fmt"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/transport"
)

// transportConn 实现了一个传输层连接
type transportConn struct {
	network.MuxedConn                                  // 多路复用连接
	network.ConnMultiaddrs                             // 连接多地址
	network.ConnSecurity                               // 连接安全性
	transport              transport.Transport         // 传输层
	scope                  network.ConnManagementScope // 连接管理范围
	stat                   network.ConnStats           // 连接统计信息

	muxer                     protocol.ID // 多路复用器协议ID
	security                  protocol.ID // 安全协议ID
	usedEarlyMuxerNegotiation bool        // 是否使用了早期多路复用器协商
}

// 确保 transportConn 实现了 transport.CapableConn 接口
var _ transport.CapableConn = &transportConn{}

// Transport 返回传输层对象
// 返回值:
//   - transport.Transport 传输层对象
func (t *transportConn) Transport() transport.Transport {
	return t.transport
}

// String 返回连接的字符串表示
// 返回值:
//   - string 连接的字符串表示
func (t *transportConn) String() string {
	// 获取传输层的字符串表示
	ts := ""
	if s, ok := t.transport.(fmt.Stringer); ok {
		ts = "[" + s.String() + "]"
	}
	// 返回格式化的连接字符串
	return fmt.Sprintf(
		"<stream.Conn%s %s (%s) <-> %s (%s)>",
		ts,
		t.LocalMultiaddr(),
		t.LocalPeer(),
		t.RemoteMultiaddr(),
		t.RemotePeer(),
	)
}

// Stat 返回连接的统计信息
// 返回值:
//   - network.ConnStats 连接统计信息
func (t *transportConn) Stat() network.ConnStats {
	return t.stat
}

// Scope 返回连接的管理范围
// 返回值:
//   - network.ConnScope 连接管理范围
func (t *transportConn) Scope() network.ConnScope {
	return t.scope
}

// Close 关闭连接
// 返回值:
//   - error 关闭时的错误信息
func (t *transportConn) Close() error {
	// 在关闭连接后标记范围完成
	defer t.scope.Done()
	return t.MuxedConn.Close()
}

// ConnState 返回连接的状态信息
// 返回值:
//   - network.ConnectionState 连接状态信息
func (t *transportConn) ConnState() network.ConnectionState {
	return network.ConnectionState{
		StreamMultiplexer:         t.muxer,                     // 多路复用器协议
		Security:                  t.security,                  // 安全协议
		Transport:                 "tcp",                       // 传输协议
		UsedEarlyMuxerNegotiation: t.usedEarlyMuxerNegotiation, // 是否使用早期多路复用器协商
	}
}
