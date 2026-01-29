// Package pathhealth 路径健康管理
package pathhealth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// TestManager_NewManager 测试创建管理器
func TestManager_NewManager(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}
}

// TestManager_StartStop 测试启动和停止
func TestManager_StartStop(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestManager_ObservePeerAddrs 测试观察 Peer 地址
func TestManager_ObservePeerAddrs(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"
	addrs := []string{
		"/ip4/192.168.1.1/udp/4001/quic-v1",
		"/ip4/10.0.0.1/tcp/4001",
	}

	manager.ObservePeerAddrs(peerID, addrs)

	// 验证路径已创建
	paths := manager.GetPeerPaths(peerID)
	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(paths))
	}
}

// TestManager_ReportProbe 测试报告探测结果
func TestManager_ReportProbe(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"
	addr := "/ip4/192.168.1.1/udp/4001/quic-v1"

	// 报告成功探测
	manager.ReportProbe(peerID, addr, 50*time.Millisecond, nil)

	stats := manager.GetPathStats(peerID, addr)
	if stats == nil {
		t.Fatal("GetPathStats returned nil")
	}

	if stats.SuccessCount != 1 {
		t.Errorf("expected SuccessCount 1, got %d", stats.SuccessCount)
	}

	if stats.State != interfaces.PathStateHealthy {
		t.Errorf("expected PathStateHealthy, got %v", stats.State)
	}

	if stats.EWMARTT != 50*time.Millisecond {
		t.Errorf("expected EWMARTT 50ms, got %v", stats.EWMARTT)
	}
}

// TestManager_ReportProbeFailure 测试报告探测失败
func TestManager_ReportProbeFailure(t *testing.T) {
	config := DefaultConfig()
	config.DeadFailureThreshold = 2
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"
	addr := "/ip4/192.168.1.1/udp/4001/quic-v1"

	// 先报告成功，建立路径
	manager.ReportProbe(peerID, addr, 50*time.Millisecond, nil)

	// 报告失败
	err := errors.New("connection timeout")
	manager.ReportProbe(peerID, addr, 0, err)

	stats := manager.GetPathStats(peerID, addr)
	if stats.FailureCount != 1 {
		t.Errorf("expected FailureCount 1, got %d", stats.FailureCount)
	}

	if stats.ConsecutiveFailures != 1 {
		t.Errorf("expected ConsecutiveFailures 1, got %d", stats.ConsecutiveFailures)
	}

	// 再次失败应该变为 Dead
	manager.ReportProbe(peerID, addr, 0, err)

	stats = manager.GetPathStats(peerID, addr)
	if stats.State != interfaces.PathStateDead {
		t.Errorf("expected PathStateDead, got %v", stats.State)
	}
}

// TestManager_EWMA 测试 EWMA 计算
func TestManager_EWMA(t *testing.T) {
	config := DefaultConfig()
	config.EWMAAlpha = 0.5 // 使用 0.5 便于计算
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"
	addr := "/ip4/192.168.1.1/udp/4001/quic-v1"

	// 第一次：100ms
	manager.ReportProbe(peerID, addr, 100*time.Millisecond, nil)
	stats := manager.GetPathStats(peerID, addr)
	if stats.EWMARTT != 100*time.Millisecond {
		t.Errorf("expected EWMARTT 100ms, got %v", stats.EWMARTT)
	}

	// 第二次：200ms, EWMA = 0.5*100 + 0.5*200 = 150ms
	manager.ReportProbe(peerID, addr, 200*time.Millisecond, nil)
	stats = manager.GetPathStats(peerID, addr)
	if stats.EWMARTT != 150*time.Millisecond {
		t.Errorf("expected EWMARTT 150ms, got %v", stats.EWMARTT)
	}
}

// TestManager_RankAddrs 测试地址排序
func TestManager_RankAddrs(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"

	// 创建路径并报告不同的 RTT
	addr1 := "/ip4/192.168.1.1/udp/4001/quic-v1"
	addr2 := "/ip4/192.168.1.2/udp/4001/quic-v1"
	addr3 := "/ip4/192.168.1.3/udp/4001/quic-v1"

	manager.ReportProbe(peerID, addr1, 100*time.Millisecond, nil)
	manager.ReportProbe(peerID, addr2, 50*time.Millisecond, nil)
	manager.ReportProbe(peerID, addr3, 200*time.Millisecond, nil)

	// 排序
	addrs := []string{addr1, addr2, addr3}
	ranked := manager.RankAddrs(peerID, addrs)

	// addr2 应该排第一（RTT 最低）
	if ranked[0] != addr2 {
		t.Errorf("expected addr2 first, got %s", ranked[0])
	}

	// addr1 应该排第二
	if ranked[1] != addr1 {
		t.Errorf("expected addr1 second, got %s", ranked[1])
	}

	// addr3 应该排第三
	if ranked[2] != addr3 {
		t.Errorf("expected addr3 third, got %s", ranked[2])
	}
}

