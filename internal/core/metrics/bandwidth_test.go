package metrics

import (
	"testing"
	"time"
)

// ============================================================================
// 接口契约测试
// ============================================================================

// TestBandwidthCounter_ImplementsInterface 验证 BandwidthCounter 实现 Reporter 接口
func TestBandwidthCounter_ImplementsInterface(t *testing.T) {
	var _ Reporter = (*BandwidthCounter)(nil)
}

// ============================================================================
// 基础功能测试
// ============================================================================

// TestBandwidthCounter_LogSentMessage 测试记录发送消息
func TestBandwidthCounter_LogSentMessage(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 记录发送消息
	bwc.LogSentMessage(1024)
	bwc.LogSentMessage(2048)

	// 获取统计
	stats := bwc.GetBandwidthTotals()

	if stats.TotalOut != 3072 {
		t.Errorf("TotalOut = %d, want 3072", stats.TotalOut)
	}

	if stats.TotalIn != 0 {
		t.Errorf("TotalIn = %d, want 0", stats.TotalIn)
	}
}

// TestBandwidthCounter_LogRecvMessage 测试记录接收消息
func TestBandwidthCounter_LogRecvMessage(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 记录接收消息
	bwc.LogRecvMessage(512)
	bwc.LogRecvMessage(1024)

	// 获取统计
	stats := bwc.GetBandwidthTotals()

	if stats.TotalIn != 1536 {
		t.Errorf("TotalIn = %d, want 1536", stats.TotalIn)
	}

	if stats.TotalOut != 0 {
		t.Errorf("TotalOut = %d, want 0", stats.TotalOut)
	}
}

// TestBandwidthCounter_GetBandwidthTotals 测试获取总带宽统计
func TestBandwidthCounter_GetBandwidthTotals(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 记录消息
	bwc.LogSentMessage(100)
	bwc.LogRecvMessage(200)
	bwc.LogSentMessage(300)
	bwc.LogRecvMessage(400)

	// 获取统计
	stats := bwc.GetBandwidthTotals()

	if stats.TotalOut != 400 {
		t.Errorf("TotalOut = %d, want 400", stats.TotalOut)
	}

	if stats.TotalIn != 600 {
		t.Errorf("TotalIn = %d, want 600", stats.TotalIn)
	}
}

// ============================================================================
// 协议级统计测试
// ============================================================================

// TestBandwidthCounter_LogSentMessageStream 测试记录流发送消息
func TestBandwidthCounter_LogSentMessageStream(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer1 := testPeerID("peer1")
	proto1 := testProtocolID("/test/1.0.0")

	// 记录流消息
	bwc.LogSentMessageStream(1024, proto1, peer1)
	bwc.LogSentMessageStream(2048, proto1, peer1)

	// 获取协议统计
	stats := bwc.GetBandwidthForProtocol(proto1)

	if stats.TotalOut != 3072 {
		t.Errorf("Protocol TotalOut = %d, want 3072", stats.TotalOut)
	}
}

// TestBandwidthCounter_GetBandwidthForProtocol 测试获取协议带宽统计
func TestBandwidthCounter_GetBandwidthForProtocol(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer1 := testPeerID("peer1")
	proto1 := testProtocolID("/test/1.0.0")
	proto2 := testProtocolID("/test/2.0.0")

	// 记录不同协议的消息
	bwc.LogSentMessageStream(100, proto1, peer1)
	bwc.LogRecvMessageStream(200, proto1, peer1)

	bwc.LogSentMessageStream(300, proto2, peer1)
	bwc.LogRecvMessageStream(400, proto2, peer1)

	// 获取协议1统计
	stats1 := bwc.GetBandwidthForProtocol(proto1)
	if stats1.TotalOut != 100 {
		t.Errorf("Proto1 TotalOut = %d, want 100", stats1.TotalOut)
	}
	if stats1.TotalIn != 200 {
		t.Errorf("Proto1 TotalIn = %d, want 200", stats1.TotalIn)
	}

	// 获取协议2统计
	stats2 := bwc.GetBandwidthForProtocol(proto2)
	if stats2.TotalOut != 300 {
		t.Errorf("Proto2 TotalOut = %d, want 300", stats2.TotalOut)
	}
	if stats2.TotalIn != 400 {
		t.Errorf("Proto2 TotalIn = %d, want 400", stats2.TotalIn)
	}
}

