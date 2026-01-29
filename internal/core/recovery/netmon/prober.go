// Package netmon 提供网络状态监控功能
package netmon

import (
	"context"
	"sync/atomic"
)

// ============================================================================
//                         IMPL-NETWORK-RESILIENCE: 主动探测
// ============================================================================

// Prober 网络探测器接口
//
// 用于主动探测网络状态，支持测试注入。
// Phase 5.2: 支持 NetworkMonitor 主动探测网络健康状态
type Prober interface {
	// Probe 执行一次探测
	// 返回探测是否成功
	Probe(ctx context.Context) ProbeResult
}

// ProbeResult 探测结果
type ProbeResult struct {
	// Success 探测是否成功
	Success bool

	// ReachablePeers 可达的节点数
	ReachablePeers int

	// TotalPeers 探测的总节点数
	TotalPeers int

	// FailedPeers 失败的节点列表
	FailedPeers []string

	// Error 探测过程中的错误（如果有）
	Error error
}

// IsHealthy 判断探测结果是否表示网络健康
func (r ProbeResult) IsHealthy() bool {
	return r.Success && r.ReachablePeers > 0
}

// IsDegraded 判断探测结果是否表示网络降级
func (r ProbeResult) IsDegraded() bool {
	return r.ReachablePeers > 0 && len(r.FailedPeers) > 0
}

// IsDown 判断探测结果是否表示网络不可用
func (r ProbeResult) IsDown() bool {
	return r.ReachablePeers == 0 && r.TotalPeers > 0
}

// ============================================================================
//                              NoOp Prober
// ============================================================================

// NoOpProber 空操作探测器（用于禁用探测或测试）
type NoOpProber struct{}

// NewNoOpProber 创建空操作探测器
func NewNoOpProber() *NoOpProber {
	return &NoOpProber{}
}

// Probe 总是返回成功
func (p *NoOpProber) Probe(_ context.Context) ProbeResult {
	return ProbeResult{
		Success:        true,
		ReachablePeers: 1,
		TotalPeers:     1,
	}
}

// ============================================================================
//                              Mock Prober (用于测试)
// ============================================================================

// MockProber 可控的模拟探测器（用于测试）
type MockProber struct {
	// 通过 atomic 安全设置
	success    atomic.Bool
	reachable  atomic.Int32
	total      atomic.Int32
	probeCount atomic.Int64
}

// NewMockProber 创建模拟探测器
func NewMockProber() *MockProber {
	mp := &MockProber{}
	mp.success.Store(true)
	mp.reachable.Store(1)
	mp.total.Store(1)
	return mp
}

// SetResult 设置探测结果
func (p *MockProber) SetResult(success bool, reachable, total int) {
	p.success.Store(success)
	p.reachable.Store(int32(reachable))
	p.total.Store(int32(total))
}

// Probe 返回预设的结果
func (p *MockProber) Probe(_ context.Context) ProbeResult {
	p.probeCount.Add(1)
	return ProbeResult{
		Success:        p.success.Load(),
		ReachablePeers: int(p.reachable.Load()),
		TotalPeers:     int(p.total.Load()),
	}
}

// GetProbeCount 获取探测次数
func (p *MockProber) GetProbeCount() int64 {
	return p.probeCount.Load()
}

// ResetProbeCount 重置探测计数
func (p *MockProber) ResetProbeCount() {
	p.probeCount.Store(0)
}
