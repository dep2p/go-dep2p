// Package bootstrap 提供引导节点发现服务
//
// 本文件实现 Liveness 探测，用于检测存储节点的存活状态。
package bootstrap

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// ProbeService 探测服务
// ════════════════════════════════════════════════════════════════════════════

// ProbeService 节点存活探测服务
type ProbeService struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	// 依赖
	host    pkgif.Host
	store   *ExtendedNodeStore
	liveness pkgif.Liveness

	// 配置
	interval      time.Duration
	batchSize     int
	timeout       time.Duration
	maxConcurrent int

	// 状态
	running atomic.Bool
	lastRun time.Time
	mu      sync.RWMutex

	// 统计
	stats ProbeStats
}

// ProbeStats 探测统计
type ProbeStats struct {
	TotalProbes     int64
	SuccessProbes   int64
	FailedProbes    int64
	LastProbeTime   time.Time
	AverageLatency  time.Duration
}

// ProbeOption 探测选项
type ProbeOption func(*ProbeService)

// WithProbeInterval 设置探测间隔
func WithProbeInterval(d time.Duration) ProbeOption {
	return func(p *ProbeService) {
		p.interval = d
	}
}

// WithProbeBatchSize 设置批量大小
func WithProbeBatchSize(size int) ProbeOption {
	return func(p *ProbeService) {
		p.batchSize = size
	}
}

// WithProbeTimeout 设置探测超时
func WithProbeTimeout(d time.Duration) ProbeOption {
	return func(p *ProbeService) {
		p.timeout = d
	}
}

// WithProbeMaxConcurrent 设置最大并发
func WithProbeMaxConcurrent(max int) ProbeOption {
	return func(p *ProbeService) {
		p.maxConcurrent = max
	}
}

// NewProbeService 创建探测服务
func NewProbeService(host pkgif.Host, store *ExtendedNodeStore, liveness pkgif.Liveness, opts ...ProbeOption) *ProbeService {
	defaults := GetDefaults()

	ctx, cancel := context.WithCancel(context.Background())

	p := &ProbeService{
		ctx:           ctx,
		ctxCancel:     cancel,
		host:          host,
		store:         store,
		liveness:      liveness,
		interval:      defaults.ProbeInterval,
		batchSize:     defaults.ProbeBatchSize,
		timeout:       defaults.ProbeTimeout,
		maxConcurrent: defaults.ProbeMaxConcurrent,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// ════════════════════════════════════════════════════════════════════════════
// 生命周期
// ════════════════════════════════════════════════════════════════════════════

// Start 启动探测服务
func (p *ProbeService) Start() error {
	if !p.running.CompareAndSwap(false, true) {
		return nil // 已在运行
	}

	go p.runLoop()
	return nil
}

// Stop 停止探测服务
func (p *ProbeService) Stop() error {
	if !p.running.CompareAndSwap(true, false) {
		return nil // 未在运行
	}

	if p.ctxCancel != nil {
		p.ctxCancel()
	}
	return nil
}

// IsRunning 检查是否在运行
func (p *ProbeService) IsRunning() bool {
	return p.running.Load()
}

// Stats 返回统计信息
func (p *ProbeService) Stats() ProbeStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// LastRun 返回最后运行时间
func (p *ProbeService) LastRun() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastRun
}

// ════════════════════════════════════════════════════════════════════════════
// 探测循环
// ════════════════════════════════════════════════════════════════════════════

// runLoop 运行探测循环
func (p *ProbeService) runLoop() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// 立即执行一次
	p.runProbe()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.runProbe()
		}
	}
}

// runProbe 执行一轮探测
func (p *ProbeService) runProbe() {
	p.mu.Lock()
	p.lastRun = time.Now()
	p.mu.Unlock()

	// 获取需要探测的节点
	entries := p.store.GetForProbe(p.batchSize)
	if len(entries) == 0 {
		return
	}

	// 使用信号量控制并发
	sem := make(chan struct{}, p.maxConcurrent)
	var wg sync.WaitGroup

	for _, entry := range entries {
		select {
		case <-p.ctx.Done():
			return
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(e *NodeEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			p.probeNode(e)
		}(entry)
	}

	wg.Wait()
}

// probeNode 探测单个节点
func (p *ProbeService) probeNode(entry *NodeEntry) {
	ctx, cancel := context.WithTimeout(p.ctx, p.timeout)
	defer cancel()

	start := time.Now()

	// 使用 Liveness 服务探测（如果可用）
	var alive bool
	var err error

	if p.liveness != nil {
		alive, err = p.probeThroughLiveness(ctx, entry)
	} else {
		alive, err = p.probeThroughConnect(ctx, entry)
	}

	latency := time.Since(start)

	// 更新统计
	p.mu.Lock()
	p.stats.TotalProbes++
	if err == nil && alive {
		p.stats.SuccessProbes++
		// 更新平均延迟（简单移动平均）
		if p.stats.AverageLatency == 0 {
			p.stats.AverageLatency = latency
		} else {
			p.stats.AverageLatency = (p.stats.AverageLatency + latency) / 2
		}
	} else {
		p.stats.FailedProbes++
	}
	p.stats.LastProbeTime = time.Now()
	p.mu.Unlock()

	// 更新存储状态
	if err == nil && alive {
		_ = p.store.MarkOnline(entry.ID)
	} else {
		_ = p.store.MarkOffline(entry.ID)
	}
}

// probeThroughLiveness 通过 Liveness 服务探测
func (p *ProbeService) probeThroughLiveness(ctx context.Context, entry *NodeEntry) (bool, error) {
	if p.liveness == nil {
		return false, nil
	}

	// 调用 Liveness 接口的 Check 方法探测
	alive, err := p.liveness.Check(ctx, string(entry.ID))
	if err != nil {
		return false, err
	}

	return alive, nil
}

// probeThroughConnect 通过连接探测
func (p *ProbeService) probeThroughConnect(ctx context.Context, entry *NodeEntry) (bool, error) {
	if p.host == nil {
		return false, nil
	}

	// 尝试连接节点
	err := p.host.Connect(ctx, string(entry.ID), entry.Addrs)
	if err != nil {
		return false, err
	}

	return true, nil
}

// ════════════════════════════════════════════════════════════════════════════
// 手动触发
// ════════════════════════════════════════════════════════════════════════════

// ProbeNow 立即执行一轮探测
func (p *ProbeService) ProbeNow() {
	go p.runProbe()
}

// ProbeOne 探测单个节点
func (p *ProbeService) ProbeOne(id types.NodeID) (bool, error) {
	entry, ok := p.store.Get(id)
	if !ok {
		return false, ErrNodeNotFound
	}

	ctx, cancel := context.WithTimeout(p.ctx, p.timeout)
	defer cancel()

	var alive bool
	var err error

	if p.liveness != nil {
		alive, err = p.probeThroughLiveness(ctx, entry)
	} else {
		alive, err = p.probeThroughConnect(ctx, entry)
	}

	if err == nil && alive {
		_ = p.store.MarkOnline(id)
	} else {
		_ = p.store.MarkOffline(id)
	}

	return alive, err
}
