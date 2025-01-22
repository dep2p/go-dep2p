//go:build !linux && !darwin && !windows

package rcmgr

import "runtime"

// TODO: 需要找到在Windows和其他系统上获取文件描述符数量的方法
// getNumFDs 获取当前系统的文件描述符数量
// 返回值:
//   - int: 文件描述符数量,在不支持的系统上返回0
func getNumFDs() int {
	// 在不支持的系统上输出警告日志
	log.Warnf("无法在 %s 系统上获取文件描述符数量", runtime.GOOS)
	// 返回0表示无法获取
	return 0
}
