// Package mocknetwork 是一个由 GoMock 生成的包
package mocknetwork

import (
	reflect "reflect"

	network "github.com/dep2p/go-dep2p/core/network"
	peer "github.com/dep2p/go-dep2p/core/peer"
	protocol "github.com/dep2p/go-dep2p/core/protocol"
	multiaddr "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	gomock "go.uber.org/mock/gomock"
)

// MockResourceManager 是 ResourceManager 接口的 mock 实现
type MockResourceManager struct {
	// ctrl 是 gomock 控制器
	ctrl *gomock.Controller
	// recorder 用于记录方法调用
	recorder *MockResourceManagerMockRecorder
	// isgomock 标识这是一个 mock 对象
	isgomock struct{}
}

// MockResourceManagerMockRecorder 是 MockResourceManager 的记录器
type MockResourceManagerMockRecorder struct {
	// mock 指向对应的 MockResourceManager 实例
	mock *MockResourceManager
}

// NewMockResourceManager 创建一个新的 mock 实例
// 参数:
//   - ctrl: gomock 控制器
//
// 返回:
//   - *MockResourceManager: 新创建的 mock 实例
func NewMockResourceManager(ctrl *gomock.Controller) *MockResourceManager {
	mock := &MockResourceManager{ctrl: ctrl}
	mock.recorder = &MockResourceManagerMockRecorder{mock}
	return mock
}

// EXPECT 返回一个用于设置期望的对象
// 返回:
//   - *MockResourceManagerMockRecorder: mock 记录器
func (m *MockResourceManager) EXPECT() *MockResourceManagerMockRecorder {
	return m.recorder
}

// Close mock 实现关闭方法
// 返回:
//   - error: 关闭操作可能返回的错误
func (m *MockResourceManager) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close 表示对 Close 方法的预期调用
// 返回:
//   - *gomock.Call: 用于链式调用的 gomock.Call 对象
func (mr *MockResourceManagerMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockResourceManager)(nil).Close))
}

// OpenConnection mock 实现打开连接的方法
// 参数:
//   - dir: 连接方向
//   - usefd: 是否使用文件描述符
//   - endpoint: 多地址端点
//
// 返回:
//   - network.ConnManagementScope: 连接管理作用域
//   - error: 可能的错误
func (m *MockResourceManager) OpenConnection(dir network.Direction, usefd bool, endpoint multiaddr.Multiaddr) (network.ConnManagementScope, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OpenConnection", dir, usefd, endpoint)
	ret0, _ := ret[0].(network.ConnManagementScope)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// OpenConnection 表示对 OpenConnection 方法的预期调用
// 参数:
//   - dir: 连接方向
//   - usefd: 是否使用文件描述符
//   - endpoint: 多地址端点
//
// 返回:
//   - *gomock.Call: 用于链式调用的 gomock.Call 对象
func (mr *MockResourceManagerMockRecorder) OpenConnection(dir, usefd, endpoint any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OpenConnection", reflect.TypeOf((*MockResourceManager)(nil).OpenConnection), dir, usefd, endpoint)
}

// OpenStream mock 实现打开流的方法
// 参数:
//   - p: 对等节点 ID
//   - dir: 流方向
//
// 返回:
//   - network.StreamManagementScope: 流管理作用域
//   - error: 可能的错误
func (m *MockResourceManager) OpenStream(p peer.ID, dir network.Direction) (network.StreamManagementScope, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "OpenStream", p, dir)
	ret0, _ := ret[0].(network.StreamManagementScope)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// OpenStream 表示对 OpenStream 方法的预期调用
// 参数:
//   - p: 对等节点 ID
//   - dir: 流方向
//
// 返回:
//   - *gomock.Call: 用于链式调用的 gomock.Call 对象
func (mr *MockResourceManagerMockRecorder) OpenStream(p, dir any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "OpenStream", reflect.TypeOf((*MockResourceManager)(nil).OpenStream), p, dir)
}

// ViewPeer mock 实现查看对等节点的方法
// 参数:
//   - arg0: 对等节点 ID
//   - arg1: 处理对等节点作用域的函数
//
// 返回:
//   - error: 可能的错误
func (m *MockResourceManager) ViewPeer(arg0 peer.ID, arg1 func(network.PeerScope) error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ViewPeer", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// ViewPeer 表示对 ViewPeer 方法的预期调用
// 参数:
//   - arg0: 对等节点 ID
//   - arg1: 处理对等节点作用域的函数
//
// 返回:
//   - *gomock.Call: 用于链式调用的 gomock.Call 对象
func (mr *MockResourceManagerMockRecorder) ViewPeer(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ViewPeer", reflect.TypeOf((*MockResourceManager)(nil).ViewPeer), arg0, arg1)
}

// ViewProtocol mock 实现查看协议的方法
// 参数:
//   - arg0: 协议 ID
//   - arg1: 处理协议作用域的函数
//
// 返回:
//   - error: 可能的错误
func (m *MockResourceManager) ViewProtocol(arg0 protocol.ID, arg1 func(network.ProtocolScope) error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ViewProtocol", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// ViewProtocol 表示对 ViewProtocol 方法的预期调用
// 参数:
//   - arg0: 协议 ID
//   - arg1: 处理协议作用域的函数
//
// 返回:
//   - *gomock.Call: 用于链式调用的 gomock.Call 对象
func (mr *MockResourceManagerMockRecorder) ViewProtocol(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ViewProtocol", reflect.TypeOf((*MockResourceManager)(nil).ViewProtocol), arg0, arg1)
}

// ViewService mock 实现查看服务的方法
// 参数:
//   - arg0: 服务名称
//   - arg1: 处理服务作用域的函数
//
// 返回:
//   - error: 可能的错误
func (m *MockResourceManager) ViewService(arg0 string, arg1 func(network.ServiceScope) error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ViewService", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// ViewService 表示对 ViewService 方法的预期调用
// 参数:
//   - arg0: 服务名称
//   - arg1: 处理服务作用域的函数
//
// 返回:
//   - *gomock.Call: 用于链式调用的 gomock.Call 对象
func (mr *MockResourceManagerMockRecorder) ViewService(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ViewService", reflect.TypeOf((*MockResourceManager)(nil).ViewService), arg0, arg1)
}

// ViewSystem mock 实现查看系统的方法
// 参数:
//   - arg0: 处理系统资源作用域的函数
//
// 返回:
//   - error: 可能的错误
func (m *MockResourceManager) ViewSystem(arg0 func(network.ResourceScope) error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ViewSystem", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// ViewSystem 表示对 ViewSystem 方法的预期调用
// 参数:
//   - arg0: 处理系统资源作用域的函数
//
// 返回:
//   - *gomock.Call: 用于链式调用的 gomock.Call 对象
func (mr *MockResourceManagerMockRecorder) ViewSystem(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ViewSystem", reflect.TypeOf((*MockResourceManager)(nil).ViewSystem), arg0)
}

// ViewTransient mock 实现查看临时资源的方法
// 参数:
//   - arg0: 处理临时资源作用域的函数
//
// 返回:
//   - error: 可能的错误
func (m *MockResourceManager) ViewTransient(arg0 func(network.ResourceScope) error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ViewTransient", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// ViewTransient 表示对 ViewTransient 方法的预期调用
// 参数:
//   - arg0: 处理临时资源作用域的函数
//
// 返回:
//   - *gomock.Call: 用于链式调用的 gomock.Call 对象
func (mr *MockResourceManagerMockRecorder) ViewTransient(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ViewTransient", reflect.TypeOf((*MockResourceManager)(nil).ViewTransient), arg0)
}
