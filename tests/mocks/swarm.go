package mocks

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MockSwarm 模拟 Swarm 接口实现
//
// 用于测试需要 Swarm 依赖的组件。
type MockSwarm struct {
	mu sync.RWMutex

	// 基本属性
	LocalPeerID   string
	ListenAddrsVal []string
	PeersVal      []string
	ConnsVal      []interfaces.Connection

	// 可覆盖的方法
	LocalPeerFunc            func() string
	ListenFunc               func(addrs ...string) error
	ListenAddrsFunc          func() []string
	PeersFunc                func() []string
	ConnsFunc                func() []interfaces.Connection
	ConnsToPeerFunc          func(peerID string) []interfaces.Connection
	ConnectednessFunc        func(peerID string) interfaces.Connectedness
	DialPeerFunc             func(ctx context.Context, peerID string) (interfaces.Connection, error)
	ClosePeerFunc            func(peerID string) error
	NewStreamFunc            func(ctx context.Context, peerID string) (interfaces.Stream, error)
	SetInboundStreamHandlerFunc func(handler interfaces.InboundStreamHandler)
	NotifyFunc               func(notifier interfaces.SwarmNotifier)
	CloseFunc                func() error

	// 内部状态
	inboundHandler interfaces.InboundStreamHandler
	notifiers      []interfaces.SwarmNotifier
	connections    map[string][]interfaces.Connection
	closed         bool

	// 调用记录
	ListenCalls    [][]string
	DialPeerCalls  []string
	NewStreamCalls []string
	ClosePeerCalls []string
}

// NewMockSwarm 创建带有默认值的 MockSwarm
func NewMockSwarm(localPeerID string) *MockSwarm {
	return &MockSwarm{
		LocalPeerID:    localPeerID,
		ListenAddrsVal: []string{"/ip4/127.0.0.1/tcp/4001"},
		PeersVal:       make([]string, 0),
		ConnsVal:       make([]interfaces.Connection, 0),
		connections:    make(map[string][]interfaces.Connection),
		notifiers:      make([]interfaces.SwarmNotifier, 0),
	}
}

// LocalPeer 返回本地节点 ID
func (m *MockSwarm) LocalPeer() string {
	if m.LocalPeerFunc != nil {
		return m.LocalPeerFunc()
	}
	return m.LocalPeerID
}

// Listen 监听指定地址
func (m *MockSwarm) Listen(addrs ...string) error {
	m.mu.Lock()
	m.ListenCalls = append(m.ListenCalls, addrs)
	m.ListenAddrsVal = append(m.ListenAddrsVal, addrs...)
	m.mu.Unlock()

	if m.ListenFunc != nil {
		return m.ListenFunc(addrs...)
	}
	return nil
}

