// Package metrics 提供了 dep2p 的指标收集和报告接口。
package metrics

import (
	"time"

	flow "github.com/dep2p/go-dep2p/p2plib/flow/metrics"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// BandwidthCounter 跟踪本地节点传输的入站和出站数据。
// 可用的指标包括总带宽、所有对等节点/协议的带宽，以及按远程对等节点ID和协议ID分段的带宽。
type BandwidthCounter struct {
	// totalIn 记录总入站流量
	totalIn flow.Meter
	// totalOut 记录总出站流量
	totalOut flow.Meter

	// protocolIn 按协议ID记录入站流量
	protocolIn flow.MeterRegistry
	// protocolOut 按协议ID记录出站流量
	protocolOut flow.MeterRegistry

	// peerIn 按对等节点ID记录入站流量
	peerIn flow.MeterRegistry
	// peerOut 按对等节点ID记录出站流量
	peerOut flow.MeterRegistry
}

// NewBandwidthCounter 创建一个新的 BandwidthCounter。
//
// 返回值:
//   - *BandwidthCounter: 新创建的带宽计数器实例
func NewBandwidthCounter() *BandwidthCounter {
	return new(BandwidthCounter)
}

// LogSentMessage 记录传出消息的大小,不关联带宽到特定对等节点或协议。
//
// 参数:
//   - size: 消息大小(字节)
func (bwc *BandwidthCounter) LogSentMessage(size int64) {
	// 记录总出站流量
	bwc.totalOut.Mark(uint64(size))
}

// LogRecvMessage 记录传入消息的大小,不关联带宽到特定对等节点或协议。
//
// 参数:
//   - size: 消息大小(字节)
func (bwc *BandwidthCounter) LogRecvMessage(size int64) {
	// 记录总入站流量
	bwc.totalIn.Mark(uint64(size))
}

// LogSentMessageStream 记录传出消息的大小,关联带宽到给定的协议ID和peer.ID。
//
// 参数:
//   - size: 消息大小(字节)
//   - proto: 协议ID
//   - p: 对等节点ID
func (bwc *BandwidthCounter) LogSentMessageStream(size int64, proto protocol.ID, p peer.ID) {
	// 记录协议相关的出站流量
	bwc.protocolOut.Get(string(proto)).Mark(uint64(size))
	// 记录对等节点相关的出站流量
	bwc.peerOut.Get(string(p)).Mark(uint64(size))
}

// LogRecvMessageStream 记录传入消息的大小,关联带宽到给定的协议ID和peer.ID。
//
// 参数:
//   - size: 消息大小(字节)
//   - proto: 协议ID
//   - p: 对等节点ID
func (bwc *BandwidthCounter) LogRecvMessageStream(size int64, proto protocol.ID, p peer.ID) {
	// 记录协议相关的入站流量
	bwc.protocolIn.Get(string(proto)).Mark(uint64(size))
	// 记录对等节点相关的入站流量
	bwc.peerIn.Get(string(p)).Mark(uint64(size))
}

// GetBandwidthForPeer 返回一个包含带宽指标的Stats结构体,关联到给定的peer.ID。
// 返回的指标包括所有发送/接收的流量,无论协议如何。
//
// 参数:
//   - p: 对等节点ID
//
// 返回值:
//   - Stats: 包含该对等节点的带宽统计信息
func (bwc *BandwidthCounter) GetBandwidthForPeer(p peer.ID) (out Stats) {
	// 获取入站流量快照
	inSnap := bwc.peerIn.Get(string(p)).Snapshot()
	// 获取出站流量快照
	outSnap := bwc.peerOut.Get(string(p)).Snapshot()

	// 返回统计信息
	return Stats{
		TotalIn:  int64(inSnap.Total),
		TotalOut: int64(outSnap.Total),
		RateIn:   inSnap.Rate,
		RateOut:  outSnap.Rate,
	}
}

// GetBandwidthForProtocol 返回一个包含带宽指标的Stats结构体,关联到给定的协议ID。
// 返回的指标包括所有发送/接收的流量,无论哪个对等节点参与。
//
// 参数:
//   - proto: 协议ID
//
// 返回值:
//   - Stats: 包含该协议的带宽统计信息
func (bwc *BandwidthCounter) GetBandwidthForProtocol(proto protocol.ID) (out Stats) {
	// 获取入站流量快照
	inSnap := bwc.protocolIn.Get(string(proto)).Snapshot()
	// 获取出站流量快照
	outSnap := bwc.protocolOut.Get(string(proto)).Snapshot()

	// 返回统计信息
	return Stats{
		TotalIn:  int64(inSnap.Total),
		TotalOut: int64(outSnap.Total),
		RateIn:   inSnap.Rate,
		RateOut:  outSnap.Rate,
	}
}

// GetBandwidthTotals 返回一个包含带宽指标的Stats结构体,关联到所有发送/接收的流量,无论协议或远程对等节点ID。
//
// 返回值:
//   - Stats: 包含总体带宽统计信息
func (bwc *BandwidthCounter) GetBandwidthTotals() (out Stats) {
	// 获取总入站流量快照
	inSnap := bwc.totalIn.Snapshot()
	// 获取总出站流量快照
	outSnap := bwc.totalOut.Snapshot()

	// 返回统计信息
	return Stats{
		TotalIn:  int64(inSnap.Total),
		TotalOut: int64(outSnap.Total),
		RateIn:   inSnap.Rate,
		RateOut:  outSnap.Rate,
	}
}

// GetBandwidthByPeer 返回一个包含所有记住的对等节点和每个对等节点的带宽指标的map。
// 该方法可能非常昂贵。
//
// 返回值:
//   - map[peer.ID]Stats: 每个对等节点的带宽统计信息映射
func (bwc *BandwidthCounter) GetBandwidthByPeer() map[peer.ID]Stats {
	// 创建结果map
	peers := make(map[peer.ID]Stats)

	// 遍历所有入站流量记录
	bwc.peerIn.ForEach(func(p string, meter *flow.Meter) {
		id := peer.ID(p)
		snap := meter.Snapshot()

		stat := peers[id]
		stat.TotalIn = int64(snap.Total)
		stat.RateIn = snap.Rate
		peers[id] = stat
	})

	// 遍历所有出站流量记录
	bwc.peerOut.ForEach(func(p string, meter *flow.Meter) {
		id := peer.ID(p)
		snap := meter.Snapshot()

		stat := peers[id]
		stat.TotalOut = int64(snap.Total)
		stat.RateOut = snap.Rate
		peers[id] = stat
	})

	return peers
}

// GetBandwidthByProtocol 返回一个包含所有记住的协议和每个协议的带宽指标的map。
// 该方法可能比较昂贵。
//
// 返回值:
//   - map[protocol.ID]Stats: 每个协议的带宽统计信息映射
func (bwc *BandwidthCounter) GetBandwidthByProtocol() map[protocol.ID]Stats {
	// 创建结果map
	protocols := make(map[protocol.ID]Stats)

	// 遍历所有入站流量记录
	bwc.protocolIn.ForEach(func(p string, meter *flow.Meter) {
		id := protocol.ID(p)
		snap := meter.Snapshot()

		stat := protocols[id]
		stat.TotalIn = int64(snap.Total)
		stat.RateIn = snap.Rate
		protocols[id] = stat
	})

	// 遍历所有出站流量记录
	bwc.protocolOut.ForEach(func(p string, meter *flow.Meter) {
		id := protocol.ID(p)
		snap := meter.Snapshot()

		stat := protocols[id]
		stat.TotalOut = int64(snap.Total)
		stat.RateOut = snap.Rate
		protocols[id] = stat
	})

	return protocols
}

// Reset 清除所有统计数据。
func (bwc *BandwidthCounter) Reset() {
	// 重置总流量计数器
	bwc.totalIn.Reset()
	bwc.totalOut.Reset()

	// 清除协议流量记录
	bwc.protocolIn.Clear()
	bwc.protocolOut.Clear()

	// 清除对等节点流量记录
	bwc.peerIn.Clear()
	bwc.peerOut.Clear()
}

// TrimIdle 清除所有自给定时间以来未使用的计时器。
//
// 参数:
//   - since: 空闲时间阈值
func (bwc *BandwidthCounter) TrimIdle(since time.Time) {
	// 清理空闲的入站对等节点记录
	bwc.peerIn.TrimIdle(since)
	// 清理空闲的出站对等节点记录
	bwc.peerOut.TrimIdle(since)
	// 清理空闲的入站协议记录
	bwc.protocolIn.TrimIdle(since)
	// 清理空闲的出站协议记录
	bwc.protocolOut.TrimIdle(since)
}
