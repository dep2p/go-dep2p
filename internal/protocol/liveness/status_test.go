// Package liveness 实现存活检测服务
package liveness

import (
	"testing"
	"time"
)

func TestPeerStatus_RecordSuccess(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 记录成功
	rtt := 50 * time.Millisecond
	ps.recordSuccess(rtt)

	status := ps.getStatus()
	if !status.Alive {
		t.Error("Status should be alive after success")
	}
	if status.LastRTT != rtt {
		t.Errorf("LastRTT = %v, want %v", status.LastRTT, rtt)
	}
	if status.FailCount != 0 {
		t.Errorf("FailCount = %d, want 0", status.FailCount)
	}
}

func TestPeerStatus_RecordFailure(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 记录失败
	ps.recordFailure()

	status := ps.getStatus()
	if status.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", status.FailCount)
	}

	// 多次失败
	ps.recordFailure()
	ps.recordFailure()

	status = ps.getStatus()
	if status.FailCount != 3 {
		t.Errorf("FailCount = %d, want 3", status.FailCount)
	}
}

func TestPeerStatus_AverageRTT(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 记录多个RTT样本
	samples := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
	}

	for _, rtt := range samples {
		ps.recordSuccess(rtt)
	}

	status := ps.getStatus()
	
	// 平均RTT应该约为 20ms
	expectedAvg := 20 * time.Millisecond
	if status.AvgRTT < 15*time.Millisecond || status.AvgRTT > 25*time.Millisecond {
		t.Errorf("AvgRTT = %v, want ~%v", status.AvgRTT, expectedAvg)
	}
}

func TestPeerStatus_SuccessResetsFailCount(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 先记录失败
	ps.recordFailure()
	ps.recordFailure()

	status := ps.getStatus()
	if status.FailCount != 2 {
		t.Errorf("FailCount = %d, want 2", status.FailCount)
	}

	// 记录成功应该重置失败计数
	ps.recordSuccess(10 * time.Millisecond)

	status = ps.getStatus()
	if status.FailCount != 0 {
		t.Errorf("FailCount = %d, want 0 after success", status.FailCount)
	}
	if !status.Alive {
		t.Error("Status should be alive after success")
	}
}

func TestPeerStatus_FailureThreshold(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 先让节点存活
	ps.recordSuccess(10 * time.Millisecond)

	status := ps.getStatus()
	if !status.Alive {
		t.Error("Status should be alive initially")
	}

	// 记录3次失败（默认阈值）
	for i := 0; i < 3; i++ {
		ps.recordFailure()
	}

	status = ps.getStatus()
	if status.Alive {
		t.Error("Status should be dead after reaching failure threshold")
	}
}

// ============================================================================
//                    增强统计功能测试 (Phase 4)
// ============================================================================

func TestPeerStatus_MinMaxRTT(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 记录不同的 RTT 值
	samples := []time.Duration{
		50 * time.Millisecond,  // 中等
		10 * time.Millisecond,  // 最小
		100 * time.Millisecond, // 最大
		30 * time.Millisecond,  // 中等
	}

	for _, rtt := range samples {
		ps.recordSuccess(rtt)
	}

	status := ps.getStatus()

	// 验证 MinRTT
	expectedMin := 10 * time.Millisecond
	if status.MinRTT != expectedMin {
		t.Errorf("MinRTT = %v, want %v", status.MinRTT, expectedMin)
	}

	// 验证 MaxRTT
	expectedMax := 100 * time.Millisecond
	if status.MaxRTT != expectedMax {
		t.Errorf("MaxRTT = %v, want %v", status.MaxRTT, expectedMax)
	}
}

func TestPeerStatus_TotalPingsAndSuccessCount(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 记录 5 次成功
	for i := 0; i < 5; i++ {
		ps.recordSuccess(time.Duration(10+i*5) * time.Millisecond)
	}

	// 记录 2 次失败
	ps.recordFailure()
	ps.recordFailure()

	status := ps.getStatus()

	// 验证 TotalPings（7 次）
	if status.TotalPings != 7 {
		t.Errorf("TotalPings = %d, want 7", status.TotalPings)
	}

	// 验证 SuccessCount（5 次）
	if status.SuccessCount != 5 {
		t.Errorf("SuccessCount = %d, want 5", status.SuccessCount)
	}
}

func TestPeerStatus_SuccessRate(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 记录 8 次成功
	for i := 0; i < 8; i++ {
		ps.recordSuccess(10 * time.Millisecond)
	}

	// 记录 2 次失败
	ps.recordFailure()
	ps.recordFailure()

	status := ps.getStatus()

	// 验证 SuccessRate（8/10 = 0.8）
	expectedRate := 0.8
	if status.SuccessRate < 0.79 || status.SuccessRate > 0.81 {
		t.Errorf("SuccessRate = %v, want ~%v", status.SuccessRate, expectedRate)
	}
}

func TestPeerStatus_SuccessRateZeroPings(t *testing.T) {
	ps := newPeerStatus("peer1")

	status := ps.getStatus()

	// 没有任何 Ping 时，成功率应该是 0
	if status.SuccessRate != 0 {
		t.Errorf("SuccessRate = %v, want 0 (no pings yet)", status.SuccessRate)
	}
	if status.TotalPings != 0 {
		t.Errorf("TotalPings = %d, want 0", status.TotalPings)
	}
}

func TestPeerStatus_MinRTTNotOverwritten(t *testing.T) {
	ps := newPeerStatus("peer1")

	// 第一次记录
	ps.recordSuccess(20 * time.Millisecond)
	status := ps.getStatus()
	if status.MinRTT != 20*time.Millisecond {
		t.Errorf("MinRTT after first ping = %v, want 20ms", status.MinRTT)
	}

	// 记录更大的 RTT，MinRTT 不应该改变
	ps.recordSuccess(50 * time.Millisecond)
	status = ps.getStatus()
	if status.MinRTT != 20*time.Millisecond {
		t.Errorf("MinRTT should not increase: got %v, want 20ms", status.MinRTT)
	}

	// 记录更小的 RTT，MinRTT 应该更新
	ps.recordSuccess(5 * time.Millisecond)
	status = ps.getStatus()
	if status.MinRTT != 5*time.Millisecond {
		t.Errorf("MinRTT should decrease: got %v, want 5ms", status.MinRTT)
	}
}
