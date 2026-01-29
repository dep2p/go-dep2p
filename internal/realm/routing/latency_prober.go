package routing

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// ============================================================================
//                              延迟探测器
// ============================================================================

// LatencyProber 延迟探测器
type LatencyProber struct {
	mu sync.RWMutex

	// 配置
	host       pkgif.Host
	interval   time.Duration
	timeout    time.Duration
	windowSize int

	// 延迟数据
	latencyHistory map[string][]time.Duration
	latencyCache   map[string]time.Duration

	// 控制
	ctx     context.Context
	cancel  context.CancelFunc
	started atomic.Bool
	ticker  *time.Ticker
}

// NewLatencyProber 创建延迟探测器
func NewLatencyProber(host pkgif.Host) *LatencyProber {
	return &LatencyProber{
		host:           host,
		interval:       30 * time.Second,
		timeout:        3 * time.Second,
		windowSize:     10,
		latencyHistory: make(map[string][]time.Duration),
		latencyCache:   make(map[string]time.Duration),
	}
}

// ============================================================================
//                              延迟测量
// ============================================================================

// PingProtocol Ping 协议 ID（使用统一定义）
// 使用标准系统 Ping 协议
var PingProtocol = string(protocol.Ping)

// PingPayloadSize Ping 负载大小
const PingPayloadSize = 32

// MeasureLatency 测量延迟
//
// 通过向目标节点发送 Ping 消息并等待 Pong 响应来测量 RTT（往返时间）。
// 协议流程：
//  1. 打开到目标节点的流
//  2. 发送随机 32 字节负载
//  3. 等待节点回复相同负载
//  4. 计算往返时间
func (lp *LatencyProber) MeasureLatency(ctx context.Context, peerID string) (time.Duration, error) {
	if lp.host == nil {
		return 0, fmt.Errorf("host is nil, cannot measure latency")
	}

	// 应用超时
	ctx, cancel := context.WithTimeout(ctx, lp.timeout)
	defer cancel()

	// 1. 建立流连接
	start := time.Now()
	stream, err := lp.host.NewStream(ctx, peerID, PingProtocol)
	if err != nil {
		return 0, fmt.Errorf("failed to open stream to %s: %w", peerID, err)
	}
	defer stream.Close()

	// 2. 生成随机负载
	payload := make([]byte, PingPayloadSize)
	for i := range payload {
		payload[i] = byte(start.UnixNano() >> (i % 8))
	}

	// 3. 发送 Ping
	if _, err := stream.Write(payload); err != nil {
		return 0, fmt.Errorf("failed to send ping: %w", err)
	}

	// 4. 读取 Pong 响应
	response := make([]byte, PingPayloadSize)
	if _, err := readFull(stream, response); err != nil {
		return 0, fmt.Errorf("failed to receive pong: %w", err)
	}

	// 5. 验证响应
	for i := range payload {
		if payload[i] != response[i] {
			return 0, fmt.Errorf("ping response mismatch at byte %d", i)
		}
	}

	// 6. 计算延迟
	latency := time.Since(start)

	// 记录延迟
	lp.RecordLatency(peerID, latency)

	return latency, nil
}

// readFull 完整读取指定长度的数据
func readFull(r interface{ Read([]byte) (int, error) }, buf []byte) (int, error) {
	var total int
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// HandlePingStream 处理 Ping 请求（作为服务端）
//
// 接收 Ping 负载并原样返回（Pong）。
func (lp *LatencyProber) HandlePingStream(stream pkgif.Stream) {
	defer stream.Close()

	// 读取 Ping 负载
	payload := make([]byte, PingPayloadSize)
	if _, err := readFull(stream, payload); err != nil {
		return
	}

	// 返回 Pong（原样返回）
	stream.Write(payload)
}

// RegisterPingHandler 注册 Ping 协议处理器
func (lp *LatencyProber) RegisterPingHandler() {
	if lp.host != nil {
		lp.host.SetStreamHandler(PingProtocol, lp.HandlePingStream)
	}
}

// GetLatency 获取缓存的延迟
func (lp *LatencyProber) GetLatency(peerID string) (time.Duration, bool) {
	lp.mu.RLock()
	defer lp.mu.RUnlock()

	latency, ok := lp.latencyCache[peerID]
	return latency, ok
}

// RecordLatency 记录延迟
func (lp *LatencyProber) RecordLatency(peerID string, latency time.Duration) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	// 添加到历史记录
	history := lp.latencyHistory[peerID]
	history = append(history, latency)

	// 保持窗口大小
	if len(history) > lp.windowSize {
		history = history[len(history)-lp.windowSize:]
	}

	lp.latencyHistory[peerID] = history

	// 更新缓存（使用平均值）
	lp.latencyCache[peerID] = lp.calculateMean(history)
}

