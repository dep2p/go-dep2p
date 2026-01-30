package mocks

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// MockHost 模拟 Host 接口实现
//
// 所有字段都是可选的函数，如果为 nil，则返回零值。
// 这种设计允许测试只模拟需要的方法。
type MockHost struct {
	// 基本属性
	IDValue    string
	AddrsValue []string
	Closed     bool

	// 可覆盖的方法
	IDFunc                         func() string
	AddrsFunc                      func() []string
	ListenFunc                     func(addrs ...string) error
	ConnectFunc                    func(ctx context.Context, peerID string, addrs []string) error
	SetStreamHandlerFunc           func(protocolID string, handler interfaces.StreamHandler)
	RemoveStreamHandlerFunc        func(protocolID string)
	NewStreamFunc                  func(ctx context.Context, peerID string, protocolIDs ...string) (interfaces.Stream, error)
	PeerstoreFunc                  func() interfaces.Peerstore
	EventBusFunc                   func() interfaces.EventBus
	NetworkFunc                    func() interfaces.Swarm
	CloseFunc                      func() error
	AdvertisedAddrsFunc            func() []string
	ShareableAddrsFunc             func() []string
	HolePunchAddrsFunc             func() []string
	SetReachabilityCoordinatorFunc func(coordinator interfaces.ReachabilityCoordinator)

	// 调用记录（用于验证）
	ConnectCalls   []ConnectCall
	NewStreamCalls []NewStreamCall
}

// ConnectCall 记录 Connect 调用参数
type ConnectCall struct {
	PeerID string
	Addrs  []string
}

// NewStreamCall 记录 NewStream 调用参数
type NewStreamCall struct {
	PeerID      string
	ProtocolIDs []string
}

// NewMockHost 创建带有默认值的 MockHost
func NewMockHost(id string) *MockHost {
	return &MockHost{
		IDValue:    id,
		AddrsValue: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
}

// ID 返回节点 ID
func (m *MockHost) ID() string {
	if m.IDFunc != nil {
		return m.IDFunc()
	}
	return m.IDValue
}

// Addrs 返回监听地址
func (m *MockHost) Addrs() []string {
	if m.AddrsFunc != nil {
		return m.AddrsFunc()
	}
	return m.AddrsValue
}

// Listen 监听地址
func (m *MockHost) Listen(addrs ...string) error {
	if m.ListenFunc != nil {
		return m.ListenFunc(addrs...)
	}
	return nil
}

// Connect 连接到远程节点
func (m *MockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
	m.ConnectCalls = append(m.ConnectCalls, ConnectCall{PeerID: peerID, Addrs: addrs})
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx, peerID, addrs)
	}
	return nil
}

// SetStreamHandler 设置流处理器
func (m *MockHost) SetStreamHandler(protocolID string, handler interfaces.StreamHandler) {
	if m.SetStreamHandlerFunc != nil {
		m.SetStreamHandlerFunc(protocolID, handler)
	}
}

// RemoveStreamHandler 移除流处理器
func (m *MockHost) RemoveStreamHandler(protocolID string) {
	if m.RemoveStreamHandlerFunc != nil {
		m.RemoveStreamHandlerFunc(protocolID)
	}
}

// NewStream 创建新流
func (m *MockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (interfaces.Stream, error) {
	m.NewStreamCalls = append(m.NewStreamCalls, NewStreamCall{PeerID: peerID, ProtocolIDs: protocolIDs})
	if m.NewStreamFunc != nil {
		return m.NewStreamFunc(ctx, peerID, protocolIDs...)
	}
	return NewMockStream(), nil
}

// NewStreamWithPriority 创建带优先级的流 (v1.2 新增)
func (m *MockHost) NewStreamWithPriority(ctx context.Context, peerID string, protocolID string, _ int) (interfaces.Stream, error) {
	return m.NewStream(ctx, peerID, protocolID)
}

// Peerstore 返回节点存储
func (m *MockHost) Peerstore() interfaces.Peerstore {
	if m.PeerstoreFunc != nil {
		return m.PeerstoreFunc()
	}
	return nil
}

// EventBus 返回事件总线
func (m *MockHost) EventBus() interfaces.EventBus {
	if m.EventBusFunc != nil {
		return m.EventBusFunc()
	}
	return nil
}

// Network 返回底层 Swarm
func (m *MockHost) Network() interfaces.Swarm {
	if m.NetworkFunc != nil {
		return m.NetworkFunc()
	}
	return nil
}

// Close 关闭主机
func (m *MockHost) Close() error {
	m.Closed = true
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// AdvertisedAddrs 返回广播地址
func (m *MockHost) AdvertisedAddrs() []string {
	if m.AdvertisedAddrsFunc != nil {
		return m.AdvertisedAddrsFunc()
	}
	return m.Addrs()
}

// ShareableAddrs 返回可共享地址
func (m *MockHost) ShareableAddrs() []string {
	if m.ShareableAddrsFunc != nil {
		return m.ShareableAddrsFunc()
	}
	return nil
}

// HolePunchAddrs 返回用于打洞协商的地址列表
func (m *MockHost) HolePunchAddrs() []string {
	if m.HolePunchAddrsFunc != nil {
		return m.HolePunchAddrsFunc()
	}
	return nil
}

// SetReachabilityCoordinator 设置可达性协调器
func (m *MockHost) SetReachabilityCoordinator(coordinator interfaces.ReachabilityCoordinator) {
	if m.SetReachabilityCoordinatorFunc != nil {
		m.SetReachabilityCoordinatorFunc(coordinator)
	}
}

// HandleInboundStream 处理入站流
func (m *MockHost) HandleInboundStream(_ interfaces.Stream) {
	// Mock 实现：不做任何处理
}

// 确保实现接口
var _ interfaces.Host = (*MockHost)(nil)
