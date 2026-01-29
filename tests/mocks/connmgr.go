package mocks

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// MockConnMgr 模拟连接管理器（简化版）
//
// 注意：这不是完整的 interfaces.ConnManager 实现，
// 仅用于测试场景中的基本连接管理功能。
type MockConnMgr struct {
	// 连接存储
	Connections map[string][]interfaces.Connection
	Tags        map[string]map[string]int
	Protected   map[string]map[string]bool
	ConnCountVal int

	// 可覆盖的方法
	TagPeerFunc       func(peerID string, tag string, weight int)
	UntagPeerFunc     func(peerID string, tag string)
	ProtectFunc       func(peerID string, tag string)
	UnprotectFunc     func(peerID string, tag string) bool
	IsProtectedFunc   func(peerID string, tag string) bool
	TrimOpenConnsFunc func(ctx context.Context)
	CloseFunc         func() error
}

// NewMockConnMgr 创建带有默认值的 MockConnMgr
func NewMockConnMgr() *MockConnMgr {
	return &MockConnMgr{
		Connections: make(map[string][]interfaces.Connection),
		Tags:        make(map[string]map[string]int),
		Protected:   make(map[string]map[string]bool),
	}
}

// TagPeer 为节点添加标签
func (m *MockConnMgr) TagPeer(peerID string, tag string, weight int) {
	if m.TagPeerFunc != nil {
		m.TagPeerFunc(peerID, tag, weight)
		return
	}
	if m.Tags[peerID] == nil {
		m.Tags[peerID] = make(map[string]int)
	}
	m.Tags[peerID][tag] = weight
}

// UntagPeer 移除节点标签
func (m *MockConnMgr) UntagPeer(peerID string, tag string) {
	if m.UntagPeerFunc != nil {
		m.UntagPeerFunc(peerID, tag)
		return
	}
	if m.Tags[peerID] != nil {
		delete(m.Tags[peerID], tag)
	}
}

// UpsertTag 更新或插入节点标签
func (m *MockConnMgr) UpsertTag(peerID string, tag string, upsert func(int) int) {
	if m.Tags[peerID] == nil {
		m.Tags[peerID] = make(map[string]int)
	}
	m.Tags[peerID][tag] = upsert(m.Tags[peerID][tag])
}

// GetTagInfo 获取节点的标签信息
func (m *MockConnMgr) GetTagInfo(peerID string) *interfaces.TagInfo {
	if m.Tags[peerID] == nil {
		return nil
	}
	info := &interfaces.TagInfo{
		Tags: make(map[string]int),
	}
	for k, v := range m.Tags[peerID] {
		info.Tags[k] = v
	}
	return info
}

// TrimOpenConns 裁剪连接
func (m *MockConnMgr) TrimOpenConns(ctx context.Context) {
	if m.TrimOpenConnsFunc != nil {
		m.TrimOpenConnsFunc(ctx)
	}
}

// Notifee 返回连接通知接口
func (m *MockConnMgr) Notifee() interfaces.SwarmNotifier {
	return nil
}

// Protect 保护节点连接
func (m *MockConnMgr) Protect(peerID string, tag string) {
	if m.ProtectFunc != nil {
		m.ProtectFunc(peerID, tag)
		return
	}
	if m.Protected[peerID] == nil {
		m.Protected[peerID] = make(map[string]bool)
	}
	m.Protected[peerID][tag] = true
}

// Unprotect 取消保护节点连接
func (m *MockConnMgr) Unprotect(peerID string, tag string) bool {
	if m.UnprotectFunc != nil {
		return m.UnprotectFunc(peerID, tag)
	}
	if m.Protected[peerID] != nil {
		wasProtected := m.Protected[peerID][tag]
		delete(m.Protected[peerID], tag)
		return wasProtected
	}
	return false
}

// IsProtected 检查节点是否被保护
func (m *MockConnMgr) IsProtected(peerID string, tag string) bool {
	if m.IsProtectedFunc != nil {
		return m.IsProtectedFunc(peerID, tag)
	}
	if m.Protected[peerID] != nil {
		return m.Protected[peerID][tag]
	}
	return false
}

// ConnCount 返回当前连接数
func (m *MockConnMgr) ConnCount() int {
	return m.ConnCountVal
}

// DialedConnCount 返回拨号连接数
func (m *MockConnMgr) DialedConnCount() int {
	return 0
}

// Close 关闭连接管理器
func (m *MockConnMgr) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}
