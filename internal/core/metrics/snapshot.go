package metrics

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/metrics")

// P2PSnapshot P2P 指标快照
//
// 周期性收集并输出 P2P 网络状态快照，便于日志分析和监控。
// 实现文档：design/_discussions/20260128-p2p-log-analysis-framework.md
type P2PSnapshot struct {
	// 时间信息
	Timestamp     time.Time     `json:"timestamp"`
	UptimeSeconds int64         `json:"uptimeSeconds"`
	Interval      time.Duration `json:"interval"`

	// 连接统计
	ConnectedPeers int `json:"connectedPeers"`
	DirectConns    int `json:"directConns"`
	RelayConns     int `json:"relayConns"`

	// 带宽统计
	BytesSent   int64   `json:"bytesSent"`
	BytesRecv   int64   `json:"bytesRecv"`
	SendRateBps float64 `json:"sendRateBps"`
	RecvRateBps float64 `json:"recvRateBps"`

	// DHT 统计
	RoutingTableSize int     `json:"routingTableSize"`
	DHTQueriesTotal  int64   `json:"dhtQueriesTotal"`
	DHTQueriesPerMin float64 `json:"dhtQueriesPerMin"`

	// Pubsub 统计
	MsgSentTotal  int64   `json:"msgSentTotal"`
	MsgRecvTotal  int64   `json:"msgRecvTotal"`
	MsgSentPerMin float64 `json:"msgSentPerMin"`
	MsgRecvPerMin float64 `json:"msgRecvPerMin"`

	// 健康检测统计
	HealthCheckSuccess int64 `json:"healthCheckSuccess"`
	HealthCheckFailed  int64 `json:"healthCheckFailed"`

	// 资源统计
	Goroutines  int     `json:"goroutines"`
	HeapAllocMB float64 `json:"heapAllocMB"`
	HeapSysMB   float64 `json:"heapSysMB"`
}

// SnapshotCollector 快照收集器配置
type SnapshotCollector struct {
	mu sync.RWMutex

	// 启动时间
	startTime time.Time

	// 数据源
	swarm     pkgif.Swarm
	bandwidth *BandwidthCounter

	// 可选数据源（通过 setter 注入）
	dhtQuerier    DHTQuerier
	pubsubMetrics PubsubMetrics

	// 计数器（累计值）
	dhtQueries         atomic.Int64
	msgSent            atomic.Int64
	msgRecv            atomic.Int64
	healthCheckSuccess atomic.Int64
	healthCheckFailed  atomic.Int64

	// 上次快照时的值（用于计算速率）
	lastSnapshot     *P2PSnapshot
	lastDHTQueries   int64
	lastMsgSent      int64
	lastMsgRecv      int64
	lastSnapshotTime time.Time

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DHTQuerier DHT 查询接口（用于获取路由表大小）
type DHTQuerier interface {
	RoutingTableSize() int
}

// PubsubMetrics Pubsub 指标接口
type PubsubMetrics interface {
	GetStats() (sent, recv int64)
}

// NewSnapshotCollector 创建快照收集器
func NewSnapshotCollector(swarm pkgif.Swarm, bandwidth *BandwidthCounter) *SnapshotCollector {
	return &SnapshotCollector{
		startTime: time.Now(),
		swarm:     swarm,
		bandwidth: bandwidth,
	}
}

// SetDHTQuerier 设置 DHT 查询器
func (c *SnapshotCollector) SetDHTQuerier(dht DHTQuerier) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dhtQuerier = dht
}

// SetPubsubMetrics 设置 Pubsub 指标
func (c *SnapshotCollector) SetPubsubMetrics(ps PubsubMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pubsubMetrics = ps
}

// RecordDHTQuery 记录 DHT 查询
func (c *SnapshotCollector) RecordDHTQuery() {
	c.dhtQueries.Add(1)
}

// RecordMsgSent 记录消息发送
func (c *SnapshotCollector) RecordMsgSent() {
	c.msgSent.Add(1)
}

// RecordMsgRecv 记录消息接收
func (c *SnapshotCollector) RecordMsgRecv() {
	c.msgRecv.Add(1)
}

// RecordHealthCheck 记录健康检测结果
func (c *SnapshotCollector) RecordHealthCheck(success bool) {
	if success {
		c.healthCheckSuccess.Add(1)
	} else {
		c.healthCheckFailed.Add(1)
	}
}

// Start 启动周期性快照
func (c *SnapshotCollector) Start(interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return // 已经启动
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.lastSnapshotTime = time.Now()
	c.mu.Unlock()

	c.wg.Add(1)
	go c.snapshotLoop(interval)

	logger.Info("P2P 指标快照收集器已启动", "interval", interval)
}

// Stop 停止快照收集
func (c *SnapshotCollector) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.mu.Unlock()

	c.wg.Wait()
	logger.Info("P2P 指标快照收集器已停止")
}

// snapshotLoop 快照循环
func (c *SnapshotCollector) snapshotLoop(interval time.Duration) {
	defer c.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			snapshot := c.Collect()
			c.logSnapshot(snapshot)
		}
	}
}

