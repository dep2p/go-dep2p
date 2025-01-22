package network

import (
	"errors"
	"net"
)

// temporaryError 是一个实现了net.Error接口的字符串类型
// 用于表示临时性错误
type temporaryError string

// Error 实现error接口，返回错误信息字符串
// 返回值: 错误信息字符串
func (e temporaryError) Error() string { return string(e) }

// Temporary 实现net.Error接口的Temporary方法
// 返回值: 总是返回true,表示这是一个临时性错误
func (e temporaryError) Temporary() bool { return true }

// Timeout 实现net.Error接口的Timeout方法
// 返回值: 总是返回false,表示这不是一个超时错误
func (e temporaryError) Timeout() bool { return false }

// 确保temporaryError类型实现了net.Error接口
var _ net.Error = temporaryError("")

// ErrNoRemoteAddrs 当尝试拨号连接对等节点但没有可用地址时返回此错误
var ErrNoRemoteAddrs = errors.New("没有远程地址")

// ErrNoConn 当使用NoDial选项尝试打开到对等节点的流但没有可用连接时返回此错误
var ErrNoConn = errors.New("没有可用的对等节点连接")

// ErrTransientConn 当尝试打开到只有临时连接的对等节点的流,但未指定UseTransient选项时返回此错误
//
// 已弃用: 请使用ErrLimitedConn替代
var ErrTransientConn = ErrLimitedConn

// ErrLimitedConn 当尝试打开到只有有限连接的对等节点的流,但未指定AllowLimitedConn选项时返回此错误
var ErrLimitedConn = errors.New("对等节点连接受限")

// ErrResourceLimitExceeded 当尝试执行的操作会超出系统资源限制时返回此错误
var ErrResourceLimitExceeded = temporaryError("超出资源限制")

// ErrResourceScopeClosed 当尝试在已关闭的资源作用域中保留资源时返回此错误
var ErrResourceScopeClosed = errors.New("资源作用域已关闭")
