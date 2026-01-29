package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// BandwidthCounter 带宽计数器
//
// BandwidthCounter 跟踪本地节点发送和接收的数据。
// 使用原子操作实现并发安全的计数器。
type BandwidthCounter struct {
	// 全局计数器（使用 atomic）
	totalIn  atomic.Int64
	totalOut atomic.Int64

	// 协议级计数器
	protocolMu sync.RWMutex
	protocolIn  map[types.ProtocolID]*atomic.Int64
	protocolOut map[types.ProtocolID]*atomic.Int64

	// 节点级计数器
	peerMu sync.RWMutex
	peerIn  map[types.PeerID]*atomic.Int64
	peerOut map[types.PeerID]*atomic.Int64

	// 速率计算器
	totalInRate  *RateMeter
	totalOutRate *RateMeter

	// 协议级速率计算器
	protocolInRate  map[types.ProtocolID]*RateMeter
	protocolOutRate map[types.ProtocolID]*RateMeter

	// 节点级速率计算器
	peerInRate  map[types.PeerID]*RateMeter
	peerOutRate map[types.PeerID]*RateMeter

	// 兼容性字段（旧实现）
	lastUpdate atomic.Int64 // Unix nano
	lastIn     atomic.Int64
	lastOut    atomic.Int64
}

// NewBandwidthCounter 创建新的 BandwidthCounter
func NewBandwidthCounter() *BandwidthCounter {
	bwc := &BandwidthCounter{
		protocolIn:  make(map[types.ProtocolID]*atomic.Int64),
		protocolOut: make(map[types.ProtocolID]*atomic.Int64),
		peerIn:      make(map[types.PeerID]*atomic.Int64),
		peerOut:     make(map[types.PeerID]*atomic.Int64),

		// 初始化速率计算器
		totalInRate:     NewRateMeter(),
		totalOutRate:    NewRateMeter(),
		protocolInRate:  make(map[types.ProtocolID]*RateMeter),
		protocolOutRate: make(map[types.ProtocolID]*RateMeter),
		peerInRate:      make(map[types.PeerID]*RateMeter),
		peerOutRate:     make(map[types.PeerID]*RateMeter),
	}
	bwc.lastUpdate.Store(time.Now().UnixNano())
	return bwc
}

// LogSentMessage 记录出站消息的大小
func (bwc *BandwidthCounter) LogSentMessage(size int64) {
	bwc.totalOut.Add(size)
	bwc.totalOutRate.Add(size)
}

// LogRecvMessage 记录入站消息的大小
func (bwc *BandwidthCounter) LogRecvMessage(size int64) {
	bwc.totalIn.Add(size)
	bwc.totalInRate.Add(size)
}

// LogSentMessageStream 记录通过流发送的消息大小
func (bwc *BandwidthCounter) LogSentMessageStream(size int64, proto types.ProtocolID, p types.PeerID) {
	// 协议统计
	bwc.protocolMu.Lock()
	counter := bwc.protocolOut[proto]
	if counter == nil {
		counter = &atomic.Int64{}
		bwc.protocolOut[proto] = counter
	}
	bwc.protocolMu.Unlock()
	counter.Add(size)

	// 节点统计
	bwc.peerMu.Lock()
	peerCounter := bwc.peerOut[p]
	if peerCounter == nil {
		peerCounter = &atomic.Int64{}
		bwc.peerOut[p] = peerCounter
	}
	bwc.peerMu.Unlock()
	peerCounter.Add(size)
}

// LogRecvMessageStream 记录通过流接收的消息大小
func (bwc *BandwidthCounter) LogRecvMessageStream(size int64, proto types.ProtocolID, p types.PeerID) {
	// 协议统计
	bwc.protocolMu.Lock()
	counter := bwc.protocolIn[proto]
	if counter == nil {
		counter = &atomic.Int64{}
		bwc.protocolIn[proto] = counter
	}
	bwc.protocolMu.Unlock()
	counter.Add(size)

	// 节点统计
	bwc.peerMu.Lock()
	peerCounter := bwc.peerIn[p]
	if peerCounter == nil {
		peerCounter = &atomic.Int64{}
		bwc.peerIn[p] = peerCounter
	}
	bwc.peerMu.Unlock()
	peerCounter.Add(size)
}

// GetBandwidthForPeer 返回节点带宽统计
func (bwc *BandwidthCounter) GetBandwidthForPeer(p types.PeerID) Stats {
	bwc.peerMu.RLock()
	inCounter := bwc.peerIn[p]
	outCounter := bwc.peerOut[p]
	inRate := bwc.peerInRate[p]
	outRate := bwc.peerOutRate[p]
	bwc.peerMu.RUnlock()

	var totalIn, totalOut int64
	var rateIn, rateOut float64

	if inCounter != nil {
		totalIn = inCounter.Load()
	}
	if outCounter != nil {
		totalOut = outCounter.Load()
	}
	if inRate != nil {
		rateIn = inRate.Rate()
	}
	if outRate != nil {
		rateOut = outRate.Rate()
	}

	return Stats{
		TotalIn:  totalIn,
		TotalOut: totalOut,
		RateIn:   rateIn,
		RateOut:  rateOut,
	}
}

