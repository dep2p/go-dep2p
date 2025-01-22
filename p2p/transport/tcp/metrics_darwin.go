//go:build darwin

package tcp

import "github.com/mikioh/tcpinfo"

// 定义计数器标志
const (
	// hasSegmentCounter 表示是否支持段计数
	hasSegmentCounter = true
	// hasByteCounter 表示是否支持字节计数
	hasByteCounter = true
)

// getSegmentsSent 获取已发送的 TCP 段数量
// 参数:
//   - info: *tcpinfo.Info TCP 连接信息
//
// 返回值:
//   - uint64 已发送的 TCP 段数量
func getSegmentsSent(info *tcpinfo.Info) uint64 {
	return info.Sys.SegsSent
}

// getSegmentsRcvd 获取已接收的 TCP 段数量
// 参数:
//   - info: *tcpinfo.Info TCP 连接信息
//
// 返回值:
//   - uint64 已接收的 TCP 段数量
func getSegmentsRcvd(info *tcpinfo.Info) uint64 {
	return info.Sys.SegsReceived
}

// getBytesSent 获取已发送的字节数
// 参数:
//   - info: *tcpinfo.Info TCP 连接信息
//
// 返回值:
//   - uint64 已发送的字节数
func getBytesSent(info *tcpinfo.Info) uint64 {
	return info.Sys.BytesSent
}

// getBytesRcvd 获取已接收的字节数
// 参数:
//   - info: *tcpinfo.Info TCP 连接信息
//
// 返回值:
//   - uint64 已接收的字节数
func getBytesRcvd(info *tcpinfo.Info) uint64 {
	return info.Sys.BytesReceived
}