// ============================================================================
// 节点级统计测试
// ============================================================================

// TestBandwidthCounter_GetBandwidthForPeer 测试获取节点带宽统计
func TestBandwidthCounter_GetBandwidthForPeer(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer1 := testPeerID("peer1")
	peer2 := testPeerID("peer2")
	proto1 := testProtocolID("/test/1.0.0")

	// 记录不同节点的消息
	bwc.LogSentMessageStream(100, proto1, peer1)
	bwc.LogRecvMessageStream(200, proto1, peer1)

	bwc.LogSentMessageStream(300, proto1, peer2)
	bwc.LogRecvMessageStream(400, proto1, peer2)

	// 获取节点1统计
	stats1 := bwc.GetBandwidthForPeer(peer1)
	if stats1.TotalOut != 100 {
		t.Errorf("Peer1 TotalOut = %d, want 100", stats1.TotalOut)
	}
	if stats1.TotalIn != 200 {
		t.Errorf("Peer1 TotalIn = %d, want 200", stats1.TotalIn)
	}

	// 获取节点2统计
	stats2 := bwc.GetBandwidthForPeer(peer2)
	if stats2.TotalOut != 300 {
		t.Errorf("Peer2 TotalOut = %d, want 300", stats2.TotalOut)
	}
	if stats2.TotalIn != 400 {
		t.Errorf("Peer2 TotalIn = %d, want 400", stats2.TotalIn)
	}
}

// ============================================================================
// 速率计算测试
// ============================================================================

// TestBandwidthCounter_RateCalculation 测试速率计算
func TestBandwidthCounter_RateCalculation(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 记录一些消息
	for i := 0; i < 10; i++ {
		bwc.LogSentMessage(1024)
		bwc.LogRecvMessage(2048)
		time.Sleep(10 * time.Millisecond)
	}

	// 获取统计
	stats := bwc.GetBandwidthTotals()

	// 验证累计值
	if stats.TotalOut != 10240 {
		t.Errorf("TotalOut = %d, want 10240", stats.TotalOut)
	}
	if stats.TotalIn != 20480 {
		t.Errorf("TotalIn = %d, want 20480", stats.TotalIn)
	}

	// 速率应该 >= 0（由 flow.Meter 计算）
	if stats.RateOut < 0 {
		t.Errorf("RateOut = %f, should be >= 0", stats.RateOut)
	}
	if stats.RateIn < 0 {
		t.Errorf("RateIn = %f, should be >= 0", stats.RateIn)
	}
}

// TestBandwidthCounter_Reset 测试重置统计
func TestBandwidthCounter_Reset(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 记录一些消息
	bwc.LogSentMessage(1024)
	bwc.LogRecvMessage(2048)

	// 重置
	bwc.Reset()

	// 验证重置后统计
	stats := bwc.GetBandwidthTotals()
	if stats.TotalOut != 0 {
		t.Errorf("After Reset, TotalOut = %d, want 0", stats.TotalOut)
	}
	if stats.TotalIn != 0 {
		t.Errorf("After Reset, TotalIn = %d, want 0", stats.TotalIn)
	}
}

// TestBandwidthCounter_TrimIdle 测试清理空闲统计
func TestBandwidthCounter_TrimIdle(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer1 := testPeerID("peer1")
	proto1 := testProtocolID("/test/1.0.0")

	// 记录消息
	bwc.LogSentMessageStream(1024, proto1, peer1)

	// 立即清理应该不影响
	bwc.TrimIdle(time.Now().Add(-1 * time.Hour))

	stats := bwc.GetBandwidthForPeer(peer1)
	if stats.TotalOut != 1024 {
		t.Errorf("After TrimIdle, TotalOut = %d, want 1024", stats.TotalOut)
	}

	// 清理未来时间的空闲（可能清除）
	time.Sleep(10 * time.Millisecond)
	bwc.TrimIdle(time.Now().Add(1 * time.Hour))

	// 测试继续工作（不崩溃）
	bwc.LogSentMessageStream(512, proto1, peer1)
}
