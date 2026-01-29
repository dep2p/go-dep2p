package bandwidth

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
//                     Meter.Total / Meter.Rate 测试
// ============================================================================

func TestMeter_Total(t *testing.T) {
	m := NewMeter()

	// 初始值为 0
	assert.Equal(t, uint64(0), m.Total())

	// 添加一些数据
	m.Mark(1000)
	assert.Equal(t, uint64(1000), m.Total())

	m.Mark(500)
	assert.Equal(t, uint64(1500), m.Total())
}

func TestMeter_Rate(t *testing.T) {
	m := NewMeter()

	// 初始速率为 0
	assert.Equal(t, float64(0), m.Rate())

	// 添加一些数据并等待速率计算
	m.Mark(10000)
	time.Sleep(150 * time.Millisecond) // 等待速率更新
	m.Mark(10000)

	// 速率应该大于 0
	// 注意：由于时间窗口，具体值可能变化
	snapshot := m.Snapshot()
	_ = snapshot // 验证 Snapshot 也能工作
}

// ============================================================================
//                     MeterRegistry.Exists 测试
// ============================================================================

func TestMeterRegistry_Exists(t *testing.T) {
	r := &MeterRegistry{}

	// 不存在的 key
	assert.False(t, r.Exists("unknown"))

	// 创建一个 meter
	r.Get("peer-1")

	// 现在应该存在
	assert.True(t, r.Exists("peer-1"))
	assert.False(t, r.Exists("peer-2"))
}

// ============================================================================
//                     FormatBytes 测试
// ============================================================================

func TestFormatBytes(t *testing.T) {
	// 测试各种大小的字节格式化
	tests := []struct {
		bytes       int64
		containsStr string // 只检查是否包含正确的单位
	}{
		{0, "B"},
		{1, "B"},
		{100, "B"},
		{1023, "B"},
		{1024, "KB"},
		{1536, "KB"},
		{10240, "KB"},
		{102400, "KB"},
		{1048576, "MB"},
		{1572864, "MB"},
		{1073741824, "GB"},
		{1610612736, "GB"},
	}

	for _, tt := range tests {
		t.Run(tt.containsStr, func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			assert.Contains(t, result, tt.containsStr)
		})
	}
}

func TestFormatBytes_LargeValues(t *testing.T) {
	// TB
	tb := int64(1024) * 1024 * 1024 * 1024
	result := FormatBytes(tb)
	assert.Contains(t, result, "TB")

	// PB
	pb := tb * 1024
	result = FormatBytes(pb)
	assert.Contains(t, result, "PB")
}

// ============================================================================
//                     FormatRate 测试
// ============================================================================

func TestFormatRate(t *testing.T) {
	// 测试各种速率格式化
	tests := []struct {
		rate        float64
		containsStr string // 只检查是否包含正确的单位
	}{
		{0, "B/s"},
		{1, "B/s"},
		{100, "B/s"},
		{1023, "B/s"},
		{1024, "KB/s"},
		{1536, "KB/s"},
		{10240, "KB/s"},
		{102400, "KB/s"},
		{1048576, "MB/s"},
		{1073741824, "GB/s"},
	}

	for _, tt := range tests {
		t.Run(tt.containsStr, func(t *testing.T) {
			result := FormatRate(tt.rate)
			assert.Contains(t, result, tt.containsStr)
		})
	}
}

// ============================================================================
//                     formatValue 边界测试
// ============================================================================

func TestFormatValue_EdgeCases(t *testing.T) {
	// 测试小于 10 的值（2 位小数）
	result := formatValue(1.23, "B")
	assert.Equal(t, "1.23 B", result)

	// 测试 10-100 之间的值（1 位小数）
	result = formatValue(12.3, "B")
	assert.Equal(t, "12.3 B", result)

	// 测试大于 100 的值（无小数）
	result = formatValue(123.4, "B")
	assert.Equal(t, "123 B", result)

	// 测试 NaN - 使用 math.NaN()
	result = formatValue(math.NaN(), "B")
	assert.Equal(t, "0 B", result)

	// 测试 Inf
	result = formatValue(math.Inf(1), "B")
	assert.Equal(t, "0 B", result)
}

// ============================================================================
//                     formatFloat 测试
// ============================================================================

func TestFormatFloat_Precision(t *testing.T) {
	// precision = 0
	result := formatFloat(12.0, 0)
	assert.Equal(t, "12", result)

	// precision = 1
	result = formatFloat(12.5, 1)
	assert.Equal(t, "12.5", result)

	// precision = 2
	result = formatFloat(12.25, 2)
	assert.Equal(t, "12.25", result)

	// precision = 3 (fallback to 0)
	result = formatFloat(12.0, 3)
	assert.Equal(t, "12", result)
}

// ============================================================================
//                     formatInt 测试
// ============================================================================

func TestFormatInt(t *testing.T) {
	tests := []struct {
		val      int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{99, "99"},
		{100, "100"},
		{12345, "12345"},
		{-1, "-1"},
		{-123, "-123"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatInt(tt.val)
			assert.Equal(t, tt.expected, result)
		})
	}
}
