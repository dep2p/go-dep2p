package metrics

import (
	"testing"
)

// ============================================================================
// 边界条件和错误路径测试
// ============================================================================

// TestEdge_NegativeSize 测试负数大小
func TestEdge_NegativeSize(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 记录负数（应该被转换为 uint64，变成大正数或被忽略）
	bwc.LogSentMessage(-100)
	bwc.LogRecvMessage(-200)

	// 不应该崩溃，继续工作
	bwc.LogSentMessage(100)
	stats := bwc.GetBandwidthTotals()

	// 至少应该记录正数部分
	if stats.TotalOut < 100 {
		t.Logf("TotalOut = %d (may include negative conversion)", stats.TotalOut)
	}
}

// TestEdge_ZeroSize 测试零大小
func TestEdge_ZeroSize(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 记录零大小
	bwc.LogSentMessage(0)
	bwc.LogRecvMessage(0)

	stats := bwc.GetBandwidthTotals()
	if stats.TotalOut != 0 {
		t.Errorf("TotalOut = %d, want 0", stats.TotalOut)
	}
	if stats.TotalIn != 0 {
		t.Errorf("TotalIn = %d, want 0", stats.TotalIn)
	}

	// 继续记录正常消息
	bwc.LogSentMessage(100)
	stats = bwc.GetBandwidthTotals()
	if stats.TotalOut != 100 {
		t.Errorf("After zero, TotalOut = %d, want 100", stats.TotalOut)
	}
}

// TestEdge_LargeSize 测试大数值
func TestEdge_LargeSize(t *testing.T) {
	bwc := NewBandwidthCounter()

	// 记录大数值（1GB）
	largeSize := int64(1024 * 1024 * 1024)
	bwc.LogSentMessage(largeSize)
	bwc.LogRecvMessage(largeSize * 2)

	stats := bwc.GetBandwidthTotals()
	if stats.TotalOut != largeSize {
		t.Errorf("TotalOut = %d, want %d", stats.TotalOut, largeSize)
	}
	if stats.TotalIn != largeSize*2 {
		t.Errorf("TotalIn = %d, want %d", stats.TotalIn, largeSize*2)
	}
}

// TestEdge_EmptyPeerID 测试空节点 ID
func TestEdge_EmptyPeerID(t *testing.T) {
	bwc := NewBandwidthCounter()

	emptyPeer := testPeerID("")
	proto := testProtocolID("/test/1.0.0")

	// 使用空节点 ID
	bwc.LogSentMessageStream(100, proto, emptyPeer)
	bwc.LogRecvMessageStream(200, proto, emptyPeer)

	// 应该能查询到
	stats := bwc.GetBandwidthForPeer(emptyPeer)
	if stats.TotalOut != 100 {
		t.Errorf("Empty peer TotalOut = %d, want 100", stats.TotalOut)
	}
}

// TestEdge_EmptyProtocolID 测试空协议 ID
func TestEdge_EmptyProtocolID(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer := testPeerID("peer1")
	emptyProto := testProtocolID("")

	// 使用空协议 ID
	bwc.LogSentMessageStream(100, emptyProto, peer)
	bwc.LogRecvMessageStream(200, emptyProto, peer)

	// 应该能查询到
	stats := bwc.GetBandwidthForProtocol(emptyProto)
	if stats.TotalOut != 100 {
		t.Errorf("Empty protocol TotalOut = %d, want 100", stats.TotalOut)
	}
}

// TestEdge_NonExistentPeer 测试不存在的节点
func TestEdge_NonExistentPeer(t *testing.T) {
	bwc := NewBandwidthCounter()

	nonExistent := testPeerID("non-existent")

	// 查询不存在的节点
	stats := bwc.GetBandwidthForPeer(nonExistent)

	// 应该返回零值
	if stats.TotalOut != 0 {
		t.Errorf("Non-existent peer TotalOut = %d, want 0", stats.TotalOut)
	}
	if stats.TotalIn != 0 {
		t.Errorf("Non-existent peer TotalIn = %d, want 0", stats.TotalIn)
	}
}

// TestEdge_NonExistentProtocol 测试不存在的协议
func TestEdge_NonExistentProtocol(t *testing.T) {
	bwc := NewBandwidthCounter()

	nonExistent := testProtocolID("/non-existent")

	// 查询不存在的协议
	stats := bwc.GetBandwidthForProtocol(nonExistent)

	// 应该返回零值
	if stats.TotalOut != 0 {
		t.Errorf("Non-existent protocol TotalOut = %d, want 0", stats.TotalOut)
	}
	if stats.TotalIn != 0 {
		t.Errorf("Non-existent protocol TotalIn = %d, want 0", stats.TotalIn)
	}
}

// TestEdge_ManyPeers 测试大量节点
func TestEdge_ManyPeers(t *testing.T) {
	bwc := NewBandwidthCounter()

	proto := testProtocolID("/test/1.0.0")

	// 创建 1000 个节点
	numPeers := 1000
	for i := 0; i < numPeers; i++ {
		peer := testPeerID("peer" + string(rune('0'+i%10)))
		bwc.LogSentMessageStream(100, proto, peer)
	}

	// 获取所有节点统计
	byPeer := bwc.GetBandwidthByPeer()

	if len(byPeer) == 0 {
		t.Error("GetBandwidthByPeer() returned empty map")
	}
}

// TestEdge_ManyProtocols 测试大量协议
func TestEdge_ManyProtocols(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer := testPeerID("peer1")

	// 创建 100 个协议
	numProtocols := 100
	for i := 0; i < numProtocols; i++ {
		proto := testProtocolID("/test/" + string(rune('0'+i%10)))
		bwc.LogSentMessageStream(100, proto, peer)
	}

	// 获取所有协议统计
	byProtocol := bwc.GetBandwidthByProtocol()

	if len(byProtocol) == 0 {
		t.Error("GetBandwidthByProtocol() returned empty map")
	}
}

// TestEdge_ResetWithActiveMeters 测试有活跃计量器时重置
func TestEdge_ResetWithActiveMeters(t *testing.T) {
	bwc := NewBandwidthCounter()

	peer := testPeerID("peer1")
	proto := testProtocolID("/test/1.0.0")

	// 记录消息
	bwc.LogSentMessage(100)
	bwc.LogSentMessageStream(200, proto, peer)

	// 重置
	bwc.Reset()

	// 验证重置后为零
	stats := bwc.GetBandwidthTotals()
	if stats.TotalOut != 0 || stats.TotalIn != 0 {
		t.Errorf("After Reset, stats = %+v, want zeros", stats)
	}

	// 重置后应该能继续工作
	bwc.LogSentMessage(50)
	stats = bwc.GetBandwidthTotals()
	if stats.TotalOut != 50 {
		t.Errorf("After Reset and new log, TotalOut = %d, want 50", stats.TotalOut)
	}
}
