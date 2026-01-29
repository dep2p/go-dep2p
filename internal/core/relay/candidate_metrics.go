package relay

import (
	"context"
	"sync"
	"time"
	
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// CandidateMetrics 候选指标收集器
//
// 定期测量候选中继的延迟、容量、可靠性。
type CandidateMetrics struct {
	mu sync.RWMutex
	
	// 指标数据
	metrics map[string]*RelayMetrics // peerID -> metrics
	
	// 依赖
	swarm pkgif.Swarm
	host  pkgif.Host
	
	// 配置
	interval time.Duration
	
	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// RelayMetrics 中继指标
type RelayMetrics struct {
	PeerID      string        // 节点 ID
	Latency     time.Duration // 延迟
	Capacity    float64       // 容量（0-1）
	Reliability float64       // 可靠性（0-1）
	
	// 统计数据
	PingCount    int       // Ping 次数
	SuccessCount int       // 成功次数
	FailureCount int       // 失败次数
	LastPing     time.Time // 最后 Ping 时间
	LastSuccess  time.Time // 最后成功时间
}

// NewCandidateMetrics 创建指标收集器
func NewCandidateMetrics(swarm pkgif.Swarm, host pkgif.Host) *CandidateMetrics {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &CandidateMetrics{
		metrics:  make(map[string]*RelayMetrics),
		swarm:    swarm,
		host:     host,
		interval: 30 * time.Second, // 每 30 秒测量一次
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start 启动指标收集
func (m *CandidateMetrics) Start() {
	m.wg.Add(1)
	go m.collectLoop()
}

// Stop 停止指标收集
func (m *CandidateMetrics) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

// collectLoop 指标收集循环
func (m *CandidateMetrics) collectLoop() {
	defer m.wg.Done()
	
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
			
		case <-ticker.C:
			m.collectMetrics()
		}
	}
}

// collectMetrics 收集所有候选的指标
func (m *CandidateMetrics) collectMetrics() {
	m.mu.RLock()
	peerIDs := make([]string, 0, len(m.metrics))
	for peerID := range m.metrics {
		peerIDs = append(peerIDs, peerID)
	}
	m.mu.RUnlock()
	
	// 并发测量
	for _, peerID := range peerIDs {
		go m.measurePeer(peerID)
	}
}

// measurePeer 测量单个节点
func (m *CandidateMetrics) measurePeer(peerID string) {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()
	
	// 测量延迟（通过 Ping）
	latency, err := m.ping(ctx, peerID)
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	metrics, exists := m.metrics[peerID]
	if !exists {
		return
	}
	
	metrics.PingCount++
	metrics.LastPing = time.Now()
	
	if err != nil {
		// Ping 失败
		metrics.FailureCount++
	} else {
		// Ping 成功
		metrics.SuccessCount++
		metrics.LastSuccess = time.Now()
		metrics.Latency = latency
	}
	
	// 计算可靠性（成功率）
	if metrics.PingCount > 0 {
		metrics.Reliability = float64(metrics.SuccessCount) / float64(metrics.PingCount)
	}
	
	// 计算容量（简化：基于延迟）
	// 延迟越低，容量越高
	if latency > 0 {
		// 100ms 以下 = 1.0，1000ms 以上 = 0.1
		if latency < 100*time.Millisecond {
			metrics.Capacity = 1.0
		} else if latency > 1000*time.Millisecond {
			metrics.Capacity = 0.1
		} else {
			// 线性插值
			metrics.Capacity = 1.0 - (float64(latency-100*time.Millisecond) / float64(900*time.Millisecond) * 0.9)
		}
	}
}

// ping 测量到节点的延迟
func (m *CandidateMetrics) ping(ctx context.Context, peerID string) (time.Duration, error) {
	if m.host == nil {
		return 0, nil
	}
	
	// 使用 NewStream 测试连接
	start := time.Now()
	
	stream, err := m.host.NewStream(ctx, peerID, string(protocol.Ping))
	if err != nil {
		return 0, err
	}
	defer stream.Close()
	
	// 发送简单的 ping 消息
	_, err = stream.Write([]byte("ping"))
	if err != nil {
		return 0, err
	}
	
	// 读取响应
	buf := make([]byte, 4)
	_, err = stream.Read(buf)
	if err != nil {
		return 0, err
	}
	
	latency := time.Since(start)
	return latency, nil
}

// AddCandidate 添加候选（开始收集指标）
func (m *CandidateMetrics) AddCandidate(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.metrics[peerID]; exists {
		return
	}
	
	m.metrics[peerID] = &RelayMetrics{
		PeerID:      peerID,
		Capacity:    0.8,  // 初始默认值
		Reliability: 0.9,  // 初始默认值
		LastPing:    time.Now(),
	}
}

// RemoveCandidate 移除候选
func (m *CandidateMetrics) RemoveCandidate(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	delete(m.metrics, peerID)
}

// GetMetrics 获取候选指标
func (m *CandidateMetrics) GetMetrics(peerID string) *RelayMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if metrics, exists := m.metrics[peerID]; exists {
		// 返回拷贝
		return &RelayMetrics{
			PeerID:       metrics.PeerID,
			Latency:      metrics.Latency,
			Capacity:     metrics.Capacity,
			Reliability:  metrics.Reliability,
			PingCount:    metrics.PingCount,
			SuccessCount: metrics.SuccessCount,
			FailureCount: metrics.FailureCount,
			LastPing:     metrics.LastPing,
			LastSuccess:  metrics.LastSuccess,
		}
	}
	
	return nil
}

// GetAllMetrics 获取所有候选指标
func (m *CandidateMetrics) GetAllMetrics() map[string]*RelayMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]*RelayMetrics, len(m.metrics))
	for peerID, metrics := range m.metrics {
		result[peerID] = &RelayMetrics{
			PeerID:       metrics.PeerID,
			Latency:      metrics.Latency,
			Capacity:     metrics.Capacity,
			Reliability:  metrics.Reliability,
			PingCount:    metrics.PingCount,
			SuccessCount: metrics.SuccessCount,
			FailureCount: metrics.FailureCount,
			LastPing:     metrics.LastPing,
			LastSuccess:  metrics.LastSuccess,
		}
	}
	
	return result
}
