// Package bandwidth 提供带宽统计模块的实现
package bandwidth

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
//                              流量计量器
// ============================================================================

// Meter 流量计量器
//
// 使用指数加权移动平均 (EWMA) 算法计算实时速率。
// 所有操作都是线程安全的。
type Meter struct {
	// 累计字节数
	total uint64

	// EWMA 速率计算
	rate      float64
	rateMu    sync.RWMutex
	lastTick  time.Time
	tickMu    sync.Mutex
	tickCount uint64

	// 上次活动时间
	lastActive atomic.Value // time.Time
}

// EWMA 参数
const (
	// alpha 是 EWMA 的平滑因子
	// 值越大，对新数据越敏感
	alpha = 0.25

	// tickInterval 是速率更新间隔
	tickInterval = time.Second
)

// NewMeter 创建新的计量器
func NewMeter() *Meter {
	m := &Meter{
		lastTick: time.Now(),
	}
	m.lastActive.Store(time.Now())
	return m
}

// Mark 记录字节数
func (m *Meter) Mark(n uint64) {
	atomic.AddUint64(&m.total, n)
	m.lastActive.Store(time.Now())
	m.updateRate()
}

// updateRate 更新速率计算
func (m *Meter) updateRate() {
	m.tickMu.Lock()
	defer m.tickMu.Unlock()

	now := time.Now()
	elapsed := now.Sub(m.lastTick)

	// 只有经过足够时间才更新
	if elapsed < tickInterval {
		return
	}

	// 计算这段时间内的字节数
	total := atomic.LoadUint64(&m.total)
	tickCount := atomic.LoadUint64(&m.tickCount)
	bytesThisTick := float64(total - tickCount*uint64(elapsed/tickInterval))

	// 计算瞬时速率
	instantRate := bytesThisTick / elapsed.Seconds()

	// 使用 EWMA 更新速率
	m.rateMu.Lock()
	if m.rate == 0 {
		m.rate = instantRate
	} else {
		m.rate = alpha*instantRate + (1-alpha)*m.rate
	}
	m.rateMu.Unlock()

	m.lastTick = now
	atomic.AddUint64(&m.tickCount, 1)
}

// Snapshot 获取统计快照
func (m *Meter) Snapshot() MeterSnapshot {
	m.rateMu.RLock()
	rate := m.rate
	m.rateMu.RUnlock()

	return MeterSnapshot{
		Total: atomic.LoadUint64(&m.total),
		Rate:  rate,
	}
}

// Total 获取累计字节数
func (m *Meter) Total() uint64 {
	return atomic.LoadUint64(&m.total)
}

// Rate 获取当前速率
func (m *Meter) Rate() float64 {
	m.rateMu.RLock()
	defer m.rateMu.RUnlock()
	return m.rate
}

// LastActive 获取上次活动时间
func (m *Meter) LastActive() time.Time {
	return m.lastActive.Load().(time.Time)
}

// Reset 重置计量器
func (m *Meter) Reset() {
	atomic.StoreUint64(&m.total, 0)
	atomic.StoreUint64(&m.tickCount, 0)

	m.rateMu.Lock()
	m.rate = 0
	m.rateMu.Unlock()

	m.tickMu.Lock()
	m.lastTick = time.Now()
	m.tickMu.Unlock()

	m.lastActive.Store(time.Now())
}

// MeterSnapshot 计量器快照
type MeterSnapshot struct {
	// Total 累计字节数
	Total uint64

	// Rate 速率 (bytes/sec)
	Rate float64
}

// ============================================================================
//                              计量器注册表
// ============================================================================

// MeterRegistry 计量器注册表
//
// 管理动态创建的计量器集合
type MeterRegistry struct {
	meters sync.Map // map[string]*Meter
}

// Get 获取或创建计量器
func (r *MeterRegistry) Get(key string) *Meter {
	if m, ok := r.meters.Load(key); ok {
		return m.(*Meter)
	}

	// 创建新计量器
	newMeter := NewMeter()
	actual, loaded := r.meters.LoadOrStore(key, newMeter)
	if loaded {
		return actual.(*Meter)
	}
	return newMeter
}

// Exists 检查计量器是否存在
func (r *MeterRegistry) Exists(key string) bool {
	_, ok := r.meters.Load(key)
	return ok
}

// Load 加载已存在的计量器，不创建新的
func (r *MeterRegistry) Load(key string) (*Meter, bool) {
	m, ok := r.meters.Load(key)
	if !ok {
		return nil, false
	}
	return m.(*Meter), true
}

// ForEach 遍历所有计量器
func (r *MeterRegistry) ForEach(fn func(key string, meter *Meter)) {
	r.meters.Range(func(k, v interface{}) bool {
		fn(k.(string), v.(*Meter))
		return true
	})
}

// Clear 清除所有计量器
func (r *MeterRegistry) Clear() {
	r.meters.Range(func(k, _ interface{}) bool {
		r.meters.Delete(k)
		return true
	})
}

// TrimIdle 清理空闲计量器
func (r *MeterRegistry) TrimIdle(since time.Time) {
	r.meters.Range(func(k, v interface{}) bool {
		meter := v.(*Meter)
		if meter.LastActive().Before(since) {
			r.meters.Delete(k)
		}
		return true
	})
}

// Count 返回计量器数量
func (r *MeterRegistry) Count() int {
	count := 0
	r.meters.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// ============================================================================
//                              辅助函数
// ============================================================================

// FormatBytes 格式化字节数为人类可读格式
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return formatValue(float64(bytes), "B")
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return formatValue(float64(bytes)/float64(div), "KMGTPE"[exp:exp+1]+"B")
}

// FormatRate 格式化速率为人类可读格式
func FormatRate(bytesPerSec float64) string {
	const unit = 1024
	if bytesPerSec < unit {
		return formatValue(bytesPerSec, "B/s")
	}
	div, exp := float64(unit), 0
	for n := bytesPerSec / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return formatValue(bytesPerSec/div, "KMGTPE"[exp:exp+1]+"B/s")
}

func formatValue(val float64, suffix string) string {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "0 " + suffix
	}
	if val < 10 {
		return formatFloat(val, 2) + " " + suffix
	}
	if val < 100 {
		return formatFloat(val, 1) + " " + suffix
	}
	return formatFloat(val, 0) + " " + suffix
}

func formatFloat(val float64, precision int) string {
	switch precision {
	case 0:
		return formatInt(int64(val))
	case 1:
		return formatInt(int64(val)) + "." + formatInt(int64((val-float64(int64(val)))*10))
	case 2:
		return formatInt(int64(val)) + "." + formatInt(int64((val-float64(int64(val)))*100))
	default:
		return formatInt(int64(val))
	}
}

func formatInt(val int64) string {
	if val < 0 {
		return "-" + formatInt(-val)
	}
	if val < 10 {
		return string(rune('0' + val))
	}
	return formatInt(val/10) + string(rune('0'+val%10))
}
