//go:build windows

package rcmgr

import (
	"math"
)

// getNumFDs 获取系统文件描述符数量
// 返回值:
//   - int: 系统支持的最大文件描述符数量
func getNumFDs() int {
	// Windows系统没有文件描述符限制,返回最大整数值
	return math.MaxInt
}
