// Package endpoint 提供 Endpoint 聚合模块的实现
package endpoint

import (
	"sync"
	"sync/atomic"
	"time"

	bandwidthif "github.com/dep2p/go-dep2p/pkg/interfaces/bandwidth"
	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Stream 实现了 coreif.Stream 接口
// 它包装了 Muxer 流，添加协议信息和连接引用
type Stream struct {
	// 底层流
	muxerStream muxerif.Stream

	// 元信息
	id         coreif.StreamID
	protocolID coreif.ProtocolID
	conn       *Connection
	priority   types.Priority

	// 状态
	state     coreif.StreamState
	stateMu   sync.RWMutex
	closed    int32 // atomic
	closeOnce sync.Once

	// 统计
	stats   types.StreamStats
	statsMu sync.RWMutex

	// 带宽计数器（可选）
	bwCounter bandwidthif.Counter

	// 创建时间
	createdAt time.Time
}

// 确保实现接口
var _ coreif.Stream = (*Stream)(nil)

// NewStream 创建新的流包装
func NewStream(
	muxerStream muxerif.Stream,
	id coreif.StreamID,
	protocolID coreif.ProtocolID,
	conn *Connection,
	priority types.Priority,
) *Stream {
	return &Stream{
		muxerStream: muxerStream,
		id:          id,
		protocolID:  protocolID,
		conn:        conn,
		priority:    priority,
		state:       coreif.StreamStateOpen,
		createdAt:   time.Now(),
	}
}

// ==================== io.Reader/Writer/Closer ====================

// Read 从流中读取数据
func (s *Stream) Read(p []byte) (n int, err error) {
	if s.IsClosed() {
		return 0, coreif.ErrStreamClosed
	}

	n, err = s.muxerStream.Read(p)
	if n > 0 {
		s.statsMu.Lock()
		s.stats.BytesRecv += uint64(n)
		s.statsMu.Unlock()

		// 记录带宽
		if s.bwCounter != nil && s.conn != nil {
			s.bwCounter.LogRecvMessageStream(int64(n), s.protocolID, s.conn.RemoteID())
		}
	}
	return n, err
}

// Write 向流写入数据
func (s *Stream) Write(p []byte) (n int, err error) {
	if s.IsClosed() {
		return 0, coreif.ErrStreamClosed
	}

	n, err = s.muxerStream.Write(p)
	if n > 0 {
		s.statsMu.Lock()
		s.stats.BytesSent += uint64(n)
		s.statsMu.Unlock()

		// 记录带宽
		if s.bwCounter != nil && s.conn != nil {
			s.bwCounter.LogSentMessageStream(int64(n), s.protocolID, s.conn.RemoteID())
		}
	}
	return n, err
}

// Close 关闭流
func (s *Stream) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil
	}

	var err error
	s.closeOnce.Do(func() {
		s.stateMu.Lock()
		s.state = coreif.StreamStateClosed
		s.stateMu.Unlock()

		err = s.muxerStream.Close()

		// 从连接中移除
		if s.conn != nil {
			s.conn.removeStream(s.id)
		}

		log.Debug("流已关闭",
			"streamID", s.id.String(),
			"protocolID", string(s.protocolID))
	})

	return err
}

// closeInternal 内部关闭方法（由 Connection.Close 调用）
// 不会调用 removeStream，因为连接关闭时会直接清空 streams map
func (s *Stream) closeInternal() error {
	if s.IsClosed() {
		return nil
	}

	var err error
	s.closeOnce.Do(func() {
		s.stateMu.Lock()
		s.state = coreif.StreamStateClosed
		s.stateMu.Unlock()

		err = s.muxerStream.Close()

		log.Debug("流已关闭（内部）",
			"streamID", s.id.String(),
			"protocolID", string(s.protocolID))
	})

	return err
}

// ==================== 元信息 ====================

// ID 返回流 ID
func (s *Stream) ID() coreif.StreamID {
	return s.id
}

// ProtocolID 返回协议 ID
func (s *Stream) ProtocolID() coreif.ProtocolID {
	return s.protocolID
}

// Connection 返回所属连接
func (s *Stream) Connection() coreif.Connection {
	return s.conn
}

// ==================== 超时控制 ====================

// SetDeadline 设置读写超时
func (s *Stream) SetDeadline(t time.Time) error {
	return s.muxerStream.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (s *Stream) SetReadDeadline(t time.Time) error {
	return s.muxerStream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (s *Stream) SetWriteDeadline(t time.Time) error {
	return s.muxerStream.SetWriteDeadline(t)
}

// ==================== 半关闭 ====================

// CloseRead 关闭读端
func (s *Stream) CloseRead() error {
	s.stateMu.Lock()
	switch s.state {
	case coreif.StreamStateOpen:
		s.state = coreif.StreamStateReadClosed
	case coreif.StreamStateWriteClosed:
		s.state = coreif.StreamStateClosed
	}
	s.stateMu.Unlock()

	return s.muxerStream.CloseRead()
}

// CloseWrite 关闭写端
func (s *Stream) CloseWrite() error {
	s.stateMu.Lock()
	switch s.state {
	case coreif.StreamStateOpen:
		s.state = coreif.StreamStateWriteClosed
	case coreif.StreamStateReadClosed:
		s.state = coreif.StreamStateClosed
	}
	s.stateMu.Unlock()

	return s.muxerStream.CloseWrite()
}

// ==================== 流控制 ====================

// SetPriority 设置流优先级（线程安全）
func (s *Stream) SetPriority(priority coreif.Priority) {
	s.stateMu.Lock()
	s.priority = priority
	s.stateMu.Unlock()
}

// Priority 返回流优先级（线程安全）
func (s *Stream) Priority() coreif.Priority {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.priority
}

// ==================== 统计信息 ====================

// Stats 返回流统计
func (s *Stream) Stats() coreif.StreamStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()

	return types.StreamStats{
		OpenedAt:  s.createdAt,
		BytesSent: s.stats.BytesSent,
		BytesRecv: s.stats.BytesRecv,
	}
}

// ==================== 状态检查 ====================

// IsClosed 检查流是否已关闭
func (s *Stream) IsClosed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

// State 返回流状态
func (s *Stream) State() coreif.StreamState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

// ==================== 带宽统计 ====================

// SetBandwidthCounter 设置带宽计数器（线程安全）
func (s *Stream) SetBandwidthCounter(counter bandwidthif.Counter) {
	s.stateMu.Lock()
	s.bwCounter = counter
	s.stateMu.Unlock()
}

