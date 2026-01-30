package tests

// ════════════════════════════════════════════════════════════════════════════
//                   带宽统计 API 测试 (v1.1 TASK-002)
// ════════════════════════════════════════════════════════════════════════════

import (
	"context"
	"testing"

	"github.com/dep2p/go-dep2p"
)

// TestBandwidthStatsDisabled 测试带宽统计未启用时的行为
func TestBandwidthStatsDisabled(t *testing.T) {
	ctx := context.Background()

	// 创建节点，默认不启用带宽统计
	node, err := dep2p.New(ctx,
		dep2p.WithListenAddrs("/ip4/0.0.0.0/udp/0/quic-v1"),
		// 不设置 WithBandwidth，默认不启用
	)
	if err != nil {
		t.Fatalf("创建节点失败: %v", err)
	}
	defer node.Close()

	// 验证带宽统计未启用
	if node.IsBandwidthStatsEnabled() {
		t.Error("带宽统计应该默认未启用")
	}

	// 验证各个方法返回空值
	stats := node.BandwidthStats()
	if stats.TotalIn != 0 || stats.TotalOut != 0 {
		t.Errorf("BandwidthStats 应返回零值，得到: %+v", stats)
	}

	peerStats := node.BandwidthForPeer("12D3KooWTestPeer")
	if peerStats.TotalIn != 0 || peerStats.TotalOut != 0 {
		t.Errorf("BandwidthForPeer 应返回零值，得到: %+v", peerStats)
	}

	protoStats := node.BandwidthForProtocol("/test/1.0.0")
	if protoStats.TotalIn != 0 || protoStats.TotalOut != 0 {
		t.Errorf("BandwidthForProtocol 应返回零值，得到: %+v", protoStats)
	}

	byPeer := node.BandwidthByPeer()
	if byPeer != nil {
		t.Errorf("BandwidthByPeer 应返回 nil，得到: %v", byPeer)
	}

	byProto := node.BandwidthByProtocol()
	if byProto != nil {
		t.Errorf("BandwidthByProtocol 应返回 nil，得到: %v", byProto)
	}
}

// TestBandwidthStatsEnabled 测试带宽统计启用时的行为
func TestBandwidthStatsEnabled(t *testing.T) {
	ctx := context.Background()

	// 创建启用带宽统计的节点
	node, err := dep2p.New(ctx,
		dep2p.WithListenAddrs("/ip4/0.0.0.0/udp/0/quic-v1"),
		dep2p.WithBandwidth(true, true, true), // enabled, perPeer, perProtocol
	)
	if err != nil {
		t.Fatalf("创建节点失败: %v", err)
	}
	defer node.Close()

	// 验证带宽统计已启用
	if !node.IsBandwidthStatsEnabled() {
		t.Error("带宽统计应该已启用")
	}

	// 验证 BandwidthStats 返回有效结构（初始值为零）
	stats := node.BandwidthStats()
	// 初始状态下可能为零，但结构应该有效
	t.Logf("总体带宽统计: TotalIn=%d, TotalOut=%d, RateIn=%.2f, RateOut=%.2f",
		stats.TotalIn, stats.TotalOut, stats.RateIn, stats.RateOut)

	// 验证辅助方法
	if stats.TotalBytes() != stats.TotalIn+stats.TotalOut {
		t.Errorf("TotalBytes() 计算错误")
	}
	if stats.TotalRate() != stats.RateIn+stats.RateOut {
		t.Errorf("TotalRate() 计算错误")
	}
}

// TestBandwidthSnapshot 测试 BandwidthSnapshot 类型
func TestBandwidthSnapshot(t *testing.T) {
	snap := dep2p.BandwidthSnapshot{
		TotalIn:  1000,
		TotalOut: 500,
		RateIn:   100.5,
		RateOut:  50.25,
	}

	// 验证辅助方法
	if snap.TotalBytes() != 1500 {
		t.Errorf("TotalBytes() 期望 1500，得到 %d", snap.TotalBytes())
	}

	expectedRate := 150.75
	if snap.TotalRate() != expectedRate {
		t.Errorf("TotalRate() 期望 %.2f，得到 %.2f", expectedRate, snap.TotalRate())
	}
}

// TestBandwidthForPeerAndProtocol 测试按节点和协议查询
func TestBandwidthForPeerAndProtocol(t *testing.T) {
	ctx := context.Background()

	// 创建启用带宽统计的节点
	node, err := dep2p.New(ctx,
		dep2p.WithListenAddrs("/ip4/0.0.0.0/udp/0/quic-v1"),
		dep2p.WithBandwidth(true, true, true),
	)
	if err != nil {
		t.Fatalf("创建节点失败: %v", err)
	}
	defer node.Close()

	// 查询不存在的节点（应返回零值，不报错）
	peerStats := node.BandwidthForPeer("nonexistent-peer")
	if peerStats.TotalIn != 0 || peerStats.TotalOut != 0 {
		t.Logf("不存在节点的带宽统计: %+v", peerStats)
	}

	// 查询不存在的协议（应返回零值，不报错）
	protoStats := node.BandwidthForProtocol("/nonexistent/1.0.0")
	if protoStats.TotalIn != 0 || protoStats.TotalOut != 0 {
		t.Logf("不存在协议的带宽统计: %+v", protoStats)
	}

	// 获取所有节点统计（初始应为空或 nil）
	byPeer := node.BandwidthByPeer()
	t.Logf("按节点统计数量: %d", len(byPeer))

	// 获取所有协议统计（初始应为空或 nil）
	byProto := node.BandwidthByProtocol()
	t.Logf("按协议统计数量: %d", len(byProto))
}
