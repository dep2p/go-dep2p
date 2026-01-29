package mocks

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// MockNode 模拟节点（简化版）
//
// 注意：这不是完整的 interfaces.Node 实现，
// 仅用于测试场景中的基本节点功能。
type MockNode struct {
	// 节点属性
	IDValue   string
	Realms    map[string]*MockRealm
	HostValue interfaces.Host
	Started   bool
	Closed    bool

	// 可覆盖的方法
	IDFunc         func() string
	HostFunc       func() interfaces.Host
	JoinRealmFunc  func(ctx context.Context, realmID string, psk []byte) (interfaces.Realm, error)
	LeaveRealmFunc func(ctx context.Context, realmID string) error
	GetRealmFunc   func(realmID string) (interfaces.Realm, bool)
	GetRealmsFunc  func() []string
	StartFunc      func(ctx context.Context) error
	StopFunc       func() error
}

// NewMockNode 创建带有默认值的 MockNode
func NewMockNode(id string) *MockNode {
	return &MockNode{
		IDValue:   id,
		Realms:    make(map[string]*MockRealm),
		HostValue: NewMockHost(id),
	}
}

// ID 返回节点 ID
func (m *MockNode) ID() string {
	if m.IDFunc != nil {
		return m.IDFunc()
	}
	return m.IDValue
}

// Host 返回 Host
func (m *MockNode) Host() interfaces.Host {
	if m.HostFunc != nil {
		return m.HostFunc()
	}
	return m.HostValue
}

// JoinRealm 加入 Realm
func (m *MockNode) JoinRealm(_ context.Context, realmID string, psk []byte) (*MockRealm, error) {
	if m.JoinRealmFunc != nil {
		return nil, nil // 简化实现
	}
	realm := NewMockRealm(realmID)
	m.Realms[realmID] = realm
	return realm, nil
}

// LeaveRealm 离开 Realm
func (m *MockNode) LeaveRealm(ctx context.Context, realmID string) error {
	if m.LeaveRealmFunc != nil {
		return m.LeaveRealmFunc(ctx, realmID)
	}
	delete(m.Realms, realmID)
	return nil
}

// GetRealm 获取 Realm
func (m *MockNode) GetRealm(realmID string) (*MockRealm, bool) {
	if m.GetRealmFunc != nil {
		return nil, false // 简化实现
	}
	realm, ok := m.Realms[realmID]
	return realm, ok
}

// GetRealms 获取所有 Realm ID
func (m *MockNode) GetRealms() []string {
	if m.GetRealmsFunc != nil {
		return m.GetRealmsFunc()
	}
	ids := make([]string, 0, len(m.Realms))
	for id := range m.Realms {
		ids = append(ids, id)
	}
	return ids
}

// Start 启动节点
func (m *MockNode) Start(ctx context.Context) error {
	if m.StartFunc != nil {
		return m.StartFunc(ctx)
	}
	m.Started = true
	return nil
}

// Stop 停止节点
func (m *MockNode) Stop() error {
	if m.StopFunc != nil {
		return m.StopFunc()
	}
	m.Closed = true
	return nil
}
