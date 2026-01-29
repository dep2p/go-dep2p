package msgrate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, 1.01, config.CapacityOverestimation)
	assert.Equal(t, 0.1, config.MeasurementImpact)
	assert.Equal(t, 2*time.Second, config.RTTMinEstimate)
	assert.Equal(t, 20*time.Second, config.RTTMaxEstimate)
}

func TestNewTracker(t *testing.T) {
	config := DefaultConfig()
	tracker := NewTracker(config, nil, 500*time.Millisecond)

	assert.NotNil(t, tracker)
	assert.Equal(t, 500*time.Millisecond, tracker.RTT())
}

func TestTracker_Capacity(t *testing.T) {
	config := DefaultConfig()
	caps := map[uint64]float64{
		1: 100.0, // 100 items/second
	}
	tracker := NewTracker(config, caps, 500*time.Millisecond)

	// 在 1 秒的目标 RTT 内，应该能处理约 100 个项目
	capacity := tracker.Capacity(1, 1*time.Second)
	assert.Greater(t, capacity, 100)

	// 未知类型应该返回最小值
	capacity = tracker.Capacity(999, 1*time.Second)
	assert.Equal(t, 1, capacity)
}

func TestTracker_Update(t *testing.T) {
	config := DefaultConfig()
	tracker := NewTracker(config, nil, 500*time.Millisecond)

	// 更新测量结果：100ms 处理 1000 个项目
	tracker.Update(1, 100*time.Millisecond, 1000)

	// 容量应该增加
	capacity := tracker.Capacity(1, 1*time.Second)
	assert.Greater(t, capacity, 1)

	// RTT 应该更新
	rtt := tracker.RTT()
	assert.NotEqual(t, 500*time.Millisecond, rtt)
}

func TestTracker_Update_ZeroItems(t *testing.T) {
	config := DefaultConfig()
	caps := map[uint64]float64{
		1: 100.0,
	}
	tracker := NewTracker(config, caps, 500*time.Millisecond)

	// 更新为零项目（超时）
	tracker.Update(1, 100*time.Millisecond, 0)

	// 容量应该降至最低
	capacity := tracker.Capacity(1, 1*time.Second)
	assert.Equal(t, 1, capacity)
}

func TestNewTrackers(t *testing.T) {
	config := DefaultConfig()
	trackers := NewTrackers(config)

	assert.NotNil(t, trackers)
}

func TestTrackers_TrackUntrack(t *testing.T) {
	config := DefaultConfig()
	trackers := NewTrackers(config)

	tracker := NewTracker(config, nil, 500*time.Millisecond)

	// 添加追踪
	err := trackers.Track("peer1", tracker)
	require.NoError(t, err)

	// 重复添加应该失败
	err = trackers.Track("peer1", tracker)
	assert.Equal(t, ErrAlreadyTracking, err)

	// 停止追踪
	err = trackers.Untrack("peer1")
	require.NoError(t, err)

	// 重复停止应该失败
	err = trackers.Untrack("peer1")
	assert.Equal(t, ErrNotTracking, err)
}

func TestTrackers_TargetRoundTrip(t *testing.T) {
	config := DefaultConfig()
	trackers := NewTrackers(config)

	// 没有追踪器时，应该返回最大估计
	targetRTT := trackers.TargetRoundTrip()
	assert.LessOrEqual(t, targetRTT, config.RTTMaxEstimate)

	// 添加追踪器
	tracker := NewTracker(config, nil, 500*time.Millisecond)
	err := trackers.Track("peer1", tracker)
	require.NoError(t, err)

	// 目标 RTT 应该更新
	time.Sleep(100 * time.Millisecond)
	newTargetRTT := trackers.TargetRoundTrip()
	assert.Greater(t, newTargetRTT, 0*time.Millisecond)
}

func TestTrackers_TargetTimeout(t *testing.T) {
	config := DefaultConfig()
	trackers := NewTrackers(config)

	timeout := trackers.TargetTimeout()
	assert.Greater(t, timeout, 0*time.Millisecond)
	assert.LessOrEqual(t, timeout, config.TTLLimit)
}

