package msgrate

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/msgrate")

// ============================================================================
//                              配置
// ============================================================================

// Config 消息速率追踪器配置
type Config struct {
	// CapacityOverestimation 容量过估计因子
	// 用于避免锁定在较低值，默认 1.01
	CapacityOverestimation float64

	// MeasurementImpact 测量影响因子
	// 新测量对现有估计的影响程度，默认 0.1
	MeasurementImpact float64

	// RTTMinEstimate 最小 RTT 估计
	RTTMinEstimate time.Duration

	// RTTMaxEstimate 最大 RTT 估计
	RTTMaxEstimate time.Duration

	// RTTPushdownFactor RTT 降低因子
	// 用于计算目标 RTT，默认 0.9
	RTTPushdownFactor float64

	// RTTMinConfidence 最小置信度
	RTTMinConfidence float64

	// TTLScaling 超时缩放因子
	TTLScaling float64

	// TTLLimit 超时上限
	TTLLimit time.Duration

	// TuningImpact 调优影响因子
	TuningImpact float64

	// TuningConfidenceCap 调优置信度上限（节点数）
	TuningConfidenceCap int
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		CapacityOverestimation: 1.01,
		MeasurementImpact:      0.1,
		RTTMinEstimate:         2 * time.Second,
		RTTMaxEstimate:         20 * time.Second,
		RTTPushdownFactor:      0.9,
		RTTMinConfidence:       0.1,
		TTLScaling:             3.0,
		TTLLimit:               60 * time.Second,
		TuningImpact:           0.25,
		TuningConfidenceCap:    10,
	}
}

// ============================================================================
//                              Tracker 接口
// ============================================================================

// Tracker 单个节点的消息速率追踪器接口
type Tracker interface {
	// Capacity 返回节点在目标 RTT 内可处理的消息数量
	Capacity(kind uint64, targetRTT time.Duration) int

	// Update 更新测量结果
	Update(kind uint64, elapsed time.Duration, items int)

	// RTT 返回估计的往返时间
	RTT() time.Duration
}

// Trackers 多个节点的追踪器集合接口
type Trackers interface {
	// Track 添加节点追踪器
	Track(id string, tracker Tracker) error

	// Untrack 停止追踪节点
	Untrack(id string) error

	// TargetRoundTrip 返回目标 RTT
	TargetRoundTrip() time.Duration

	// TargetTimeout 返回基于 RTT 的超时
	TargetTimeout() time.Duration

	// MedianRoundTrip 返回中位数 RTT
	MedianRoundTrip() time.Duration

	// MeanCapacities 返回平均容量
	MeanCapacities() map[uint64]float64

	// Capacity 获取指定节点的容量
	Capacity(id string, kind uint64, targetRTT time.Duration) int

	// Update 更新指定节点的测量结果
	Update(id string, kind uint64, elapsed time.Duration, items int)
}

// ============================================================================
//                              Tracker 实现
// ============================================================================

// tracker 单个节点的消息速率追踪器实现
type tracker struct {
	config Config

	// capacity 每种消息类型的容量（items/second）
	capacity map[uint64]float64

	// roundtrip 往返时间估计
	roundtrip time.Duration

	mu sync.RWMutex
}

var _ Tracker = (*tracker)(nil)

// NewTracker 创建新的追踪器
func NewTracker(config Config, caps map[uint64]float64, rtt time.Duration) Tracker {
	if caps == nil {
		caps = make(map[uint64]float64)
	}
	return &tracker{
		config:    config,
		capacity:  caps,
		roundtrip: rtt,
	}
}

// Capacity 返回节点在目标 RTT 内可处理的消息数量
func (t *tracker) Capacity(kind uint64, targetRTT time.Duration) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// 计算实际测量的吞吐量
	throughput := t.capacity[kind] * float64(targetRTT) / float64(time.Second)

	// 返回过估计值，避免锁定在较低值
	return roundCapacity(1 + t.config.CapacityOverestimation*throughput)
}

// roundCapacity 将容量值转换为整数
func roundCapacity(cap float64) int {
	result := int(math.Min(math.MaxInt32, math.Max(1, math.Ceil(cap))))
	return result
}

// Update 更新测量结果
func (t *tracker) Update(kind uint64, elapsed time.Duration, items int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 如果没有任何交付（超时/数据不可用），将吞吐量降至最低
	if items == 0 {
		t.capacity[kind] = 0
		return
	}

	// 更新吞吐量
	if elapsed <= 0 {
		elapsed = 1 // 确保非零除数
	}
	measured := float64(items) / (float64(elapsed) / float64(time.Second))

	impact := t.config.MeasurementImpact
	t.capacity[kind] = (1-impact)*t.capacity[kind] + impact*measured
	t.roundtrip = time.Duration((1-impact)*float64(t.roundtrip) + impact*float64(elapsed))
}

// RTT 返回估计的往返时间
func (t *tracker) RTT() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.roundtrip
}

// ============================================================================
//                              Trackers 实现
// ============================================================================

// trackers 多个节点的追踪器集合实现
type trackers struct {
	config Config

	trackerMap map[string]Tracker

	// roundtrip 当前最佳 RTT 估计
	roundtrip time.Duration

	// confidence 置信度因子
	confidence float64

	// tuned 上次调优时间
	tuned time.Time

	mu sync.RWMutex
}

var _ Trackers = (*trackers)(nil)

// NewTrackers 创建追踪器集合
func NewTrackers(config Config) Trackers {
	return &trackers{
		config:     config,
		trackerMap: make(map[string]Tracker),
		roundtrip:  config.RTTMaxEstimate,
		confidence: 1.0,
		tuned:      time.Now(),
	}
}

