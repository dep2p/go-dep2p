package rcmgr

import (
	"errors"

	"github.com/dep2p/go-dep2p/core/network"
)

// ErrStreamOrConnLimitExceeded 定义了流或连接超出限制的错误结构体
type ErrStreamOrConnLimitExceeded struct {
	current   int   // 当前值
	attempted int   // 尝试值
	limit     int   // 限制值
	err       error // 错误信息
}

// Error 实现 error 接口，返回错误信息
// 返回值:
//   - string: 错误信息字符串
func (e *ErrStreamOrConnLimitExceeded) Error() string { return e.err.Error() }

// Unwrap 返回原始错误
// 返回值:
//   - error: 原始错误对象
func (e *ErrStreamOrConnLimitExceeded) Unwrap() error { return e.err }

// logValuesStreamLimit 记录流限制相关的日志值
// 参数:
//   - scope: string - 作用域
//   - edge: string - 边缘标识(可为空)
//   - dir: network.Direction - 方向
//   - stat: network.ScopeStat - 作用域统计信息
//   - err: error - 错误信息
//
// 返回值:
//   - []interface{}: 日志值切片
func logValuesStreamLimit(scope, edge string, dir network.Direction, stat network.ScopeStat, err error) []interface{} {
	// 初始化日志值切片
	logValues := make([]interface{}, 0, 2*8)
	// 添加作用域信息
	logValues = append(logValues, "scope", scope)
	// 如果存在边缘标识，则添加
	if edge != "" {
		logValues = append(logValues, "edge", edge)
	}
	// 添加方向信息
	logValues = append(logValues, "direction", dir)
	var e *ErrStreamOrConnLimitExceeded
	// 如果错误类型匹配，添加详细错误信息
	if errors.As(err, &e) {
		logValues = append(logValues,
			"current", e.current,
			"attempted", e.attempted,
			"limit", e.limit,
		)
	}
	// 添加统计信息和错误信息并返回
	return append(logValues, "stat", stat, "error", err)
}

// logValuesConnLimit 记录连接限制相关的日志值
// 参数:
//   - scope: string - 作用域
//   - edge: string - 边缘标识(可为空)
//   - dir: network.Direction - 方向
//   - usefd: bool - 是否使用文件描述符
//   - stat: network.ScopeStat - 作用域统计信息
//   - err: error - 错误信息
//
// 返回值:
//   - []interface{}: 日志值切片
func logValuesConnLimit(scope, edge string, dir network.Direction, usefd bool, stat network.ScopeStat, err error) []interface{} {
	// 初始化日志值切片
	logValues := make([]interface{}, 0, 2*9)
	// 添加作用域信息
	logValues = append(logValues, "scope", scope)
	// 如果存在边缘标识，则添加
	if edge != "" {
		logValues = append(logValues, "edge", edge)
	}
	// 添加方向和文件描述符使用信息
	logValues = append(logValues, "direction", dir, "usefd", usefd)
	var e *ErrStreamOrConnLimitExceeded
	// 如果错误类型匹配，添加详细错误信息
	if errors.As(err, &e) {
		logValues = append(logValues,
			"current", e.current,
			"attempted", e.attempted,
			"limit", e.limit,
		)
	}
	// 添加统计信息和错误信息并返回
	return append(logValues, "stat", stat, "error", err)
}

// ErrMemoryLimitExceeded 定义了内存超出限制的错误结构体
type ErrMemoryLimitExceeded struct {
	current   int64 // 当前内存使用量
	attempted int64 // 尝试使用的内存量
	limit     int64 // 内存限制
	priority  uint8 // 优先级
	err       error // 错误信息
}

// Error 实现 error 接口，返回错误信息
// 返回值:
//   - string: 错误信息字符串
func (e *ErrMemoryLimitExceeded) Error() string { return e.err.Error() }

// Unwrap 返回原始错误
// 返回值:
//   - error: 原始错误对象
func (e *ErrMemoryLimitExceeded) Unwrap() error { return e.err }

// logValuesMemoryLimit 记录内存限制相关的日志值
// 参数:
//   - scope: string - 作用域
//   - edge: string - 边缘标识(可为空)
//   - stat: network.ScopeStat - 作用域统计信息
//   - err: error - 错误信息
//
// 返回值:
//   - []interface{}: 日志值切片
func logValuesMemoryLimit(scope, edge string, stat network.ScopeStat, err error) []interface{} {
	// 初始化日志值切片
	logValues := make([]interface{}, 0, 2*8)
	// 添加作用域信息
	logValues = append(logValues, "scope", scope)
	// 如果存在边缘标识，则添加
	if edge != "" {
		logValues = append(logValues, "edge", edge)
	}
	var e *ErrMemoryLimitExceeded
	// 如果错误类型匹配，添加详细错误信息
	if errors.As(err, &e) {
		logValues = append(logValues,
			"current", e.current,
			"attempted", e.attempted,
			"priority", e.priority,
			"limit", e.limit,
		)
	}
	// 添加统计信息和错误信息并返回
	return append(logValues, "stat", stat, "error", err)
}
