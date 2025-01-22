package reuseport

import (
	"net"
	"os"
)

const (
	EADDRINUSE   = "地址已被使用"
	ECONNREFUSED = "连接被拒绝"
)

// reuseErrShouldRetry 判断是否需要在重用错误后重试
// 参数:
//   - err: error 错误对象
//
// 返回值:
//   - bool 是否需要重试
//
// 注意:
//   - 如果绑定失败,我们应该重试
//   - 如果绑定成功但这是一个真实的拨号错误(远端没有响应),则不应该重试
func reuseErrShouldRetry(err error) bool {
	// 如果没有错误,说明操作成功,不需要重试
	if err == nil {
		return false
	}

	// 如果是网络超时错误,这是一个合法的失败,不需要重试
	if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
		return false
	}

	// 尝试将错误转换为网络操作错误
	e, ok := err.(*net.OpError)
	if !ok {
		return true
	}

	// 尝试将错误转换为路径错误
	e1, ok := e.Err.(*os.PathError)
	if !ok {
		return true
	}

	// 根据具体错误类型判断是否需要重试
	switch e1.Err.Error() {
	case EADDRINUSE:
		return true // 地址被占用时需要重试
	case ECONNREFUSED:
		return false // 连接被拒绝时不需要重试
	default:
		return true // 默认乐观地选择重试
	}
}