// GetBandwidthForProtocol 返回协议带宽统计
func (bwc *BandwidthCounter) GetBandwidthForProtocol(proto types.ProtocolID) Stats {
	bwc.protocolMu.RLock()
	inCounter := bwc.protocolIn[proto]
	outCounter := bwc.protocolOut[proto]
	inRate := bwc.protocolInRate[proto]
	outRate := bwc.protocolOutRate[proto]
	bwc.protocolMu.RUnlock()

	var totalIn, totalOut int64
	var rateIn, rateOut float64

	if inCounter != nil {
		totalIn = inCounter.Load()
	}
	if outCounter != nil {
		totalOut = outCounter.Load()
	}
	if inRate != nil {
		rateIn = inRate.Rate()
	}
	if outRate != nil {
		rateOut = outRate.Rate()
	}

	return Stats{
		TotalIn:  totalIn,
		TotalOut: totalOut,
		RateIn:   rateIn,
		RateOut:  rateOut,
	}
}

// GetBandwidthTotals 返回总带宽统计
func (bwc *BandwidthCounter) GetBandwidthTotals() Stats {
	totalIn := bwc.totalIn.Load()
	totalOut := bwc.totalOut.Load()

	// 使用 RateMeter 计算速率
	rateIn := bwc.totalInRate.Rate()
	rateOut := bwc.totalOutRate.Rate()

	return Stats{
		TotalIn:  totalIn,
		TotalOut: totalOut,
		RateIn:   rateIn,   // 真实速率（字节/秒）
		RateOut:  rateOut,  // 真实速率（字节/秒）
	}
}

// GetBandwidthByPeer 返回所有节点带宽统计
func (bwc *BandwidthCounter) GetBandwidthByPeer() map[types.PeerID]Stats {
	bwc.peerMu.RLock()
	defer bwc.peerMu.RUnlock()

	result := make(map[types.PeerID]Stats, len(bwc.peerIn))

	// 入站
	for peer, counter := range bwc.peerIn {
		stats := result[peer]
		stats.TotalIn = counter.Load()
		result[peer] = stats
	}

	// 出站
	for peer, counter := range bwc.peerOut {
		stats := result[peer]
		stats.TotalOut = counter.Load()
		result[peer] = stats
	}

	return result
}

// GetBandwidthByProtocol 返回所有协议带宽统计
func (bwc *BandwidthCounter) GetBandwidthByProtocol() map[types.ProtocolID]Stats {
	bwc.protocolMu.RLock()
	defer bwc.protocolMu.RUnlock()

	result := make(map[types.ProtocolID]Stats, len(bwc.protocolIn))

	// 入站
	for proto, counter := range bwc.protocolIn {
		stats := result[proto]
		stats.TotalIn = counter.Load()
		result[proto] = stats
	}

	// 出站
	for proto, counter := range bwc.protocolOut {
		stats := result[proto]
		stats.TotalOut = counter.Load()
		result[proto] = stats
	}

	return result
}

// Reset 清除所有统计
func (bwc *BandwidthCounter) Reset() {
	bwc.totalIn.Store(0)
	bwc.totalOut.Store(0)

	bwc.protocolMu.Lock()
	bwc.protocolIn = make(map[types.ProtocolID]*atomic.Int64)
	bwc.protocolOut = make(map[types.ProtocolID]*atomic.Int64)
	bwc.protocolMu.Unlock()

	bwc.peerMu.Lock()
	bwc.peerIn = make(map[types.PeerID]*atomic.Int64)
	bwc.peerOut = make(map[types.PeerID]*atomic.Int64)
	bwc.peerMu.Unlock()

	bwc.lastUpdate.Store(time.Now().UnixNano())
	bwc.lastIn.Store(0)
	bwc.lastOut.Store(0)
}

// TrimIdle 清理空闲统计
func (bwc *BandwidthCounter) TrimIdle(since time.Time) {
	// 清理 5 分钟未活动的协议统计
	bwc.protocolMu.Lock()
	for proto, rateMeter := range bwc.protocolInRate {
		if rateMeter != nil && rateMeter.LastUpdate().Before(since) {
			// 清理入站统计
			delete(bwc.protocolInRate, proto)
			delete(bwc.protocolIn, proto)
		}
	}
	for proto, rateMeter := range bwc.protocolOutRate {
		if rateMeter != nil && rateMeter.LastUpdate().Before(since) {
			// 清理出站统计
			delete(bwc.protocolOutRate, proto)
			delete(bwc.protocolOut, proto)
		}
	}
	bwc.protocolMu.Unlock()

	// 清理 5 分钟未活动的节点统计
	bwc.peerMu.Lock()
	for peer, rateMeter := range bwc.peerInRate {
		if rateMeter != nil && rateMeter.LastUpdate().Before(since) {
			// 清理入站统计
			delete(bwc.peerInRate, peer)
			delete(bwc.peerIn, peer)
		}
	}
	for peer, rateMeter := range bwc.peerOutRate {
		if rateMeter != nil && rateMeter.LastUpdate().Before(since) {
			// 清理出站统计
			delete(bwc.peerOutRate, peer)
			delete(bwc.peerOut, peer)
		}
	}
	bwc.peerMu.Unlock()
}
