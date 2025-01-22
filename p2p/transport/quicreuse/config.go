package quicreuse

import (
	"time"

	"github.com/quic-go/quic-go"
)

// quicConfig 定义了 QUIC 连接的配置参数
// 参数说明:
//   - MaxIncomingStreams: 最大入站流数量,用于限制并发连接数
//   - MaxIncomingUniStreams: 最大入站单向流数量,用于支持 WebTransport 协议
//   - MaxStreamReceiveWindow: 单个流的最大接收窗口大小,用于流量控制(10MB)
//   - MaxConnectionReceiveWindow: 连接的最大接收窗口大小,用于连接级流量控制(15MB)
//   - KeepAlivePeriod: 保活周期,用于维持长连接
//   - Versions: 支持的 QUIC 协议版本列表
//   - EnableDatagrams: 是否启用数据报功能,WebTransport 协议需要此特性
//
// 注意:
//   - 窗口大小的设置会影响内存使用和传输性能
//   - 保活周期不宜设置过短,以免产生过多开销
var quicConfig = &quic.Config{
	MaxIncomingStreams:         256,                           // 设置最大入站流数量为 256
	MaxIncomingUniStreams:      5,                             // 设置最大入站单向流数量为 5,支持 WebTransport
	MaxStreamReceiveWindow:     10 * (1 << 20),                // 设置单个流接收窗口为 10 MB
	MaxConnectionReceiveWindow: 15 * (1 << 20),                // 设置连接接收窗口为 15 MB
	KeepAlivePeriod:            15 * time.Second,              // 设置保活周期为 15 秒
	Versions:                   []quic.Version{quic.Version1}, // 仅支持 QUIC v1 版本
	EnableDatagrams:            true,                          // 启用数据报功能,支持 WebTransport
}
