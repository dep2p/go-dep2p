package yamux

import (
	"io"
	"math"
	"net"

	"github.com/dep2p/go-dep2p/core/network"

	"github.com/dep2p/go-dep2p/p2plib/yamux"
)

// 默认的传输层实例
var DefaultTransport *Transport

// yamux 协议标识符
const ID = "/yamux/1.0.0"

// init 初始化默认传输层配置
func init() {
	// 获取默认配置
	config := yamux.DefaultConfig()
	// 将最大流窗口大小设置为16MiB以提高吞吐量
	// 1MiB在100ms延迟的连接上最佳情况下只能达到10MiB/s(83.89Mbps)
	// 默认的2.4MiB完全无法接受
	config.MaxStreamWindowSize = uint32(16 * 1024 * 1024)
	// 禁用日志输出以避免垃圾信息
	config.LogOutput = io.Discard
	// 由于我们总是运行在内部有缓冲的安全传输层上(即使用块密码),所以禁用读缓冲
	config.ReadBufSize = 0
	// 将入站流的限制设置为最大值
	// 现在由资源管理器动态限制
	config.MaxIncomingStreams = math.MaxUint32
	// 将配置转换为Transport并设置为默认传输层
	DefaultTransport = (*Transport)(config)
}

// Transport 实现了 mux.Multiplexer 接口,用于构造基于yamux的多路复用连接
type Transport yamux.Config

// 确保 Transport 实现了 network.Multiplexer 接口
var _ network.Multiplexer = &Transport{}

// NewConn 创建新的多路复用连接
// 参数:
//   - nc: net.Conn - 底层网络连接
//   - isServer: bool - 是否作为服务端
//   - scope: network.PeerScope - 对等节点资源作用域
//
// 返回:
//   - network.MuxedConn: 多路复用连接
//   - error: 创建过程中的错误,如果没有错误则返回nil
func (t *Transport) NewConn(nc net.Conn, isServer bool, scope network.PeerScope) (network.MuxedConn, error) {
	// 定义内存管理器创建函数
	var newSpan func() (yamux.MemoryManager, error)
	// 如果提供了作用域,则使用其创建内存管理器
	if scope != nil {
		newSpan = func() (yamux.MemoryManager, error) { return scope.BeginSpan() }
	}

	// 声明yamux会话和错误变量
	var s *yamux.Session
	var err error
	// 根据角色创建服务端或客户端会话
	if isServer {
		s, err = yamux.Server(nc, t.Config(), newSpan)
	} else {
		s, err = yamux.Client(nc, t.Config(), newSpan)
	}
	// 如果发生错误则返回
	if err != nil {
		log.Debugf("创建yamux会话失败: %v", err)
		return nil, err
	}
	// 将yamux会话包装为多路复用连接并返回
	return NewMuxedConn(s), nil
}

// Config 获取yamux配置
// 返回:
//   - *yamux.Config: yamux配置对象
func (t *Transport) Config() *yamux.Config {
	// 将Transport转换回yamux.Config并返回
	return (*yamux.Config)(t)
}
