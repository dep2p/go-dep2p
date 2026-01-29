// Package bandwidth 提供带宽统计模块的实现
package bandwidth

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              带宽计数器
// ============================================================================

// Counter 带宽计数器实现
//
// 实现 interfaces.BandwidthCounter 接口
type Counter struct {
	config interfaces.BandwidthConfig

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
var _ interfaces.BandwidthCounter = (*Counter)(nil)

// NewCounter 创建带宽计数器
func NewCounter(config interfaces.BandwidthConfig) *Counter {
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

// LogSentStream 记录流上发送的消息
func (c *Counter) LogSentStream(size int64, proto string, peer string) {
	if !c.config.Enabled || size <= 0 {
		return
	}

	// 总量
	c.totalOut.Mark(uint64(size))

	// 按协议
	if c.config.TrackByProtocol {
		c.protocolOut.Get(proto).Mark(uint64(size))
	}

	// 按 Peer
	if c.config.TrackByPeer {
		c.peerOut.Get(peer).Mark(uint64(size))
	}
}

// LogRecvStream 记录流上接收的消息
func (c *Counter) LogRecvStream(size int64, proto string, peer string) {
	if !c.config.Enabled || size <= 0 {
		return
	}

	// 总量
	c.totalIn.Mark(uint64(size))

	// 按协议
	if c.config.TrackByProtocol {
		c.protocolIn.Get(proto).Mark(uint64(size))
	}

	// 按 Peer
	if c.config.TrackByPeer {
		c.peerIn.Get(peer).Mark(uint64(size))
	}
}

// ==================== 获取统计 ====================

// GetTotals 获取总带宽统计
func (c *Counter) GetTotals() interfaces.BandwidthStats {
	inSnap := c.totalIn.Snapshot()
	outSnap := c.totalOut.Snapshot()

	return interfaces.BandwidthStats{
		TotalIn:  int64(inSnap.Total),
		TotalOut: int64(outSnap.Total),
		RateIn:   inSnap.Rate,
		RateOut:  outSnap.Rate,
	}
}

// GetForPeer 获取指定 Peer 的带宽统计
func (c *Counter) GetForPeer(peer string) interfaces.BandwidthStats {
	var stats interfaces.BandwidthStats

	// 使用 Load 而不是 Get，避免创建不存在的条目
	if inMeter, ok := c.peerIn.Load(peer); ok {
		inSnap := inMeter.Snapshot()
		stats.TotalIn = int64(inSnap.Total)
		stats.RateIn = inSnap.Rate
	}

	if outMeter, ok := c.peerOut.Load(peer); ok {
		outSnap := outMeter.Snapshot()
		stats.TotalOut = int64(outSnap.Total)
		stats.RateOut = outSnap.Rate
	}

	return stats
}

// GetForProtocol 获取指定协议的带宽统计
func (c *Counter) GetForProtocol(proto string) interfaces.BandwidthStats {
	var stats interfaces.BandwidthStats

	// 使用 Load 而不是 Get，避免创建不存在的条目
	if inMeter, ok := c.protocolIn.Load(proto); ok {
		inSnap := inMeter.Snapshot()
		stats.TotalIn = int64(inSnap.Total)
		stats.RateIn = inSnap.Rate
	}

	if outMeter, ok := c.protocolOut.Load(proto); ok {
		outSnap := outMeter.Snapshot()
		stats.TotalOut = int64(outSnap.Total)
		stats.RateOut = outSnap.Rate
	}

	return stats
}

// GetByPeer 获取所有 Peer 的带宽统计
func (c *Counter) GetByPeer() map[string]interfaces.BandwidthStats {
	peers := make(map[string]interfaces.BandwidthStats)

	// 收集入站统计
	c.peerIn.ForEach(func(key string, meter *Meter) {
		snap := meter.Snapshot()

		stat := peers[key]
		stat.TotalIn = int64(snap.Total)
		stat.RateIn = snap.Rate
		peers[key] = stat
	})

	// 收集出站统计
	c.peerOut.ForEach(func(key string, meter *Meter) {
		snap := meter.Snapshot()

		stat := peers[key]
		stat.TotalOut = int64(snap.Total)
		stat.RateOut = snap.Rate
		peers[key] = stat
	})

	return peers
}

// GetByProtocol 获取所有协议的带宽统计
func (c *Counter) GetByProtocol() map[string]interfaces.BandwidthStats {
	protocols := make(map[string]interfaces.BandwidthStats)

	// 收集入站统计
	c.protocolIn.ForEach(func(key string, meter *Meter) {
		snap := meter.Snapshot()

		stat := protocols[key]
		stat.TotalIn = int64(snap.Total)
		stat.RateIn = snap.Rate
		protocols[key] = stat
	})

	// 收集出站统计
	c.protocolOut.ForEach(func(key string, meter *Meter) {
		snap := meter.Snapshot()

		stat := protocols[key]
		stat.TotalOut = int64(snap.Total)
		stat.RateOut = snap.Rate
		protocols[key] = stat
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
