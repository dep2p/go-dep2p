// Package mocknetwork 是一个由 GoMock 生成的包
package mocknetwork

import (
	reflect "reflect"

	network "github.com/dep2p/go-dep2p/core/network"
	peer "github.com/dep2p/go-dep2p/core/peer"
	gomock "go.uber.org/mock/gomock"
)

// MockConnManagementScope 是 ConnManagementScope 接口的模拟实现
type MockConnManagementScope struct {
	// ctrl 是 gomock 控制器
	ctrl *gomock.Controller
	// recorder 用于记录方法调用
	recorder *MockConnManagementScopeMockRecorder
	// isgomock 标识这是一个 mock 对象
	isgomock struct{}
}

// MockConnManagementScopeMockRecorder 是 MockConnManagementScope 的记录器
type MockConnManagementScopeMockRecorder struct {
	mock *MockConnManagementScope
}

// NewMockConnManagementScope 创建一个新的 mock 实例
// 参数:
//   - ctrl: gomock 控制器
//
// 返回:
//   - *MockConnManagementScope: 新创建的 mock 实例
func NewMockConnManagementScope(ctrl *gomock.Controller) *MockConnManagementScope {
	mock := &MockConnManagementScope{ctrl: ctrl}
	mock.recorder = &MockConnManagementScopeMockRecorder{mock}
	return mock
}

// EXPECT 返回一个用于设置期望的对象
// 返回:
//   - *MockConnManagementScopeMockRecorder: mock 记录器
func (m *MockConnManagementScope) EXPECT() *MockConnManagementScopeMockRecorder {
	return m.recorder
}

// BeginSpan 模拟 BeginSpan 方法
// 返回:
//   - network.ResourceScopeSpan: 资源作用域范围
//   - error: 可能的错误
func (m *MockConnManagementScope) BeginSpan() (network.ResourceScopeSpan, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BeginSpan")
	ret0, _ := ret[0].(network.ResourceScopeSpan)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BeginSpan 表示对 BeginSpan 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockConnManagementScopeMockRecorder) BeginSpan() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BeginSpan", reflect.TypeOf((*MockConnManagementScope)(nil).BeginSpan))
}

// Done 模拟 Done 方法
func (m *MockConnManagementScope) Done() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Done")
}

// Done 表示对 Done 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockConnManagementScopeMockRecorder) Done() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Done", reflect.TypeOf((*MockConnManagementScope)(nil).Done))
}

// PeerScope 模拟 PeerScope 方法
// 返回:
//   - network.PeerScope: 对等节点作用域
func (m *MockConnManagementScope) PeerScope() network.PeerScope {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PeerScope")
	ret0, _ := ret[0].(network.PeerScope)
	return ret0
}

// PeerScope 表示对 PeerScope 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockConnManagementScopeMockRecorder) PeerScope() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PeerScope", reflect.TypeOf((*MockConnManagementScope)(nil).PeerScope))
}

// ReleaseMemory 模拟 ReleaseMemory 方法
// 参数:
//   - size: 要释放的内存大小
func (m *MockConnManagementScope) ReleaseMemory(size int) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReleaseMemory", size)
}

// ReleaseMemory 表示对 ReleaseMemory 的预期调用
// 参数:
//   - size: 要释放的内存大小
//
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockConnManagementScopeMockRecorder) ReleaseMemory(size any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReleaseMemory", reflect.TypeOf((*MockConnManagementScope)(nil).ReleaseMemory), size)
}

// ReserveMemory 模拟 ReserveMemory 方法
// 参数:
//   - size: 要预留的内存大小
//   - prio: 优先级
//
// 返回:
//   - error: 可能的错误
func (m *MockConnManagementScope) ReserveMemory(size int, prio uint8) error {
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
func (mr *MockConnManagementScopeMockRecorder) ReserveMemory(size, prio any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReserveMemory", reflect.TypeOf((*MockConnManagementScope)(nil).ReserveMemory), size, prio)
}

// SetPeer 模拟 SetPeer 方法
// 参数:
//   - arg0: 对等节点 ID
//
// 返回:
//   - error: 可能的错误
func (m *MockConnManagementScope) SetPeer(arg0 peer.ID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetPeer", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetPeer 表示对 SetPeer 的预期调用
// 参数:
//   - arg0: 对等节点 ID
//
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockConnManagementScopeMockRecorder) SetPeer(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetPeer", reflect.TypeOf((*MockConnManagementScope)(nil).SetPeer), arg0)
}

// Stat 模拟 Stat 方法
// 返回:
//   - network.ScopeStat: 作用域统计信息
func (m *MockConnManagementScope) Stat() network.ScopeStat {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stat")
	ret0, _ := ret[0].(network.ScopeStat)
	return ret0
}

// Stat 表示对 Stat 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockConnManagementScopeMockRecorder) Stat() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stat", reflect.TypeOf((*MockConnManagementScope)(nil).Stat))
}
