//go:build linux

package tcp

import "github.com/mikioh/tcpinfo"

// 计数器标志常量
const (
	// hasSegmentCounter 表示是否支持分段计数
	hasSegmentCounter = true
	// hasByteCounter 表示是否支持字节计数
	hasByteCounter = false
)

// getSegmentsSent 获取已发送的TCP分段数
// 参数:
//   - info: *tcpinfo.Info TCP连接信息
//
// 返回值:
//   - uint64 已发送的分段数
func getSegmentsSent(info *tcpinfo.Info) uint64 { return uint64(info.Sys.SegsOut) }

// getSegmentsRcvd 获取已接收的TCP分段数
// 参数:
//   - info: *tcpinfo.Info TCP连接信息
//
// 返回值:
//   - uint64 已接收的分段数
func getSegmentsRcvd(info *tcpinfo.Info) uint64 { return uint64(info.Sys.SegsIn) }

// getBytesSent 获取已发送的字节数(当前未实现)
// 参数:
//   - info: *tcpinfo.Info TCP连接信息
//
// 返回值:
//   - uint64 已发送的字节数
func getBytesSent(info *tcpinfo.Info) uint64 { return 0 }

// getBytesRcvd 获取已接收的字节数(当前未实现)
// 参数:
//   - info: *tcpinfo.Info TCP连接信息
//
// 返回值:
//   - uint64 已接收的字节数
func getBytesRcvd(info *tcpinfo.Info) uint64 { return 0 }