// ListenAddrs 返回所有监听地址
func (m *MockSwarm) ListenAddrs() []string {
	if m.ListenAddrsFunc != nil {
		return m.ListenAddrsFunc()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ListenAddrsVal
}

// Peers 返回所有已连接的节点 ID
func (m *MockSwarm) Peers() []string {
	if m.PeersFunc != nil {
		return m.PeersFunc()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 从连接中提取节点 ID
	peers := make([]string, 0, len(m.connections))
	for peerID := range m.connections {
		peers = append(peers, peerID)
	}
	if len(peers) == 0 {
		return m.PeersVal
	}
	return peers
}

// Conns 返回所有活跃连接
func (m *MockSwarm) Conns() []interfaces.Connection {
	if m.ConnsFunc != nil {
		return m.ConnsFunc()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var conns []interfaces.Connection
	for _, peerConns := range m.connections {
		conns = append(conns, peerConns...)
	}
	if len(conns) == 0 {
		return m.ConnsVal
	}
	return conns
}

// ConnsToPeer 返回到指定节点的所有连接
func (m *MockSwarm) ConnsToPeer(peerID string) []interfaces.Connection {
	if m.ConnsToPeerFunc != nil {
		return m.ConnsToPeerFunc(peerID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[peerID]
}

// Connectedness 返回与指定节点的连接状态
func (m *MockSwarm) Connectedness(peerID string) interfaces.Connectedness {
	if m.ConnectednessFunc != nil {
		return m.ConnectednessFunc(peerID)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if conns, ok := m.connections[peerID]; ok && len(conns) > 0 {
		return interfaces.Connected
	}
	return interfaces.NotConnected
}

// DialPeer 拨号连接到指定节点
func (m *MockSwarm) DialPeer(ctx context.Context, peerID string) (interfaces.Connection, error) {
	m.mu.Lock()
	m.DialPeerCalls = append(m.DialPeerCalls, peerID)
	m.mu.Unlock()

	if m.DialPeerFunc != nil {
		return m.DialPeerFunc(ctx, peerID)
	}

	// 创建模拟连接
	conn := NewMockConnection(types.PeerID(m.LocalPeerID), types.PeerID(peerID))
	m.AddConnection(peerID, conn)
	return conn, nil
}

// ClosePeer 关闭与指定节点的所有连接
func (m *MockSwarm) ClosePeer(peerID string) error {
	m.mu.Lock()
	m.ClosePeerCalls = append(m.ClosePeerCalls, peerID)
	delete(m.connections, peerID)
	m.mu.Unlock()

	if m.ClosePeerFunc != nil {
		return m.ClosePeerFunc(peerID)
	}
	return nil
}

// NewStream 创建到指定节点的新流
func (m *MockSwarm) NewStream(ctx context.Context, peerID string) (interfaces.Stream, error) {
	m.mu.Lock()
	m.NewStreamCalls = append(m.NewStreamCalls, peerID)
	m.mu.Unlock()

	if m.NewStreamFunc != nil {
		return m.NewStreamFunc(ctx, peerID)
	}

	// 检查是否有连接
	m.mu.RLock()
	conns := m.connections[peerID]
	m.mu.RUnlock()

	if len(conns) == 0 {
		// 自动建立连接
		conn, err := m.DialPeer(ctx, peerID)
		if err != nil {
			return nil, err
		}
		return conn.NewStream(ctx)
	}

	return conns[0].NewStream(ctx)
}

// SetInboundStreamHandler 设置入站流处理器
func (m *MockSwarm) SetInboundStreamHandler(handler interfaces.InboundStreamHandler) {
	if m.SetInboundStreamHandlerFunc != nil {
		m.SetInboundStreamHandlerFunc(handler)
		return
	}
	m.mu.Lock()
	m.inboundHandler = handler
	m.mu.Unlock()
}

// Notify 注册连接事件通知
func (m *MockSwarm) Notify(notifier interfaces.SwarmNotifier) {
	if m.NotifyFunc != nil {
		m.NotifyFunc(notifier)
		return
	}
	m.mu.Lock()
	m.notifiers = append(m.notifiers, notifier)
	m.mu.Unlock()
}

// Close 关闭 Swarm
func (m *MockSwarm) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

// ============================================================================
// 测试辅助方法
// ============================================================================

// AddConnection 添加模拟连接（用于测试设置）
func (m *MockSwarm) AddConnection(peerID string, conn interfaces.Connection) {
	m.mu.Lock()
	m.connections[peerID] = append(m.connections[peerID], conn)
	m.mu.Unlock()

	// 通知监听器
	m.mu.RLock()
	for _, notifier := range m.notifiers {
		notifier.Connected(conn)
	}
	m.mu.RUnlock()
}

// RemoveConnection 移除模拟连接（用于测试设置）
func (m *MockSwarm) RemoveConnection(peerID string, conn interfaces.Connection) {
	m.mu.Lock()
	conns := m.connections[peerID]
	for i, c := range conns {
		if c == conn {
			m.connections[peerID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	m.mu.Unlock()

	// 通知监听器
	m.mu.RLock()
	for _, notifier := range m.notifiers {
		notifier.Disconnected(conn)
	}
	m.mu.RUnlock()
}

// AddPeer 添加模拟节点（用于测试设置）
func (m *MockSwarm) AddPeer(peerID string) {
	m.mu.Lock()
	m.PeersVal = append(m.PeersVal, peerID)
	m.mu.Unlock()
}

// SimulateInboundStream 模拟入站流（用于测试）
func (m *MockSwarm) SimulateInboundStream(stream interfaces.Stream) {
	m.mu.RLock()
	handler := m.inboundHandler
	m.mu.RUnlock()

	if handler != nil {
		handler(stream)
	}
}

// IsClosed 检查是否已关闭
func (m *MockSwarm) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// AddInboundConnection 添加入站连接到连接池
func (m *MockSwarm) AddInboundConnection(conn interfaces.Connection) {
	if conn == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnsVal = append(m.ConnsVal, conn)
}

// 确保实现接口
var _ interfaces.Swarm = (*MockSwarm)(nil)
