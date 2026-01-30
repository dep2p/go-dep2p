// Package quic 实现 QUIC 传输
package quic

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/quic-go/quic-go"
)

// 确保实现了接口
var _ pkgif.Stream = (*Stream)(nil)

// Stream QUIC 流
type Stream struct {
	quicStream *quic.Stream
	conn       *Connection
	protocol   string
	closed     int32 // atomic: 0 = open, 1 = closed
	priority   int   // 流优先级 (v1.2 新增)

	// 状态机
	state   types.StreamState
	stateMu sync.RWMutex

	// 统计信息
	bytesRead    int64     // atomic: 已读取字节数
	bytesWritten int64     // atomic: 已写入字节数
	openedAt     time.Time // 打开时间
}

// newStream 创建新流（默认优先级）
func newStream(quicStream *quic.Stream, conn *Connection) *Stream {
	return newStreamWithPriority(quicStream, conn, int(pkgif.StreamPriorityNormal))
}

// newStreamWithPriority 创建新流（指定优先级）(v1.2 新增)
func newStreamWithPriority(quicStream *quic.Stream, conn *Connection, priority int) *Stream {
	s := &Stream{
		quicStream: quicStream,
		conn:       conn,
		protocol:   "", // 协议 ID 需要通过协议协商确定
		priority:   priority,
		state:      types.StreamStateOpen,
		openedAt:   time.Now(),
	}

	// 记录优先级到日志（用于调试）
	if priority != int(pkgif.StreamPriorityNormal) {
		logger.Debug("创建带优先级的流",
			"streamID", quicStream.StreamID(),
			"priority", priority)
	}

	// TODO: 当 quic-go 支持时，调用 quicStream.SetPriority(priority)
	// 目前 quic-go 的优先级支持需要通过 HTTP/3 层配置

	return s
}

// Priority 返回流优先级 (v1.2 新增)
func (s *Stream) Priority() int {
	return s.priority
}

// Read 读取数据
func (s *Stream) Read(p []byte) (n int, err error) {
	n, err = s.quicStream.Read(p)
	if n > 0 {
		atomic.AddInt64(&s.bytesRead, int64(n))
	}
	return n, err
}

// Write 写入数据
func (s *Stream) Write(p []byte) (n int, err error) {
	n, err = s.quicStream.Write(p)
	if n > 0 {
		atomic.AddInt64(&s.bytesWritten, int64(n))
	}
	return n, err
}

// Close 关闭流
func (s *Stream) Close() error {
	atomic.StoreInt32(&s.closed, 1)
	s.stateMu.Lock()
	s.state = types.StreamStateClosed
	s.stateMu.Unlock()
	return s.quicStream.Close()
}

// CloseRead 关闭读取端
func (s *Stream) CloseRead() error {
	s.stateMu.Lock()
	switch s.state {
	case types.StreamStateOpen:
		s.state = types.StreamStateReadClosed
	case types.StreamStateWriteClosed:
		s.state = types.StreamStateClosed
	}
	s.stateMu.Unlock()
	s.quicStream.CancelRead(0)
	return nil
}

// CloseWrite 关闭写入端
func (s *Stream) CloseWrite() error {
	s.stateMu.Lock()
	switch s.state {
	case types.StreamStateOpen:
		s.state = types.StreamStateWriteClosed
	case types.StreamStateReadClosed:
		s.state = types.StreamStateClosed
	}
	s.stateMu.Unlock()
	return s.quicStream.Close()
}

// Reset 重置流
func (s *Stream) Reset() error {
	atomic.StoreInt32(&s.closed, 1)
	s.stateMu.Lock()
	s.state = types.StreamStateReset
	s.stateMu.Unlock()
	s.quicStream.CancelWrite(0)
	return nil
}

// SetDeadline 设置读写截止时间
func (s *Stream) SetDeadline(t time.Time) error {
	return s.quicStream.SetDeadline(t)
}

// SetReadDeadline 设置读取截止时间
func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.quicStream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写入截止时间
func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.quicStream.SetWriteDeadline(t)
}

// ID 返回流 ID
func (s *Stream) ID() string {
	return fmt.Sprintf("%d", s.quicStream.StreamID())
}

// Protocol 返回流使用的协议 ID
func (s *Stream) Protocol() string {
	return s.protocol
}

// SetProtocol 设置流协议 ID
func (s *Stream) SetProtocol(protocol string) {
	s.protocol = protocol
}

// Conn 返回底层连接
func (s *Stream) Conn() pkgif.Connection {
	return s.conn
}

// IsClosed 检查流是否已关闭
func (s *Stream) IsClosed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

// Stat 返回流统计信息
func (s *Stream) Stat() types.StreamStat {
	conn := s.conn
	direction := types.DirUnknown
	if conn != nil {
		connStat := conn.Stat()
		// 转换 interfaces.Direction 到 types.Direction
		switch connStat.Direction {
		case pkgif.DirInbound:
			direction = types.DirInbound
		case pkgif.DirOutbound:
			direction = types.DirOutbound
		}
	}

	return types.StreamStat{
		Direction:    direction,
		Opened:       s.openedAt,
		Protocol:     types.ProtocolID(s.protocol),
		BytesRead:    atomic.LoadInt64(&s.bytesRead),
		BytesWritten: atomic.LoadInt64(&s.bytesWritten),
	}
}

// State 返回流当前状态
func (s *Stream) State() types.StreamState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}
