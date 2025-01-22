//go:build !linux && !darwin && !windows && !riscv64 && !loong64

package tcp

import "github.com/mikioh/tcpinfo"

// 定义常量
const (
	// hasSegmentCounter 表示是否支持段计数器
	hasSegmentCounter = false
	// hasByteCounter 表示是否支持字节计数器
	hasByteCounter = false
)

// getSegmentsSent 获取已发送的TCP段数量
// 参数:
//   - info: *tcpinfo.Info TCP连接信息
//
// 返回值:
//   - uint64 已发送的TCP段数量
func getSegmentsSent(info *tcpinfo.Info) uint64 { return 0 }

// getSegmentsRcvd 获取已接收的TCP段数量
// 参数:
//   - info: *tcpinfo.Info TCP连接信息
//
// 返回值:
//   - uint64 已接收的TCP段数量
func getSegmentsRcvd(info *tcpinfo.Info) uint64 { return 0 }

// getBytesSent 获取已发送的字节数
// 参数:
//   - info: *tcpinfo.Info TCP连接信息
//
// 返回值:
//   - uint64 已发送的字节数
func getBytesSent(info *tcpinfo.Info) uint64 { return 0 }

// getBytesRcvd 获取已接收的字节数
// 参数:
//   - info: *tcpinfo.Info TCP连接信息
//
// 返回值:
//   - uint64 已接收的字节数
func getBytesRcvd(info *tcpinfo.Info) uint64 { return 0 }
