package swarm

import (
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// SwarmStream Swarm 流封装
type SwarmStream struct {
	conn     *SwarmConn
	stream   pkgif.Stream // 底层流
	protocol string       // 协商后的协议 ID
}

// newSwarmStream 创建 Swarm 流
func newSwarmStream(conn *SwarmConn, stream pkgif.Stream) *SwarmStream {
	return &SwarmStream{
		conn:     conn,
		stream:   stream,
		protocol: "",
	}
}

// Read 读取数据
func (s *SwarmStream) Read(p []byte) (n int, err error) {
	n, err = s.stream.Read(p)
	if n > 0 {
		if bw := s.conn.getBandwidthCounter(); bw != nil {
			peer := string(s.conn.RemotePeer())
			proto := s.Protocol()
			bw.LogRecvStream(int64(n), proto, peer)
		}
	}
	return n, err
}

// Write 写入数据
func (s *SwarmStream) Write(p []byte) (n int, err error) {
	n, err = s.stream.Write(p)
	if n > 0 {
		if bw := s.conn.getBandwidthCounter(); bw != nil {
			peer := string(s.conn.RemotePeer())
			proto := s.Protocol()
			bw.LogSentStream(int64(n), proto, peer)
		}
	}
	return n, err
}

// Close 关闭流
func (s *SwarmStream) Close() error {
	// 从连接中移除
	s.conn.removeStream(s)
	
	// 关闭底层流
	return s.stream.Close()
}

// Protocol 返回协议 ID
func (s *SwarmStream) Protocol() string {
	// 优先返回本地设置的协议 ID
	if s.protocol != "" {
		return s.protocol
	}
	return s.stream.Protocol()
}

// SetProtocol 设置协议 ID（协议协商时使用）
func (s *SwarmStream) SetProtocol(protocol string) {
	s.protocol = protocol
	// 同时设置底层流的协议（如果支持）
	s.stream.SetProtocol(protocol)
}

// Conn 返回所属连接
func (s *SwarmStream) Conn() pkgif.Connection {
	return s.conn
}

// Reset 重置流
func (s *SwarmStream) Reset() error {
	return s.stream.Reset()
}

// CloseWrite 关闭写端（半关闭）
//
// 发送 FIN 信号告知对方"我已发送完毕"，但仍可读取对方的数据。
func (s *SwarmStream) CloseWrite() error {
	return s.stream.CloseWrite()
}

// CloseRead 关闭读端（半关闭）
//
// 告知传输层不再需要接收数据，但仍可写入。
func (s *SwarmStream) CloseRead() error {
	return s.stream.CloseRead()
}

// SetDeadline 设置读写超时
func (s *SwarmStream) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (s *SwarmStream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (s *SwarmStream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

// IsClosed 检查流是否已关闭
func (s *SwarmStream) IsClosed() bool {
	return s.stream.IsClosed()
}

// Stat 返回流统计信息
func (s *SwarmStream) Stat() types.StreamStat {
	return s.stream.Stat()
}

// State 返回流当前状态
func (s *SwarmStream) State() types.StreamState {
	return s.stream.State()
}
