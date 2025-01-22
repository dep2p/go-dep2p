//go:build !plan9

package reuseport

import (
	"net"
	"syscall"
)

// reuseErrShouldRetry 判断是否需要在重用端口出错后重试
// 参数:
//   - err: error 错误对象
//
// 返回值:
//   - bool 是否需要重试
//
// 注意:
//   - 如果绑定失败则需要重试
//   - 如果绑定成功但是远端未响应(真实的拨号错误)则不需要重试
func reuseErrShouldRetry(err error) bool {
	// 如果没有错误,说明操作成功,不需要重试
	if err == nil {
		return false
	}

	// 如果是网络超时错误,这是一个合法的失败,不需要重试
	if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
		return false
	}

	// 尝试将错误转换为系统错误码
	errno, ok := err.(syscall.Errno)
	if !ok { // 如果不是系统错误码,不确定是什么错误,保险起见重试
		return true
	}

	// 根据具体的系统错误码判断是否需要重试
	switch errno {
	case syscall.EADDRINUSE, syscall.EADDRNOTAVAIL:
		return true // 绑定失败,需要重试
	case syscall.ECONNREFUSED:
		return false // 真实的拨号错误,不需要重试
	default:
		return true // 默认乐观处理,选择重试
	}
}
