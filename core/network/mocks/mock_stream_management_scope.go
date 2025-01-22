// Package mocknetwork 是一个由 GoMock 生成的包
package mocknetwork

import (
	reflect "reflect"

	network "github.com/dep2p/go-dep2p/core/network"
	protocol "github.com/dep2p/go-dep2p/core/protocol"
	gomock "go.uber.org/mock/gomock"
)

// MockStreamManagementScope 是 StreamManagementScope 接口的模拟实现
type MockStreamManagementScope struct {
	// ctrl 是 gomock 控制器
	ctrl *gomock.Controller
	// recorder 用于记录方法调用
	recorder *MockStreamManagementScopeMockRecorder
	// isgomock 标识这是一个 mock 对象
	isgomock struct{}
}

// MockStreamManagementScopeMockRecorder 是 MockStreamManagementScope 的记录器
type MockStreamManagementScopeMockRecorder struct {
	// mock 指向关联的 MockStreamManagementScope 实例
	mock *MockStreamManagementScope
}

// NewMockStreamManagementScope 创建一个新的 mock 实例
// 参数:
//   - ctrl: gomock 控制器
//
// 返回:
//   - *MockStreamManagementScope: 新创建的 mock 实例
func NewMockStreamManagementScope(ctrl *gomock.Controller) *MockStreamManagementScope {
	mock := &MockStreamManagementScope{ctrl: ctrl}
	mock.recorder = &MockStreamManagementScopeMockRecorder{mock}
	return mock
}

// EXPECT 返回一个用于设置期望的对象
// 返回:
//   - *MockStreamManagementScopeMockRecorder: mock 记录器
func (m *MockStreamManagementScope) EXPECT() *MockStreamManagementScopeMockRecorder {
	return m.recorder
}

// BeginSpan 模拟 StreamManagementScope 的 BeginSpan 方法
// 返回:
//   - network.ResourceScopeSpan: 资源作用域跨度
//   - error: 可能的错误
func (m *MockStreamManagementScope) BeginSpan() (network.ResourceScopeSpan, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BeginSpan")
	ret0, _ := ret[0].(network.ResourceScopeSpan)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BeginSpan 表示对 BeginSpan 的预期调用
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) BeginSpan() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BeginSpan", reflect.TypeOf((*MockStreamManagementScope)(nil).BeginSpan))
}

// Done 模拟 StreamManagementScope 的 Done 方法
func (m *MockStreamManagementScope) Done() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Done")
}

// Done 表示对 Done 的预期调用
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) Done() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Done", reflect.TypeOf((*MockStreamManagementScope)(nil).Done))
}

// PeerScope 模拟 StreamManagementScope 的 PeerScope 方法
// 返回:
//   - network.PeerScope: 对等节点作用域
func (m *MockStreamManagementScope) PeerScope() network.PeerScope {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PeerScope")
	ret0, _ := ret[0].(network.PeerScope)
	return ret0
}

// PeerScope 表示对 PeerScope 的预期调用
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) PeerScope() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PeerScope", reflect.TypeOf((*MockStreamManagementScope)(nil).PeerScope))
}

// ProtocolScope 模拟 StreamManagementScope 的 ProtocolScope 方法
// 返回:
//   - network.ProtocolScope: 协议作用域
func (m *MockStreamManagementScope) ProtocolScope() network.ProtocolScope {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ProtocolScope")
	ret0, _ := ret[0].(network.ProtocolScope)
	return ret0
}

// ProtocolScope 表示对 ProtocolScope 的预期调用
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) ProtocolScope() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ProtocolScope", reflect.TypeOf((*MockStreamManagementScope)(nil).ProtocolScope))
}

// ReleaseMemory 模拟 StreamManagementScope 的 ReleaseMemory 方法
// 参数:
//   - size: 要释放的内存大小
func (m *MockStreamManagementScope) ReleaseMemory(size int) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReleaseMemory", size)
}

// ReleaseMemory 表示对 ReleaseMemory 的预期调用
// 参数:
//   - size: 要释放的内存大小
//
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) ReleaseMemory(size any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReleaseMemory", reflect.TypeOf((*MockStreamManagementScope)(nil).ReleaseMemory), size)
}

// ReserveMemory 模拟 StreamManagementScope 的 ReserveMemory 方法
// 参数:
//   - size: 要预留的内存大小
//   - prio: 优先级
//
// 返回:
//   - error: 可能的错误
func (m *MockStreamManagementScope) ReserveMemory(size int, prio uint8) error {
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
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) ReserveMemory(size, prio any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReserveMemory", reflect.TypeOf((*MockStreamManagementScope)(nil).ReserveMemory), size, prio)
}

// ServiceScope 模拟 StreamManagementScope 的 ServiceScope 方法
// 返回:
//   - network.ServiceScope: 服务作用域
func (m *MockStreamManagementScope) ServiceScope() network.ServiceScope {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ServiceScope")
	ret0, _ := ret[0].(network.ServiceScope)
	return ret0
}

// ServiceScope 表示对 ServiceScope 的预期调用
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) ServiceScope() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ServiceScope", reflect.TypeOf((*MockStreamManagementScope)(nil).ServiceScope))
}

// SetProtocol 模拟 StreamManagementScope 的 SetProtocol 方法
// 参数:
//   - proto: 协议 ID
//
// 返回:
//   - error: 可能的错误
func (m *MockStreamManagementScope) SetProtocol(proto protocol.ID) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetProtocol", proto)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetProtocol 表示对 SetProtocol 的预期调用
// 参数:
//   - proto: 协议 ID
//
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) SetProtocol(proto any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetProtocol", reflect.TypeOf((*MockStreamManagementScope)(nil).SetProtocol), proto)
}

// SetService 模拟 StreamManagementScope 的 SetService 方法
// 参数:
//   - srv: 服务名称
//
// 返回:
//   - error: 可能的错误
func (m *MockStreamManagementScope) SetService(srv string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetService", srv)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetService 表示对 SetService 的预期调用
// 参数:
//   - srv: 服务名称
//
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) SetService(srv any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetService", reflect.TypeOf((*MockStreamManagementScope)(nil).SetService), srv)
}

// Stat 模拟 StreamManagementScope 的 Stat 方法
// 返回:
//   - network.ScopeStat: 作用域统计信息
func (m *MockStreamManagementScope) Stat() network.ScopeStat {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stat")
	ret0, _ := ret[0].(network.ScopeStat)
	return ret0
}

// Stat 表示对 Stat 的预期调用
// 返回:
//   - *gomock.Call: 用于链式调用的 mock 对象
func (mr *MockStreamManagementScopeMockRecorder) Stat() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stat", reflect.TypeOf((*MockStreamManagementScope)(nil).Stat))
}
