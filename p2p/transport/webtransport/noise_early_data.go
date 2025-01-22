package dep2pwebtransport

import (
	"context"
	"net"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/p2p/security/noise"
	"github.com/dep2p/go-dep2p/p2p/security/noise/pb"
)

// earlyDataHandler 处理 Noise 协议的早期数据
type earlyDataHandler struct {
	earlyData *pb.NoiseExtensions                        // 早期数据扩展
	receive   func(extensions *pb.NoiseExtensions) error // 接收处理函数
}

// 确保实现了 noise.EarlyDataHandler 接口
var _ noise.EarlyDataHandler = &earlyDataHandler{}

// newEarlyDataSender 创建一个新的早期数据发送器
// 参数:
//   - earlyData: *pb.NoiseExtensions 要发送的早期数据
//
// 返回值:
//   - noise.EarlyDataHandler 早期数据处理器
func newEarlyDataSender(earlyData *pb.NoiseExtensions) noise.EarlyDataHandler {
	return &earlyDataHandler{earlyData: earlyData}
}

// newEarlyDataReceiver 创建一个新的早期数据接收器
// 参数:
//   - receive: func(*pb.NoiseExtensions) error 接收数据的处理函数
//
// 返回值:
//   - noise.EarlyDataHandler 早期数据处理器
func newEarlyDataReceiver(receive func(*pb.NoiseExtensions) error) noise.EarlyDataHandler {
	return &earlyDataHandler{receive: receive}
}

// Send 发送早期数据
// 参数:
//   - context.Context 上下文
//   - net.Conn 网络连接
//   - peer.ID 对等节点 ID
//
// 返回值:
//   - *pb.NoiseExtensions 要发送的早期数据扩展
func (e *earlyDataHandler) Send(context.Context, net.Conn, peer.ID) *pb.NoiseExtensions {
	return e.earlyData
}

// Received 处理接收到的早期数据
// 参数:
//   - context.Context 上下文
//   - net.Conn 网络连接
//   - ext: *pb.NoiseExtensions 接收到的早期数据扩展
//
// 返回值:
//   - error 处理过程中的错误
func (e *earlyDataHandler) Received(_ context.Context, _ net.Conn, ext *pb.NoiseExtensions) error {
	// 如果没有设置接收处理函数,直接返回 nil
	if e.receive == nil {
		return nil
	}
	// 调用接收处理函数处理数据
	return e.receive(ext)
}
