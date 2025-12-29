// Package yamux 提供基于 yamux 的多路复用实现
package yamux

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

// Muxer 封装 yamux.Session，实现 muxer.Muxer 接口
type Muxer struct {
	session    *yamux.Session
	isServer   bool
	closed     int32 // atomic
	numStreams int32 // atomic

	streamsMu sync.RWMutex
	streams   map[uint32]*Stream
}

// 确保实现 muxer.Muxer 接口
var _ muxerif.Muxer = (*Muxer)(nil)

// NewMuxer 从 yamux.Session 创建 Muxer 封装
func NewMuxer(session *yamux.Session, isServer bool) *Muxer {
	return &Muxer{
		session:  session,
		isServer: isServer,
		streams:  make(map[uint32]*Stream),
	}
}

// NewStream 创建新流
func (m *Muxer) NewStream(ctx context.Context) (muxerif.Stream, error) {
	if m.IsClosed() {
		return nil, fmt.Errorf("muxer 已关闭")
	}

	// yamux 的 OpenStream 不支持 context，我们需要在单独的 goroutine 中处理
	type result struct {
		stream *yamux.Stream
		err    error
	}
	resultCh := make(chan result, 1)

	go func() {
		s, err := m.session.OpenStream()
		select {
		case resultCh <- result{stream: s, err: err}:
			// 成功发送结果
		default:
			// context 已取消，关闭孤立的流以防止泄漏
			if s != nil {
				_ = s.Close()
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-resultCh:
		if r.err != nil {
			return nil, fmt.Errorf("创建流失败: %w", r.err)
		}

		stream := NewStream(r.stream)
		m.addStream(stream)
		return stream, nil
	}
}

// AcceptStream 接受新流
func (m *Muxer) AcceptStream() (muxerif.Stream, error) {
	if m.IsClosed() {
		return nil, fmt.Errorf("muxer 已关闭")
	}

	s, err := m.session.AcceptStream()
	if err != nil {
		return nil, fmt.Errorf("接受流失败: %w", err)
	}

	stream := NewStream(s)
	m.addStream(stream)
	return stream, nil
}

// Close 关闭多路复用器
func (m *Muxer) Close() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return nil // 已经关闭
	}

	// 关闭所有流，收集错误
	var closeErrors []error
	m.streamsMu.Lock()
	for _, stream := range m.streams {
		if err := stream.Close(); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	m.streams = make(map[uint32]*Stream)
	m.streamsMu.Unlock()

	// 关闭 session
	if err := m.session.Close(); err != nil {
		closeErrors = append(closeErrors, err)
	}

	// 如果有错误，返回第一个（保持接口兼容性）
	if len(closeErrors) > 0 {
		return closeErrors[0]
	}
	return nil
}

// IsClosed 检查是否已关闭
func (m *Muxer) IsClosed() bool {
	return atomic.LoadInt32(&m.closed) == 1 || m.session.IsClosed()
}

// NumStreams 返回当前流数量
func (m *Muxer) NumStreams() int {
	return int(atomic.LoadInt32(&m.numStreams))
}

// IsServer 返回是否是服务端
func (m *Muxer) IsServer() bool {
	return m.isServer
}

// Session 返回底层 yamux.Session
func (m *Muxer) Session() *yamux.Session {
	return m.session
}

// addStream 添加流到管理列表
func (m *Muxer) addStream(stream *Stream) {
	m.streamsMu.Lock()
	defer m.streamsMu.Unlock()

	// 防止重复添加同一 streamID
	if _, exists := m.streams[stream.ID()]; !exists {
		m.streams[stream.ID()] = stream
		atomic.AddInt32(&m.numStreams, 1)
	}
}

// removeStream 从管理列表移除流
func (m *Muxer) removeStream(streamID uint32) {
	m.streamsMu.Lock()
	defer m.streamsMu.Unlock()

	if _, ok := m.streams[streamID]; ok {
		delete(m.streams, streamID)
		atomic.AddInt32(&m.numStreams, -1)
	}
}

// GetStream 获取指定 ID 的流
func (m *Muxer) GetStream(streamID uint32) (*Stream, bool) {
	m.streamsMu.RLock()
	defer m.streamsMu.RUnlock()

	stream, ok := m.streams[streamID]
	return stream, ok
}

// AllStreams 返回所有流
func (m *Muxer) AllStreams() []*Stream {
	m.streamsMu.RLock()
	defer m.streamsMu.RUnlock()

	streams := make([]*Stream, 0, len(m.streams))
	for _, stream := range m.streams {
		streams = append(streams, stream)
	}
	return streams
}

// Ping 发送 ping 消息
func (m *Muxer) Ping() (time.Duration, error) {
	if m.IsClosed() {
		return 0, fmt.Errorf("muxer 已关闭")
	}

	return m.session.Ping()
}

