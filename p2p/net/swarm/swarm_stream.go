package swarm

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// 验证 Stream 是否符合 go-dep2p-net Stream 接口
var _ network.Stream = &Stream{}

// Stream 是 swarm 使用的流类型。通常情况下,你不会直接使用这个类型
type Stream struct {
	// 流的唯一标识符
	id uint64

	// 底层的多路复用流
	stream network.MuxedStream
	// 关联的连接对象
	conn *Conn
	// 流管理作用域
	scope network.StreamManagementScope

	// 关闭锁,用于保护关闭相关的操作
	closeMx sync.Mutex
	// 标记流是否已关闭
	isClosed bool
	// acceptStreamGoroutineCompleted 表示处理入站流的 goroutine 是否已退出
	acceptStreamGoroutineCompleted bool

	// 流的协议标识符
	protocol atomic.Pointer[protocol.ID]

	// 流的统计信息
	stat network.Stats
}

// ID 返回流的唯一标识符字符串
// 格式: <peer id 的前10个字符>-<全局连接序号>-<全局流序号>
//
// 返回值:
//   - string 流的唯一标识符
func (s *Stream) ID() string {
	return fmt.Sprintf("%s-%d", s.conn.ID(), s.id)
}

// String 返回流的字符串表示
//
// 返回值:
//   - string 包含传输、本地地址、本地节点、远程地址、远程节点信息的字符串
func (s *Stream) String() string {
	return fmt.Sprintf(
		"<swarm.Stream[%s] %s (%s) <-> %s (%s)>",
		s.conn.conn.Transport(),
		s.conn.LocalMultiaddr(),
		s.conn.LocalPeer(),
		s.conn.RemoteMultiaddr(),
		s.conn.RemotePeer(),
	)
}

// Conn 返回与此流关联的连接对象
//
// 返回值:
//   - network.Conn 关联的网络连接对象
func (s *Stream) Conn() network.Conn {
	return s.conn
}

// Read 从流中读取字节
//
// 参数:
//   - p: []byte 用于存储读取数据的字节切片
//
// 返回值:
//   - int 实际读取的字节数
//   - error 读取过程中的错误,如果有的话
func (s *Stream) Read(p []byte) (int, error) {
	n, err := s.stream.Read(p)
	// TODO: 将此逻辑下推到更低层以提高准确性
	if s.conn.swarm.bwc != nil {
		s.conn.swarm.bwc.LogRecvMessage(int64(n))
		s.conn.swarm.bwc.LogRecvMessageStream(int64(n), s.Protocol(), s.Conn().RemotePeer())
	}
	return n, err
}

// Write 向流中写入字节,每次调用都会刷新
//
// 参数:
//   - p: []byte 要写入的字节切片
//
// 返回值:
//   - int 实际写入的字节数
//   - error 写入过程中的错误,如果有的话
func (s *Stream) Write(p []byte) (int, error) {
	n, err := s.stream.Write(p)
	// TODO: 将此逻辑下推到更低层以提高准确性
	if s.conn.swarm.bwc != nil {
		s.conn.swarm.bwc.LogSentMessage(int64(n))
		s.conn.swarm.bwc.LogSentMessageStream(int64(n), s.Protocol(), s.Conn().RemotePeer())
	}
	return n, err
}

// Close 关闭流,关闭两端并释放所有相关资源
//
// 返回值:
//   - error 关闭过程中的错误,如果有的话
func (s *Stream) Close() error {
	err := s.stream.Close()
	s.closeAndRemoveStream()
	return err
}

// Reset 重置流,向两端发送错误信号并释放所有相关资源
//
// 返回值:
//   - error 重置过程中的错误,如果有的话
func (s *Stream) Reset() error {
	err := s.stream.Reset()
	s.closeAndRemoveStream()
	return err
}

// closeAndRemoveStream 关闭并从连接中移除流
func (s *Stream) closeAndRemoveStream() {
	s.closeMx.Lock()
	defer s.closeMx.Unlock()
	if s.isClosed {
		return
	}
	s.isClosed = true
	// 在流处理程序退出之前,我们不希望阻止 swarm 关闭
	s.conn.swarm.refs.Done()
	// 仅在流处理程序完成后才从连接中清理流
	if s.acceptStreamGoroutineCompleted {
		s.conn.removeStream(s)
	}
}

// CloseWrite 关闭流的写入端,刷新所有数据并发送 EOF
// 此函数不释放资源,使用完流后需调用 Close 或 Reset
//
// 返回值:
//   - error 关闭写入端过程中的错误,如果有的话
func (s *Stream) CloseWrite() error {
	return s.stream.CloseWrite()
}

// CloseRead 关闭流的读取端
// 此函数不释放资源,使用完流后需调用 Close 或 Reset
//
// 返回值:
//   - error 关闭读取端过程中的错误,如果有的话
func (s *Stream) CloseRead() error {
	return s.stream.CloseRead()
}

// completeAcceptStreamGoroutine 标记流处理 goroutine 已完成
func (s *Stream) completeAcceptStreamGoroutine() {
	s.closeMx.Lock()
	defer s.closeMx.Unlock()
	if s.acceptStreamGoroutineCompleted {
		return
	}
	s.acceptStreamGoroutineCompleted = true
	if s.isClosed {
		s.conn.removeStream(s)
	}
}

// Protocol 返回在此流上协商的协议(如果已设置)
//
// 返回值:
//   - protocol.ID 协议标识符
func (s *Stream) Protocol() protocol.ID {
	p := s.protocol.Load()
	if p == nil {
		return ""
	}
	return *p
}

// SetProtocol 设置此流的协议
//
// 参数:
//   - p: protocol.ID 要设置的协议标识符
//
// 返回值:
//   - error 设置过程中的错误,如果有的话
//
// 注意:
//   - 这实际上只是记录了我们在此流上使用的协议,并不会执行实际的协议协商
//   - 协议协商通常由 Host 完成
func (s *Stream) SetProtocol(p protocol.ID) error {
	if err := s.scope.SetProtocol(p); err != nil {
		log.Debugf("设置协议失败: %v", err)
		return err
	}

	s.protocol.Store(&p)
	return nil
}

// SetDeadline 设置流的读写超时时间
//
// 参数:
//   - t: time.Time 超时时间点
//
// 返回值:
//   - error 设置过程中的错误,如果有的话
func (s *Stream) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

// SetReadDeadline 设置流的读取超时时间
//
// 参数:
//   - t: time.Time 超时时间点
//
// 返回值:
//   - error 设置过程中的错误,如果有的话
func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

// SetWriteDeadline 设置流的写入超时时间
//
// 参数:
//   - t: time.Time 超时时间点
//
// 返回值:
//   - error 设置过程中的错误,如果有的话
func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

// Stat 返回此流的元数据信息
//
// 返回值:
//   - network.Stats 流的统计信息
func (s *Stream) Stat() network.Stats {
	return s.stat
}

// Scope 返回流的作用域
//
// 返回值:
//   - network.StreamScope 流的作用域对象
func (s *Stream) Scope() network.StreamScope {
	return s.scope
}
