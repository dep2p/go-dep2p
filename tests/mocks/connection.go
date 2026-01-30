package mocks

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MockConnection 模拟 Connection 接口实现
type MockConnection struct {
	// 基本属性
	LocalPeerID   types.PeerID
	LocalAddr     types.Multiaddr
	RemotePeerID  types.PeerID
	RemoteAddr    types.Multiaddr
	Closed        bool
	Streams       []interfaces.Stream
	StatValue     interfaces.ConnectionStat
	ConnTypeValue interfaces.ConnectionType //

	// 可覆盖的方法
	LocalPeerFunc       func() types.PeerID
	LocalMultiaddrFunc  func() types.Multiaddr
	RemotePeerFunc      func() types.PeerID
	RemoteMultiaddrFunc func() types.Multiaddr
	NewStreamFunc       func(ctx context.Context) (interfaces.Stream, error)
	AcceptStreamFunc    func() (interfaces.Stream, error)
	GetStreamsFunc      func() []interfaces.Stream
	StatFunc            func() interfaces.ConnectionStat
	CloseFunc           func() error
	IsClosedFunc        func() bool
	ConnTypeFunc        func() interfaces.ConnectionType //

	// 调用记录
	NewStreamCalls int
}

// NewMockConnection 创建带有默认值的 MockConnection
func NewMockConnection(localPeer, remotePeer types.PeerID) *MockConnection {
	return &MockConnection{
		LocalPeerID:  localPeer,
		RemotePeerID: remotePeer,
		Streams:      make([]interfaces.Stream, 0),
		StatValue: interfaces.ConnectionStat{
			Direction: interfaces.DirOutbound,
			Opened:    time.Now().Unix(),
		},
	}
}

// LocalPeer 返回本地节点 ID
func (m *MockConnection) LocalPeer() types.PeerID {
	if m.LocalPeerFunc != nil {
		return m.LocalPeerFunc()
	}
	return m.LocalPeerID
}

// LocalMultiaddr 返回本地多地址
func (m *MockConnection) LocalMultiaddr() types.Multiaddr {
	if m.LocalMultiaddrFunc != nil {
		return m.LocalMultiaddrFunc()
	}
	return m.LocalAddr
}

// RemotePeer 返回远端节点 ID
func (m *MockConnection) RemotePeer() types.PeerID {
	if m.RemotePeerFunc != nil {
		return m.RemotePeerFunc()
	}
	return m.RemotePeerID
}

// RemoteMultiaddr 返回远端多地址
func (m *MockConnection) RemoteMultiaddr() types.Multiaddr {
	if m.RemoteMultiaddrFunc != nil {
		return m.RemoteMultiaddrFunc()
	}
	return m.RemoteAddr
}

// NewStream 在此连接上创建新流
func (m *MockConnection) NewStream(ctx context.Context) (interfaces.Stream, error) {
	m.NewStreamCalls++
	if m.NewStreamFunc != nil {
		return m.NewStreamFunc(ctx)
	}
	stream := NewMockStream()
	m.Streams = append(m.Streams, stream)
	return stream, nil
}

// NewStreamWithPriority 创建带优先级的流 (v1.2 新增)
func (m *MockConnection) NewStreamWithPriority(ctx context.Context, _ int) (interfaces.Stream, error) {
	return m.NewStream(ctx)
}

// SupportsStreamPriority Mock 连接不支持流优先级 (v1.2 新增)
func (m *MockConnection) SupportsStreamPriority() bool {
	return false
}

// AcceptStream 接受对方创建的流
func (m *MockConnection) AcceptStream() (interfaces.Stream, error) {
	if m.AcceptStreamFunc != nil {
		return m.AcceptStreamFunc()
	}
	return NewMockStream(), nil
}

// GetStreams 获取此连接上的所有流
func (m *MockConnection) GetStreams() []interfaces.Stream {
	if m.GetStreamsFunc != nil {
		return m.GetStreamsFunc()
	}
	return m.Streams
}

// Stat 返回连接统计
func (m *MockConnection) Stat() interfaces.ConnectionStat {
	if m.StatFunc != nil {
		return m.StatFunc()
	}
	m.StatValue.NumStreams = len(m.Streams)
	return m.StatValue
}

// Close 关闭连接
func (m *MockConnection) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	m.Closed = true
	return nil
}

// IsClosed 检查连接是否已关闭
func (m *MockConnection) IsClosed() bool {
	if m.IsClosedFunc != nil {
		return m.IsClosedFunc()
	}
	return m.Closed
}

// ConnType 返回连接类型（v2.0 新增）
func (m *MockConnection) ConnType() interfaces.ConnectionType {
	if m.ConnTypeFunc != nil {
		return m.ConnTypeFunc()
	}
	return m.ConnTypeValue // 默认为 ConnectionTypeDirect (0)
}

// 确保实现接口
var _ interfaces.Connection = (*MockConnection)(nil)
