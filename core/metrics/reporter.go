// Package metrics 提供了 dep2p 的指标收集和报告接口。
package metrics

import (
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// Stats 表示带宽指标的时间点快照。
//
// TotalIn 和 TotalOut 字段记录累计发送/接收的字节数。
// RateIn 和 RateOut 字段记录每秒发送/接收的字节数。
type Stats struct {
	TotalIn  int64
	TotalOut int64
	RateIn   float64
	RateOut  float64
}

// Reporter 提供记录和检索指标的方法。
type Reporter interface {
	LogSentMessage(int64)
	LogRecvMessage(int64)
	LogSentMessageStream(int64, protocol.ID, peer.ID)
	LogRecvMessageStream(int64, protocol.ID, peer.ID)
	GetBandwidthForPeer(peer.ID) Stats
	GetBandwidthForProtocol(protocol.ID) Stats
	GetBandwidthTotals() Stats
	GetBandwidthByPeer() map[peer.ID]Stats
	GetBandwidthByProtocol() map[protocol.ID]Stats
}
