//go:build linux || darwin

package rcmgr

import (
	"golang.org/x/sys/unix"
)

// getNumFDs 获取当前系统的文件描述符数量
// 返回值:
//   - int: 文件描述符数量,获取失败时返回0
func getNumFDs() int {
	// 定义变量存储资源限制信息
	var l unix.Rlimit
	// 获取系统文件描述符数量限制
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &l); err != nil {
		// 获取失败时记录错误日志
		log.Debugf("获取文件描述符限制失败: %v", err)
		return 0
	}
	// 返回当前文件描述符数量
	return int(l.Cur)
}