func TestTrackers_MedianRoundTrip(t *testing.T) {
	config := DefaultConfig()
	trackers := NewTrackers(config)

	// 没有追踪器时
	median := trackers.MedianRoundTrip()
	assert.Equal(t, config.RTTMaxEstimate, median)

	// 添加多个追踪器
	for i := 0; i < 5; i++ {
		rtt := time.Duration(100*(i+1)) * time.Millisecond
		tracker := NewTracker(config, nil, rtt)
		err := trackers.Track("peer"+string(rune('0'+i)), tracker)
		require.NoError(t, err)
	}

	median = trackers.MedianRoundTrip()
	// 应该在最小和最大估计之间
	assert.GreaterOrEqual(t, median, config.RTTMinEstimate)
	assert.LessOrEqual(t, median, config.RTTMaxEstimate)
}

func TestTrackers_MeanCapacities(t *testing.T) {
	config := DefaultConfig()
	trackers := NewTrackers(config)

	// 添加追踪器
	caps1 := map[uint64]float64{1: 100.0, 2: 200.0}
	tracker1 := NewTracker(config, caps1, 500*time.Millisecond)
	err := trackers.Track("peer1", tracker1)
	require.NoError(t, err)

	caps2 := map[uint64]float64{1: 200.0, 2: 400.0}
	tracker2 := NewTracker(config, caps2, 500*time.Millisecond)
	err = trackers.Track("peer2", tracker2)
	require.NoError(t, err)

	// 获取平均容量
	mean := trackers.MeanCapacities()
	assert.Equal(t, 150.0, mean[1]) // (100 + 200) / 2
	assert.Equal(t, 300.0, mean[2]) // (200 + 400) / 2
}

func TestTrackers_Capacity(t *testing.T) {
	config := DefaultConfig()
	trackers := NewTrackers(config)

	// 未注册的节点应该返回最小值
	capacity := trackers.Capacity("unknown", 1, 1*time.Second)
	assert.Equal(t, 1, capacity)

	// 注册节点
	caps := map[uint64]float64{1: 100.0}
	tracker := NewTracker(config, caps, 500*time.Millisecond)
	err := trackers.Track("peer1", tracker)
	require.NoError(t, err)

	capacity = trackers.Capacity("peer1", 1, 1*time.Second)
	assert.Greater(t, capacity, 1)
}

func TestTrackers_Update(t *testing.T) {
	config := DefaultConfig()
	trackers := NewTrackers(config)

	tracker := NewTracker(config, nil, 500*time.Millisecond)
	err := trackers.Track("peer1", tracker)
	require.NoError(t, err)

	// 更新测量结果
	trackers.Update("peer1", 1, 100*time.Millisecond, 1000)

	// 容量应该增加
	capacity := trackers.Capacity("peer1", 1, 1*time.Second)
	assert.Greater(t, capacity, 1)
}

func TestTrackers_Stats(t *testing.T) {
	config := DefaultConfig()
	ts := NewTrackers(config).(*trackers)

	tracker := NewTracker(config, nil, 500*time.Millisecond)
	err := ts.Track("peer1", tracker)
	require.NoError(t, err)

	stats := ts.Stats()

	assert.Equal(t, 1, stats.TrackedPeers)
	assert.Greater(t, stats.CurrentRTT, 0*time.Millisecond)
	assert.Greater(t, stats.Confidence, 0.0)
}

func TestRoundCapacity(t *testing.T) {
	// 最小值
	assert.Equal(t, 1, roundCapacity(0.5))
	assert.Equal(t, 1, roundCapacity(0))
	assert.Equal(t, 1, roundCapacity(-1))

	// 正常值
	assert.Equal(t, 2, roundCapacity(1.5))
	assert.Equal(t, 100, roundCapacity(99.1))

	// 大值
	assert.Equal(t, 1000000, roundCapacity(999999.5))
}