// Track 添加节点追踪器
func (t *trackers) Track(id string, tracker Tracker) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.trackerMap[id]; ok {
		return ErrAlreadyTracking
	}
	t.trackerMap[id] = tracker
	t.detune()

	logger.Debug("开始追踪节点", "peer", id)
	return nil
}

// Untrack 停止追踪节点
func (t *trackers) Untrack(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.trackerMap[id]; !ok {
		return ErrNotTracking
	}
	delete(t.trackerMap, id)

	logger.Debug("停止追踪节点", "peer", id)
	return nil
}

// TargetRoundTrip 返回目标 RTT
func (t *trackers) TargetRoundTrip() time.Duration {
	t.tune()
	t.mu.RLock()
	defer t.mu.RUnlock()
	return time.Duration(float64(t.roundtrip) * t.config.RTTPushdownFactor)
}

// TargetTimeout 返回基于 RTT 的超时
func (t *trackers) TargetTimeout() time.Duration {
	t.tune()
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.targetTimeout()
}

// targetTimeout 内部无锁版本
func (t *trackers) targetTimeout() time.Duration {
	timeout := time.Duration(t.config.TTLScaling * float64(t.roundtrip) / t.confidence)
	if timeout > t.config.TTLLimit {
		timeout = t.config.TTLLimit
	}
	return timeout
}

// MedianRoundTrip 返回中位数 RTT
func (t *trackers) MedianRoundTrip() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.medianRoundTrip()
}

// medianRoundTrip 内部无锁版本
func (t *trackers) medianRoundTrip() time.Duration {
	rtts := make([]float64, 0, len(t.trackerMap))
	for _, tt := range t.trackerMap {
		rtts = append(rtts, float64(tt.RTT()))
	}

	if len(rtts) == 0 {
		return t.config.RTTMaxEstimate
	}

	// 排序并取中位数
	sort.Float64s(rtts)

	var median time.Duration
	switch len(rtts) {
	case 1:
		median = time.Duration(rtts[0])
	default:
		// 使用 sqrt(n) 作为索引，偏向较小的 RTT
		idx := int(math.Sqrt(float64(len(rtts))))
		if idx >= len(rtts) {
			idx = len(rtts) - 1
		}
		median = time.Duration(rtts[idx])
	}

	// 限制在配置范围内
	if median < t.config.RTTMinEstimate {
		median = t.config.RTTMinEstimate
	}
	if median > t.config.RTTMaxEstimate {
		median = t.config.RTTMaxEstimate
	}
	return median
}

// MeanCapacities 返回平均容量
func (t *trackers) MeanCapacities() map[uint64]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	capacities := make(map[uint64]float64)
	count := 0

	for _, tt := range t.trackerMap {
		if tr, ok := tt.(*tracker); ok {
			tr.mu.RLock()
			for key, val := range tr.capacity {
				capacities[key] += val
			}
			tr.mu.RUnlock()
			count++
		}
	}

	if count > 0 {
		for key := range capacities {
			capacities[key] /= float64(count)
		}
	}
	return capacities
}

// Capacity 获取指定节点的容量
func (t *trackers) Capacity(id string, kind uint64, targetRTT time.Duration) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	tracker := t.trackerMap[id]
	if tracker == nil {
		return 1 // 未注册，返回最小值
	}
	return tracker.Capacity(kind, targetRTT)
}

// Update 更新指定节点的测量结果
func (t *trackers) Update(id string, kind uint64, elapsed time.Duration, items int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if tracker := t.trackerMap[id]; tracker != nil {
		tracker.Update(kind, elapsed, items)
	}
}

// tune 调优内部缓存
func (t *trackers) tune() {
	t.mu.RLock()
	dirty := time.Since(t.tuned) > t.roundtrip
	t.mu.RUnlock()

	if !dirty {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if dirty := time.Since(t.tuned) > t.roundtrip; !dirty {
		return // 并发请求已调优
	}

	// 更新 RTT 估计
	medianRTT := t.medianRoundTrip()
	t.roundtrip = time.Duration((1-t.config.TuningImpact)*float64(t.roundtrip) + t.config.TuningImpact*float64(medianRTT))

	// 更新置信度
	t.confidence += (1 - t.confidence) / 2

	t.tuned = time.Now()
}

// detune 降低置信度
func (t *trackers) detune() {
	if len(t.trackerMap) == 1 {
		t.confidence = 1.0
		return
	}

	if len(t.trackerMap) >= t.config.TuningConfidenceCap {
		return
	}

	peers := float64(len(t.trackerMap))
	t.confidence = t.confidence * (peers - 1) / peers
	if t.confidence < t.config.RTTMinConfidence {
		t.confidence = t.config.RTTMinConfidence
	}
}

// ============================================================================
//                              统计
// ============================================================================

// Stats 返回追踪器统计
func (t *trackers) Stats() TrackerStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TrackerStats{
		TrackedPeers:   len(t.trackerMap),
		CurrentRTT:     t.roundtrip,
		Confidence:     t.confidence,
		TargetRTT:      time.Duration(float64(t.roundtrip) * t.config.RTTPushdownFactor),
		TargetTimeout:  t.targetTimeout(),
		MeanCapacities: t.MeanCapacities(),
	}
}

// TrackerStats 追踪器统计
type TrackerStats struct {
	TrackedPeers   int
	CurrentRTT     time.Duration
	Confidence     float64
	TargetRTT      time.Duration
	TargetTimeout  time.Duration
	MeanCapacities map[uint64]float64
}
