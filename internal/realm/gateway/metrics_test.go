package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
//                              Metrics 测试
// ============================================================================

// TestMetrics_RecordRelay 测试记录中继
func TestMetrics_RecordRelay(t *testing.T) {
	metrics := NewMetrics()

	metrics.RecordRelay()
	metrics.RecordRelay()

	m := metrics.GetMetrics()
	assert.Equal(t, int64(2), m.RelayCount)
}

// TestMetrics_RecordSuccess 测试记录成功
func TestMetrics_RecordSuccess(t *testing.T) {
	metrics := NewMetrics()

	metrics.RecordSuccess()
	metrics.RecordSuccess()
	metrics.RecordSuccess()

	m := metrics.GetMetrics()
	assert.Equal(t, int64(3), m.RelaySuccess)
}

// TestMetrics_RecordFailure 测试记录失败
func TestMetrics_RecordFailure(t *testing.T) {
	metrics := NewMetrics()

	metrics.RecordFailure()

	m := metrics.GetMetrics()
	assert.Equal(t, int64(1), m.RelayFailed)
}

// TestMetrics_RecordBytes 测试记录字节传输
func TestMetrics_RecordBytes(t *testing.T) {
	metrics := NewMetrics()

	metrics.RecordBytes(1024, 2048)
	metrics.RecordBytes(512, 256)

	m := metrics.GetMetrics()
	assert.Equal(t, int64(1024+512+2048+256), m.BytesTransferred)
}

// TestMetrics_SetActiveConnections 测试设置活跃连接数
func TestMetrics_SetActiveConnections(t *testing.T) {
	metrics := NewMetrics()

	metrics.SetActiveConnections(10)

	m := metrics.GetMetrics()
	assert.Equal(t, 10, m.ActiveConnections)
}

// TestMetrics_RecordLatency 测试记录延迟
func TestMetrics_RecordLatency(t *testing.T) {
	metrics := NewMetrics()

	metrics.RecordLatency(100 * time.Millisecond)
	metrics.RecordLatency(200 * time.Millisecond)

	m := metrics.GetMetrics()
	assert.Greater(t, m.AvgLatency, time.Duration(0))
	assert.LessOrEqual(t, m.AvgLatency, 200*time.Millisecond)
}

// TestMetrics_Reset 测试重置指标
func TestMetrics_Reset(t *testing.T) {
	metrics := NewMetrics()

	// 记录一些数据
	metrics.RecordRelay()
	metrics.RecordSuccess()
	metrics.RecordBytes(1024, 2048)
	metrics.SetActiveConnections(5)

	// 重置
	metrics.Reset()

	// 验证全部清零
	m := metrics.GetMetrics()
	assert.Equal(t, int64(0), m.RelayCount)
	assert.Equal(t, int64(0), m.RelaySuccess)
	assert.Equal(t, int64(0), m.BytesTransferred)
	assert.Equal(t, 0, m.ActiveConnections)
}

// TestMetrics_GetMetrics 测试获取完整指标
func TestMetrics_GetMetrics(t *testing.T) {
	metrics := NewMetrics()

	// 记录各种数据
	metrics.RecordRelay()
	metrics.RecordRelay()
	metrics.RecordSuccess()
	metrics.RecordFailure()
	metrics.RecordBytes(1024, 512)
	metrics.SetActiveConnections(3)
	metrics.RecordLatency(50 * time.Millisecond)

	// 获取指标
	m := metrics.GetMetrics()

	assert.Equal(t, int64(2), m.RelayCount)
	assert.Equal(t, int64(1), m.RelaySuccess)
	assert.Equal(t, int64(1), m.RelayFailed)
	assert.Equal(t, int64(1024+512), m.BytesTransferred)
	assert.Equal(t, 3, m.ActiveConnections)
	assert.Greater(t, m.AvgLatency, time.Duration(0))
}