// TestManager_GetBestPath 测试获取最佳路径
func TestManager_GetBestPath(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"

	addr1 := "/ip4/192.168.1.1/udp/4001/quic-v1"
	addr2 := "/ip4/192.168.1.2/udp/4001/quic-v1"

	manager.ReportProbe(peerID, addr1, 100*time.Millisecond, nil)
	manager.ReportProbe(peerID, addr2, 50*time.Millisecond, nil)

	best := manager.GetBestPath(peerID)
	if best == nil {
		t.Fatal("GetBestPath returned nil")
	}

	// addr2 应该是最佳（RTT 最低）
	expectedPathID := GeneratePathID(addr2, interfaces.PathTypeDirect)
	if best.PathID != expectedPathID {
		t.Errorf("expected best path to be %s, got %s", expectedPathID, best.PathID)
	}
}

// TestManager_ShouldSwitch_CurrentDead 测试当前路径死亡时切换
func TestManager_ShouldSwitch_CurrentDead(t *testing.T) {
	config := DefaultConfig()
	config.DeadFailureThreshold = 1
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"

	addr1 := "/ip4/192.168.1.1/udp/4001/quic-v1"
	addr2 := "/ip4/192.168.1.2/udp/4001/quic-v1"

	// addr1 失败，变为 Dead
	manager.ReportProbe(peerID, addr1, 0, errors.New("timeout"))

	// addr2 健康
	manager.ReportProbe(peerID, addr2, 50*time.Millisecond, nil)

	currentPath := GeneratePathID(addr1, interfaces.PathTypeDirect)
	decision := manager.ShouldSwitch(peerID, currentPath)

	if !decision.ShouldSwitch {
		t.Error("expected ShouldSwitch to be true")
	}

	if decision.Reason != interfaces.SwitchReasonCurrentDead {
		t.Errorf("expected reason CurrentDead, got %v", decision.Reason)
	}
}

// TestManager_ShouldSwitch_BetterPath 测试发现更好路径时切换
func TestManager_ShouldSwitch_BetterPath(t *testing.T) {
	config := DefaultConfig()
	config.SwitchHysteresis = 0.3   // 30% 改善才切换
	config.StabilityWindow = 0      // 禁用稳定性检查
	config.DirectPathBonus = 1.0    // 禁用直连加成，便于测试
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"

	addr1 := "/ip4/192.168.1.1/udp/4001/quic-v1"
	addr2 := "/ip4/192.168.1.2/udp/4001/quic-v1"

	// addr1: 200ms
	manager.ReportProbe(peerID, addr1, 200*time.Millisecond, nil)

	// addr2: 50ms (改善 75%，超过 30% 阈值)
	manager.ReportProbe(peerID, addr2, 50*time.Millisecond, nil)

	// 调试：打印评分
	stats1 := manager.GetPathStats(peerID, addr1)
	stats2 := manager.GetPathStats(peerID, addr2)
	t.Logf("addr1 score: %f, addr2 score: %f", stats1.Score, stats2.Score)
	t.Logf("improvement: %f", (stats1.Score-stats2.Score)/stats1.Score)

	currentPath := GeneratePathID(addr1, interfaces.PathTypeDirect)
	decision := manager.ShouldSwitch(peerID, currentPath)

	t.Logf("decision: ShouldSwitch=%v, Reason=%v, CurrentScore=%f, TargetScore=%f",
		decision.ShouldSwitch, decision.Reason, decision.CurrentScore, decision.TargetScore)

	if !decision.ShouldSwitch {
		t.Error("expected ShouldSwitch to be true")
	}

	if decision.Reason != interfaces.SwitchReasonBetterPath {
		t.Errorf("expected reason BetterPath, got %v", decision.Reason)
	}
}

