// riscv64 see: https://github.com/marten-seemann/tcp/pull/1

//go:build windows || riscv64 || loong64

package tcp

import manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"

// aggregatingCollector 用于收集和聚合连接指标的结构体
type aggregatingCollector struct{}

// newTracingConn 创建一个新的带跟踪功能的连接
// 参数:
//   - c: manet.Conn 原始网络连接对象
//   - collector: *aggregatingCollector 指标收集器
//   - isClient: bool 是否为客户端连接
//
// 返回值:
//   - manet.Conn 带跟踪功能的连接对象
//   - error 创建过程中的错误信息
func newTracingConn(c manet.Conn, collector *aggregatingCollector, isClient bool) (manet.Conn, error) {
	// 由于当前构建标记(windows/riscv64/loong64)不支持指标收集,直接返回原始连接
	return c, nil
}

// newTracingListener 创建一个新的带跟踪功能的监听器
// 参数:
//   - l: manet.Listener 原始网络监听器
//   - collector: *aggregatingCollector 指标收集器
//
// 返回值:
//   - manet.Listener 带跟踪功能的监听器
func newTracingListener(l manet.Listener, collector *aggregatingCollector) manet.Listener {
	// 由于当前构建标记(windows/riscv64/loong64)不支持指标收集,直接返回原始监听器
	return l
}
