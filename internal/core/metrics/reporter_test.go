package metrics

import (
	"testing"
)

// ============================================================================
// Reporter 接口测试
// ============================================================================

// TestReporter_Stats 测试 Reporter 统计功能
func TestReporter_Stats(t *testing.T) {
	var reporter Reporter = NewBandwidthCounter()

	// 记录消息
	reporter.LogSentMessage(100)
	reporter.LogRecvMessage(200)

	// 获取统计
	stats := reporter.GetBandwidthTotals()

	if stats.TotalOut != 100 {
		t.Errorf("TotalOut = %d, want 100", stats.TotalOut)
	}
	if stats.TotalIn != 200 {
		t.Errorf("TotalIn = %d, want 200", stats.TotalIn)
	}
}

// TestReporter_GetBandwidthByPeer 测试获取所有节点带宽
func TestReporter_GetBandwidthByPeer(t *testing.T) {
	var reporter Reporter = NewBandwidthCounter()

	peer1 := testPeerID("peer1")
	peer2 := testPeerID("peer2")
	proto1 := testProtocolID("/test/1.0.0")

	// 记录不同节点的消息
	reporter.LogSentMessageStream(100, proto1, peer1)
	reporter.LogSentMessageStream(200, proto1, peer2)

	// 获取所有节点带宽
	byPeer := reporter.GetBandwidthByPeer()

	if len(byPeer) != 2 {
		t.Errorf("GetBandwidthByPeer() returned %d peers, want 2", len(byPeer))
	}

	// 验证节点1
	if stats, ok := byPeer[peer1]; ok {
		if stats.TotalOut != 100 {
			t.Errorf("Peer1 TotalOut = %d, want 100", stats.TotalOut)
		}
	} else {
		t.Error("Peer1 not found in results")
	}

	// 验证节点2
	if stats, ok := byPeer[peer2]; ok {
		if stats.TotalOut != 200 {
			t.Errorf("Peer2 TotalOut = %d, want 200", stats.TotalOut)
		}
	} else {
		t.Error("Peer2 not found in results")
	}
}

// TestReporter_GetBandwidthByProtocol 测试获取所有协议带宽
func TestReporter_GetBandwidthByProtocol(t *testing.T) {
	var reporter Reporter = NewBandwidthCounter()

	peer1 := testPeerID("peer1")
	proto1 := testProtocolID("/test/1.0.0")
	proto2 := testProtocolID("/test/2.0.0")

	// 记录不同协议的消息
	reporter.LogSentMessageStream(100, proto1, peer1)
	reporter.LogSentMessageStream(200, proto2, peer1)

	// 获取所有协议带宽
	byProtocol := reporter.GetBandwidthByProtocol()

	if len(byProtocol) != 2 {
		t.Errorf("GetBandwidthByProtocol() returned %d protocols, want 2", len(byProtocol))
	}

	// 验证协议1
	if stats, ok := byProtocol[proto1]; ok {
		if stats.TotalOut != 100 {
			t.Errorf("Proto1 TotalOut = %d, want 100", stats.TotalOut)
		}
	} else {
		t.Error("Proto1 not found in results")
	}

	// 验证协议2
	if stats, ok := byProtocol[proto2]; ok {
		if stats.TotalOut != 200 {
			t.Errorf("Proto2 TotalOut = %d, want 200", stats.TotalOut)
		}
	} else {
		t.Error("Proto2 not found in results")
	}
}

// TestReporter_MixedOperations 测试混合操作
func TestReporter_MixedOperations(t *testing.T) {
	var reporter Reporter = NewBandwidthCounter()

	peer1 := testPeerID("peer1")
	peer2 := testPeerID("peer2")
	proto1 := testProtocolID("/test/1.0.0")
	proto2 := testProtocolID("/test/2.0.0")

	// 混合操作
	reporter.LogSentMessage(1000)              // 全局发送
	reporter.LogRecvMessage(2000)              // 全局接收
	reporter.LogSentMessageStream(100, proto1, peer1)  // 流发送
	reporter.LogRecvMessageStream(200, proto1, peer1)  // 流接收
	reporter.LogSentMessageStream(300, proto2, peer2)
	reporter.LogRecvMessageStream(400, proto2, peer2)

	// 验证全局统计（不包含流消息）
	totals := reporter.GetBandwidthTotals()
	if totals.TotalOut != 1000 {
		t.Errorf("Global TotalOut = %d, want 1000", totals.TotalOut)
	}
	if totals.TotalIn != 2000 {
		t.Errorf("Global TotalIn = %d, want 2000", totals.TotalIn)
	}

	// 验证协议统计
	proto1Stats := reporter.GetBandwidthForProtocol(proto1)
	if proto1Stats.TotalOut != 100 {
		t.Errorf("Proto1 TotalOut = %d, want 100", proto1Stats.TotalOut)
	}

	// 验证节点统计
	peer1Stats := reporter.GetBandwidthForPeer(peer1)
	if peer1Stats.TotalOut != 100 {
		t.Errorf("Peer1 TotalOut = %d, want 100", peer1Stats.TotalOut)
	}
}
