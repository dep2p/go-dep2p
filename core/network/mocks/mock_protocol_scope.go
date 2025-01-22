// Package mocknetwork 是一个由 GoMock 生成的包
package mocknetwork

import (
	reflect "reflect"

	network "github.com/dep2p/go-dep2p/core/network"
	protocol "github.com/dep2p/go-dep2p/core/protocol"
	gomock "go.uber.org/mock/gomock"
)

// MockProtocolScope 是 ProtocolScope 接口的模拟实现
type MockProtocolScope struct {
	// ctrl 是 gomock 控制器
	ctrl *gomock.Controller
	// recorder 用于记录方法调用
	recorder *MockProtocolScopeMockRecorder
	// isgomock 标识这是一个 mock 对象
	isgomock struct{}
}

// MockProtocolScopeMockRecorder 是 MockProtocolScope 的记录器
type MockProtocolScopeMockRecorder struct {
	mock *MockProtocolScope
}

// NewMockProtocolScope 创建一个新的 mock 实例
// 参数:
//   - ctrl: gomock 控制器
//
// 返回:
//   - *MockProtocolScope: 新创建的 mock 实例
func NewMockProtocolScope(ctrl *gomock.Controller) *MockProtocolScope {
	mock := &MockProtocolScope{ctrl: ctrl}
	mock.recorder = &MockProtocolScopeMockRecorder{mock}
	return mock
}

// EXPECT 返回一个用于设置期望的对象
// 返回:
//   - *MockProtocolScopeMockRecorder: mock 记录器
func (m *MockProtocolScope) EXPECT() *MockProtocolScopeMockRecorder {
	return m.recorder
}

// BeginSpan 模拟 BeginSpan 方法
// 返回:
//   - network.ResourceScopeSpan: 资源作用域范围
//   - error: 可能的错误
func (m *MockProtocolScope) BeginSpan() (network.ResourceScopeSpan, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BeginSpan")
	ret0, _ := ret[0].(network.ResourceScopeSpan)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BeginSpan 表示对 BeginSpan 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockProtocolScopeMockRecorder) BeginSpan() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BeginSpan", reflect.TypeOf((*MockProtocolScope)(nil).BeginSpan))
}

// Protocol 模拟 Protocol 方法
// 返回:
//   - protocol.ID: 协议标识符
func (m *MockProtocolScope) Protocol() protocol.ID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Protocol")
	ret0, _ := ret[0].(protocol.ID)
	return ret0
}

// Protocol 表示对 Protocol 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockProtocolScopeMockRecorder) Protocol() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Protocol", reflect.TypeOf((*MockProtocolScope)(nil).Protocol))
}

// ReleaseMemory 模拟 ReleaseMemory 方法
// 参数:
//   - size: 要释放的内存大小
func (m *MockProtocolScope) ReleaseMemory(size int) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReleaseMemory", size)
}

// ReleaseMemory 表示对 ReleaseMemory 的预期调用
// 参数:
//   - size: 要释放的内存大小
//
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockProtocolScopeMockRecorder) ReleaseMemory(size any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReleaseMemory", reflect.TypeOf((*MockProtocolScope)(nil).ReleaseMemory), size)
}

// ReserveMemory 模拟 ReserveMemory 方法
// 参数:
//   - size: 要预留的内存大小
//   - prio: 优先级
//
// 返回:
//   - error: 可能的错误
func (m *MockProtocolScope) ReserveMemory(size int, prio uint8) error {
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
func (mr *MockProtocolScopeMockRecorder) ReserveMemory(size, prio any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReserveMemory", reflect.TypeOf((*MockProtocolScope)(nil).ReserveMemory), size, prio)
}

// Stat 模拟 Stat 方法
// 返回:
//   - network.ScopeStat: 作用域统计信息
func (m *MockProtocolScope) Stat() network.ScopeStat {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stat")
	ret0, _ := ret[0].(network.ScopeStat)
	return ret0
}

// Stat 表示对 Stat 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockProtocolScopeMockRecorder) Stat() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stat", reflect.TypeOf((*MockProtocolScope)(nil).Stat))
}
