package mocks

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MockTransport 模拟 Transport 接口实现
type MockTransport struct {
	// 可覆盖的方法
	DialFunc      func(ctx context.Context, addr types.Multiaddr, peerID types.PeerID) (interfaces.Connection, error)
	ListenFunc    func(addr types.Multiaddr) (interfaces.Listener, error)
	CanDialFunc   func(addr types.Multiaddr) bool
	ProtocolsFunc func() []string

	// 调用记录
	DialCalls   []DialCall
	ListenCalls []ListenCall
}

// DialCall 记录 Dial 调用
type DialCall struct {
	Addr   types.Multiaddr
	PeerID types.PeerID
}

// ListenCall 记录 Listen 调用
type ListenCall struct {
	Addr types.Multiaddr
}

// NewMockTransport 创建带有默认值的 MockTransport
func NewMockTransport() *MockTransport {
	return &MockTransport{}
}

// Dial 拨号连接
func (m *MockTransport) Dial(ctx context.Context, addr types.Multiaddr, peerID types.PeerID) (interfaces.Connection, error) {
	m.DialCalls = append(m.DialCalls, DialCall{Addr: addr, PeerID: peerID})
	
	if m.DialFunc != nil {
		return m.DialFunc(ctx, addr, peerID)
	}
	return NewMockConnection("local-peer", peerID), nil
}

// Listen 监听地址
func (m *MockTransport) Listen(addr types.Multiaddr) (interfaces.Listener, error) {
	m.ListenCalls = append(m.ListenCalls, ListenCall{Addr: addr})
	
	if m.ListenFunc != nil {
		return m.ListenFunc(addr)
	}
	return &MockListener{AddrValue: addr}, nil
}

// CanDial 检查是否可以拨号
func (m *MockTransport) CanDial(addr types.Multiaddr) bool {
	if m.CanDialFunc != nil {
		return m.CanDialFunc(addr)
	}
	return true
}

// Protocols 返回支持的协议
func (m *MockTransport) Protocols() []string {
	if m.ProtocolsFunc != nil {
		return m.ProtocolsFunc()
	}
	return []string{"tcp", "quic"}
}

// Close 关闭传输层
func (m *MockTransport) Close() error {
	return nil
}

// ============================================================================
// MockListener
// ============================================================================

// MockListener 模拟 Listener（简化版）
type MockListener struct {
	AddrValue types.Multiaddr
	Closed    bool

	AcceptFunc    func() (interfaces.Connection, error)
	CloseFunc     func() error
	MultiaddrFunc func() types.Multiaddr
}

// Accept 接受连接
func (m *MockListener) Accept() (interfaces.Connection, error) {
	if m.AcceptFunc != nil {
		return m.AcceptFunc()
	}
	return NewMockConnection("local", "remote"), nil
}

// Close 关闭监听器
func (m *MockListener) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	m.Closed = true
	return nil
}

// Multiaddr 返回监听地址
func (m *MockListener) Multiaddr() types.Multiaddr {
	if m.MultiaddrFunc != nil {
		return m.MultiaddrFunc()
	}
	return m.AddrValue
}

// Addr 返回监听地址（兼容性方法）
func (m *MockListener) Addr() types.Multiaddr {
	return m.Multiaddr()
}
