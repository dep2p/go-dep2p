package metrics

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// Reporter 提供记录和检索指标的方法
//
// Reporter 接口兼容 go-libp2p 的 metrics.Reporter 接口。
type Reporter interface {
	// LogSentMessage 记录发送消息大小
	LogSentMessage(int64)

	// LogRecvMessage 记录接收消息大小
	LogRecvMessage(int64)

	// LogSentMessageStream 记录流发送消息大小
	LogSentMessageStream(int64, types.ProtocolID, types.PeerID)

	// LogRecvMessageStream 记录流接收消息大小
	LogRecvMessageStream(int64, types.ProtocolID, types.PeerID)

	// GetBandwidthForPeer 获取节点带宽统计
	GetBandwidthForPeer(types.PeerID) Stats

	// GetBandwidthForProtocol 获取协议带宽统计
	GetBandwidthForProtocol(types.ProtocolID) Stats

	// GetBandwidthTotals 获取总带宽统计
	GetBandwidthTotals() Stats

	// GetBandwidthByPeer 获取所有节点带宽统计
	GetBandwidthByPeer() map[types.PeerID]Stats

	// GetBandwidthByProtocol 获取所有协议带宽统计
	GetBandwidthByProtocol() map[types.ProtocolID]Stats

	// Reset 重置所有统计
	Reset()

	// TrimIdle 清理空闲统计
	TrimIdle(since time.Time)
}

// 确保 BandwidthCounter 实现 Reporter 接口
var _ Reporter = (*BandwidthCounter)(nil)
