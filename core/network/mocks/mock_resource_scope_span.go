// Package mocknetwork 是一个由 GoMock 生成的包
package mocknetwork

import (
	reflect "reflect"

	network "github.com/dep2p/go-dep2p/core/network"
	gomock "go.uber.org/mock/gomock"
)

// MockResourceScopeSpan 是 ResourceScopeSpan 接口的模拟实现
type MockResourceScopeSpan struct {
	// ctrl 是 gomock 控制器
	ctrl *gomock.Controller
	// recorder 用于记录方法调用
	recorder *MockResourceScopeSpanMockRecorder
	// isgomock 标识这是一个 mock 对象
	isgomock struct{}
}

// MockResourceScopeSpanMockRecorder 是 MockResourceScopeSpan 的记录器
type MockResourceScopeSpanMockRecorder struct {
	// mock 指向对应的 MockResourceScopeSpan 实例
	mock *MockResourceScopeSpan
}

// NewMockResourceScopeSpan 创建一个新的 mock 实例
// 参数:
//   - ctrl: gomock 控制器
//
// 返回:
//   - *MockResourceScopeSpan: 新创建的 mock 实例
func NewMockResourceScopeSpan(ctrl *gomock.Controller) *MockResourceScopeSpan {
	mock := &MockResourceScopeSpan{ctrl: ctrl}
	mock.recorder = &MockResourceScopeSpanMockRecorder{mock}
	return mock
}

// EXPECT 返回一个用于设置期望的对象
// 返回:
//   - *MockResourceScopeSpanMockRecorder: mock 记录器
func (m *MockResourceScopeSpan) EXPECT() *MockResourceScopeSpanMockRecorder {
	return m.recorder
}

// BeginSpan 模拟 BeginSpan 方法
// 返回:
//   - network.ResourceScopeSpan: 资源作用域范围
//   - error: 可能的错误
func (m *MockResourceScopeSpan) BeginSpan() (network.ResourceScopeSpan, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BeginSpan")
	ret0, _ := ret[0].(network.ResourceScopeSpan)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BeginSpan 表示对 BeginSpan 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockResourceScopeSpanMockRecorder) BeginSpan() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BeginSpan", reflect.TypeOf((*MockResourceScopeSpan)(nil).BeginSpan))
}

// Done 模拟 Done 方法
func (m *MockResourceScopeSpan) Done() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Done")
}

// Done 表示对 Done 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockResourceScopeSpanMockRecorder) Done() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Done", reflect.TypeOf((*MockResourceScopeSpan)(nil).Done))
}

// ReleaseMemory 模拟 ReleaseMemory 方法
// 参数:
//   - size: 要释放的内存大小
func (m *MockResourceScopeSpan) ReleaseMemory(size int) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReleaseMemory", size)
}

// ReleaseMemory 表示对 ReleaseMemory 的预期调用
// 参数:
//   - size: 要释放的内存大小
//
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockResourceScopeSpanMockRecorder) ReleaseMemory(size any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReleaseMemory", reflect.TypeOf((*MockResourceScopeSpan)(nil).ReleaseMemory), size)
}

// ReserveMemory 模拟 ReserveMemory 方法
// 参数:
//   - size: 要预留的内存大小
//   - prio: 优先级
//
// 返回:
//   - error: 可能的错误
func (m *MockResourceScopeSpan) ReserveMemory(size int, prio uint8) error {
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
func (mr *MockResourceScopeSpanMockRecorder) ReserveMemory(size, prio any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReserveMemory", reflect.TypeOf((*MockResourceScopeSpan)(nil).ReserveMemory), size, prio)
}

// Stat 模拟 Stat 方法
// 返回:
//   - network.ScopeStat: 作用域统计信息
func (m *MockResourceScopeSpan) Stat() network.ScopeStat {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stat")
	ret0, _ := ret[0].(network.ScopeStat)
	return ret0
}

// Stat 表示对 Stat 的预期调用
// 返回:
//   - *gomock.Call: gomock 调用对象
func (mr *MockResourceScopeSpanMockRecorder) Stat() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stat", reflect.TypeOf((*MockResourceScopeSpan)(nil).Stat))
}