// ============================================================================
//                              延迟统计
// ============================================================================

// GetStatistics 获取延迟统计
func (lp *LatencyProber) GetStatistics(peerID string) *interfaces.LatencyStats {
	lp.mu.RLock()
	defer lp.mu.RUnlock()

	history, ok := lp.latencyHistory[peerID]
	if !ok || len(history) == 0 {
		return &interfaces.LatencyStats{}
	}

	// 排序（用于百分位计算）
	sorted := make([]time.Duration, len(history))
	copy(sorted, history)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	stats := &interfaces.LatencyStats{
		Mean: lp.calculateMean(history),
		Min:  sorted[0],
		Max:  sorted[len(sorted)-1],
	}

	// 计算百分位
	if len(sorted) > 0 {
		stats.P50 = sorted[len(sorted)*50/100]
		stats.P95 = sorted[int(math.Min(float64(len(sorted)-1), float64(len(sorted)*95/100)))]
		stats.P99 = sorted[int(math.Min(float64(len(sorted)-1), float64(len(sorted)*99/100)))]
	}

	return stats
}

// calculateMean 计算平均延迟
func (lp *LatencyProber) calculateMean(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}

	total := time.Duration(0)
	for _, l := range latencies {
		total += l
	}

	return total / time.Duration(len(latencies))
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动探测器
func (lp *LatencyProber) Start(_ context.Context) error {
	if lp.started.Load() {
		return ErrAlreadyStarted
	}

	// 使用 context.Background() 保证后台循环不受上层 ctx 取消的影响
	lp.ctx, lp.cancel = context.WithCancel(context.Background())
	lp.started.Store(true)

	// 注册 Ping 协议处理器
	lp.RegisterPingHandler()

	// 启动定期探测（如果有 host）
	if lp.host != nil {
		lp.ticker = time.NewTicker(lp.interval)
		go lp.probeLoop()
	}

	return nil
}

// Stop 停止探测器
func (lp *LatencyProber) Stop(_ context.Context) error {
	if !lp.started.Load() {
		return ErrNotStarted
	}

	lp.started.Store(false)

	if lp.cancel != nil {
		lp.cancel()
	}

	if lp.ticker != nil {
		lp.ticker.Stop()
	}

	// 移除 Ping 协议处理器
	if lp.host != nil {
		lp.host.RemoveStreamHandler(PingProtocol)
	}

	return nil
}

// probeLoop 探测循环
//
// 定期对所有已记录的节点进行延迟探测，更新延迟缓存。
func (lp *LatencyProber) probeLoop() {
	for {
		select {
		case <-lp.ticker.C:
			lp.probeAllPeers()

		case <-lp.ctx.Done():
			return
		}
	}
}

// probeAllPeers 探测所有已知节点
func (lp *LatencyProber) probeAllPeers() {
	lp.mu.RLock()
	peerIDs := make([]string, 0, len(lp.latencyHistory))
	for peerID := range lp.latencyHistory {
		peerIDs = append(peerIDs, peerID)
	}
	lp.mu.RUnlock()

	// 并发探测（限制并发数）
	sem := make(chan struct{}, 10) // 最多 10 个并发
	for _, peerID := range peerIDs {
		sem <- struct{}{}
		go func(pid string) {
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(lp.ctx, lp.timeout)
			defer cancel()

			_, _ = lp.MeasureLatency(ctx, pid)
		}(peerID)
	}
}

// AddPeer 添加需要探测的节点
func (lp *LatencyProber) AddPeer(peerID string) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if _, exists := lp.latencyHistory[peerID]; !exists {
		lp.latencyHistory[peerID] = make([]time.Duration, 0, lp.windowSize)
	}
}

// RemovePeer 移除节点
func (lp *LatencyProber) RemovePeer(peerID string) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	delete(lp.latencyHistory, peerID)
	delete(lp.latencyCache, peerID)
}

// 确保实现接口
var _ interfaces.LatencyProber = (*LatencyProber)(nil)
