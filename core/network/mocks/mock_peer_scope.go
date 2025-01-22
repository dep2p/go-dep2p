// Package mocknetwork 是一个由 GoMock 生成的包
package mocknetwork

import (
	reflect "reflect"

	network "github.com/dep2p/go-dep2p/core/network"
	peer "github.com/dep2p/go-dep2p/core/peer"
	gomock "go.uber.org/mock/gomock"
)

// MockPeerScope 是 PeerScope 接口的模拟实现
type MockPeerScope struct {
	// ctrl 是 gomock 控制器
	ctrl *gomock.Controller
	// recorder 用于记录方法调用
	recorder *MockPeerScopeMockRecorder
	// isgomock 标识这是一个 mock 对象
	isgomock struct{}
}

// MockPeerScopeMockRecorder 是 MockPeerScope 的记录器
type MockPeerScopeMockRecorder struct {
	mock *MockPeerScope
}

// NewMockPeerScope 创建一个新的 mock 实例
// 参数:
//   - ctrl: gomock 控制器
//
// 返回:
//   - *MockPeerScope: 新创建的 mock 实例
func NewMockPeerScope(ctrl *gomock.Controller) *MockPeerScope {
	mock := &MockPeerScope{ctrl: ctrl}
	mock.recorder = &MockPeerScopeMockRecorder{mock}
	return mock
}

// EXPECT 返回一个用于设置期望的对象
// 返回:
//   - *MockPeerScopeMockRecorder: mock 记录器
func (m *MockPeerScope) EXPECT() *MockPeerScopeMockRecorder {
	return m.recorder
}

// BeginSpan 模拟 BeginSpan 方法
// 返回:
//   - network.ResourceScopeSpan: 资源作用域范围
//   - error: 可能的错误
func (m *MockPeerScope) BeginSpan() (network.ResourceScopeSpan, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BeginSpan")
	ret0, _ := ret[0].(network.ResourceScopeSpan)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BeginSpan 表示对 BeginSpan 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockPeerScopeMockRecorder) BeginSpan() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BeginSpan", reflect.TypeOf((*MockPeerScope)(nil).BeginSpan))
}

// Peer 模拟 Peer 方法
// 返回:
//   - peer.ID: 对等节点 ID
func (m *MockPeerScope) Peer() peer.ID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Peer")
	ret0, _ := ret[0].(peer.ID)
	return ret0
}

// Peer 表示对 Peer 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockPeerScopeMockRecorder) Peer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Peer", reflect.TypeOf((*MockPeerScope)(nil).Peer))
}

// ReleaseMemory 模拟 ReleaseMemory 方法
// 参数:
//   - size: 要释放的内存大小
func (m *MockPeerScope) ReleaseMemory(size int) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReleaseMemory", size)
}

// ReleaseMemory 表示对 ReleaseMemory 的预期调用
// 参数:
//   - size: 要释放的内存大小
//
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockPeerScopeMockRecorder) ReleaseMemory(size any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReleaseMemory", reflect.TypeOf((*MockPeerScope)(nil).ReleaseMemory), size)
}

// ReserveMemory 模拟 ReserveMemory 方法
// 参数:
//   - size: 要预留的内存大小
//   - prio: 优先级
//
// 返回:
//   - error: 可能的错误
func (m *MockPeerScope) ReserveMemory(size int, prio uint8) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReserveMemory", size, prio)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReserveMemory 表示对 ReserveMemory 的预期调用
// 参数:
//   - size: 要预留的内存大小
//   - prio: 优先级
//
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockPeerScopeMockRecorder) ReserveMemory(size, prio any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReserveMemory", reflect.TypeOf((*MockPeerScope)(nil).ReserveMemory), size, prio)
}

// Stat 模拟 Stat 方法
// 返回:
//   - network.ScopeStat: 作用域统计信息
func (m *MockPeerScope) Stat() network.ScopeStat {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stat")
	ret0, _ := ret[0].(network.ScopeStat)
	return ret0
}

// Stat 表示对 Stat 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockPeerScopeMockRecorder) Stat() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stat", reflect.TypeOf((*MockPeerScope)(nil).Stat))
}