// TestManager_ShouldSwitch_Hysteresis 测试滞后阈值
func TestManager_ShouldSwitch_Hysteresis(t *testing.T) {
	config := DefaultConfig()
	config.SwitchHysteresis = 0.5   // 50% 改善才切换
	config.StabilityWindow = 0      // 禁用稳定性检查
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"

	addr1 := "/ip4/192.168.1.1/udp/4001/quic-v1"
	addr2 := "/ip4/192.168.1.2/udp/4001/quic-v1"

	// addr1: 100ms
	manager.ReportProbe(peerID, addr1, 100*time.Millisecond, nil)

	// addr2: 80ms (改善 20%，未超过 50% 阈值)
	manager.ReportProbe(peerID, addr2, 80*time.Millisecond, nil)

	currentPath := GeneratePathID(addr1, interfaces.PathTypeDirect)
	decision := manager.ShouldSwitch(peerID, currentPath)

	if decision.ShouldSwitch {
		t.Error("expected ShouldSwitch to be false due to hysteresis")
	}
}

// TestManager_PathTypes 测试路径类型检测
func TestManager_PathTypes(t *testing.T) {
	tests := []struct {
		addr     string
		expected interfaces.PathType
	}{
		{"/ip4/192.168.1.1/udp/4001/quic-v1", interfaces.PathTypeDirect},
		{"/ip4/10.0.0.1/tcp/4001", interfaces.PathTypeDirect},
		{"/p2p-circuit/p2p/QmXXX", interfaces.PathTypeRelay},
		{"/relay/1.2.3.4/4001", interfaces.PathTypeRelay},
	}

	for _, tt := range tests {
		pathType := DetectPathType(tt.addr)
		if pathType != tt.expected {
			t.Errorf("DetectPathType(%s) = %v, expected %v", tt.addr, pathType, tt.expected)
		}
	}
}

// TestManager_DirectPathBonus 测试直连路径加成
func TestManager_DirectPathBonus(t *testing.T) {
	config := DefaultConfig()
	config.DirectPathBonus = 0.5 // 直连路径评分乘以 0.5
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"

	// 直连路径
	directAddr := "/ip4/192.168.1.1/udp/4001/quic-v1"
	// 中继路径
	relayAddr := "/p2p-circuit/relay/1.2.3.4"

	// 相同 RTT
	manager.ReportProbe(peerID, directAddr, 100*time.Millisecond, nil)
	manager.ReportProbe(peerID, relayAddr, 100*time.Millisecond, nil)

	directStats := manager.GetPathStats(peerID, directAddr)
	relayStats := manager.GetPathStats(peerID, relayAddr)

	// 直连路径评分应该更低（更好）
	if directStats.Score >= relayStats.Score {
		t.Errorf("direct path score (%f) should be lower than relay (%f)", 
			directStats.Score, relayStats.Score)
	}
}

// TestManager_RemovePeer 测试移除 Peer
func TestManager_RemovePeer(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"
	addr := "/ip4/192.168.1.1/udp/4001/quic-v1"

	manager.ReportProbe(peerID, addr, 50*time.Millisecond, nil)

	// 验证路径存在
	if manager.GetPathStats(peerID, addr) == nil {
		t.Fatal("path should exist")
	}

	// 移除 Peer
	manager.RemovePeer(peerID)

	// 验证路径已移除
	if manager.GetPathStats(peerID, addr) != nil {
		t.Error("path should be removed")
	}
}

// TestManager_Reset 测试重置
func TestManager_Reset(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// 添加一些数据
	manager.ReportProbe("peer-1", "/ip4/1.1.1.1/udp/4001", 50*time.Millisecond, nil)
	manager.ReportProbe("peer-2", "/ip4/2.2.2.2/udp/4001", 50*time.Millisecond, nil)

	// 重置
	manager.Reset()

	// 验证数据已清空
	if manager.GetPeerPaths("peer-1") != nil {
		t.Error("peer-1 paths should be cleared")
	}
	if manager.GetPeerPaths("peer-2") != nil {
		t.Error("peer-2 paths should be cleared")
	}
}

// TestManager_OnNetworkChange 测试网络变更通知
func TestManager_OnNetworkChange(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	peerID := "test-peer-1"
	addr := "/ip4/192.168.1.1/udp/4001/quic-v1"

	// 报告失败使路径变为可疑
	manager.ReportProbe(peerID, addr, 50*time.Millisecond, nil)
	manager.ReportProbe(peerID, addr, 0, errors.New("timeout"))

	// 网络变更
	manager.OnNetworkChange(ctx, "wifi_change")

	// 验证连续失败计数被重置
	stats := manager.GetPathStats(peerID, addr)
	if stats.ConsecutiveFailures != 0 {
		t.Errorf("expected ConsecutiveFailures to be reset, got %d", stats.ConsecutiveFailures)
	}
}