// Collect 收集当前快照
func (c *SnapshotCollector) Collect() *P2PSnapshot {
	now := time.Now()

	c.mu.RLock()
	lastTime := c.lastSnapshotTime
	lastDHT := c.lastDHTQueries
	lastSent := c.lastMsgSent
	lastRecv := c.lastMsgRecv
	dhtQuerier := c.dhtQuerier
	c.mu.RUnlock()

	// 计算时间间隔
	elapsed := now.Sub(lastTime)
	elapsedMinutes := elapsed.Minutes()
	if elapsedMinutes <= 0 {
		elapsedMinutes = 1.0 / 60.0 // 最小 1 秒
	}

	// 连接统计
	var connectedPeers, directConns, relayConns int
	if c.swarm != nil {
		conns := c.swarm.Conns()
		connectedPeers = len(c.swarm.Peers())
		for _, conn := range conns {
			if conn.ConnType().IsDirect() {
				directConns++
			} else {
				relayConns++
			}
		}
	}

	// 带宽统计
	var bytesSent, bytesRecv int64
	var sendRate, recvRate float64
	if c.bandwidth != nil {
		stats := c.bandwidth.GetBandwidthTotals()
		bytesSent = stats.TotalOut
		bytesRecv = stats.TotalIn
		sendRate = stats.RateOut
		recvRate = stats.RateIn
	}

	// DHT 统计
	var routingTableSize int
	if dhtQuerier != nil {
		routingTableSize = dhtQuerier.RoutingTableSize()
	}
	dhtQueriesTotal := c.dhtQueries.Load()
	dhtQueriesPerMin := float64(dhtQueriesTotal-lastDHT) / elapsedMinutes

	// 消息统计
	msgSentTotal := c.msgSent.Load()
	msgRecvTotal := c.msgRecv.Load()
	msgSentPerMin := float64(msgSentTotal-lastSent) / elapsedMinutes
	msgRecvPerMin := float64(msgRecvTotal-lastRecv) / elapsedMinutes

	// 健康检测统计
	healthSuccess := c.healthCheckSuccess.Load()
	healthFailed := c.healthCheckFailed.Load()

	// 资源统计
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	snapshot := &P2PSnapshot{
		Timestamp:     now,
		UptimeSeconds: int64(now.Sub(c.startTime).Seconds()),
		Interval:      elapsed,

		ConnectedPeers: connectedPeers,
		DirectConns:    directConns,
		RelayConns:     relayConns,

		BytesSent:   bytesSent,
		BytesRecv:   bytesRecv,
		SendRateBps: sendRate,
		RecvRateBps: recvRate,

		RoutingTableSize: routingTableSize,
		DHTQueriesTotal:  dhtQueriesTotal,
		DHTQueriesPerMin: dhtQueriesPerMin,

		MsgSentTotal:  msgSentTotal,
		MsgRecvTotal:  msgRecvTotal,
		MsgSentPerMin: msgSentPerMin,
		MsgRecvPerMin: msgRecvPerMin,

		HealthCheckSuccess: healthSuccess,
		HealthCheckFailed:  healthFailed,

		Goroutines:  runtime.NumGoroutine(),
		HeapAllocMB: float64(memStats.HeapAlloc) / 1024 / 1024,
		HeapSysMB:   float64(memStats.HeapSys) / 1024 / 1024,
	}

	// 更新上次快照值
	c.mu.Lock()
	c.lastSnapshot = snapshot
	c.lastSnapshotTime = now
	c.lastDHTQueries = dhtQueriesTotal
	c.lastMsgSent = msgSentTotal
	c.lastMsgRecv = msgRecvTotal
	c.mu.Unlock()

	return snapshot
}

// logSnapshot 输出快照日志
func (c *SnapshotCollector) logSnapshot(s *P2PSnapshot) {
	logger.Info("P2P 指标快照",
		// 时间
		"uptime", s.UptimeSeconds,
		// 连接
		"connectedPeers", s.ConnectedPeers,
		"directConns", s.DirectConns,
		"relayConns", s.RelayConns,
		// 带宽
		"bytesSent", s.BytesSent,
		"bytesRecv", s.BytesRecv,
		"sendRateBps", formatRate(s.SendRateBps),
		"recvRateBps", formatRate(s.RecvRateBps),
		// DHT
		"routingTableSize", s.RoutingTableSize,
		"dhtQueriesPerMin", formatFloat(s.DHTQueriesPerMin),
		// 消息
		"msgSentPerMin", formatFloat(s.MsgSentPerMin),
		"msgRecvPerMin", formatFloat(s.MsgRecvPerMin),
		// 健康
		"healthSuccess", s.HealthCheckSuccess,
		"healthFailed", s.HealthCheckFailed,
		// 资源
		"goroutines", s.Goroutines,
		"heapAllocMB", formatFloat(s.HeapAllocMB),
	)
}

// GetLastSnapshot 获取最新快照
func (c *SnapshotCollector) GetLastSnapshot() *P2PSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSnapshot
}

// formatRate 格式化速率
func formatRate(bps float64) string {
	if bps < 1024 {
		return formatFloat(bps) + " B/s"
	} else if bps < 1024*1024 {
		return formatFloat(bps/1024) + " KB/s"
	} else {
		return formatFloat(bps/1024/1024) + " MB/s"
	}
}

// formatFloat 格式化浮点数（保留2位小数）
func formatFloat(f float64) string {
	return formatFloatPrec(f, 2)
}

// formatFloatPrec 格式化浮点数（指定精度）
func formatFloatPrec(f float64, prec int) string {
	switch prec {
	case 0:
		return intToString(int64(f))
	case 1:
		return intToString(int64(f*10)/10) + "." + intToString(int64(f*10)%10)
	default: // 2
		whole := int64(f)
		frac := int64((f - float64(whole)) * 100)
		if frac < 0 {
			frac = -frac
		}
		if frac < 10 {
			return intToString(whole) + ".0" + intToString(frac)
		}
		return intToString(whole) + "." + intToString(frac)
	}
}

// intToString 整数转字符串（避免 fmt 依赖）
func intToString(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
