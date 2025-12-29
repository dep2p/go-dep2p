package bandwidth

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	bandwidthif "github.com/dep2p/go-dep2p/pkg/interfaces/bandwidth"
)

// 包级别日志实例
var log = logger.Logger("bandwidth")

// ============================================================================
//                              报告器实现
// ============================================================================

// Reporter 带宽报告器
type Reporter struct {
	counter  *Counter
	interval time.Duration
	stopCh   chan struct{}
	running  int32 // 使用 atomic 操作
	mu       sync.Mutex
}

// NewReporter 创建报告器
func NewReporter(counter *Counter) *Reporter {
	return &Reporter{
		counter: counter,
	}
}

// Start 启动定期报告
func (r *Reporter) Start(interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if atomic.LoadInt32(&r.running) == 1 {
		return
	}

	r.interval = interval
	r.stopCh = make(chan struct{})
	atomic.StoreInt32(&r.running, 1)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				report := r.Report()
				r.logReport(report)
			case <-r.stopCh:
				return
			}
		}
	}()
}

// Stop 停止报告
func (r *Reporter) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if atomic.LoadInt32(&r.running) == 0 {
		return
	}

	atomic.StoreInt32(&r.running, 0)
		if r.stopCh != nil {
			close(r.stopCh)
		r.stopCh = nil
		}
}

// Report 生成报告
func (r *Reporter) Report() *bandwidthif.Report {
	return &bandwidthif.Report{
		Timestamp:    time.Now(),
		Duration:     r.interval,
		Total:        r.counter.GetBandwidthTotals(),
		ByPeer:       r.counter.GetBandwidthByPeer(),
		ByProtocol:   r.counter.GetBandwidthByProtocol(),
		TopPeers:     r.counter.TopPeers(10),
		TopProtocols: r.counter.TopProtocols(10),
	}
}

// logReport 记录报告
func (r *Reporter) logReport(report *bandwidthif.Report) {
	log.Info("带宽统计报告",
		"totalIn", FormatBytes(report.Total.TotalIn),
		"totalOut", FormatBytes(report.Total.TotalOut),
		"rateIn", FormatRate(report.Total.RateIn),
		"rateOut", FormatRate(report.Total.RateOut),
		"peers", len(report.ByPeer),
		"protocols", len(report.ByProtocol),
	)
}

