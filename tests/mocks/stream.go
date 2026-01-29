package mocks

import (
	"io"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MockStream 模拟 Stream 接口实现
type MockStream struct {
	// 数据存储
	ReadData  []byte // 用于 Read 的预设数据
	WriteData []byte // 写入的数据会追加到这里
	ReadPos   int    // 当前读取位置

	// 状态
	Closed     bool
	ResetCalled bool
	ProtocolID string
	ConnValue  interfaces.Connection

	// 可覆盖的方法
	ReadFunc          func(p []byte) (n int, err error)
	WriteFunc         func(p []byte) (n int, err error)
	CloseFunc         func() error
	ResetFunc         func() error
	ProtocolFunc      func() string
	SetProtocolFunc   func(protocol string)
	ConnFunc          func() interfaces.Connection
	IsClosedFunc      func() bool
	CloseWriteFunc    func() error
	CloseReadFunc     func() error
	SetDeadlineFunc   func(t time.Time) error
	SetReadDeadlineFunc  func(t time.Time) error
	SetWriteDeadlineFunc func(t time.Time) error
	StatFunc          func() types.StreamStat
	StateFunc         func() types.StreamState
}

// NewMockStream 创建带有默认值的 MockStream
func NewMockStream() *MockStream {
	return &MockStream{
		WriteData:  make([]byte, 0),
		ProtocolID: "/test/1.0.0",
	}
}

// NewMockStreamWithData 创建带有预设读取数据的 MockStream
func NewMockStreamWithData(data []byte) *MockStream {
	return &MockStream{
		ReadData:   data,
		WriteData:  make([]byte, 0),
		ProtocolID: "/test/1.0.0",
	}
}

// Read 读取数据
func (m *MockStream) Read(p []byte) (n int, err error) {
	if m.ReadFunc != nil {
		return m.ReadFunc(p)
	}
	if m.Closed {
		return 0, io.EOF
	}
	if m.ReadPos >= len(m.ReadData) {
		return 0, io.EOF
	}
	n = copy(p, m.ReadData[m.ReadPos:])
	m.ReadPos += n
	return n, nil
}

// Write 写入数据
func (m *MockStream) Write(p []byte) (n int, err error) {
	if m.WriteFunc != nil {
		return m.WriteFunc(p)
	}
	if m.Closed {
		return 0, io.ErrClosedPipe
	}
	m.WriteData = append(m.WriteData, p...)
	return len(p), nil
}

// Close 关闭流
func (m *MockStream) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	m.Closed = true
	return nil
}

// Reset 重置流
func (m *MockStream) Reset() error {
	if m.ResetFunc != nil {
		return m.ResetFunc()
	}
	m.Closed = true
	m.ResetCalled = true
	return nil
}

// Protocol 返回协议 ID
func (m *MockStream) Protocol() string {
	if m.ProtocolFunc != nil {
		return m.ProtocolFunc()
	}
	return m.ProtocolID
}

// SetProtocol 设置协议 ID
func (m *MockStream) SetProtocol(protocol string) {
	if m.SetProtocolFunc != nil {
		m.SetProtocolFunc(protocol)
		return
	}
	m.ProtocolID = protocol
}

// Conn 返回关联的连接
func (m *MockStream) Conn() interfaces.Connection {
	if m.ConnFunc != nil {
		return m.ConnFunc()
	}
	return m.ConnValue
}

// IsClosed 返回是否已关闭
func (m *MockStream) IsClosed() bool {
	if m.IsClosedFunc != nil {
		return m.IsClosedFunc()
	}
	return m.Closed
}

// CloseWrite 关闭写入
func (m *MockStream) CloseWrite() error {
	if m.CloseWriteFunc != nil {
		return m.CloseWriteFunc()
	}
	return nil
}

// CloseRead 关闭读取
func (m *MockStream) CloseRead() error {
	if m.CloseReadFunc != nil {
		return m.CloseReadFunc()
	}
	return nil
}

// SetDeadline 设置截止时间
func (m *MockStream) SetDeadline(t time.Time) error {
	if m.SetDeadlineFunc != nil {
		return m.SetDeadlineFunc(t)
	}
	return nil
}

// SetReadDeadline 设置读取截止时间
func (m *MockStream) SetReadDeadline(t time.Time) error {
	if m.SetReadDeadlineFunc != nil {
		return m.SetReadDeadlineFunc(t)
	}
	return nil
}

// SetWriteDeadline 设置写入截止时间
func (m *MockStream) SetWriteDeadline(t time.Time) error {
	if m.SetWriteDeadlineFunc != nil {
		return m.SetWriteDeadlineFunc(t)
	}
	return nil
}

// Stat 返回流统计信息
func (m *MockStream) Stat() types.StreamStat {
	if m.StatFunc != nil {
		return m.StatFunc()
	}
	return types.StreamStat{
		Direction:    types.DirUnknown,
		Opened:       time.Now(),
		Protocol:     types.ProtocolID(m.ProtocolID),
		BytesRead:    int64(m.ReadPos),
		BytesWritten: int64(len(m.WriteData)),
	}
}

// State 返回流状态
func (m *MockStream) State() types.StreamState {
	if m.StateFunc != nil {
		return m.StateFunc()
	}
	if m.Closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// 确保实现接口
var _ interfaces.Stream = (*MockStream)(nil)
