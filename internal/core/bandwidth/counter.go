package bandwidth

import (
	"time"

	bandwidthif "github.com/dep2p/go-dep2p/pkg/interfaces/bandwidth"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              带宽计数器
// ============================================================================

// Counter 带宽计数器实现
//
// 实现 bandwidthif.Counter 接口
type Counter struct {
	config bandwidthif.Config

	// 总量计量器
	totalIn  *Meter
	totalOut *Meter

	// 按 Protocol 的计量器
	protocolIn  MeterRegistry
	protocolOut MeterRegistry

	// 按 Peer 的计量器
	peerIn  MeterRegistry
	peerOut MeterRegistry
}

// 确保实现接口
var _ bandwidthif.Counter = (*Counter)(nil)

// NewCounter 创建带宽计数器
func NewCounter(config bandwidthif.Config) *Counter {
	return &Counter{
		config:   config,
		totalIn:  NewMeter(),
		totalOut: NewMeter(),
	}
}

// ==================== 记录流量 ====================

// LogSentMessage 记录发送的消息大小
func (c *Counter) LogSentMessage(size int64) {
	if !c.config.Enabled || size <= 0 {
		return
	}
	c.totalOut.Mark(uint64(size))
}

// LogRecvMessage 记录接收的消息大小
func (c *Counter) LogRecvMessage(size int64) {
	if !c.config.Enabled || size <= 0 {
		return
	}
	c.totalIn.Mark(uint64(size))
}

// LogSentMessageStream 记录流上发送的消息
func (c *Counter) LogSentMessageStream(size int64, proto endpoint.ProtocolID, peer endpoint.NodeID) {
	if !c.config.Enabled || size <= 0 {
		return
	}

	// 总量
	c.totalOut.Mark(uint64(size))

	// 按协议
	if c.config.TrackByProtocol {
		c.protocolOut.Get(string(proto)).Mark(uint64(size))
	}

	// 按 Peer
	if c.config.TrackByPeer {
		c.peerOut.Get(peer.String()).Mark(uint64(size))
	}
}

// LogRecvMessageStream 记录流上接收的消息
func (c *Counter) LogRecvMessageStream(size int64, proto endpoint.ProtocolID, peer endpoint.NodeID) {
	if !c.config.Enabled || size <= 0 {
		return
	}

	// 总量
	c.totalIn.Mark(uint64(size))

	// 按协议
	if c.config.TrackByProtocol {
		c.protocolIn.Get(string(proto)).Mark(uint64(size))
	}

	// 按 Peer
	if c.config.TrackByPeer {
		c.peerIn.Get(peer.String()).Mark(uint64(size))
	}
}

// ==================== 获取统计 ====================

// GetBandwidthTotals 获取总带宽统计
func (c *Counter) GetBandwidthTotals() bandwidthif.Stats {
	inSnap := c.totalIn.Snapshot()
	outSnap := c.totalOut.Snapshot()

	return bandwidthif.Stats{
		TotalIn:  int64(inSnap.Total),
		TotalOut: int64(outSnap.Total),
		RateIn:   inSnap.Rate,
		RateOut:  outSnap.Rate,
	}
}

// GetBandwidthForPeer 获取指定 Peer 的带宽统计
func (c *Counter) GetBandwidthForPeer(peer endpoint.NodeID) bandwidthif.Stats {
	key := peer.String()

	var stats bandwidthif.Stats

	// 使用 Load 而不是 Get，避免创建不存在的条目
	if inMeter, ok := c.peerIn.Load(key); ok {
		inSnap := inMeter.Snapshot()
		stats.TotalIn = int64(inSnap.Total)
		stats.RateIn = inSnap.Rate
	}

	if outMeter, ok := c.peerOut.Load(key); ok {
		outSnap := outMeter.Snapshot()
		stats.TotalOut = int64(outSnap.Total)
		stats.RateOut = outSnap.Rate
	}

	return stats
}

// GetBandwidthForProtocol 获取指定协议的带宽统计
func (c *Counter) GetBandwidthForProtocol(proto endpoint.ProtocolID) bandwidthif.Stats {
	key := string(proto)

	var stats bandwidthif.Stats

	// 使用 Load 而不是 Get，避免创建不存在的条目
	if inMeter, ok := c.protocolIn.Load(key); ok {
		inSnap := inMeter.Snapshot()
		stats.TotalIn = int64(inSnap.Total)
		stats.RateIn = inSnap.Rate
	}

	if outMeter, ok := c.protocolOut.Load(key); ok {
		outSnap := outMeter.Snapshot()
		stats.TotalOut = int64(outSnap.Total)
		stats.RateOut = outSnap.Rate
	}

	return stats
}

// GetBandwidthByPeer 获取所有 Peer 的带宽统计
func (c *Counter) GetBandwidthByPeer() map[endpoint.NodeID]bandwidthif.Stats {
	peers := make(map[endpoint.NodeID]bandwidthif.Stats)

	// 收集入站统计
	c.peerIn.ForEach(func(key string, meter *Meter) {
		nodeID, err := types.ParseNodeID(key)
		if err != nil {
			return
		}
		snap := meter.Snapshot()

		stat := peers[nodeID]
		stat.TotalIn = int64(snap.Total)
		stat.RateIn = snap.Rate
		peers[nodeID] = stat
	})

	// 收集出站统计
	c.peerOut.ForEach(func(key string, meter *Meter) {
		nodeID, err := types.ParseNodeID(key)
		if err != nil {
			return
		}
		snap := meter.Snapshot()

		stat := peers[nodeID]
		stat.TotalOut = int64(snap.Total)
		stat.RateOut = snap.Rate
		peers[nodeID] = stat
	})

	return peers
}

// GetBandwidthByProtocol 获取所有协议的带宽统计
func (c *Counter) GetBandwidthByProtocol() map[endpoint.ProtocolID]bandwidthif.Stats {
	protocols := make(map[endpoint.ProtocolID]bandwidthif.Stats)

	// 收集入站统计
	c.protocolIn.ForEach(func(key string, meter *Meter) {
		protoID := endpoint.ProtocolID(key)
		snap := meter.Snapshot()

		stat := protocols[protoID]
		stat.TotalIn = int64(snap.Total)
		stat.RateIn = snap.Rate
		protocols[protoID] = stat
	})

	// 收集出站统计
	c.protocolOut.ForEach(func(key string, meter *Meter) {
		protoID := endpoint.ProtocolID(key)
		snap := meter.Snapshot()

		stat := protocols[protoID]
		stat.TotalOut = int64(snap.Total)
		stat.RateOut = snap.Rate
		protocols[protoID] = stat
	})

	return protocols
}

// ==================== 管理 ====================

// Reset 重置所有统计
func (c *Counter) Reset() {
	c.totalIn.Reset()
	c.totalOut.Reset()

	c.protocolIn.Clear()
	c.protocolOut.Clear()

	c.peerIn.Clear()
	c.peerOut.Clear()
}

// TrimIdle 清理空闲条目
func (c *Counter) TrimIdle(since time.Time) {
	c.peerIn.TrimIdle(since)
	c.peerOut.TrimIdle(since)
	c.protocolIn.TrimIdle(since)
	c.protocolOut.TrimIdle(since)
}

// ==================== 额外方法 ====================

// PeerCount 返回跟踪的 Peer 数量
func (c *Counter) PeerCount() int {
	inCount := c.peerIn.Count()
	outCount := c.peerOut.Count()
	if inCount > outCount {
		return inCount
	}
	return outCount
}

// ProtocolCount 返回跟踪的协议数量
func (c *Counter) ProtocolCount() int {
	inCount := c.protocolIn.Count()
	outCount := c.protocolOut.Count()
	if inCount > outCount {
		return inCount
	}
	return outCount
}

// TopPeers 返回流量最大的 N 个 Peer
func (c *Counter) TopPeers(n int) []bandwidthif.PeerStats {
	peers := c.GetBandwidthByPeer()
	if len(peers) == 0 {
		return nil
	}

	// 转换为切片
	result := make([]bandwidthif.PeerStats, 0, len(peers))
	for peer, stats := range peers {
		result = append(result, bandwidthif.PeerStats{
			Peer:  peer,
			Stats: stats,
		})
	}

	// 按总流量排序（简单冒泡排序）
	for i := 0; i < len(result)-1 && i < n; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Stats.TotalBytes() > result[i].Stats.TotalBytes() {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	if n > len(result) {
		n = len(result)
	}
	return result[:n]
}

// TopProtocols 返回流量最大的 N 个协议
func (c *Counter) TopProtocols(n int) []bandwidthif.ProtocolStats {
	protocols := c.GetBandwidthByProtocol()
	if len(protocols) == 0 {
		return nil
	}

	// 转换为切片
	result := make([]bandwidthif.ProtocolStats, 0, len(protocols))
	for proto, stats := range protocols {
		result = append(result, bandwidthif.ProtocolStats{
			Protocol: proto,
			Stats:    stats,
		})
	}

	// 按总流量排序（简单冒泡排序）
	for i := 0; i < len(result)-1 && i < n; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Stats.TotalBytes() > result[i].Stats.TotalBytes() {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	if n > len(result) {
		n = len(result)
	}
	return result[:n]
}

